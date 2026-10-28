package guardrails

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	bifrostcore "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	// HeaderRequestToken carries a reference to the caller's identity from Obot's LLM proxy to
	// the guardrail plugins running inside the gateway. It is set by Obot, never by callers.
	HeaderRequestToken = "x-obot-guardrails-token"

	// InputPluginName and OutputPluginName are the gateway plugins. The input plugin runs
	// first, so content is redacted before any other plugin (including request logging)
	// sees it; the output plugin runs last, which makes its response hook run first.
	InputPluginName  = "obot-guardrails-input"
	OutputPluginName = "obot-guardrails-output"

	guardrailErrorType = "guardrail_intervention"
)

var (
	requestBindings sync.Map // token -> requestBinding

	outputStateKey = schemas.BifrostContextKey("obot-guardrails-output")

	_ schemas.LLMPlugin = (*InputPlugin)(nil)
	_ schemas.LLMPlugin = (*OutputPlugin)(nil)
)

// outputState carries what the output plugin needs from the request to the response.
type outputState struct {
	plan    Plan
	request Request
	stream  *streamState
}

// InputPlugin applies input policies to model requests.
type InputPlugin struct {
	engine *Engine
}

// OutputPlugin applies output policies to model responses, including streamed ones.
type OutputPlugin struct {
	engine *Engine
}

// AttemptCheck reports whether a request may be served by an attempt's model: whether the
// caller may use it and has budget left for it. An *AttemptError refuses the attempt with its
// status, error type and message; any other error is an internal failure, reported to the
// caller as a 500 without its details.
type AttemptCheck func(ctx context.Context, attempt Attempt) error

// Attempt is one try the gateway makes to serve a request.
type Attempt struct {
	// Model is the attempted model, named "provider/model".
	Model string
	// Subscription is true when the gateway sends the attempt with the caller's own Claude
	// subscription token rather than a configured provider key, so the caller's subscription
	// pays for it.
	Subscription bool
}

// AttemptError refuses an attempt, such as with 429 budget_exceeded.
type AttemptError struct {
	Status  int
	Type    string
	Message string
}

// DetectionSink receives what the policies of one stage (input or output) did to a request.
// Each evaluation of a stage replaces the stage's earlier detections: the gateway checks the
// input again for every model it tries.
type DetectionSink func(stage string, detections []Detection)

// Cost is what the gateway's model catalog says a response cost, in USD, and the token
// usage it priced. Input includes CacheRead and CacheWrite, and InputTokens includes
// CacheReadTokens and CacheWriteTokens.
type Cost struct {
	Input, CacheRead, CacheWrite, Output float64

	InputTokens, CacheReadTokens, CacheWriteTokens, OutputTokens, ReasoningTokens int
}

// CostSink receives what a response cost. A streamed response may report it more than once,
// as its usage grows.
type CostSink func(cost Cost)

// RequestHooks are what Obot hands the gateway along with a request.
type RequestHooks struct {
	// AttemptCheck, when set, is checked for every model the gateway tries: the gateway's
	// routing rules and fallbacks can serve a request with a model other than the one the
	// caller asked for.
	AttemptCheck AttemptCheck
	// Detections, when set, receives what the guardrails did to the request, so it can be
	// recorded with the request.
	Detections DetectionSink
	// Cost, when set, receives what the response cost, priced by the gateway for the model
	// that served it.
	Cost CostSink
}

// requestBinding is what Obot knows about a request it sent to the gateway.
type requestBinding struct {
	identity Identity
	RequestHooks
}

func (e *AttemptError) Error() string { return e.Message }

// BindRequest registers the identity of a request that is about to be sent to the gateway,
// with its hooks, and returns the token to send in HeaderRequestToken. Call release once the
// response has been fully delivered.
func BindRequest(identity Identity, hooks RequestHooks) (token string, release func()) {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	token = hex.EncodeToString(buf)
	requestBindings.Store(token, requestBinding{identity: identity, RequestHooks: hooks})
	return token, func() { requestBindings.Delete(token) }
}

// Plugins returns the gateway plugins that enforce guardrails.
func (e *Engine) Plugins() (*InputPlugin, *OutputPlugin) {
	return &InputPlugin{engine: e}, &OutputPlugin{engine: e}
}

func requestHeaders(ctx *schemas.BifrostContext) map[string]string {
	headers, _ := ctx.Value(schemas.BifrostContextKeyRequestHeaders).(map[string]string)
	return headers
}

// bindingFromContext finds what Obot bound to the request. Requests Obot didn't bind, such
// as model listing or the LLM judge's own calls, aren't guarded.
func bindingFromContext(ctx *schemas.BifrostContext) (requestBinding, bool) {
	token := ""
	for k, v := range requestHeaders(ctx) {
		if strings.EqualFold(k, HeaderRequestToken) {
			token = v
		}
	}
	if token == "" {
		return requestBinding{}, false
	}
	binding, ok := requestBindings.Load(token)
	if !ok {
		return requestBinding{}, false
	}
	return binding.(requestBinding), true
}

// identityFromContext finds the identity Obot bound to the request, and the request headers
// policies may see.
func identityFromContext(ctx *schemas.BifrostContext) (Identity, map[string]string, bool) {
	binding, ok := bindingFromContext(ctx)
	if !ok {
		return Identity{}, nil, false
	}
	headers := requestHeaders(ctx)
	visible := map[string]string{}
	for k, v := range headers {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-bf-") || lower == HeaderRequestToken || lower == "authorization" || lower == "cookie" || lower == "x-api-key" {
			continue
		}
		visible[lower] = v
	}
	return binding.identity, visible, true
}

// reportDetections hands what a stage's policies did to the request to whoever bound it.
func reportDetections(ctx *schemas.BifrostContext, stage string, d Decision) {
	if binding, ok := bindingFromContext(ctx); ok && binding.Detections != nil {
		binding.Detections(stage, d.Detections)
	}
}

// reportCost prices a response that carries usage, for the model that served it, and hands
// the cost to whoever bound the request. It runs before guardrails change the response.
func reportCost(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse) {
	if resp == nil {
		return
	}
	binding, ok := bindingFromContext(ctx)
	if !ok || binding.Cost == nil {
		return
	}
	// A model without a price still reports its token usage, which budgets count.
	breakdown, usage := ctx.CalculateCostBreakdown(resp), responseUsage(resp)
	if breakdown == nil && usage == nil {
		return
	}
	var cost Cost
	if breakdown != nil {
		cost.Input, cost.Output = breakdown.InputCost, breakdown.OutputCost
		if d := breakdown.InputCostDetails; d != nil {
			cost.CacheRead, cost.CacheWrite = d.CachedReadCost, d.CachedWriteCost
		}
	}
	if usage != nil {
		cost.InputTokens, cost.OutputTokens = usage.PromptTokens, usage.CompletionTokens
		if d := usage.PromptTokensDetails; d != nil {
			cost.CacheReadTokens, cost.CacheWriteTokens = d.CachedReadTokens, d.CachedWriteTokens
		}
		if d := usage.CompletionTokensDetails; d != nil {
			cost.ReasoningTokens = d.ReasoningTokens
		}
	}
	binding.Cost(cost)
}

// responseUsage returns the token usage of the kinds of response Obot serves through the
// gateway.
func responseUsage(resp *schemas.BifrostResponse) *schemas.BifrostLLMUsage {
	switch {
	case resp.ChatResponse != nil && resp.ChatResponse.Usage != nil:
		return resp.ChatResponse.Usage
	case resp.ResponsesResponse != nil && resp.ResponsesResponse.Usage != nil:
		return resp.ResponsesResponse.Usage.ToBifrostLLMUsage()
	case resp.ResponsesStreamResponse != nil && resp.ResponsesStreamResponse.Response != nil && resp.ResponsesStreamResponse.Response.Usage != nil:
		return resp.ResponsesStreamResponse.Response.Usage.ToBifrostLLMUsage()
	case resp.TextCompletionResponse != nil && resp.TextCompletionResponse.Usage != nil:
		return resp.TextCompletionResponse.Usage
	case resp.EmbeddingResponse != nil && resp.EmbeddingResponse.Usage != nil:
		return resp.EmbeddingResponse.Usage
	}
	return nil
}

// attemptError refuses an attempt. Fallbacks stay allowed, so the gateway goes on to the
// next model, which is checked the same way.
func attemptError(err error) *schemas.BifrostError {
	status, errType, message := http.StatusInternalServerError, "api_error", "failed to check whether the request may use this model"
	if refusal := (*AttemptError)(nil); errors.As(err, &refusal) {
		status, errType, message = refusal.Status, refusal.Type, refusal.Message
	} else {
		slog.Error("failed to check an LLM gateway attempt", "error", err)
	}
	allowFallbacks := true
	return &schemas.BifrostError{
		Type:           &errType,
		StatusCode:     &status,
		AllowFallbacks: &allowFallbacks,
		Error:          &schemas.ErrorField{Type: &errType, Message: message},
	}
}

// checkAttempt refuses an attempt on a model the request may not be served by. It runs for
// every attempt, after the gateway's routing rules chose the model and for each fallback.
func checkAttempt(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) *schemas.LLMPluginShortCircuit {
	binding, ok := bindingFromContext(ctx)
	if !ok || binding.AttemptCheck == nil {
		return nil
	}
	if _, name, _ := req.GetRequestFields(); name == "" {
		// Not a model call, such as listing a provider's models.
		return nil
	}
	model, provider := requestFields(req)
	attempt := Attempt{
		Model: model,
		// The engine skips key selection and forwards the caller's OAuth token for Claude Code
		// requests to Anthropic; any other provider uses a configured key.
		Subscription: provider == string(schemas.Anthropic) && anthropic.IsClaudeCodeMaxMode(ctx),
	}
	if err := binding.AttemptCheck(ctx, attempt); err != nil {
		return &schemas.LLMPluginShortCircuit{Error: attemptError(err)}
	}
	return nil
}

func conditionVarsForRequest(req *schemas.BifrostRequest, headers map[string]string) (map[string]any, string) {
	model, provider := requestFields(req)
	params := map[string]any{}
	if req.ChatRequest != nil && req.ChatRequest.Params != nil {
		if req.ChatRequest.Params.MaxCompletionTokens != nil {
			params["max_tokens"] = int64(*req.ChatRequest.Params.MaxCompletionTokens)
		}
	}
	return map[string]any{"model": model, "provider": provider, "headers": headers, "params": params}, model
}

func guardrailError(message string) *schemas.BifrostError {
	status := http.StatusBadRequest
	errType := guardrailErrorType
	allowFallbacks := false
	return &schemas.BifrostError{
		Type:           &errType,
		StatusCode:     &status,
		AllowFallbacks: &allowFallbacks,
		Error:          &schemas.ErrorField{Type: &errType, Message: message},
	}
}

func (p *InputPlugin) GetName() string { return InputPluginName }

func (p *InputPlugin) Cleanup() error { return nil }

func (p *InputPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (p *InputPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	// The input plugin runs first, so no other plugin or provider sees an attempt the request
	// may not be served by.
	if denied := checkAttempt(ctx, req); denied != nil {
		return req, denied, nil
	}
	if trimmed := withoutUnsupportedReasoning(req); trimmed != req {
		disableRawPassthrough(ctx)
		req = trimmed
	}
	identity, headers, ok := identityFromContext(ctx)
	if !ok {
		return req, nil, nil
	}
	vars, _ := conditionVarsForRequest(req, headers)
	plan, err := p.engine.Plan(ctx, types.GuardrailTargetLLM, StageInput, identity, vars)
	if err != nil {
		slog.Error("failed to load guardrail policies; blocking the request", "error", err)
		return req, &schemas.LLMPluginShortCircuit{Error: guardrailError("Request blocked: guardrails could not be evaluated.")}, nil
	}
	if plan.Empty() {
		return req, nil, nil
	}
	disableRawPassthrough(ctx)
	refs := requestText(req)
	d := p.engine.Evaluate(ctx, plan, Request{
		Target: types.GuardrailTargetLLM, Stage: StageInput, Identity: identity, Vars: vars,
		Segments: refs.segments(),
	})
	reportDetections(ctx, StageInput, d)
	if d.Blocked {
		return req, &schemas.LLMPluginShortCircuit{Error: guardrailError(d.Message)}, nil
	}
	if d.Redacted {
		refs.apply(d.Segments)
	}
	return req, nil, nil
}

// PostLLMHook runs last of the gateway's response hooks, so it sees each attempt's error as the
// gateway will act on it.
func (p *InputPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	markToolUseStop(resp)
	return resp, providerError(bifrostErr), nil
}

// withoutUnsupportedReasoning returns a copy of an OpenAI or Azure chat request without its
// reasoning settings when the model doesn't reason, or the request itself otherwise. Clients
// such as Goose send reasoning_effort with every request, and those models reject it (HTTP
// 400 "Unrecognized request argument"). The gateway engine decides whether a model reasons
// the same way when it converts a Responses request. Other providers are left alone: a
// self-hosted reasoning model may be missing from the model data, and the name check would
// then strip what it accepts. The request is copied, not changed, because the engine builds
// fallback attempts from the original, and a fallback model may reason.
func withoutUnsupportedReasoning(req *schemas.BifrostRequest) *schemas.BifrostRequest {
	if req == nil || req.ChatRequest == nil || req.ChatRequest.Params == nil || req.ChatRequest.Params.Reasoning == nil {
		return req
	}
	chat := *req.ChatRequest
	if chat.Provider != schemas.OpenAI && chat.Provider != schemas.Azure {
		return req
	}
	if schemas.ResolveModelCaps(chat.Provider, chat.Model).SupportsReasoning(openai.IsOpenAIReasoningModel(chat.Model)) {
		return req
	}
	params := *chat.Params
	params.Reasoning = nil
	chat.Params = &params
	trimmed := *req
	trimmed.ChatRequest = &chat
	return &trimmed
}

// markToolUseStop sets the stop reason of a streamed response that ended with a tool call
// but carries none, as OpenAI's never do. The gateway engine turns a stream into an
// Anthropic Messages stream with stop_reason end_turn unless the response says otherwise,
// though for a whole response it infers tool_use from the tool calls. Without this, a
// streaming Messages client such as Claude Code using an OpenAI model is told the turn
// ended instead of that it should run the tool.
func markToolUseStop(resp *schemas.BifrostResponse) {
	if resp == nil || resp.ResponsesStreamResponse == nil {
		return
	}
	completed := resp.ResponsesStreamResponse
	if completed.Type != schemas.ResponsesStreamResponseTypeCompleted || completed.Response == nil ||
		completed.Response.StopReason != nil || completed.Response.IncompleteDetails != nil {
		return
	}
	for _, item := range completed.Response.Output {
		if item.Type != nil && *item.Type == schemas.ResponsesMessageTypeFunctionCall {
			completed.Response.StopReason = new("tool_calls")
			return
		}
	}
}

// providerError sets how the gateway handles and reports an attempt's error:
//   - A request the provider rejected as invalid, such as with HTTP 400, isn't sent on to
//     fallbacks. The gateway would otherwise try every fallback, which would reject it too,
//     after sending the request, and its content, to each of their providers. An error that
//     already says whether to fall back, such as a guardrail's or an access refusal, is kept.
//     So is one the gateway raised itself before reaching a provider, such as no key serving
//     the model: a fallback on another provider or key can still serve the request.
//   - A provider that didn't answer in time is reported in Obot's words, without the engine's
//     internal error and setting names.
func providerError(err *schemas.BifrostError) *schemas.BifrostError {
	if err == nil {
		return nil
	}
	if err.AllowFallbacks == nil && rejectedByProvider(err) {
		allowFallbacks := false
		err.AllowFallbacks = &allowFallbacks
	}
	if err.Error != nil && err.Error.Type != nil && *err.Error.Type == schemas.RequestTimedOut {
		if provider := err.ExtraFields.RoutingInfo.Provider; provider != "" {
			err.Error.Message = fmt.Sprintf("The request timed out: provider %q didn't answer within its configured request timeout.", provider)
		} else {
			err.Error.Message = "The request timed out: the provider didn't answer within its configured request timeout."
		}
		err.Error.Error = nil
	}
	return err
}

// rejectedByProvider reports whether a provider refused the request itself as invalid. The
// gateway engine declares an error type on the errors it raises itself (no key serving the
// model, say) and leaves it empty on a provider's, so a declared type means no provider saw
// the request.
func rejectedByProvider(err *schemas.BifrostError) bool {
	return err.ExtraFields.ErrorType == "" && bifrostcore.ClassifyFailure(err) == schemas.FailureClassCallerFault
}

func (p *OutputPlugin) GetName() string { return OutputPluginName }

func (p *OutputPlugin) Cleanup() error { return nil }

func (p *OutputPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (p *OutputPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	identity, headers, ok := identityFromContext(ctx)
	if !ok {
		return req, nil, nil
	}
	vars, _ := conditionVarsForRequest(req, headers)
	plan, err := p.engine.Plan(ctx, types.GuardrailTargetLLM, StageOutput, identity, vars)
	if err != nil {
		slog.Error("failed to load guardrail policies; blocking the request", "error", err)
		return req, &schemas.LLMPluginShortCircuit{Error: guardrailError("Request blocked: guardrails could not be evaluated.")}, nil
	}
	if plan.Empty() {
		return req, nil, nil
	}
	disableRawPassthrough(ctx)
	state := &outputState{
		plan: plan,
		request: Request{
			Target: types.GuardrailTargetLLM, Stage: StageOutput, Identity: identity, Vars: vars,
			// The (already input-checked) request is context for output checks.
			History: requestText(req).segments(),
		},
	}
	if bifrostcore.IsStreamRequestType(req.RequestType) {
		state.stream = newStreamState()
		if plan.Modifies() {
			// Hold the stream until the complete response has been checked.
			ctx.PauseStream()
			state.stream.held = streamPaused(ctx)
			if !state.stream.held {
				slog.Warn("could not hold a streamed response for guardrail checks; findings can only end the stream")
			}
		}
	}
	ctx.SetValue(outputStateKey, state)
	return req, nil, nil
}

// disableRawPassthrough makes the gateway send the request it parsed, and return the response
// it parsed, instead of forwarding the client's raw request body and the provider's raw
// response. Guardrails check and rewrite the parsed forms, so raw passthrough would bypass
// them: the Anthropic integration forwards raw bodies for Claude Code, for example. This is
// the same switch the gateway makes when it falls back to a provider that can't take the raw
// body.
func disableRawPassthrough(ctx *schemas.BifrostContext) {
	ctx.SetValue(schemas.BifrostContextKeyUseRawRequestBody, false)
	ctx.ClearValue(schemas.BifrostContextKeyRawRequestBodyTextRewriter)
	ctx.ClearValue(schemas.BifrostContextKeyRawRequestBodyTextTransformer)
	ctx.ClearValue(schemas.BifrostContextKeyRawStreamTextCodec)
	ctx.ClearValue(schemas.BifrostContextKeyRawResponseTextTransformer)
	ctx.SetValue(schemas.BifrostContextKeySendBackRawResponse, false)
	ctx.SetValue(schemas.BifrostContextKeyPassthroughOverridesPresent, false)
	ctx.ClearValue(schemas.BifrostContextKeyURLPath)
}

func streamPaused(ctx *schemas.BifrostContext) bool {
	tracer, traceID, err := bifrostcore.GetTracerFromContext(ctx)
	if err != nil || tracer == nil || traceID == "" {
		return false
	}
	return tracer.IsStreamPaused(traceID)
}

func (p *OutputPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	reportCost(ctx, resp)
	state, _ := ctx.Value(outputStateKey).(*outputState)
	if state == nil || resp == nil {
		if state != nil && state.stream != nil && state.stream.held && bifrostcore.IsFinalChunk(ctx) {
			ctx.ResumeStream()
		}
		return resp, bifrostErr, nil
	}
	if state.stream != nil {
		return p.postStream(ctx, state, resp, bifrostErr)
	}

	refs := responseText(resp)
	req := state.request
	req.Segments = refs.segments()
	d := p.engine.Evaluate(ctx, state.plan, req)
	reportDetections(ctx, StageOutput, d)
	if d.Blocked {
		return nil, guardrailError(d.Message), nil
	}
	if d.Redacted {
		refs.apply(d.Segments)
	}
	return resp, bifrostErr, nil
}

func (p *OutputPlugin) postStream(ctx *schemas.BifrostContext, state *outputState, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	st := state.stream
	st.observe(resp)
	final := bifrostcore.IsFinalChunk(ctx)
	if !final {
		if st.held {
			// Nothing reaches the client until the response is checked; downstream plugins
			// such as request logging only see content after it has been checked.
			return st.blank(resp), bifrostErr, nil
		}
		return resp, bifrostErr, nil
	}

	req := state.request
	req.Segments = st.segments()
	req.Delivered = !st.held
	d := p.engine.Evaluate(ctx, state.plan, req)
	reportDetections(ctx, StageOutput, d)
	switch {
	case d.Blocked:
		if st.held {
			if err := ctx.ClearPausedStreamBuffer(); err != nil {
				slog.Warn("failed to discard a blocked stream's buffered content", "error", err)
			}
		}
		ctx.EndStream(guardrailError(d.Message))
		return resp, bifrostErr, nil
	case !st.held:
		return resp, bifrostErr, nil
	}

	finalText := st.finalText(d)
	emitted := map[string]bool{}
	if err := ctx.TransformPausedStreamBuffer(func(chunks []*schemas.BifrostStreamChunk) (schemas.PausedStreamBufferTransformResult, error) {
		out := make([]*schemas.BifrostStreamChunk, len(chunks))
		for i, c := range chunks {
			out[i] = st.rewriteChunk(c, finalText, emitted)
		}
		return schemas.PausedStreamBufferTransformResult{Chunks: out, ReleaseCount: len(out)}, nil
	}); err != nil {
		// The client would otherwise receive the blanked content; end the stream instead.
		ctx.EndStream(guardrailError(fmt.Sprintf("Response could not be delivered after guardrail checks: %v", err)))
		return resp, bifrostErr, nil
	}
	resp = st.rewriteResponse(resp, finalText, emitted)
	ctx.ResumeStream()
	return resp, bifrostErr, nil
}
