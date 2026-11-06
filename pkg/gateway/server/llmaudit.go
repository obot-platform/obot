package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/streaming"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaycontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	defaultLLMAuditLogResponseCaptureLimit = 5 << 20 // 5MiB
	claudeCodeSessionIDHeader              = "X-Claude-Code-Session-Id"
)

type llmAuditRecorder struct {
	finalResponse *schemas.BifrostResponse
	observed      int
	streamID      string
	streamModel   string

	once        sync.Once
	mu          sync.Mutex
	closed      bool
	stream      bool
	accumulator *streaming.Accumulator
	captured    int
	log         types.LLMAuditLog
	// guardrailFindings, when set, returns what the LLM gateway's guardrails did to the
	// request. It is read when the entry is written, once the response has been delivered.
	guardrailFindings func() []apitypes.LLMAuditGuardrailFinding
	// route, when set, returns how the LLM gateway served the request. It is read when the
	// entry is written.
	route                 func() llmRoute
	responseJSON          bytes.Buffer
	responseCaptureLimit  int
	responseCaptureFilled bool
}

func newLLMAuditRecorder(req *http.Request, user user.Info, responseCaptureLimit int) *llmAuditRecorder {
	now := time.Now()
	requestID := gatewaycontext.GetRequestID(req.Context())
	if requestID == "" {
		requestID = uuid.New().String()
	}

	userID := ""
	if user != nil {
		userID = user.GetUID()
	}
	var apiKeyID *uint
	apiKeyName := ""
	if attribution, ok := principal.APIKeyAttributionFromUser(user); ok {
		apiKeyID = &attribution.ID
		apiKeyName = attribution.Name
	}
	return &llmAuditRecorder{
		responseCaptureLimit: responseCaptureLimit,
		log: types.LLMAuditLog{
			ID:             uuid.New().String(),
			CreatedAt:      now,
			UserID:         userID,
			APIKeyID:       apiKeyID,
			APIKeyName:     apiKeyName,
			RequestPath:    req.URL.Path,
			RequestMethod:  req.Method,
			RequestHeaders: redactedHeaders(req.Header),
			RequestID:      requestID,
			UserAgent:      req.UserAgent(),
			ClientIP:       requestinfo.GetSourceIP(req),
		},
	}
}

func (r *llmAuditRecorder) setModel(modelProvider, modelID, targetModel string) {
	if r == nil {
		return
	}
	r.log.ModelProvider = modelProvider
	r.log.ModelID = modelID
	r.log.TargetModel = targetModel
}

func (r *llmAuditRecorder) setRequestBody(body []byte) {
	if r == nil {
		return
	}
	r.log.RequestBody = body
}

func (r *llmAuditRecorder) setClientSessionID(dialect llmtypes.Dialect, headers http.Header, body []byte) {
	if r == nil {
		return
	}
	r.log.ClientSessionID = extractLLMClientSessionID(dialect, headers, body)
}

func (r *llmAuditRecorder) setReasoningEffort(modelProvider string, body []byte) {
	if r == nil {
		return
	}
	r.log.ReasoningEffort = extractLLMReasoningEffort(modelProvider, body)
}

func (r *llmAuditRecorder) setGuardrailFindings(findings func() []apitypes.LLMAuditGuardrailFinding) {
	if r == nil {
		return
	}
	r.guardrailFindings = findings
}

func (r *llmAuditRecorder) setRoute(route func() llmRoute) {
	if r == nil {
		return
	}
	r.route = route
}

func (r *llmAuditRecorder) recordResponse(resp *http.Response) {
	if r == nil || resp == nil {
		return
	}
	r.stream = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	r.log.ResponseStatus = resp.StatusCode
	r.log.ResponseHeaders = redactedHeaders(resp.Header)
}

func (r *llmAuditRecorder) recordResponseStatus(status int) {
	if r == nil {
		return
	}
	r.log.ResponseStatus = status
}

func (r *llmAuditRecorder) captureResponseChunk(p []byte) {
	if r == nil || len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.responseCaptureFilled {
		return
	}
	r.captured += len(p)
	if r.captured > r.responseCaptureLimit {
		r.responseCaptureFilled = true
		r.responseJSON.Reset()
		r.discardAccumulator()
		return
	}
	if !r.stream {
		_, _ = r.responseJSON.Write(p)
	}
}

func (r *llmAuditRecorder) setTokenUsage(usage types.TokenUsage) {
	if r == nil {
		return
	}
	r.log.InputTokens = usage.InputTokens
	r.log.OutputTokens = usage.OutputTokens
	r.log.CacheReadTokens = usage.CacheReadTokens
	r.log.CacheWriteTokens = usage.CacheWriteTokens
	r.log.ReasoningTokens = usage.ThinkingTokens
	r.log.Spend = usage.TotalSpend
}

func (r *llmAuditRecorder) finish(c *client.Client, err error) {
	if r == nil || c == nil {
		return
	}
	r.once.Do(func() {
		r.log.Duration = time.Since(r.log.CreatedAt).Milliseconds()
		r.setOutcomeAndResponseStatus(err)
		if r.guardrailFindings != nil {
			r.log.GuardrailFindings = r.guardrailFindings()
		}
		if r.route != nil {
			route := r.route()
			r.log.ServedProvider = route.Provider
			r.log.ServedModel = route.Model
			r.log.ProviderKey = route.Key
			r.log.Fallback = route.Fallback
			r.log.ClaudeSubscription = route.Subscription
		}
		r.finishBody()
		c.LogLLMAuditEntry(r.log)
	})
}

func (r *llmAuditRecorder) setOutcomeAndResponseStatus(err error) {
	if r == nil {
		return
	}
	r.log.Outcome, r.log.Error, r.log.ResponseStatus = llmOutcome(r.log.ResponseStatus, err)
}

// llmOutcome is the outcome of an LLM request (success, error or canceled), given the status
// of its response, if it got one, and why it failed, if it did. It also returns the error
// message and the response status, which comes from the error when there was no response.
func llmOutcome(status int, err error) (outcome, message string, responseStatus int) {
	outcome, responseStatus = types.LLMAuditOutcomeSuccess, status
	if status >= http.StatusBadRequest {
		outcome = types.LLMAuditOutcomeError
	}
	if err == nil {
		return outcome, "", responseStatus
	}
	message, outcome = err.Error(), types.LLMAuditOutcomeError
	if errors.Is(err, context.Canceled) {
		outcome = types.LLMAuditOutcomeCanceled
	}
	if responseStatus == 0 {
		if errHTTP := (*apitypes.ErrHTTP)(nil); errors.As(err, &errHTTP) {
			responseStatus = errHTTP.Code
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			responseStatus = int(errStatus.ErrStatus.Code)
		} else {
			responseStatus = http.StatusInternalServerError
		}
	}
	return outcome, message, responseStatus
}

func redactedHeaders(headers http.Header) json.RawMessage {
	out := make(http.Header, len(headers))
	for k, values := range headers {
		if shouldRedactHeader(k) {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		out[k] = slices.Clone(values)
	}
	b, _ := json.Marshal(out)
	return b
}

func shouldRedactHeader(key string) bool {
	k := strings.ToLower(key)
	switch k {
	case "x-ratelimit-limit-tokens",
		"x-ratelimit-remaining-tokens",
		"x-ratelimit-reset-tokens",
		"anthropic-ratelimit-input-tokens-limit",
		"anthropic-ratelimit-input-tokens-remaining",
		"anthropic-ratelimit-input-tokens-reset",
		"anthropic-ratelimit-output-tokens-limit",
		"anthropic-ratelimit-output-tokens-remaining",
		"anthropic-ratelimit-output-tokens-reset",
		"anthropic-ratelimit-requests-limit",
		"anthropic-ratelimit-requests-remaining",
		"anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-tokens-limit",
		"anthropic-ratelimit-tokens-remaining",
		"anthropic-ratelimit-tokens-reset":
		return false
	}
	if k == "authorization" || k == "cookie" || k == "set-cookie" || k == "x-api-key" || k == "x-obot-machine-fingerprint" {
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "key") || strings.Contains(k, "credential")
}

func extractLLMClientSessionID(dialect llmtypes.Dialect, headers http.Header, body []byte) string {
	if sessionID := headers.Get(claudeCodeSessionIDHeader); sessionID != "" {
		return sessionID
	}

	switch dialect {
	case llmtypes.DialectOpenAIResponses:
		return gjson.GetBytes(body, "client_metadata.session_id").String()
	case llmtypes.DialectAnthropicMessages:
		userID := gjson.GetBytes(body, "metadata.user_id").String()
		if !gjson.Valid(userID) {
			return ""
		}
		return gjson.Get(userID, "session_id").String()
	default:
		return ""
	}
}

func extractLLMReasoningEffort(modelProvider string, body []byte) string {
	switch modelProvider {
	case system.LLMGatewayModelProvider:
		if effort := gjson.GetBytes(body, "reasoning.effort").String(); effort != "" {
			return effort
		}
		if effort := gjson.GetBytes(body, "reasoning_effort").String(); effort != "" {
			return effort
		}
		return gjson.GetBytes(body, "output_config.effort").String()
	default:
		return ""
	}
}

// observeResponseChunk assembles client-bound core chunks after guardrail replay.
func (r *llmAuditRecorder) observeResponseChunk(chunk *schemas.BifrostStreamChunk, a *streaming.Accumulator) {
	if r == nil || chunk == nil {
		return
	}
	r.mu.Lock()
	stopped := r.closed || r.responseCaptureFilled
	r.mu.Unlock()
	if stopped {
		return
	}
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	id := gjson.GetBytes(data, "id").String()
	model := gjson.GetBytes(data, "model").String()
	if nested := gjson.GetBytes(data, "response.id").String(); nested != "" {
		id = nested
	}
	if nested := gjson.GetBytes(data, "response.model").String(); nested != "" {
		model = nested
	}
	typ := gjson.GetBytes(data, "type").String()
	terminal := typ == "response.completed" || typ == "response.failed" || typ == "response.incomplete"
	// Terminal events carry complete fields that incremental snapshots may not reconstruct.
	var complete *schemas.BifrostResponsesResponse
	if terminal && len(data) <= r.responseCaptureLimit && chunk.BifrostResponsesStreamResponse != nil && chunk.Response != nil {
		if raw, err := json.Marshal(chunk.Response); err == nil {
			var response schemas.BifrostResponsesResponse
			if json.Unmarshal(raw, &response) == nil {
				complete = &response
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Capture may have closed or overflowed while this chunk was serialized.
	if r.closed || r.responseCaptureFilled {
		return
	}
	if id != "" {
		r.streamID = id
	}
	if model != "" {
		r.streamModel = model
	}
	// Bound retained chunks even when the provider runs ahead of the proxy reader.
	r.observed += len(data)
	if r.observed > r.responseCaptureLimit {
		r.responseCaptureFilled = true
		r.discardAccumulator()
		return
	}
	r.accumulator = a
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	ctx.SetValue(schemas.BifrostContextKeyAccumulatorID, r.log.ID)
	ctx.SetValue(schemas.BifrostContextKeyStreamEndIndicator, terminal)
	resp := &schemas.BifrostResponse{ChatResponse: chunk.BifrostChatResponse, ResponsesStreamResponse: chunk.BifrostResponsesStreamResponse}
	// Keep accumulation serialized with close/overflow so cleanup cannot be undone by a late chunk.
	processed, err := a.ProcessStreamingResponse(ctx, resp, chunk.BifrostError)
	if terminal && processed != nil {
		r.finalResponse = processed.ToBifrostResponse()
	}
	if complete != nil {
		r.finalResponse = &schemas.BifrostResponse{ResponsesResponse: complete}
	}
	if err != nil {
		r.responseCaptureFilled = true
		r.discardAccumulator()
	}
}

// discardAccumulator requires r.mu to serialize cleanup with chunk accumulation.
func (r *llmAuditRecorder) discardAccumulator() {
	r.finalResponse = nil
	if r.accumulator != nil {
		r.accumulator.ForceCleanupStreamAccumulator(r.log.ID)
		r.accumulator = nil
	}
}

// finishBody closes capture under r.mu; response formatting uses only the detached state.
func (r *llmAuditRecorder) finishBody() {
	r.mu.Lock()
	r.closed = true
	accumulator, resp := r.accumulator, r.finalResponse
	if accumulator != nil && resp == nil && !r.responseCaptureFilled {
		resp = accumulator.GetAccumulatedResponse(r.log.ID)
	}
	var unary []byte
	if accumulator == nil && !r.stream && !r.responseCaptureFilled {
		unary = bytes.Clone(r.responseJSON.Bytes())
	}
	r.accumulator, r.finalResponse = nil, nil
	r.mu.Unlock()
	// Keep accumulator-owned data alive until serialization finishes. Closed capture
	// prevents concurrent callbacks from adding chunks or cleaning up this detached state.
	if accumulator != nil {
		defer accumulator.ForceCleanupStreamAccumulator(r.log.ID)
	}
	if r.responseCaptureFilled {
		r.log.ResponseBody = nil
		return
	}
	if resp != nil {
		var body any
		switch {
		case resp.ChatResponse != nil:
			body = resp.ChatResponse
		case resp.ResponsesResponse != nil:
			body = resp.ResponsesResponse
			if strings.HasSuffix(strings.TrimRight(r.log.RequestPath, "/"), "/messages") {
				body = anthropic.ToAnthropicResponsesResponse(schemas.NewBifrostContext(context.Background(), time.Time{}), resp.ResponsesResponse)
			}
		}
		if body != nil {
			r.log.ResponseBody, _ = json.Marshal(body)
		}
		r.log.ResponseBody = stripEngineFields(r.log.ResponseBody)
		if r.streamID != "" {
			r.log.ResponseBody, _ = sjson.SetBytes(r.log.ResponseBody, "id", r.streamID)
		}
		if r.streamModel != "" {
			r.log.ResponseBody, _ = sjson.SetBytes(r.log.ResponseBody, "model", r.streamModel)
		}
	} else if json.Valid(unary) {
		r.log.ResponseBody = unary
	}
	if len(r.log.ResponseBody) > r.responseCaptureLimit {
		r.log.ResponseBody = nil
	}
	if r.log.ResponseID == "" {
		r.log.ResponseID = gjson.GetBytes(r.log.ResponseBody, "id").String()
	}
}

func (r *llmAuditRecorder) captureStreamError(body []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed && !r.responseCaptureFilled && len(body) <= r.responseCaptureLimit {
		r.log.ResponseBody = bytes.Clone(body)
	}
}
