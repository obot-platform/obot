package handlers

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

var (
	// gatewayRegion matches a cloud region, such as us-central1 or eu-west-2.
	gatewayRegion = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)
)

// LLMGatewayHandler owns the public management HTTP boundary. Engine operations
// validate and persist desired configuration directly; Obot also owns budgets and model sync.
type LLMGatewayHandler struct {
	engine    *llmengine.Engine
	mapHelper *modelaccesspolicy.Helper
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

func NewLLMGatewayHandler(engine *llmengine.Engine, mapHelper *modelaccesspolicy.Helper) *LLMGatewayHandler {
	return &LLMGatewayHandler{
		engine:    engine,
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

// Metrics serves inference metrics for this gateway replica.
func (h *LLMGatewayHandler) Metrics(req api.Context) error {
	if err := h.requireEngine(); err != nil {
		return err
	}
	h.engine.ServeMetrics(req.ResponseWriter, req.Request)
	return nil
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

// adminHandler applies readiness and path-encoding checks to registered admin routes.
func (h *LLMGatewayHandler) adminHandler(next api.HandlerFunc) api.HandlerFunc {
	return func(req api.Context) error {
		if err := h.requireEngine(); err != nil {
			return err
		}
		// Reject encoded separators and dot segments before named identifiers reach storage.
		escaped := strings.ToLower(req.URL.EscapedPath())
		if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%2e") || strings.Contains(escaped, "%5c") || strings.Contains(req.URL.Path, "\\") {
			return types.NewErrNotFound("LLM gateway API %q is not available", req.URL.Path)
		}
		return next(req)
	}
}

func readGatewayAdminBody(req api.Context) ([]byte, error) {
	body, err := req.Body(api.BodyOptions{MaxBytes: 32 * 1024 * 1024})
	if err != nil {
		return nil, err
	}
	if err := rejectUnsafeConfig(body); err != nil {
		return nil, err
	}
	return body, nil
}

// Provider and key writes share credential restrictions; route handlers select defaults.
func (h *LLMGatewayHandler) validateProviderBody(req api.Context, body []byte) (string, error) {
	name := req.PathValue("provider")
	providerType := h.providerType(req.Context(), name, body)
	if err := rejectAmbientCredentialRedirects(body, providerType); err != nil {
		return "", err
	}
	if isAnthropicProvider(name, body) {
		settings, err := req.GatewayClient.GetLLMGatewaySettings(req.Context())
		if err != nil {
			return "", err
		}
		if settings.SubscriptionPassthrough {
			if err := rejectSubscriptionRedirects(body); err != nil {
				return "", err
			}
		}
	}
	return providerType, nil
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

// Obot controls gateway authentication; client settings cannot enable a separate login.
// Match every spelling that the engine's JSON decoder accepts.
func rejectEngineAuthConfig(body []byte) error {
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

// Engine mutations finish accepted writes after disconnects; their follow-up must also finish.
func (h *LLMGatewayHandler) providerChanged(req api.Context) {
	h.engine.ApplyEgressProxy(context.WithoutCancel(req.Context()))
	triggerGatewayModelSync(req)
}

func triggerGatewayModelSync(req api.Context) {
	if err := llmengine.TriggerModelSync(context.WithoutCancel(req.Context()), req.Storage); err != nil {
		slog.Warn("failed to trigger LLM gateway model sync", "error", err)
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
