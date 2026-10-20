package server

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// settingsCacheTTL is how long the proxy uses the LLM gateway settings it read, so a change
	// reaches every replica within seconds without a database read per request.
	settingsCacheTTL = 5 * time.Second

	// headerServedProvider and headerServedModel name the provider and model that served a
	// gateway response, which routing rules and fallbacks can make differ from the request's.
	headerServedProvider = "X-Obot-LLM-Provider"
	headerServedModel    = "X-Obot-LLM-Model"
)

var (
	// clientGatewayHeaders are the request headers callers may use to tune the gateway for
	// their own requests, and the engine headers they map to. Everything else the engine reads
	// from headers (such as direct keys) is controlled by Obot.
	clientGatewayHeaders = map[string]string{
		"X-Obot-Session-ID": "x-bf-session-id",
	}
)

// bifrostProviderBackend sends LLM proxy requests to the embedded Bifrost gateway. Obot
// still authenticates the caller, checks model access, enforces budgets and guardrails,
// and records audit logs and usage; Bifrost then routes to the upstream provider
// with load balancing, fallbacks and routing rules.
type bifrostProviderBackend struct {
	engine   *llmengine.Engine
	settings settingsCache
}

// settingsCache holds the LLM gateway settings the proxy last read.
type settingsCache struct {
	lock     sync.Mutex
	settings gatewaytypes.LLMGatewaySettings
	read     time.Time
}

// bifrostRoute maps a client path under /api/llm-proxy/v1 to a Bifrost route.
type bifrostRoute struct {
	// base is the Bifrost path prefix the client path is joined to.
	base string
	// dialect is the API shape of the request and response.
	dialect llmtypes.Dialect
}

// sanitizingTransport strips implementation details from gateway responses before they
// reach clients: engine-specific headers (which include upstream key names), the engine's
// CORS headers, and the engine's extra response fields. The provider and model that served
// the request are passed on under Obot's own header names.
type sanitizingTransport struct {
	next http.RoundTripper
}

// sseSanitizer strips the engine's fields from each "data:" event of a stream, one line at a
// time so streaming isn't delayed.
type sseSanitizer struct {
	r       *bufio.Reader
	c       io.Closer
	pending []byte
	routing *gatewayRouting
}

// gatewayRouting is how the gateway routed one request, as the engine reported it: the
// provider, model and key of the attempt that answered, and whether that attempt was a
// fallback. The engine reports it in response headers and in its own response fields, which
// sanitizingTransport strips before the response reaches the client, so it is kept here for
// Obot's own records. The proxy puts it in the request's context for the transport to fill.
type gatewayRouting struct {
	lock  sync.Mutex
	route llmRoute
}

type gatewayRoutingContextKey struct{}

// gatewayAttempts tracks the models the gateway tries for one request. Its routing rules and
// fallbacks can serve a request with another model than the requested one, so each attempt
// is checked against the caller's access and the budgets of the model it tries.
type gatewayAttempts struct {
	unbind func()
	// subscription is true when the caller's Claude subscription token was kept for the
	// gateway to forward. Attempts the gateway sends with it are paid for by the caller's
	// subscription, not Obot's provider keys, so they don't spend budgets.
	subscription bool
	// identity is who the request is for, with the gateway profiles that apply to them, and
	// isAdmin whether they are an administrator, which some budgets exempt.
	identity guardrails.Identity
	isAdmin  bool

	lock     sync.Mutex
	budgets  map[string][]string
	exceeded *client.LLMGatewayBudgetExceeded
	// admitted is the last attempt admitted: the model ("provider/model") that served the
	// request when it succeeded, since the gateway tries models one at a time.
	admitted string
	// admittedSubscription is true when the caller's Claude subscription pays for the last
	// attempt admitted.
	admittedSubscription bool
	// refused is true when the last attempt was refused, so no model served the request.
	refused bool
	// detections are what the guardrails did to the request, by stage (input or output).
	detections map[string][]guardrails.Detection
	// priced is what the gateway priced the response at, when it did.
	priced *guardrails.Cost
}

func (c *settingsCache) get(ctx context.Context, gatewayClient *client.Client) gatewaytypes.LLMGatewaySettings {
	c.lock.Lock()
	defer c.lock.Unlock()
	if time.Since(c.read) < settingsCacheTTL {
		return c.settings
	}
	settings, err := gatewayClient.GetLLMGatewaySettings(ctx)
	if err != nil {
		// Keep the last settings read; they are the safest guess until the next read.
		slog.Warn("failed to read LLM gateway settings", "error", err)
		return c.settings
	}
	c.settings, c.read = settings, time.Now()
	return settings
}

// SetLLMGateway attaches the embedded Bifrost gateway, enabling the
// /api/llm-proxy/v1 routes.
func (s *Server) SetLLMGateway(engine *llmengine.Engine) {
	s.llmGateway = engine
}

// LLMGateway returns the embedded Bifrost gateway.
func (s *Server) LLMGateway() *llmengine.Engine {
	return s.llmGateway
}

func (s *Server) newBifrostLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		backend:   &bifrostProviderBackend{engine: s.llmGateway},
		mapHelper: s.mapHelper,
	}
}

func (b *bifrostProviderBackend) modelProviderName() string {
	return system.LLMGatewayModelProvider
}

// resolveBifrostRoute decides which Bifrost route serves a client path. Only model-bearing
// inference endpoints and model listing are exposed, because Obot authorizes each request
// by the model it names.
func resolveBifrostRoute(req *http.Request) (bifrostRoute, error) {
	reqPath := strings.Trim(req.PathValue("path"), "/")
	endpoint, hasV1 := strings.CutPrefix(reqPath, "v1/")
	if !hasV1 {
		return bifrostRoute{}, types2.NewErrBadRequest("unsupported LLM gateway path %q: paths must start with v1/", reqPath)
	}

	switch endpoint {
	case "messages", "messages/count_tokens":
		return bifrostRoute{base: "/anthropic", dialect: llmtypes.DialectAnthropicMessages}, nil
	case "models":
		// Anthropic clients send anthropic-version on every request, including model
		// listing, and expect Anthropic's list format back.
		if req.Header.Get("anthropic-version") != "" {
			return bifrostRoute{base: "/anthropic", dialect: llmtypes.DialectAnthropicMessages}, nil
		}
		return bifrostRoute{base: "/"}, nil
	case "responses", "responses/input_tokens":
		return bifrostRoute{base: "/", dialect: llmtypes.DialectOpenAIResponses}, nil
	case "chat/completions":
		return bifrostRoute{base: "/", dialect: llmtypes.DialectOpenAIChatCompletions}, nil
	case "embeddings":
		// Embedding responses report usage in the Chat Completions shape.
		return bifrostRoute{base: "/", dialect: llmtypes.DialectOpenAIChatCompletions}, nil
	default:
		// Other request types (legacy completions, images, audio, rerank) aren't exposed until
		// Obot can meter them against budgets and apply guardrails to them.
		return bifrostRoute{}, types2.NewErrBadRequest("unsupported LLM gateway endpoint %q", reqPath)
	}
}

func (b *bifrostProviderBackend) upstreamURL(req *http.Request) (url.URL, llmtypes.Dialect, error) {
	if !b.engine.Ready() {
		if err := b.engine.Err(); err != nil {
			return url.URL{}, "", types2.NewErrHTTP(http.StatusServiceUnavailable, "the LLM gateway failed to start")
		}
		return url.URL{}, "", types2.NewErrHTTP(http.StatusServiceUnavailable, "the LLM gateway is starting")
	}
	route, err := resolveBifrostRoute(req)
	if err != nil {
		return url.URL{}, "", err
	}
	u := b.engine.BaseURL()
	u.Path = route.base
	return u, route.dialect, nil
}

func (b *bifrostProviderBackend) maxRequestBodyBytes() int64 {
	return b.engine.MaxRequestBodyBytes()
}

func (b *bifrostProviderBackend) transport() (http.RoundTripper, error) {
	return sanitizingTransport{next: b.engine.Transport()}, nil
}

func withGatewayRouting(ctx context.Context, routing *gatewayRouting) context.Context {
	return context.WithValue(ctx, gatewayRoutingContextKey{}, routing)
}

func gatewayRoutingFrom(ctx context.Context) *gatewayRouting {
	routing, _ := ctx.Value(gatewayRoutingContextKey{}).(*gatewayRouting)
	return routing
}

// get returns the route the engine last reported.
func (g *gatewayRouting) get() llmRoute {
	if g == nil {
		return llmRoute{}
	}
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.route
}

func (g *gatewayRouting) set(route llmRoute) {
	if g == nil || (route.Provider == "" && route.Key == "") {
		return
	}
	g.lock.Lock()
	defer g.lock.Unlock()
	g.route = route
}

// recordHeaders keeps the route from the engine's response headers, which it sends on
// every route, including those whose bodies have no room for its fields, such as Anthropic's.
func (g *gatewayRouting) recordHeaders(header http.Header) {
	route := llmRoute{
		Provider: cmp.Or(header.Get("X-Bifrost-Routing-Info-Provider"), header.Get("X-Bifrost-Provider")),
		Model:    cmp.Or(header.Get("X-Bifrost-Routing-Info-Model"), header.Get("X-Bifrost-Resolved-Model")),
		Key:      header.Get("X-Bifrost-Routing-Info-Key"),
		Fallback: header.Get("X-Bifrost-Routing-Info-Is-Fallback") == "true" || header.Get("X-Bifrost-Fallback-Index") != "",
	}
	g.set(route)
}

// recordBody keeps the route from the engine's fields in a response object or stream event,
// which name the attempt that produced it. Errors carry them too.
func (g *gatewayRouting) recordBody(body []byte) {
	if g == nil {
		return
	}
	for _, path := range []string{"extra_fields.routing_info", "response.extra_fields.routing_info"} {
		info := gjson.GetBytes(body, path)
		if !info.IsObject() {
			continue
		}
		g.set(llmRoute{
			Provider: info.Get("provider").String(),
			Model:    info.Get("model").String(),
			Key:      info.Get("key").String(),
			Fallback: info.Get("is_fallback").Bool(),
		})
		return
	}
}

func (t sanitizingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	routing := gatewayRoutingFrom(req.Context())
	routing.recordHeaders(resp.Header)
	provider := resp.Header.Get("X-Bifrost-Provider")
	model := resp.Header.Get("X-Bifrost-Resolved-Model")
	for name := range resp.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-bifrost-") || strings.HasPrefix(lower, "x-bf-") ||
			strings.HasPrefix(lower, "access-control-") || lower == "server" || lower == "vary" {
			resp.Header.Del(name)
		}
	}
	if provider != "" {
		resp.Header.Set(headerServedProvider, provider)
	}
	if model != "" {
		resp.Header.Set(headerServedModel, model)
	}

	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/event-stream"):
		resp.Body = &sseSanitizer{r: bufio.NewReader(resp.Body), c: resp.Body, routing: routing}
	case strings.Contains(contentType, "application/json"):
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		routing.recordBody(body)
		body = stripEngineFields(body)
		if resp.StatusCode >= http.StatusBadRequest {
			body = llmengine.ScrubEngineNameBytes(body)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	return resp, nil
}

// stripEngineFields removes the engine's own fields from a JSON response object. Responses
// API stream events carry the response object nested under "response", where the engine
// adds its fields too. Model lists carry the health of each provider key the engine listed
// them with ("key_statuses"), which callers have no business seeing.
func stripEngineFields(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	for _, path := range []string{"extra_fields", "is_bifrost_error", "key_statuses", "response.extra_fields", "response.is_bifrost_error"} {
		if gjson.GetBytes(body, path).Exists() {
			if stripped, err := sjson.DeleteBytes(body, path); err == nil {
				body = stripped
			}
		}
	}
	return body
}

func (s *sseSanitizer) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		line, err := s.r.ReadBytes('\n')
		if len(line) > 0 {
			if data, ok := bytes.CutPrefix(line, []byte("data: ")); ok && len(data) > 0 && data[0] == '{' {
				trimmed := bytes.TrimRight(data, "\r\n")
				// The last event that names a route names the attempt that finished the stream.
				s.routing.recordBody(trimmed)
				event := stripEngineFields(trimmed)
				if gjson.GetBytes(event, "error").Exists() {
					event = llmengine.ScrubEngineNameBytes(event)
				}
				line = append(append([]byte("data: "), event...), data[len(trimmed):]...)
			}
			s.pending = line
		}
		if err != nil {
			if len(s.pending) == 0 {
				return 0, err
			}
			break
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *sseSanitizer) Close() error {
	return s.c.Close()
}

// trackerDialect lets usage tracking follow the route: every model is served in every
// dialect, so the model's native dialect says nothing about the response shape.
func (b *bifrostProviderBackend) trackerDialect(routeDialect llmtypes.Dialect) llmtypes.Dialect {
	return routeDialect
}

// unlistedModels returns the Anthropic models Obot lists for Claude subscriptions, which the
// gateway's own model listing leaves out while the Anthropic provider has no enabled keys.
// Claude Code discovers the models it offers from the model listing.
func (b *bifrostProviderBackend) unlistedModels(ctx context.Context) []llmengine.ListedModel {
	if !b.engine.Ready() {
		return nil
	}
	return b.engine.ClaudeSubscriptionModels(ctx)
}

// addListedModels adds models to a successful model listing response that doesn't list them,
// in its format: Anthropic's, which names models without their provider, or OpenAI's. They
// are added to the last page of a paginated listing.
func addListedModels(resp *http.Response, models []llmengine.ListedModel, anthropic bool) error {
	if len(models) == 0 || resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := copyBody(&resp.Body)
	if err != nil {
		return err
	}
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() || gjson.GetBytes(body, "has_more").Bool() {
		return nil
	}
	listed := map[string]bool{}
	for _, id := range data.Get("#.id").Array() {
		listed[id.String()] = true
	}
	added := false
	for _, m := range models {
		provider, id, _ := strings.Cut(m.ID, "/")
		var entry map[string]any
		if anthropic {
			entry = map[string]any{"type": "model", "id": id, "display_name": m.Name, "created_at": "1970-01-01T00:00:00Z"}
		} else {
			id = m.ID
			entry = map[string]any{"id": id, "object": "model", "created": 0, "owned_by": provider}
		}
		if listed[id] {
			continue
		}
		listed[id] = true
		if body, err = sjson.SetBytes(body, "data.-1", entry); err != nil {
			return nil
		}
		added = true
	}
	if !added {
		return nil
	}
	if body, err = rewriteAnthropicListCursors(body); err != nil {
		return nil
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Del("Content-Encoding")
	return nil
}

// resolveModel resolves a model name that names no gateway model exactly: a bare
// provider-native ID, or an undated alias of a dated model ID, with or without its provider.
// When several models match, it prefers one the caller may use.
func (b *bifrostProviderBackend) resolveModel(l *llmProviderProxy, user kuser.Info, reference string) (*v1.Model, error) {
	return l.mapHelper.ResolveUpstreamModelName(system.LLMGatewayModelProvider, reference, func(m *v1.Model) bool {
		ok, err := l.mapHelper.UserHasAccessToModel(user, m.Name)
		return err == nil && ok
	})
}

// authorizeModel resolves a model the gateway may serve a request with and checks that the
// caller may use it. what names the model in errors, such as "fallback model".
func (b *bifrostProviderBackend) authorizeModel(l *llmProviderProxy, req api.Context, reference, what string) (*v1.Model, error) {
	model, err := l.mapHelper.ResolveModelReference(req.Context(), req.Storage, system.DefaultNamespace, system.LLMGatewayModelProvider, reference)
	if apierrors.IsNotFound(err) {
		model, err = b.resolveModel(l, req.User, reference)
	}
	if err != nil || model == nil || model.Spec.Manifest.ModelProvider != system.LLMGatewayModelProvider {
		return nil, types2.NewErrBadRequest("%s %q is not available through this endpoint", what, reference)
	}
	if ok, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name); err != nil {
		return nil, err
	} else if !ok {
		return nil, types2.NewErrForbidden("user does not have permission to use %s %q", what, reference)
	}
	return model, nil
}

// served returns the model the last admitted attempt tried and the budgets that admitted it.
func (a *gatewayAttempts) served() (string, []string) {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.admitted, a.budgets[a.admitted]
}

// paidBySubscription reports whether the caller's Claude subscription paid for the last
// attempt admitted, rather than one of the gateway's provider keys.
func (a *gatewayAttempts) paidBySubscription() bool {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.admittedSubscription
}

func (a *gatewayAttempts) release() { a.unbind() }

// recordDetections keeps what a stage's guardrail policies did to the request. The input is
// checked again for every model the gateway tries, and the last check is the one that held.
func (a *gatewayAttempts) recordDetections(stage string, detections []guardrails.Detection) {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.detections == nil {
		a.detections = map[string][]guardrails.Detection{}
	}
	a.detections[stage] = detections
}

// recordCost keeps what the gateway priced the response at. A stream reports usage as it
// grows, so the highest price, or for a model without a price the most tokens, is the one for
// the whole response.
func (a *gatewayAttempts) recordCost(cost guardrails.Cost) {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.priced == nil {
		a.priced = &cost
		return
	}
	price, prior := cost.Input+cost.Output, a.priced.Input+a.priced.Output
	if price > prior || (price == prior && cost.InputTokens+cost.OutputTokens >= a.priced.InputTokens+a.priced.OutputTokens) {
		a.priced = &cost
	}
}

func (a *gatewayAttempts) cost() (guardrails.Cost, bool) {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.priced == nil {
		return guardrails.Cost{}, false
	}
	return *a.priced, true
}

// guardrailFindings returns what the guardrails did to the request, input first.
func (a *gatewayAttempts) guardrailFindings() []types2.LLMAuditGuardrailFinding {
	a.lock.Lock()
	defer a.lock.Unlock()
	var findings []types2.LLMAuditGuardrailFinding
	for _, stage := range []string{guardrails.StageInput, guardrails.StageOutput} {
		for _, d := range a.detections[stage] {
			findings = append(findings, types2.LLMAuditGuardrailFinding{
				Stage:         d.Stage,
				PolicyID:      d.PolicyID,
				PolicyName:    d.PolicyName,
				GuardrailName: d.GuardrailName,
				GuardrailType: d.GuardrailType,
				Outcome:       d.Outcome,
				Categories:    slices.Clone(d.Categories),
				Error:         d.Error,
			})
		}
	}
	return findings
}

// budgetExceeded is the budget that refused the last attempt, if one did.
func (a *gatewayAttempts) budgetExceeded() *client.LLMGatewayBudgetExceeded {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.exceeded
}

// check admits one attempt: the caller must be allowed to use the attempted model, and every
// budget that applies to it must have room. A caller may only be served by models they may
// use, whatever routed them there, and spends the budgets of the model that serves them,
// unless their own Claude subscription pays for it.
func (a *gatewayAttempts) check(b *bifrostProviderBackend, l *llmProviderProxy, apiReq api.Context, requested *v1.Model) guardrails.AttemptCheck {
	checkAttempt := a.checkAttempt(b, l, apiReq, requested)
	return func(ctx context.Context, attempt guardrails.Attempt) error {
		err := checkAttempt(ctx, attempt)
		a.lock.Lock()
		a.refused = err != nil
		a.lock.Unlock()
		return err
	}
}

// lastRefused reports whether the last attempt was refused, so no model served the request.
func (a *gatewayAttempts) lastRefused() bool {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.refused
}

// checkAttempt checks one attempt, and records the attempts it admits.
func (a *gatewayAttempts) checkAttempt(b *bifrostProviderBackend, l *llmProviderProxy, apiReq api.Context, requested *v1.Model) guardrails.AttemptCheck {
	return func(_ context.Context, attempt guardrails.Attempt) error {
		reference := attempt.Model
		if a.subscription && !strings.HasPrefix(reference, "anthropic/") {
			// The engine would send the caller's subscription token with this attempt, and only
			// Anthropic may receive it: a routing rule or fallback can't carry it elsewhere,
			// such as to a custom provider built on Anthropic's API.
			return &guardrails.AttemptError{Status: http.StatusForbidden, Type: "permission_denied", Message: fmt.Sprintf("requests made with a Claude subscription can only be served by Anthropic models, not %q", reference)}
		}
		model := requested
		if requested == nil || reference != requested.Spec.Manifest.TargetModel {
			// The requested model was checked before the request was sent.
			var err error
			if model, err = b.authorizeModel(l, apiReq, reference, "model"); err != nil {
				if httpErr, ok := errors.AsType[*types2.ErrHTTP](err); ok {
					// The gateway reports the message with its own status.
					return &guardrails.AttemptError{Status: httpErr.Code, Type: "permission_denied", Message: httpErr.Message}
				}
				return err
			}
		}
		if a.subscription && attempt.Subscription {
			a.lock.Lock()
			a.budgets[reference] = nil
			a.admitted, a.admittedSubscription = reference, true
			a.lock.Unlock()
			return nil
		}
		// A budget covers only its own profile's models: only the caller's profiles that grant
		// this model bring their budgets. Guardrail policies still apply through every profile.
		profileIDs, err := l.mapHelper.UserProfileIDsForModel(apiReq.User, model.Name)
		if err != nil {
			return &guardrails.AttemptError{Status: http.StatusInternalServerError, Type: "api_error", Message: "failed to check LLM gateway budgets"}
		}
		ids, err := admit(apiReq.Context(), apiReq.GatewayClient, a.identity.UserID, profileIDs, a.isAdmin, model)
		if exceeded := (*client.LLMGatewayBudgetExceeded)(nil); errors.As(err, &exceeded) {
			a.lock.Lock()
			a.exceeded = exceeded
			a.lock.Unlock()
			return &guardrails.AttemptError{Status: http.StatusTooManyRequests, Type: budgetExceededType, Message: exceeded.Error()}
		} else if err != nil {
			return &guardrails.AttemptError{Status: http.StatusInternalServerError, Type: "api_error", Message: "failed to check LLM gateway budgets"}
		}
		a.lock.Lock()
		a.budgets[reference] = ids
		a.admitted, a.admittedSubscription = reference, false
		a.lock.Unlock()
		return nil
	}
}

// authorizeExtraModels checks every fallback model the request names. Bifrost tries
// fallbacks when the primary model fails, so an unchecked fallback would let a caller reach
// a model their access policy doesn't allow. A fallback the caller may not use is skipped, as
// the gateway skips any attempt on such a model, so the request's other fallbacks can still
// serve it. A fallback that isn't available at all fails the request, since it is likely a
// mistake the caller should hear about.
func (b *bifrostProviderBackend) authorizeExtraModels(l *llmProviderProxy, req api.Context, body []byte) ([]byte, error) {
	fallbacks := gjson.GetBytes(body, "fallbacks")
	if !fallbacks.Exists() {
		return body, nil
	}
	if !fallbacks.IsArray() {
		return nil, types2.NewErrBadRequest("fallbacks must be an array of model names")
	}

	resolved := []string{}
	for _, f := range fallbacks.Array() {
		reference := f.String()
		if f.IsObject() {
			reference = f.Get("provider").String() + "/" + f.Get("model").String()
		}
		model, err := b.authorizeModel(l, req, reference, "fallback model")
		if httpErr, ok := errors.AsType[*types2.ErrHTTP](err); ok && httpErr.Code == http.StatusForbidden {
			continue
		} else if err != nil {
			return nil, err
		}
		resolved = append(resolved, model.Spec.Manifest.TargetModel)
	}

	// Send Bifrost the resolved provider-qualified names.
	return sjson.SetBytes(body, "fallbacks", resolved)
}

// isAnthropicOAuthBearer reports whether an Authorization header carries an Anthropic OAuth
// access token, which Claude Code sends when signed in with a Claude subscription.
func isAnthropicOAuthBearer(header string) bool {
	return strings.HasPrefix(strings.ToLower(header), "bearer sk-ant-oat")
}

// stripCallerCredentials removes the caller's credentials and engine headers. A caller's
// Claude subscription token is kept, when an admin allows it, for an Anthropic model: it is
// then the upstream credential, which the engine forwards to Anthropic for Claude Code
// requests instead of choosing a configured key. It reports whether the token was kept.
func stripCallerCredentials(req *http.Request, model *v1.Model, subscriptionPassthrough bool) bool {
	subscription := subscriptionPassthrough && model != nil &&
		strings.HasPrefix(model.Spec.Manifest.TargetModel, "anthropic/") &&
		isAnthropicOAuthBearer(req.Header.Get("Authorization"))
	for _, h := range []string{"Authorization", "X-Api-Key", "Api-Key", "X-Goog-Api-Key", "Cookie", guardrails.HeaderRequestToken, HeaderObotAPIKey} {
		if subscription && h == "Authorization" {
			continue
		}
		req.Header.Del(h)
	}
	// Never let callers pick a virtual key or set other engine options of their own.
	for name := range req.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-bf-") {
			req.Header.Del(name)
		}
	}
	return subscription
}

// decorateRequest removes the caller's credentials and adds the guardrails token, which tells
// the gateway's guardrail plugins who the request is for and checks each model it tries.
func (b *bifrostProviderBackend) decorateRequest(l *llmProviderProxy, apiReq api.Context, model *v1.Model) (gatewayRequest, error) {
	ctx, gatewayClient, req, user := apiReq.Context(), apiReq.GatewayClient, apiReq.Request, apiReq.User
	allowSubscription := b.settings.get(ctx, gatewayClient).SubscriptionPassthrough
	if allowSubscription && isAnthropicOAuthBearer(req.Header.Get("Authorization")) {
		// Checked per request, not cached, so a provider change can't redirect the token.
		allowSubscription = b.engine.AnthropicIsDirect(ctx)
	}
	subscription := stripCallerCredentials(req, model, allowSubscription)
	for client, engine := range clientGatewayHeaders {
		if v := req.Header.Get(client); v != "" {
			req.Header.Set(engine, v)
			req.Header.Del(client)
		}
	}

	// Tell the gateway's guardrail plugins who the request is for. Only a reference is sent;
	// the identity itself stays in this process.
	identity, isAdmin, err := l.gatewayIdentity(user)
	if err != nil {
		return nil, err
	}
	attempts := &gatewayAttempts{budgets: map[string][]string{}, subscription: subscription, identity: identity, isAdmin: isAdmin}
	token, unbind := guardrails.BindRequest(identity, guardrails.RequestHooks{
		AttemptCheck: attempts.check(b, l, apiReq, model),
		Detections:   attempts.recordDetections,
		Cost:         attempts.recordCost,
	})
	attempts.unbind = unbind
	req.Header.Set(guardrails.HeaderRequestToken, token)
	return attempts, nil
}

// admit enforces LLM gateway budgets. It replaces the daily token quota for gateway traffic:
// budgets are enforced by Obot from the usage it records in its own database, so usage is
// counted exactly across replicas. A request is admitted while its budgets have room and its
// usage is recorded when it finishes, so requests running at the same time can together go
// past a limit. profileIDs are the caller's profiles that grant the model; only their budgets
// apply.
func admit(ctx context.Context, gatewayClient *client.Client, userID string, profileIDs []string, isAdmin bool, model *v1.Model) ([]string, error) {
	budgets, err := gatewayClient.LLMGatewayBudgetsForUser(ctx, profileIDs, isAdmin, model.Name)
	if err != nil {
		return nil, err
	}
	if err := gatewayClient.CheckLLMGatewayBudgets(ctx, budgets, userID); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(budgets))
	for _, budget := range budgets {
		ids = append(ids, budget.ID)
	}
	return ids, nil
}

// gatewayIdentity resolves whose guardrails apply to a request: the caller, and
// the gateway profiles that apply to them.
func (l *llmProviderProxy) gatewayIdentity(user kuser.Info) (guardrails.Identity, bool, error) {
	identity, isAdmin, err := guardrails.ResolveIdentity(context.Background(), nil, user)
	if err != nil {
		return guardrails.Identity{}, false, err
	}
	if identity.ProfileIDs, err = l.mapHelper.UserProfileIDs(user); err != nil {
		return guardrails.Identity{}, false, fmt.Errorf("failed to resolve gateway profiles: %w", err)
	}
	return identity, isAdmin, nil
}
