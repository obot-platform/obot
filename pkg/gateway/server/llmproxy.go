package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/guardrails"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// budgetExceededType is the error type of a request refused because a budget is exhausted.
	budgetExceededType = "budget_exceeded"
)

type responseModifier struct {
	user          kuser.Info
	model         string
	modelProvider string
	routeDialect  llmtypes.Dialect
	client        *client.Client
	b             *bufio.Reader
	c             io.Closer
	stream        bool
	leftover      []byte
	// Non-streaming JSON is accumulated while forwarded, then parsed once at EOF or Close.
	nonStreamResponse bytes.Buffer
	nonStreamParsed   bool

	// Token usage and model access gating
	tokenUsageTracker *threadSafeTokenUsageTracker
	// gateway tracks an LLM gateway request: the budgets its usage counts against, which
	// are those of the model that served it. It is nil for other backends.
	gateway   gatewayRequest
	modelID   string
	mapHelper *modelaccesspolicy.Helper
	audit     *llmAuditRecorder
	usage     *llmUsageRecorder
	// unlisted returns the models a model listing response must include although the
	// upstream didn't list them.
	unlisted func(ctx context.Context) []llmengine.ListedModel
	// clientHeader is the header of the response to the client, which Obot's API server
	// has already added to.
	clientHeader http.Header
}

type preparedLLMProxyRequest struct {
	body              []byte
	model             string
	tokenUsageTracker *threadSafeTokenUsageTracker
}

// llmProviderProxyBackend is the LLM gateway, which serves every model.
type llmProviderProxyBackend interface {
	modelProviderName() string
	upstreamURL(req *http.Request) (url.URL, llmtypes.Dialect, error)
	transport() (http.RoundTripper, error)
	// decorateRequest adds headers for the upstream and checks each model the gateway tries
	// for the request.
	decorateRequest(l *llmProviderProxy, req api.Context, model *v1.Model) (gatewayRequest, error)
}

// trackerDialectBackend is implemented by backends that choose the usage tracker from the
// route's dialect rather than the model's.
type trackerDialectBackend interface {
	trackerDialect(routeDialect llmtypes.Dialect) llmtypes.Dialect
}

// modelResolvingBackend is implemented by backends with an additional way to resolve a
// model reference that the standard lookup did not find. A nil model means no match.
type modelResolvingBackend interface {
	resolveModel(l *llmProviderProxy, user kuser.Info, reference string) (*v1.Model, error)
}

// extraModelAuthorizingBackend is implemented by backends whose requests can name models
// besides the primary one (such as fallbacks), each of which must be authorized.
type extraModelAuthorizingBackend interface {
	authorizeExtraModels(l *llmProviderProxy, req api.Context, body []byte) ([]byte, error)
}

// modelListingBackend is implemented by backends that serve models their upstream's model
// listing doesn't list, which model listing responses must include.
type modelListingBackend interface {
	unlistedModels(ctx context.Context) []llmengine.ListedModel
}

// bodyLimitingBackend is implemented by backends that accept request bodies only up to a size.
type bodyLimitingBackend interface {
	maxRequestBodyBytes() int64
}

// gatewayRequest is what the LLM gateway backend tracks about one request. The gateway's
// routing rules and fallbacks choose the model that serves it, and its traffic is governed by
// budgets, which apply to that model, instead of the daily token quota.
type gatewayRequest interface {
	// release ends the request once its response has been delivered.
	release()
	// served returns the model ("provider/model") of the last attempt the gateway made, which
	// served the request if it succeeded, and the budgets its usage counts against.
	served() (model string, budgetIDs []string)
	// paidBySubscription reports whether the caller's Claude subscription paid for the last
	// attempt, rather than one of the gateway's provider keys.
	paidBySubscription() bool
	// budgetExceeded is the budget that refused the last attempt, if one did.
	budgetExceeded() *client.LLMGatewayBudgetExceeded
	// lastRefused reports whether the last attempt was refused, so no model served the request.
	lastRefused() bool
	// guardrailFindings is what the gateway's guardrails did to the request so far.
	guardrailFindings() []types2.LLMAuditGuardrailFinding
	// cost is what the gateway priced the response at, for the model that served it, if it
	// priced it.
	cost() (guardrails.Cost, bool)
}

type llmProviderProxy struct {
	backend       llmProviderProxyBackend
	modelProvider *v1.ModelProvider
	mapHelper     *modelaccesspolicy.Helper
	lock          sync.RWMutex
}

// extractModelFromBody extracts the model name from an arbitrary JSON payload or body,
// checking top-level "model" first, then nested paths such as "message.model" and "response.model"
// used by different providers or event/response shapes.
func extractModelFromBody(body []byte) string {
	if model := gjson.GetBytes(body, "model").String(); model != "" {
		return model
	}
	if model := gjson.GetBytes(body, "message.model").String(); model != "" {
		return model
	}
	return gjson.GetBytes(body, "response.model").String()
}

func rewriteModelInBody(body []byte, model string) ([]byte, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, err
	}
	if _, ok := bodyMap["model"]; ok {
		bodyMap["model"] = model
	}
	return json.Marshal(bodyMap)
}

// copyBody returns a copy of the bytes in a request or response body.
// If the copy was successful the body is restored to its original state before returning so that
// it can be reused.
// The returned byte slice is safe to modify without affecting the restored body.
func copyBody(body *io.ReadCloser) ([]byte, error) {
	b, err := io.ReadAll(*body)
	(*body).Close()

	if err != nil {
		// b can be partial results on error, don't restore the body
		return nil, err
	}

	// Read was successful, restore the body with a copy.
	*body = io.NopCloser(bytes.NewReader(slices.Clone(b)))
	return b, nil
}

func (r *responseModifier) modifyResponse(resp *http.Response) error {
	if isModelsListRequest(resp.Request) {
		if r.unlisted != nil {
			if err := addListedModels(resp, r.unlisted(resp.Request.Context()), r.routeDialect == llmtypes.DialectAnthropicMessages); err != nil {
				return err
			}
		}
		allowedTargetModels, allowAllModels, err := r.mapHelper.GetUserAllowedTargetModels(r.user, r.modelProvider, string(r.routeDialect))
		if err != nil {
			return fmt.Errorf("failed to determine accessible models: %w", err)
		}
		if r.modelProvider == system.LLMGatewayModelProvider {
			// The gateway's Anthropic-format model list uses bare model IDs, which the
			// gateway also accepts in requests, so allow the bare form of each target too.
			for target := range maps.Clone(allowedTargetModels) {
				if _, bare, ok := strings.Cut(target, "/"); ok {
					allowedTargetModels[bare] = true
				}
			}
		}
		if err := filterModelListResponse(resp, allowedTargetModels, allowAllModels); err != nil {
			return err
		}
		r.audit.recordResponse(resp)
		wrapRecordingResponse(resp, r.audit, nil, r.client)
		return nil
	}

	r.rewriteBudgetExceeded(resp)
	r.applyServedModel(resp)
	r.audit.recordResponse(resp)
	r.usage.recordResponseStatus(resp.StatusCode)

	if !shouldWrapLLMResponse(resp) && !r.shouldWrapGatewayResponse(resp) {
		// A response an output guardrail blocked reaches the caller as an error, but the
		// provider produced and billed it, and the gateway priced it.
		if r.gateway != nil {
			if cost, ok := r.gateway.cost(); ok {
				r.recordUsage(r.withGatewayUsage(gatewaytypes.TokenUsage{}, cost))
			}
		}
		wrapRecordingResponse(resp, r.audit, r.usage, r.client)
		return nil
	}

	r.c = resp.Body
	r.b = bufio.NewReader(resp.Body)
	r.stream = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	resp.Body = r
	return nil
}

// applyServedModel costs and records a gateway response's usage by the model that served it.
// The gateway's routing rules and fallbacks can serve a request with another model than the
// requested one, which may cost more or less. The LLM audit log keeps the requested model, and
// records the served one separately.
func (r *responseModifier) applyServedModel(resp *http.Response) {
	// A refused request names the model it was routed to, but wasn't served by it.
	if r.modelProvider != system.LLMGatewayModelProvider || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return
	}
	// The last attempt the gateway admitted names the serving model reliably; the response
	// headers carry the engine's wire name, which differs when a key maps model aliases.
	header := resp.Header
	if r.gateway != nil {
		if served, _ := r.gateway.served(); served != "" {
			if provider, model, ok := strings.Cut(served, "/"); ok {
				header = http.Header{}
				header.Set(headerServedProvider, provider)
				header.Set(headerServedModel, model)
			}
		}
	}
	m := servedModelChange(header, r.model, func(served string) (*v1.Model, error) {
		return r.mapHelper.ResolveTargetModel(system.LLMGatewayModelProvider, served)
	})
	if m == nil {
		return
	}
	r.model = m.Spec.Manifest.TargetModel
	r.modelID = m.Name
	r.tokenUsageTracker.setModelCost(*m)
}

// servedModelChange returns the model that served a gateway response when it isn't the
// requested one, and nil when it is or can't be told.
func servedModelChange(header http.Header, requested string, resolve func(string) (*v1.Model, error)) *v1.Model {
	provider, model := header.Get(headerServedProvider), header.Get(headerServedModel)
	if provider == "" || model == "" {
		return nil
	}
	served := provider + "/" + model
	if served == requested {
		return nil
	}
	m, err := resolve(served)
	if err != nil || m == nil {
		slog.Warn("LLM gateway response was served by an unknown model; recording it against the requested model", "requested", requested, "served", served, "error", err)
		return nil
	}
	return m
}

func shouldWrapLLMResponse(resp *http.Response) bool {
	if resp.StatusCode != http.StatusOK {
		return false
	}
	switch resp.Request.URL.Path {
	case "/v1/messages", "/anthropic/v1/messages", "/v1/responses", "/openai/v1/responses":
		return true
	default:
		return false
	}
}

// shouldWrapGatewayResponse reports whether an LLM gateway response carries usage that
// Obot must meter. The gateway also serves Chat Completions and embeddings, which the
// per-provider routes pass through unmetered.
func (r *responseModifier) shouldWrapGatewayResponse(resp *http.Response) bool {
	if r.modelProvider != system.LLMGatewayModelProvider || resp.StatusCode != http.StatusOK {
		return false
	}
	switch resp.Request.URL.Path {
	case "/v1/chat/completions", "/v1/embeddings":
		return true
	default:
		return false
	}
}

// filterModelListResponse rewrites a provider models-list response, dropping any
// model whose id is not in allowedTargetModels (the set the user may use). Both
// the Anthropic and OpenAI list endpoints return a top-level
// {"data": [{"id": ...}, ...]} envelope, so the same filtering applies to both
// passthroughs. Surviving entries are kept as their exact upstream bytes — no
// map[string]any round-trip that would reorder object keys or reformat numbers.
// The body is copied up front and restored, so any failure (or an unrecognized
// payload) just forwards the original response untouched.
func filterModelListResponse(resp *http.Response, allowedTargetModels map[string]bool, allowAllModels bool) error {
	if resp.StatusCode != http.StatusOK || allowAllModels {
		return nil
	}

	// copyBody restores resp.Body, so every early return below forwards the
	// original response unchanged; we only swap in the filtered body once it's
	// fully built.
	body, err := copyBody(&resp.Body)
	if err != nil {
		return err
	}

	// Only rewrite a recognized {"data": [...]} list envelope.
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() {
		return nil
	}

	// Keep each allowed entry exactly as the provider encoded it (its raw bytes).
	var kept []string
	data.ForEach(func(_, entry gjson.Result) bool {
		if allowedTargetModels[entry.Get("id").String()] {
			kept = append(kept, entry.Raw)
		}
		return true
	})

	out, err := sjson.SetRawBytes(body, "data", []byte("["+strings.Join(kept, ",")+"]"))
	if err != nil {
		return nil
	}
	out, err = rewriteAnthropicListCursors(out)
	if err != nil {
		return nil
	}

	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	resp.Header.Del("Content-Encoding")

	return nil
}

// rewriteAnthropicListCursors repoints first_id/last_id at the (already filtered)
// data array so the Anthropic pagination cursors never reference a model the user
// can't see (the SDK paginates off last_id). OpenAI lists carry no such fields
// and are returned unchanged. On an empty page the cursors are removed rather
// than nulled, except last_id is kept when more pages remain so the client can
// still advance.
func rewriteAnthropicListCursors(body []byte) ([]byte, error) {
	hasFirst := gjson.GetBytes(body, "first_id").Exists()
	hasLast := gjson.GetBytes(body, "last_id").Exists()
	if !hasFirst && !hasLast {
		return body, nil
	}

	var err error
	if ids := gjson.GetBytes(body, "data.#.id").Array(); len(ids) > 0 {
		if hasFirst {
			if body, err = sjson.SetBytes(body, "first_id", ids[0].String()); err != nil {
				return nil, err
			}
		}
		if hasLast {
			if body, err = sjson.SetBytes(body, "last_id", ids[len(ids)-1].String()); err != nil {
				return nil, err
			}
		}
		return body, nil
	}

	// Empty page.
	if hasFirst {
		if body, err = sjson.DeleteBytes(body, "first_id"); err != nil {
			return nil, err
		}
	}
	if hasLast && !gjson.GetBytes(body, "has_more").Bool() {
		if body, err = sjson.DeleteBytes(body, "last_id"); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func (r *responseModifier) Read(p []byte) (int, error) {
	if !r.stream {
		n, err := r.b.Read(p)
		if n > 0 {
			_, _ = r.nonStreamResponse.Write(p[:n])
			r.audit.captureResponseChunk(p[:n])
		}
		if errors.Is(err, io.EOF) {
			r.parseNonStreamTokenUsage()
		}
		return n, err
	}

	if len(r.leftover) > 0 {
		n := copy(p, r.leftover)
		r.leftover = r.leftover[n:]
		r.audit.captureResponseChunk(p[:n])
		return n, nil
	}

	line, err := r.b.ReadBytes('\n')
	if len(line) > 0 && errors.Is(err, io.EOF) {
		// Don't send an EOF until we read everything.
		err = nil
	}
	if err != nil {
		n := copy(p, line)
		r.audit.captureResponseChunk(p[:n])
		return n, err
	}

	var prefix []byte
	if r.stream {
		prefix = []byte("data: ")
		rest, ok := bytes.CutPrefix(line, prefix)
		if !ok {
			// This isn't a data line, so send it through.
			n := copy(p, line)
			r.audit.captureResponseChunk(p[:n])
			return n, nil
		}
		line = rest
	}

	r.tokenUsageTracker.addTokenUsage(line)

	n := copy(p, prefix)
	if n < len(prefix) {
		// We couldn't copy the entire prefix, so save the leftover for the next read and return.
		r.leftover = append(prefix[n:], line...)
		r.audit.captureResponseChunk(p[:n])
		return n, nil
	}

	n += copy(p[n:], line)

	// If we didn't copy the entire line, then save the leftover for the next read.
	r.leftover = line[n-len(prefix):]

	r.audit.captureResponseChunk(p[:n])
	return n, nil
}

func (r *responseModifier) parseNonStreamTokenUsage() {
	if r.stream || r.nonStreamParsed {
		return
	}
	r.tokenUsageTracker.addTokenUsage(r.nonStreamResponse.Bytes())
	r.nonStreamParsed = true
}

func (r *responseModifier) Close() error {
	r.parseNonStreamTokenUsage()
	if r.tokenUsageTracker != nil {
		usage := r.tokenUsageTracker.getTokenUsage()
		if r.gateway != nil {
			if cost, ok := r.gateway.cost(); ok {
				usage = r.withGatewayUsage(usage, cost)
			}
		}
		r.recordUsage(usage)
	}
	// ReverseProxy does not return body-copy errors to the handler, so Close is
	// the terminal point for proxied responses. The handler defer is a fallback.
	err := r.c.Close()
	r.audit.finish(r.client, err)
	r.usage.finish(err)
	return err
}

// recordUsage records a response's usage in the audit log and token usage, and charges a
// gateway request's budgets with it. A response the caller's Claude subscription paid for
// cost Obot nothing, so its tokens are recorded without spend.
func (r *responseModifier) recordUsage(usage gatewaytypes.TokenUsage) {
	if r.gateway != nil && r.gateway.paidBySubscription() {
		usage.InputSpend, usage.CacheReadSpend, usage.CacheWriteSpend, usage.OutputSpend, usage.TotalSpend = 0, 0, 0, 0, 0
	}
	r.audit.setTokenUsage(usage)
	r.usage.setTokenUsage(r.model, usage)
	if r.gateway != nil {
		_, budgetIDs := r.gateway.served()
		// Budgets are counted against the owner, like token usage.
		if err := r.client.RecordLLMGatewayBudgetUsage(context.Background(), budgetIDs, r.user.GetUID(), r.modelID, usage); err != nil {
			slog.Warn("failed to record LLM gateway budget usage", "userID", r.user.GetUID(), "error", err)
		}
	}
}

// withGatewayUsage completes usage parsed from a gateway response with what the gateway
// priced it at. The gateway prices with its model catalog, which knows more about how each
// provider bills than Obot's per-token rates; an administrator's cost override for the model
// still wins. When the response carried no usage the caller could see, such as one an output
// guardrail blocked, the gateway's token counts are used.
func (r *responseModifier) withGatewayUsage(usage gatewaytypes.TokenUsage, cost guardrails.Cost) gatewaytypes.TokenUsage {
	if usage.TotalTokens == 0 {
		usage = gatewayTokenUsage(cost)
		if r.tokenUsageTracker.costOverridden() {
			return r.tokenUsageTracker.price(usage)
		}
	}
	if r.tokenUsageTracker.costOverridden() {
		return usage
	}
	return withGatewayCost(usage, cost)
}

// llmRewriteRequest rewrites requests for the upstream model provider. Director
// made ReverseProxy append the client IP to X-Forwarded-For automatically, so
// that is replicated here. SetXForwarded is deliberately not used: it would
// additionally send X-Forwarded-Host and X-Forwarded-Proto, which Director never
// did and which would expose internal hostnames to upstream model providers.
func llmRewriteRequest(u url.URL) func(*httputil.ProxyRequest) {
	return func(r *httputil.ProxyRequest) {
		if clientIP, _, err := net.SplitHostPort(r.In.RemoteAddr); err == nil {
			if prior := r.In.Header.Values("X-Forwarded-For"); len(prior) > 0 {
				clientIP = strings.Join(prior, ", ") + ", " + clientIP
			}
			r.Out.Header.Set("X-Forwarded-For", clientIP)
		}

		urlCopy := u // avoid mutating the original url.URL across requests
		req := r.Out
		reqPath := req.PathValue("path")
		switch {
		case urlCopy.Path == "":
			// Upstream base has no path. Ensure exactly one /v1 prefix in the
			// final URL regardless of whether the client supplied it.
			if strings.HasPrefix(reqPath, "v1/") || reqPath == "v1" {
				urlCopy.Path = "/"
			} else {
				urlCopy.Path = "/v1"
			}
		case strings.HasSuffix(urlCopy.Path, "/v1"):
			// The upstream base already ends in /v1. Strip the same prefix from
			// the client path so we don't produce /v1/v1/...
			reqPath = strings.TrimPrefix(reqPath, "v1/")
			if reqPath == "v1" {
				reqPath = ""
			}
		}
		urlCopy.Path = path.Join(urlCopy.Path, reqPath)
		req.URL = &urlCopy
		req.Host = urlCopy.Host

		// Ensure the upstream transport can transparently decode compressed responses.
		// If we forward Accept-Encoding from the client, net/http transport will not
		// auto-decompress and token usage parsing can see compressed bytes.
		req.Header.Del("Accept-Encoding")
	}
}

func (l *llmProviderProxy) proxy(req api.Context) (retErr error) {
	l.lock.RLock()
	modelProvider := l.modelProvider
	l.lock.RUnlock()

	if modelProvider == nil {
		modelProvider = new(v1.ModelProvider)
		if err := req.Get(modelProvider, l.backend.modelProviderName()); err != nil {
			return fmt.Errorf("model provider %s not found: %w", l.backend.modelProviderName(), err)
		}

		l.lock.Lock()
		l.modelProvider = modelProvider
		l.lock.Unlock()
	}

	var (
		audit *llmAuditRecorder
		// usage records the request in token usage once it is sent to the gateway.
		usage *llmUsageRecorder
	)
	if req.GatewayClient.LLMAuditLogEnabled() {
		audit = newLLMAuditRecorder(req.Request, req.User, defaultLLMAuditLogResponseCaptureLimit)
	}
	defer func() {
		audit.finish(req.GatewayClient, retErr)
		usage.finish(retErr)
	}()
	audit.setModel(l.backend.modelProviderName(), "", "")

	u, routeDialect, err := l.backend.upstreamURL(req.Request)
	if err != nil {
		return err
	}

	var limit int64
	if limiter, ok := l.backend.(bodyLimitingBackend); ok {
		limit = limiter.maxRequestBodyBytes()
	}
	if limit > 0 {
		if req.ContentLength > limit {
			return writeRequestTooLarge(req, audit, limit)
		}
		req.Request.Body = http.MaxBytesReader(req.ResponseWriter, req.Request.Body, limit)
	}
	body, err := copyBody(&req.Request.Body)
	if err != nil {
		if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
			return writeRequestTooLarge(req, audit, limit)
		}
		return fmt.Errorf("failed to copy body: %w", err)
	}
	audit.setRequestBody(body)
	audit.setClientSessionID(routeDialect, req.Request.Header, body)
	audit.setReasoningEffort(l.backend.modelProviderName(), body)

	prepared := &preparedLLMProxyRequest{body: body}
	var resolvedModel *v1.Model

	targetModel := extractModelFromBody(body)
	if targetModel == "" {
		if _, gated := l.backend.(modelResolvingBackend); gated && !isModelsListRequest(req.Request) {
			// Backends that can reach any model authorize every request by the model it
			// names, so a request that names none cannot be authorized.
			return types2.NewErrBadRequest("a model is required")
		}
	}
	if targetModel != "" {
		audit.setModel(modelProvider.Name, "", targetModel)

		model, err := l.mapHelper.ResolveModelReference(req.Context(), req.Storage, modelProvider.Namespace, modelProvider.Name, targetModel)
		if resolver, ok := l.backend.(modelResolvingBackend); ok && apierrors.IsNotFound(err) {
			if fallback, fallbackErr := resolver.resolveModel(l, req.User, targetModel); fallbackErr == nil && fallback != nil {
				model, err = fallback, nil
			}
		}
		if apierrors.IsNotFound(err) {
			return types2.NewErrNotFound("model %q is not available through this endpoint", targetModel)
		}
		if err != nil {
			return fmt.Errorf("failed to get model: %w", err)
		}
		resolvedModel = model
		if model.Spec.Manifest.ModelProvider != modelProvider.Name {
			return types2.NewErrBadRequest("requested model does not match configured provider %q", targetModel)
		}
		prepared.model = model.Spec.Manifest.TargetModel
		audit.setModel(modelProvider.Name, model.Name, prepared.model)

		hasAccess, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name)
		if err != nil {
			return fmt.Errorf("failed to check user access to model %q: %w", model.Name, err)
		}
		if !hasAccess {
			return types2.NewErrForbidden("user does not have permission to use model %q", targetModel)
		}

		if b, ok := l.backend.(trackerDialectBackend); ok {
			prepared.tokenUsageTracker = newTokenUsageTrackerForDialect(*model, b.trackerDialect(routeDialect))
		} else {
			prepared.tokenUsageTracker = newTokenUsageTracker(*model)
		}

		prepared.body, err = rewriteModelInBody(body, prepared.model)
		if err != nil {
			return fmt.Errorf("failed to rewrite model in request body: %w", err)
		}
		if authorizer, ok := l.backend.(extraModelAuthorizingBackend); ok {
			if prepared.body, err = authorizer.authorizeExtraModels(l, req, prepared.body); err != nil {
				return err
			}
		}
		audit.setRequestBody(prepared.body)
	}

	req.Request.Body = io.NopCloser(bytes.NewReader(prepared.body))
	req.ContentLength = int64(len(prepared.body))

	// The transport fills in how the gateway routed the request as the response arrives.
	routing := &gatewayRouting{}
	req.Request = req.WithContext(withGatewayRouting(req.Context(), routing))
	var gateway gatewayRequest
	route := func() llmRoute {
		return servedRoute(gateway, routing.get())
	}
	audit.setRoute(route)
	if resolvedModel != nil && meteredLLMEndpoint(req.PathValue("path")) {
		usage = newLLMUsageRecorder(req.User, req.GatewayClient, prepared.model, route)
	}

	// Budgets are checked for each model the gateway tries, when it tries it.
	gateway, err = l.backend.decorateRequest(l, req, resolvedModel)
	if err != nil {
		return err
	}
	defer gateway.release()
	audit.setGuardrailFindings(gateway.guardrailFindings)

	transport, err := l.backend.transport()
	if err != nil {
		return err
	}

	modifier := &responseModifier{
		user:              req.User,
		model:             prepared.model,
		modelProvider:     modelProvider.Name,
		routeDialect:      routeDialect,
		client:            req.GatewayClient,
		tokenUsageTracker: prepared.tokenUsageTracker,
		gateway:           gateway,
		modelID:           modelName(resolvedModel),
		mapHelper:         l.mapHelper,
		audit:             audit,
		usage:             usage,
		clientHeader:      req.ResponseWriter.Header(),
	}
	if lister, ok := l.backend.(modelListingBackend); ok && isModelsListRequest(req.Request) {
		modifier.unlisted = lister.unlistedModels
	}

	var proxyErr error
	(&httputil.ReverseProxy{
		Rewrite:   llmRewriteRequest(u),
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			proxyErr = err
			audit.recordResponseStatus(http.StatusBadGateway)
			audit.finish(req.GatewayClient, err)
			usage.recordResponseStatus(http.StatusBadGateway)
			usage.finish(err)
			slog.Warn("LLM provider proxy error", "error", err)
			llmtypes.WriteError(w, req.Request, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		},
		ModifyResponse: modifier.modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)
	if proxyErr != nil {
		return nil
	}

	return nil
}

// isModelsListRequest reports whether req targets the provider models-list
// endpoint (GET .../v1/models) on the passthrough routes.
func isModelsListRequest(req *http.Request) bool {
	if req.Method != http.MethodGet {
		return false
	}
	// The {path...} route value holds the client path, e.g. "v1/models".
	return strings.TrimPrefix(strings.Trim(req.PathValue("path"), "/"), "v1/") == "models"
}

// servedRoute is how the gateway served a request. The model the gateway last admitted, as
// Obot names it, is the reliable name of the serving model; the engine reports the gateway
// key, whether a fallback served the request, and the route of requests it refused before
// admitting a model.
func servedRoute(gateway gatewayRequest, reported llmRoute) llmRoute {
	if gateway == nil {
		return reported
	}
	if gateway.lastRefused() {
		// The engine reports the refused attempt's route, but nothing served the request.
		return llmRoute{}
	}
	if admitted, _ := gateway.served(); admitted != "" {
		if provider, model, ok := strings.Cut(admitted, "/"); ok {
			if provider != reported.Provider {
				// The engine reported another attempt, so its key isn't the admitted one's.
				reported.Key = ""
			}
			reported.Provider, reported.Model = provider, model
		}
		if gateway.paidBySubscription() {
			// The caller's own credential served it, not a gateway key.
			reported.Subscription, reported.Key = true, ""
		}
	}
	return reported
}

func modelName(model *v1.Model) string {
	if model == nil {
		return ""
	}
	return model.Name
}

// writeRequestTooLarge refuses a request whose body is over the gateway's request size
// limit, with a 413 in the client's API format.
func writeRequestTooLarge(req api.Context, audit *llmAuditRecorder, limit int64) error {
	message := fmt.Sprintf("the request body is larger than the LLM gateway's limit of %d MB", limit>>20)
	var body any
	if p := strings.Trim(req.PathValue("path"), "/"); strings.HasSuffix(p, "/messages") || strings.HasSuffix(p, "/messages/count_tokens") {
		body = map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "request_too_large", "message": message},
		}
	} else {
		body = map[string]any{
			"error": map[string]any{"message": message, "type": "invalid_request_error", "code": "request_too_large"},
		}
	}
	data, _ := json.Marshal(body)
	audit.recordResponseStatus(http.StatusRequestEntityTooLarge)
	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	req.ResponseWriter.Header().Set("X-Should-Retry", "false")
	req.WriteHeader(http.StatusRequestEntityTooLarge)
	_, err := req.ResponseWriter.Write(append(data, '\n'))
	return err
}

// budgetExceededResponse is the response to a request refused because a budget is
// exhausted: a 429 in the client's API format, with X-Should-Retry: false so clients such
// as Claude Code don't retry it, and Retry-After when the budget's window resets.
func budgetExceededResponse(path string, exceeded *client.LLMGatewayBudgetExceeded, now time.Time) (http.Header, []byte) {
	var body any
	if strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/messages/count_tokens") {
		body = map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "rate_limit_error", "message": exceeded.Error()},
		}
	} else {
		body = map[string]any{
			"error": map[string]any{"message": exceeded.Error(), "type": budgetExceededType, "code": budgetExceededType},
		}
	}
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("X-Should-Retry", "false")
	if !exceeded.ResetAt.IsZero() && exceeded.ResetAt.After(now) {
		header.Set("Retry-After", strconv.Itoa(int(math.Ceil(exceeded.ResetAt.Sub(now).Seconds()))))
	}
	data, _ := json.Marshal(body)
	return header, append(data, '\n')
}

// isBudgetRefusal reports whether a gateway 429 is the budget refusal itself. The gateway
// writes the refusal in the client's API format: OpenAI formats keep its budget_exceeded
// type, but the Anthropic format replaces it with rate_limit_error and keeps only the
// message.
func isBudgetRefusal(body []byte, exceeded *client.LLMGatewayBudgetExceeded) bool {
	return gjson.GetBytes(body, "error.type").String() == budgetExceededType ||
		gjson.GetBytes(body, "type").String() == budgetExceededType ||
		gjson.GetBytes(body, "error.message").String() == exceeded.Error()
}

// rewriteBudgetExceeded turns the gateway's refusal of a request whose budgets are exhausted
// into the budget error clients are given.
func (r *responseModifier) rewriteBudgetExceeded(resp *http.Response) {
	if r.gateway == nil || resp.StatusCode != http.StatusTooManyRequests {
		return
	}
	exceeded := r.gateway.budgetExceeded()
	if exceeded == nil {
		return
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || !isBudgetRefusal(body, exceeded) {
		// A provider's own rate limit, after a budget refused an earlier attempt.
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return
	}
	header, data := budgetExceededResponse(resp.Request.URL.Path, exceeded, time.Now())
	if r.clientHeader != nil {
		// Obot's per-second API rate limit headers would say the caller has requests left.
		ratelimiter.ClearHeaders(r.clientHeader)
	}
	maps.Copy(resp.Header, header)
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	resp.ContentLength = int64(len(data))
	resp.Body = io.NopCloser(bytes.NewReader(data))
}
