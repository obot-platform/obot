package handlers

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// allowedBifrostAPIPrefixes are the Bifrost management routes exposed through Obot. Bifrost
	// also has MCP, skills, prompt repository, and session APIs; Obot has its own versions of
	// those features, so they stay hidden. Its plugins and response cache aren't exposed
	// either: Obot's guardrail plugins must stay enabled, and responses aren't cached. Its
	// request log is off, since Obot keeps its own records of requests (the LLM audit log and
	// token usage), so its log APIs aren't exposed. Its feature flags only turn on engine
	// features Obot doesn't expose, so they aren't exposed either.
	allowedBifrostAPIPrefixes = []string{
		"providers",
		"keys",
		"models",
		"governance/routing-rules",
		"governance/pricing-overrides",
		"config",
		"routing/",
		"version",
		"pricing/",
		"proxy-config",
	}

	// busyStoreRetryTimeout bounds how long a change waits for the gateway's store.
	busyStoreRetryTimeout = 30 * time.Second

	// gatewayRegion matches a cloud region, such as us-central1 or eu-west-2.
	gatewayRegion = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)
)

// LLMGatewayHandler serves Obot's management API for the embedded Bifrost gateway. Most
// of it is a guarded pass-through to Bifrost's own management API; Obot adds its own
// budgets (enforced by Obot, not Bifrost), model sync, and import of legacy model provider
// credentials.
type LLMGatewayHandler struct {
	engine    *llmengine.Engine
	revisions LLMGatewayRevisions
	mapHelper *modelaccesspolicy.Helper
}

// LLMGatewayRevisions reads and advances the gateway configuration revision that tells the
// replicas to reload their gateways. The controller owns the revision, and the handler gets
// these from it rather than importing the controller.
type LLMGatewayRevisions struct {
	Current func(context.Context, kclient.Client) (int64, error)
	Advance func(context.Context, kclient.Client) (int64, error)
}

// LLMGatewayStatus describes the embedded gateway.
type LLMGatewayStatus struct {
	Enabled           bool   `json:"enabled"`
	Ready             bool   `json:"ready"`
	Error             string `json:"error,omitempty"`
	HasProviders      bool   `json:"hasProviders"`
	ModelProviderName string `json:"modelProviderName"`
	ProxyPath         string `json:"proxyPath"`
}

func NewLLMGatewayHandler(engine *llmengine.Engine, revisions LLMGatewayRevisions, mapHelper *modelaccesspolicy.Helper) *LLMGatewayHandler {
	return &LLMGatewayHandler{
		engine:    engine,
		revisions: revisions,
		mapHelper: mapHelper,
	}
}

func (h *LLMGatewayHandler) Status(req api.Context) error {
	status := LLMGatewayStatus{
		Enabled:           true,
		ModelProviderName: system.LLMGatewayModelProvider,
		ProxyPath:         "/api/llm-proxy",
		Ready:             h.engine.Ready(),
		HasProviders:      h.engine.HasUpstreamProviders(req.Context()),
	}
	if err := h.engine.Err(); err != nil {
		status.Error = llmengine.ScrubEngineName(err.Error())
	}
	return req.Write(status)
}

// isProviderWrite reports whether a request creates (POST providers) or updates (PUT
// providers/<name>) a provider.
func isProviderWrite(method, path string) bool {
	if method == http.MethodPost {
		return path == "providers"
	}
	name, ok := strings.CutPrefix(path, "providers/")
	return method == http.MethodPut && ok && name != "" && !strings.Contains(name, "/")
}

// withProviderDefaults fills in the network settings a new provider leaves unset, such as
// retries, which also let requests fail over between its keys. For a new or updated provider,
// it drops an API version from the end of the base URL when the engine adds it to every path.
func withProviderDefaults(body []byte, created bool, providerType string) ([]byte, error) {
	var provider map[string]any
	if err := json.Unmarshal(body, &provider); err != nil {
		return nil, types.NewErrBadRequest("invalid provider: %v", err)
	}
	trimmed := llmengine.TrimProviderBaseURLVersion(provider, providerType)
	if !created && !trimmed {
		return body, nil
	}
	if created {
		llmengine.ApplyProviderDefaults(provider)
	}
	return json.Marshal(provider)
}

func (h *LLMGatewayHandler) requireEngine() error {
	if !h.engine.Ready() {
		if err := h.engine.Err(); err != nil {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "the LLM gateway failed to start: "+err.Error())
		}
		return types.NewErrHTTP(http.StatusServiceUnavailable, "the LLM gateway is starting")
	}
	return nil
}

// Proxy forwards /api/llm-gateway/admin/{path...} to Bifrost's /api/{path...}.
func (h *LLMGatewayHandler) Proxy(req api.Context) error {
	if err := h.requireEngine(); err != nil {
		return err
	}

	path, ok := allowedBifrostAPIPath(req.PathValue("path"), req.URL.EscapedPath())
	if !ok {
		return types.NewErrNotFound("LLM gateway API %q is not available", req.PathValue("path"))
	}

	body, err := req.Body(api.BodyOptions{MaxBytes: 32 * 1024 * 1024})
	if err != nil {
		return err
	}
	if len(body) > 0 {
		if err := rejectUnsafeConfig(body); err != nil {
			return err
		}
		if err := rejectEngineAuthConfig(path, body); err != nil {
			return err
		}
		if err := rejectGlobalProxySkipVerify(path, body); err != nil {
			return err
		}
		if path == "config" && req.Method != http.MethodGet {
			if body, err = clientConfigOnly(body); err != nil {
				return err
			}
		}
		providerType := h.providerType(req.Context(), path, body)
		if err := rejectAmbientCredentialRedirects(body, providerType); err != nil {
			return err
		}
		if isAnthropicProvider(path, body) {
			settings, err := req.GatewayClient.GetLLMGatewaySettings(req.Context())
			if err != nil {
				return err
			}
			if settings.SubscriptionPassthrough {
				if err := rejectSubscriptionRedirects(body); err != nil {
					return err
				}
			}
		}
		if isProviderWrite(req.Method, path) {
			if body, err = withProviderDefaults(body, req.Method == http.MethodPost, providerType); err != nil {
				return err
			}
		}
	}

	// removeKeys confirms that changing a provider's base URL removes its keys (see
	// llmengine.GuardProviderChange). It is Obot's, so it isn't passed on.
	query := req.URL.Query()
	removeKeys := query.Get("removeKeys") == "true"
	query.Del("removeKeys")

	ctx := req.Context()
	if req.Method != http.MethodGet {
		// Once a change reaches the gateway it must finish and be announced to the other
		// replicas even if the client goes away, so the client can't cancel it midway.
		ctx = context.WithoutCancel(ctx)
	}
	serve := func(method, path string, body []byte) (*httptest.ResponseRecorder, error) {
		out, err := http.NewRequestWithContext(ctx, method, (&url.URL{Path: "/api/" + path, RawQuery: query.Encode()}).String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if ct := req.Request.Header.Get("Content-Type"); ct != "" && body != nil {
			out.Header.Set("Content-Type", ct)
		}
		out.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.engine.ServeHTTP(rec, out)
		return rec, nil
	}

	var (
		rec      *httptest.ResponseRecorder
		guardErr error
	)
	if req.Method == http.MethodGet {
		if rec, err = serve(req.Method, path, body); err != nil {
			return err
		}
	} else if err := h.change(ctx, req, func() (bool, bool) {
		// Checked here, under the lock every replica shares, so the check sees the
		// configuration the change is made to.
		change, err := h.engine.GuardProviderChange(ctx, req.Method, path, body, removeKeys)
		if err != nil {
			guardErr = err
			return false, false
		}
		for i, id := range change.RemoveKeys {
			keyPath := strings.TrimSuffix(path, "/") + "/keys/" + url.PathEscape(id)
			if rec, err = serveRetryingBusyStore(ctx, func() (*httptest.ResponseRecorder, error) {
				return serve(http.MethodDelete, keyPath, nil)
			}); err != nil {
				guardErr = err
				return i > 0, false
			} else if rec.Code >= 300 {
				// Keys removed so far are gone; reload to be safe.
				return i > 0 || rec.Code >= 500, false
			}
		}
		rec, err = serveRetryingBusyStore(ctx, func() (*httptest.ResponseRecorder, error) {
			return serve(req.Method, path, change.Body)
		})
		if err != nil {
			guardErr = err
			return len(change.RemoveKeys) > 0, false
		}
		switch {
		case rec.Code < 300:
			h.afterMutation(ctx, req, path)
			// Some settings, such as the request size limit, only take effect when the
			// engine starts, so this replica reloads a config change too.
			return true, path != "config" && len(change.RemoveKeys) == 0
		case rec.Code < 500 && len(change.RemoveKeys) == 0:
			// Rejected; nothing changed.
			return false, false
		default:
			// The change may have been saved before the failure, in the stores, in memory or
			// both, so every replica, this one included, reloads to be safe.
			return true, false
		}
	}); err != nil {
		return types.NewErrHTTP(http.StatusInternalServerError, llmengine.ScrubEngineName(err.Error()))
	} else if guardErr != nil {
		return guardErr
	}

	if req.Method == http.MethodGet && path == "config" && rec.Code < 300 {
		rec.Body = bytes.NewBuffer(hideEngineFrameworkConfig(rec.Body.Bytes()))
	}
	if rec.Code >= http.StatusBadRequest {
		return engineError(rec.Code, rec.Body.Bytes())
	}
	for k, vs := range rec.Header() {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			req.ResponseWriter.Header().Add(k, v)
		}
	}
	req.WriteHeader(rec.Code)
	_, err = req.ResponseWriter.Write(rec.Body.Bytes())
	return err
}

// hideEngineFrameworkConfig removes from the gateway's config the settings of the engine's
// own MCP server library, which Obot doesn't use and which name the engine vendor's site.
func hideEngineFrameworkConfig(body []byte) []byte {
	var config map[string]any
	if json.Unmarshal(body, &config) != nil {
		return body
	}
	framework, ok := config["framework_config"].(map[string]any)
	if !ok {
		return body
	}
	for key := range framework {
		if strings.HasPrefix(key, "mcp_library_") {
			delete(framework, key)
		}
	}
	if out, err := json.Marshal(config); err == nil {
		return out
	}
	return body
}

// serveRetryingBusyStore sends a change to the gateway, and sends it again while the gateway's
// store reports it is busy. With SQLite, a write fails at once while the gateway writes its
// model catalog, which it does for a few seconds after every start or reload. The gateway
// undoes a change whose store write failed, so sending it again is safe.
func serveRetryingBusyStore(ctx context.Context, serve func() (*httptest.ResponseRecorder, error)) (*httptest.ResponseRecorder, error) {
	deadline := time.Now().Add(busyStoreRetryTimeout)
	for delay := 100 * time.Millisecond; ; delay = min(delay*2, 2*time.Second) {
		rec, err := serve()
		if err != nil || !llmengine.IsStoreBusy(rec.Code, rec.Body.Bytes()) || time.Now().Add(delay).After(deadline) {
			return rec, err
		}
		select {
		case <-ctx.Done():
			return rec, nil
		case <-time.After(delay):
		}
	}
}

// engineError turns an error response from the engine's API into an Obot API error carrying
// only its message, so the engine's own error format and name don't reach clients.
func engineError(code int, body []byte) error {
	var errBody struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &errBody) == nil {
		message = cmp.Or(errBody.Error.Message, errBody.Message)
	}
	if message == "" {
		message = http.StatusText(code)
	}
	return types.NewErrHTTP(code, llmengine.ScrubEngineName(message))
}

// allowedBifrostAPIPath validates the requested Bifrost API path against the allowlist. The
// path is matched after cleaning, and any dot segment or encoded separator is refused, so a
// path like "providers/%2e%2e/session" can't reach an API that isn't listed.
func allowedBifrostAPIPath(raw, escaped string) (string, bool) {
	raw = strings.TrimPrefix(raw, "/")
	lowerEscaped := strings.ToLower(escaped)
	if strings.Contains(lowerEscaped, "%2f") || strings.Contains(lowerEscaped, "%2e") || strings.Contains(lowerEscaped, "%5c") {
		return "", false
	}
	for segment := range strings.SplitSeq(raw, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	if strings.Contains(raw, "\\") {
		return "", false
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if cleaned != strings.TrimSuffix(raw, "/") {
		return "", false
	}
	for _, prefix := range allowedBifrostAPIPrefixes {
		prefix = strings.TrimSuffix(prefix, "/")
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return cleaned, true
		}
	}
	return "", false
}

// clientConfigOnly keeps only client_config from a config update. The rest of the engine's
// config is Obot's: the framework settings point the engine at model data files Obot
// maintains, which the engine refuses to accept over its API, so passing back the config the
// client read would fail. The engine also refuses a client config whose log retention is
// under a day; Obot keeps the engine's request log off, so the retention is raised to that
// minimum rather than refusing the change.
func clientConfigOnly(body []byte) ([]byte, error) {
	var update struct {
		ClientConfig map[string]any `json:"client_config"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		return nil, types.NewErrBadRequest("invalid config: %v", err)
	}
	if update.ClientConfig == nil {
		return nil, types.NewErrBadRequest("invalid config: client_config is required")
	}
	if days, ok := update.ClientConfig["log_retention_days"].(float64); !ok || days < 1 {
		update.ClientConfig["log_retention_days"] = 1
	}
	return json.Marshal(update)
}

// rejectEngineAuthConfig refuses a change to the engine's own login. Obot reaches the engine
// with its setup token, which the engine accepts for creating its first admin account, so
// without this an Obot admin could turn on the engine's login through this API and lock Obot
// out of its own gateway on every replica. Obot controls access to the gateway itself. Keys
// are matched without regard to case, as the engine's JSON decoding matches them.
func rejectEngineAuthConfig(path string, body []byte) error {
	if path != "config" {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil
	}
	for key, value := range fields {
		if strings.EqualFold(key, "auth_config") && strings.TrimSpace(string(value)) != "null" {
			return types.NewErrBadRequest("the LLM gateway's own login can't be configured: Obot controls access to the gateway")
		}
	}
	return nil
}

// rejectUnsafeConfig refuses request bodies that would let an administrator obtain the Obot
// server's own secrets through a provider in these ways (rejectAmbientCredentialRedirects
// covers the server's cloud identity in others):
//   - A value Bifrost resolves from the server's environment or a vault ("env.NAME",
//     "vault.path", or the object forms of those). Bifrost would substitute it and send it
//     to the configured provider URL.
//   - A region that isn't a plain region name. Some providers build their host from the
//     region (Vertex: "<region>-aiplatform.googleapis.com") and, without credentials on the
//     key, authenticate with the server's own cloud identity, so a region such as
//     "attacker.example/x?" would send that identity's token elsewhere.
//   - Vertex credentials other than a service account key. The engine also accepts other
//     Google credential types, some of which make the server read a file or URL of its
//     choosing, run a command, or use its own AWS identity to get a token.
//
// Every string and object in the body is decoded the way Bifrost decodes a secret, from the
// original bytes, and field names are matched in any case as Bifrost matches them, so a
// spelling Bifrost accepts (such as "TYPE" for "type") can't slip past.
func rejectUnsafeConfig(body []byte) error {
	if !json.Valid(body) {
		// Not JSON; Bifrost will reject it on its own.
		return nil
	}
	var walk func(raw json.RawMessage) error
	walk = func(raw json.RawMessage) error {
		if llmengine.IsSecretReference(raw) {
			return types.NewErrBadRequest("environment and vault references are not supported; enter the value itself")
		}
		switch trimmed := bytes.TrimSpace(raw); {
		case len(trimmed) > 0 && trimmed[0] == '{':
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(trimmed, &fields); err != nil {
				return nil
			}
			for name, child := range fields {
				// Also refuse anything that asks for a reference at all, even in a form
				// Bifrost would read as a plain value.
				var v any
				_ = json.Unmarshal(child, &v)
				if strings.EqualFold(name, "from_env") && v == true {
					return types.NewErrBadRequest("environment references are not supported; enter the value itself")
				}
				if t, _ := v.(string); strings.EqualFold(name, "type") && (strings.EqualFold(t, "env") || strings.EqualFold(t, "vault")) {
					return types.NewErrBadRequest("environment and vault references are not supported; enter the value itself")
				}
				if strings.EqualFold(name, "vertex_key_config") {
					if err := checkVertexCredentials(child); err != nil {
						return err
					}
				}
				if strings.EqualFold(name, "region") {
					if region := llmengine.SecretValue(child); region != "" && !gatewayRegion.MatchString(region) {
						return types.NewErrBadRequest("region %q is not a region name, such as us-central1", region)
					}
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		case len(trimmed) > 0 && trimmed[0] == '[':
			var items []json.RawMessage
			if err := json.Unmarshal(trimmed, &items); err != nil {
				return nil
			}
			for _, child := range items {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(body)
}

// checkVertexCredentials refuses a Vertex key config whose auth_credentials isn't a service
// account key. Empty (or null) credentials are allowed: the key then uses the server's own identity,
// which rejectUnsafeConfig and rejectAmbientCredentialRedirects guard. So is a redacted value
// as the engine lists it, which keeps the stored credentials.
func checkVertexCredentials(vertexConfig json.RawMessage) error {
	var cfg map[string]json.RawMessage
	if json.Unmarshal(vertexConfig, &cfg) != nil {
		return nil
	}
	for name, raw := range cfg {
		if !strings.EqualFold(name, "auth_credentials") {
			continue
		}
		creds := llmengine.SecretValue(raw)
		if string(bytes.TrimSpace(raw)) == "null" || strings.TrimSpace(creds) == "" || llmengine.IsMaskedSecret(raw) {
			continue
		}
		if !isServiceAccountKey(creds) {
			return types.NewErrBadRequest("Vertex credentials must be a service account key")
		}
	}
	return nil
}

// isServiceAccountKey reports whether a Google credentials JSON is a service account key. Every
// "type" field, in any case and including duplicates, must say so, so no JSON decoder can read
// another type from it, however it matches names or picks among duplicate fields.
func isServiceAccountKey(creds string) bool {
	if !json.Valid([]byte(creds)) {
		return false
	}
	dec := json.NewDecoder(strings.NewReader(creds))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false
	}
	found := false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		name, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return false
		}
		if !strings.EqualFold(name, "type") {
			continue
		}
		var t string
		if json.Unmarshal(value, &t) != nil || t != "service_account" {
			return false
		}
		found = true
	}
	return found
}

// afterMutation keeps Obot in step with changes made through the proxy.
func (h *LLMGatewayHandler) afterMutation(ctx context.Context, req api.Context, path string) {
	switch {
	case strings.HasPrefix(path, "providers"), strings.HasPrefix(path, "models"), strings.HasPrefix(path, "keys"):
		if strings.HasPrefix(path, "providers") && req.Method != http.MethodDelete {
			h.engine.ApplyEgressProxy(ctx)
		}
		if req.Method == http.MethodDelete && strings.TrimSuffix(path, "/") == "providers/anthropic" {
			// Claude subscriptions are a setting of the Anthropic provider: removing the provider
			// turns them off, so adding it back later doesn't quietly forward them again.
			disableSubscriptionPassthrough(ctx, req)
		}
		if err := llmengine.TriggerModelSync(ctx, req.Storage); err != nil {
			slog.Warn("failed to trigger LLM gateway model sync", "error", err)
		}
	}
}

func disableSubscriptionPassthrough(ctx context.Context, req api.Context) {
	settings, err := req.GatewayClient.GetLLMGatewaySettings(ctx)
	if err != nil || !settings.SubscriptionPassthrough {
		if err != nil {
			slog.Warn("failed to read LLM gateway settings", "error", err)
		}
		return
	}
	settings.SubscriptionPassthrough = false
	if err := req.GatewayClient.SetLLMGatewaySettings(ctx, settings); err != nil {
		slog.Warn("failed to turn off Claude subscriptions after removing the Anthropic provider", "error", err)
	}
}

// change makes a change to the gateway's configuration and announces it to the other
// replicas. It holds a lock every replica shares, and first reloads what other replicas
// announced, so the change is never made to a stale copy of the configuration and doesn't
// overwrite theirs.
func (h *LLMGatewayHandler) change(ctx context.Context, req api.Context, fn func() (changed, applied bool)) error {
	return req.GatewayClient.WithLLMGatewayLock(ctx, func() error {
		revision, err := h.revisions.Current(ctx, req.Storage)
		if err != nil {
			return fmt.Errorf("failed to read the LLM gateway configuration revision: %w", err)
		}
		if _, err := h.engine.Reload(revision); err != nil {
			return fmt.Errorf("failed to reload the LLM gateway configuration: %w", err)
		}
		return h.engine.Change(ctx, fn, h.announceChange(req.Storage))
	})
}

// announceChange advances the gateway's configuration revision, so the other replicas reload
// their gateways from the shared stores.
func (h *LLMGatewayHandler) announceChange(storage kclient.Client) func(context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		return h.revisions.Advance(ctx, storage)
	}
}

// Metrics serves the gateway's Prometheus metrics: request counts, latencies, tokens and cost
// by provider, model and key.
func (h *LLMGatewayHandler) Metrics(req api.Context) error {
	if err := h.requireEngine(); err != nil {
		return err
	}
	out, err := http.NewRequestWithContext(req.Context(), http.MethodGet, "/metrics", nil)
	if err != nil {
		return err
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, out)
	// Publish the metrics under Obot's own name.
	metrics := strings.ReplaceAll(rec.Body.String(), "bifrost_", "obot_llm_gateway_")
	req.ResponseWriter.Header().Set("Content-Type", rec.Header().Get("Content-Type"))
	req.WriteHeader(rec.Code)
	_, err = req.ResponseWriter.Write([]byte(metrics))
	return err
}

func (h *LLMGatewayHandler) SyncModels(req api.Context) error {
	if err := h.requireEngine(); err != nil {
		return err
	}
	if err := llmengine.TriggerModelSync(req.Context(), req.Storage); err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// GetSettings returns the LLM gateway settings Obot enforces.
func (h *LLMGatewayHandler) GetSettings(req api.Context) error {
	settings, err := req.GatewayClient.GetLLMGatewaySettings(req.Context())
	if err != nil {
		return err
	}
	return req.Write(settings)
}

// UpdateSettings replaces the LLM gateway settings Obot enforces.
func (h *LLMGatewayHandler) UpdateSettings(req api.Context) error {
	var settings gatewaytypes.LLMGatewaySettings
	if err := req.Read(&settings); err != nil {
		return types.NewErrBadRequest("invalid settings: %v", err)
	}
	previous, err := req.GatewayClient.GetLLMGatewaySettings(req.Context())
	if err != nil {
		return err
	}
	if err := req.GatewayClient.SetLLMGatewaySettings(req.Context(), settings); err != nil {
		return err
	}
	if previous.SubscriptionPassthrough != settings.SubscriptionPassthrough {
		// An Anthropic provider without keys lists its models only while subscriptions are
		// allowed.
		if err := llmengine.TriggerModelSync(req.Context(), req.Storage); err != nil {
			slog.Warn("failed to trigger LLM gateway model sync", "error", err)
		}
	}
	return req.Write(settings)
}

func (h *LLMGatewayHandler) ListBudgets(req api.Context) error {
	budgets, err := req.GatewayClient.ListLLMGatewayBudgets(req.Context(), req.URL.Query().Get("profileID"))
	if err != nil {
		return err
	}
	if budgets == nil {
		budgets = []gatewaytypes.LLMGatewayBudget{}
	}
	return req.Write(map[string]any{"items": budgets})
}

func (h *LLMGatewayHandler) GetBudget(req api.Context) error {
	budget, err := h.getBudget(req)
	if err != nil {
		return err
	}
	return req.Write(budget)
}

func (h *LLMGatewayHandler) getBudget(req api.Context) (*gatewaytypes.LLMGatewayBudget, error) {
	budget, err := req.GatewayClient.GetLLMGatewayBudget(req.Context(), req.PathValue("budget_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("budget %q not found", req.PathValue("budget_id"))
	}
	return budget, err
}

func (h *LLMGatewayHandler) CreateBudget(req api.Context) error {
	var budget gatewaytypes.LLMGatewayBudget
	if err := req.Read(&budget); err != nil {
		return types.NewErrBadRequest("invalid budget: %v", err)
	}
	if err := h.requireBudgetProfile(req, &budget); err != nil {
		return err
	}
	if err := req.GatewayClient.CreateLLMGatewayBudget(req.Context(), &budget); err != nil {
		return types.NewErrBadRequest("%v", err)
	}
	if err := confirmProfile(req, budget.ProfileID, func() error {
		return req.GatewayClient.DeleteLLMGatewayBudget(req.Context(), budget.ID)
	}); err != nil {
		return err
	}
	return req.WriteCreated(budget)
}

// requireBudgetProfile checks that a budget's profile exists and grants every model the budget
// names: a budget covers only its own profile's models.
func (h *LLMGatewayHandler) requireBudgetProfile(req api.Context, budget *gatewaytypes.LLMGatewayBudget) error {
	profile, err := getProfile(req, budget.ProfileID)
	if err != nil || len(budget.Models) == 0 {
		return err
	}
	granted := h.mapHelper.PolicyAllowedModels(profile)
	var missing []string
	for _, model := range budget.Models {
		if !granted[model] {
			missing = append(missing, model)
		}
	}
	if len(missing) > 0 {
		return types.NewErrBadRequest("a budget can only limit its profile's models, and profile %q doesn't grant %s", cmp.Or(profile.Spec.Manifest.DisplayName, profile.Name), strings.Join(missing, ", "))
	}
	return nil
}

func (h *LLMGatewayHandler) UpdateBudget(req api.Context) error {
	existing, err := h.getBudget(req)
	if err != nil {
		return err
	}
	var budget gatewaytypes.LLMGatewayBudget
	if err := req.Read(&budget); err != nil {
		return types.NewErrBadRequest("invalid budget: %v", err)
	}
	budget.ID = existing.ID
	if err := h.requireBudgetProfile(req, &budget); err != nil {
		return err
	}
	if err := req.GatewayClient.UpdateLLMGatewayBudget(req.Context(), &budget); err != nil {
		return types.NewErrBadRequest("%v", err)
	}
	if err := confirmProfile(req, budget.ProfileID, func() error {
		return req.GatewayClient.DeleteLLMGatewayBudget(req.Context(), budget.ID)
	}); err != nil {
		return err
	}
	updated, err := h.getBudget(req)
	if err != nil {
		return err
	}
	return req.Write(updated)
}

func (h *LLMGatewayHandler) DeleteBudget(req api.Context) error {
	if _, err := h.getBudget(req); err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteLLMGatewayBudget(req.Context(), req.PathValue("budget_id")); err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// BudgetUsage reports usage in each limit's current window: per user for per-user budgets,
// or one total for shared budgets.
func (h *LLMGatewayHandler) BudgetUsage(req api.Context) error {
	budget, err := h.getBudget(req)
	if err != nil {
		return err
	}
	entries, err := req.GatewayClient.LLMGatewayBudgetUsageReport(req.Context(), budget)
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"items": entries})
}
