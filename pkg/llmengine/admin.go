package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/uuid"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type configPricingOverride = tables.TablePricingOverride
type configContextKey struct{}

func (e *Engine) config(ctx context.Context) (llmcoreclient.Config, error) {
	if cfg, ok := ctx.Value(configContextKey{}).(*llmcoreclient.Config); ok {
		return *cfg, nil
	}
	return e.store.Load(ctx)
}
func adminJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func adminError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		code = apiErr.StatusCode
	}
	adminJSON(w, code, map[string]any{"error": map[string]any{"message": ScrubEngineName(err.Error())}})
}
func missing(what string) error { return &APIError{StatusCode: 404, Message: what + " not found"} }

func (e *Engine) serveAdmin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		adminError(w, err)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == "pricing/force-sync" && r.Method == http.MethodPost {
		if err := e.prices.ForceReloadPricing(r.Context()); err != nil {
			adminError(w, &APIError{StatusCode: http.StatusServiceUnavailable, Message: err.Error()})
			return
		}
		adminJSON(w, http.StatusOK, map[string]any{"status": "success", "message": "pricing synced successfully"})
		return
	}
	parts := strings.Split(path, "/")
	if r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "providers" && parts[2] == "refresh-models" {
		if _, err := e.Sync(r.Context()); err != nil {
			adminError(w, &APIError{StatusCode: 503, Message: err.Error()})
			return
		}
		cfg, err := e.config(r.Context())
		if err != nil {
			adminError(w, &APIError{StatusCode: 503, Message: err.Error()})
			return
		}
		provider := schemas.ModelProvider(parts[1])
		if _, ok := cfg.Providers[provider]; !ok {
			adminError(w, missing("provider"))
			return
		}
		if _, err := e.listGatewayModels(r.Context(), provider, true); err != nil {
			adminError(w, &APIError{StatusCode: 502, Message: err.Error()})
			return
		}
		adminJSON(w, http.StatusOK, map[string]any{"status": "refreshed"})
		return
	}
	if r.Method == http.MethodGet {
		cfg, err := e.config(r.Context())
		if err != nil {
			adminError(w, &APIError{StatusCode: http.StatusServiceUnavailable, Message: err.Error()})
			return
		}
		out, err := e.adminRead(r.Context(), cfg, path, r)
		if err != nil {
			adminError(w, err)
			return
		}
		adminJSON(w, 200, out)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var out any
	var rejected error
	// Once accepted, finish the durable write even if the caller disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	err = e.mutateAdmin(ctx, path, r.Method, func(cfg *llmcoreclient.Config) error {
		rejected = nil
		// Guard reads and masked-secret merges use the latest owning resource on every retry.
		ctx := context.WithValue(ctx, configContextKey{}, cfg)
		change, err := e.GuardProviderChange(ctx, r.Method, path, body, r.URL.Query().Get("removeKeys") == "true")
		if err != nil {
			rejected = err
			return err
		}
		if len(change.RemoveKeys) > 0 {
			cfg.Keys[schemas.ModelProvider(parts[1])] = nil
		}
		out, err = e.adminWrite(ctx, cfg, path, r.Method, change.Body)
		rejected = err
		return err
	})
	if err != nil {
		if rejected == nil {
			status := http.StatusServiceUnavailable
			if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
				status = http.StatusConflict
			}
			if apierrors.IsNotFound(err) {
				status = http.StatusNotFound
			}
			err = &APIError{StatusCode: status, Message: err.Error()}
		}
		adminError(w, err)
		return
	}
	adminJSON(w, 200, out)
}

// mutateAdmin dispatches a write to its owning resource. The decoded view is local
// to that resource; provider guards can use it without reading unrelated secrets.
func (e *Engine) mutateAdmin(ctx context.Context, path, method string, change func(*llmcoreclient.Config) error) error {
	parts := strings.Split(path, "/")
	cfg := llmcoreclient.Config{
		Providers: map[schemas.ModelProvider]schemas.ProviderConfig{},
		Keys:      map[schemas.ModelProvider][]schemas.Key{},
		Rules:     map[string]tables.TableRoutingRule{},
		Settings:  map[string]json.RawMessage{},
	}
	switch parts[0] {
	case "providers":
		if len(parts) == 1 {
			if err := change(&cfg); err != nil {
				return err
			}
			for name, pc := range cfg.Providers {
				return e.store.CreateProvider(ctx, string(name), llmcoreclient.Provider{Config: pc, Keys: cfg.Keys[name]})
			}
			return fmt.Errorf("provider name required")
		}
		name := schemas.ModelProvider(parts[1])
		if len(parts) == 2 && method == http.MethodDelete {
			return e.store.DeleteProvider(ctx, string(name))
		}
		return e.store.MutateProvider(ctx, string(name), func(provider *llmcoreclient.Provider) error {
			cfg.Providers[name], cfg.Keys[name] = provider.Config, provider.Keys
			if err := change(&cfg); err != nil {
				return err
			}
			provider.Config, provider.Keys = cfg.Providers[name], cfg.Keys[name]
			return nil
		})
	case "config", "proxy-config":
		return e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			cfg.Settings = settings
			return change(&cfg)
		})
	case "governance":
		if len(parts) > 1 && parts[1] == "pricing-overrides" {
			return e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
				cfg.Settings = settings
				return change(&cfg)
			})
		}
		if len(parts) > 1 && parts[1] == "routing-rules" {
			if len(parts) == 3 {
				if method == http.MethodDelete {
					return e.store.DeleteRule(ctx, parts[2])
				}
				return e.store.MutateRule(ctx, parts[2], func(rule *tables.TableRoutingRule) error {
					cfg.Rules[rule.ID] = *rule
					if err := change(&cfg); err != nil {
						return err
					}
					*rule = cfg.Rules[rule.ID]
					return nil
				})
			}
			if err := change(&cfg); err != nil {
				return err
			}
			for _, rule := range cfg.Rules {
				return e.store.PutRule(ctx, rule)
			}
		}
	}
	return missing("gateway API")
}

func providerView(name schemas.ModelProvider, pc schemas.ProviderConfig, keys []schemas.Key) map[string]any {
	pc.CheckAndSetDefaults()
	raw, _ := json.Marshal(pc)
	var provider configstore.ProviderConfig
	_ = json.Unmarshal(raw, &provider)
	provider.Keys = keys
	out := object(provider.Redacted())
	out["name"] = name
	out["provider"] = name
	out["provider_status"] = "active"
	return out
}
func object(v any) map[string]any {
	raw, _ := json.Marshal(v)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
func (e *Engine) adminRead(_ context.Context, cfg llmcoreclient.Config, path string, r *http.Request) (any, error) {
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "keys":
		out := []any{}
		for provider, keys := range cfg.Keys {
			view := providerView(provider, cfg.Providers[provider], keys)
			out = append(out, view["keys"].([]any)...)
		}
		return map[string]any{"keys": out}, nil
	case "providers":
		if len(parts) == 1 {
			out := []any{}
			for _, name := range slices.Sorted(maps.Keys(cfg.Providers)) {
				out = append(out, providerView(name, cfg.Providers[name], cfg.Keys[name]))
			}
			return map[string]any{"providers": out}, nil
		}
		name := schemas.ModelProvider(parts[1])
		pc, ok := cfg.Providers[name]
		if !ok {
			return nil, missing("provider")
		}
		view := providerView(name, pc, cfg.Keys[name])
		if len(parts) == 2 {
			return view, nil
		}
		if parts[2] == "keys" {
			keys := view["keys"]
			if len(parts) == 3 {
				return map[string]any{"keys": keys}, nil
			}
			for _, key := range keys.([]any) {
				if object(key)["id"] == parts[3] {
					return key, nil
				}
			}
			return nil, missing("key")
		}
	case "config":
		client := map[string]any{"drop_excess_requests": false, "max_request_body_size_mb": 100, "prometheus_labels": []string{}}
		if raw := cfg.Settings["client"]; len(raw) > 0 {
			_ = json.Unmarshal(raw, &client)
		}
		pricing, params := modeldata.Paths(e.opts.DataDir)
		return map[string]any{"client_config": client, "framework_config": map[string]any{"pricing_url": modeldata.FileURL(pricing), "model_parameters_url": modeldata.FileURL(params)}}, nil

	case "proxy-config":
		proxy := map[string]any{"type": "none"}
		if raw := cfg.Settings["proxy"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &proxy); err != nil {
				return nil, err
			}
		}
		if password, ok := proxy["password"].(string); ok && password != "" {
			proxy["password"] = (&schemas.SecretVar{Val: password}).Redacted().GetValue()
		}
		return proxy, nil

	case "version":
		return map[string]any{"version": Version}, nil
	case "models":
		out := []any{}
		for provider := range cfg.Providers {
			if p := r.URL.Query().Get("provider"); p != "" && p != string(provider) {
				continue
			}
			for _, model := range e.prices.GetUnfilteredModelsForProvider(provider) {
				if e.modelIsUnavailable(string(provider) + "/" + model) {
					continue
				}
				if q := r.URL.Query().Get("query"); q != "" && !strings.Contains(strings.ToLower(model), strings.ToLower(q)) {
					continue
				}
				out = append(out, map[string]any{"name": model, "provider": provider})
			}
		}
		return map[string]any{"models": out}, nil
	case "governance":
		if len(parts) > 1 && parts[1] == "routing-rules" {
			if len(parts) == 3 {
				v, ok := cfg.Rules[parts[2]]
				if !ok {
					return nil, missing("rule")
				}
				return map[string]any{"rule": v}, nil
			}
			out := []tables.TableRoutingRule{}
			for _, v := range cfg.Rules {
				out = append(out, v)
			}
			slices.SortFunc(out, func(a, b tables.TableRoutingRule) int { return a.Priority - b.Priority })
			return map[string]any{"rules": out}, nil
		}
		if len(parts) > 1 && parts[1] == "pricing-overrides" {
			var rows []configPricingOverride
			_ = json.Unmarshal(cfg.Settings["pricing-overrides"], &rows)
			return map[string]any{"pricing_overrides": rows}, nil
		}
	}
	return nil, missing("gateway API")
}
func mergeJSON(current any, body []byte) ([]byte, error) {
	base := object(current)
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return nil, err
	}
	var merge func(map[string]any, map[string]any)
	merge = func(dst, src map[string]any) {
		for k, v := range src {
			if child, ok := v.(map[string]any); ok {
				if prior, ok := dst[k].(map[string]any); ok {
					merge(prior, child)
					continue
				}
			}
			dst[k] = v
		}
	}
	merge(base, patch)
	// Restore only masked values which exactly match the redaction of the current value.
	restoreMasked(base, object(current))
	return json.Marshal(base)
}
func restoreMasked(dst, prior map[string]any) {
	for k, v := range dst {
		if child, ok := v.(map[string]any); ok {
			if old, ok := prior[k].(map[string]any); ok {
				restoreMasked(child, old)
			}
		} else if text, ok := v.(string); ok && (strings.Contains(text, "*") || text == "<REDACTED>") {
			if old, ok := prior[k].(string); ok {
				secret := schemas.NewSecretVar(old)
				if text == secret.Redacted().GetValue() || text == "<REDACTED>" {
					dst[k] = old
				}
			}
		}
	}
}

func (e *Engine) adminWrite(_ context.Context, cfg *llmcoreclient.Config, path, method string, body []byte) (any, error) {
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "providers":
		if len(parts) == 1 {
			if method != http.MethodPost {
				return nil, fmt.Errorf("invalid provider operation")
			}
			var in struct {
				Provider string `json:"provider"`
			}
			if err := json.Unmarshal(body, &in); err != nil {
				return nil, err
			}
			if in.Provider == "" || strings.ContainsAny(in.Provider, "/\\ ") {
				return nil, fmt.Errorf("provider name required")
			}
			name := schemas.ModelProvider(in.Provider)
			if _, ok := cfg.Providers[name]; ok {
				return nil, fmt.Errorf("provider already exists")
			}
			var pc schemas.ProviderConfig
			defaults := object(json.RawMessage(body))
			ApplyProviderDefaults(defaults)
			raw, _ := json.Marshal(defaults)
			if err := json.Unmarshal(raw, &pc); err != nil {
				return nil, err
			}
			if err := validateProvider(name, pc); err != nil {
				return nil, err
			}
			pc.CheckAndSetDefaults()
			if err := validateProvider(name, pc); err != nil {
				return nil, err
			}
			cfg.Providers[name] = pc
			return providerView(name, pc, nil), nil
		}
		name := schemas.ModelProvider(parts[1])
		pc, ok := cfg.Providers[name]
		if !ok {
			return nil, missing("provider")
		}
		if len(parts) == 2 {
			if method == http.MethodDelete {
				delete(cfg.Providers, name)
				delete(cfg.Keys, name)
				return map[string]any{"status": "deleted"}, nil
			}
			// Nested provider settings use replacement semantics when explicitly supplied.
			current := object(pc)
			var patch map[string]any
			if err := json.Unmarshal(body, &patch); err != nil {
				return nil, err
			}
			for _, field := range []string{"network_config", "proxy_config", "concurrency_and_buffer_size", "custom_provider_config"} {
				if value, ok := patch[field]; ok {
					current[field] = value
				}
			}
			restoreMasked(current, object(pc))
			raw, err := mergeJSON(current, body)
			if err != nil {
				return nil, err
			}
			var next schemas.ProviderConfig
			if err := json.Unmarshal(raw, &next); err != nil {
				return nil, err
			}
			if err := validateProvider(name, next); err != nil {
				return nil, err
			}
			next.CheckAndSetDefaults()
			if err := validateProvider(name, next); err != nil {
				return nil, err
			}
			cfg.Providers[name] = next
			return providerView(name, next, cfg.Keys[name]), nil
		}
		if parts[2] == "keys" {
			keys := cfg.Keys[name]
			index := -1
			if len(parts) == 4 {
				index = slices.IndexFunc(keys, func(k schemas.Key) bool { return k.ID == parts[3] })
				if index < 0 {
					return nil, missing("key")
				}
			}
			if method == http.MethodDelete {
				if index < 0 {
					return nil, missing("key")
				}
				cfg.Keys[name] = slices.Delete(keys, index, index+1)
				return map[string]any{"status": "deleted"}, nil
			}
			key := schemas.Key{ID: uuid.NewString(), Weight: 1, Models: schemas.WhiteList{"*"}}
			if index >= 0 {
				key = keys[index]
			}
			raw, err := mergeJSON(key, body)
			if err != nil {
				return nil, err
			}
			if err = json.Unmarshal(raw, &key); err != nil {
				return nil, err
			}
			if index >= 0 {
				key.ID = keys[index].ID
			} else {
				key.ID = uuid.NewString()
			}
			if slices.ContainsFunc(keys, func(existing schemas.Key) bool { return existing.ID != key.ID && existing.Name == key.Name }) {
				return nil, fmt.Errorf("key name already exists")
			}
			if key.Name == "" || key.Weight < 0 {
				return nil, fmt.Errorf("key name and nonnegative weight required")
			}
			if err := key.Models.Validate(); err != nil {
				return nil, err
			}
			if err := key.BlacklistedModels.Validate(); err != nil {
				return nil, err
			}
			if err := key.Aliases.Validate(name); err != nil {
				return nil, err
			}
			if index >= 0 {
				keys[index] = key
			} else {
				keys = append(keys, key)
			}
			cfg.Keys[name] = keys
			redacted := providerView(name, pc, []schemas.Key{key})
			return redacted["keys"].([]any)[0], nil
		}
	case "config":
		var in struct {
			Client map[string]any `json:"client_config"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
		prior := map[string]any{}
		_ = json.Unmarshal(cfg.Settings["client"], &prior)
		maps.Copy(prior, in.Client)
		if v, ok := prior["max_request_body_size_mb"].(float64); ok && (v <= 0 || v > 1048576) {
			return nil, fmt.Errorf("invalid request body size")
		}
		raw, err := json.Marshal(prior)
		if err != nil {
			return nil, err
		}
		cfg.Settings["client"] = raw
		return map[string]any{"client_config": prior}, nil

	case "proxy-config":
		var previous map[string]any
		if raw := cfg.Settings["proxy"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &previous); err != nil {
				return nil, err
			}
		}
		raw, err := mergeJSON(previous, body)
		if err != nil {
			return nil, err
		}
		cfg.Settings["proxy"] = raw
		return map[string]any{"status": "success"}, nil

	case "governance":
		if len(parts) > 1 && parts[1] == "routing-rules" {
			var in tables.TableRoutingRule
			if len(parts) == 3 {
				var ok bool
				in, ok = cfg.Rules[parts[2]]
				if !ok {
					return nil, missing("rule")
				}
				if method == http.MethodDelete {
					delete(cfg.Rules, parts[2])
					return map[string]any{"status": "deleted"}, nil
				}
			}
			raw, err := mergeJSON(in, body)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			if len(parts) == 3 {
				in.ID = parts[2]
			} else {
				in.ID = uuid.NewString()
			}
			if in.Scope == "" {
				in.Scope = "global"
			}
			if in.Scope != "global" || in.ScopeID != nil {
				return nil, fmt.Errorf("only global routing rules are supported")
			}
			if err := rules.ValidateCELExpression(in.CelExpression); err != nil {
				return nil, err
			}
			if usesUnsupportedBudgetVariable(in.CelExpression) {
				return nil, fmt.Errorf("routing budget variables are not supported")
			}
			if len(in.Targets) == 0 {
				return nil, fmt.Errorf("routing targets required")
			}
			for i := range in.Targets {
				target := &in.Targets[i]
				target.RuleID = in.ID
				if target.Weight < 0 {
					return nil, fmt.Errorf("target weight must be nonnegative")
				}
				if target.TTFTTimeoutMs != nil && (*target.TTFTTimeoutMs < 0 || *target.TTFTTimeoutMs > 300000) {
					return nil, fmt.Errorf("invalid first-token timeout")
				}
			}
			cfg.Rules[in.ID] = in
			return map[string]any{"rule": in}, nil
		}
		if len(parts) > 1 && parts[1] == "pricing-overrides" {
			var rows []configPricingOverride
			_ = json.Unmarshal(cfg.Settings["pricing-overrides"], &rows)
			var in configPricingOverride
			index := -1
			if len(parts) == 3 {
				index = slices.IndexFunc(rows, func(v configPricingOverride) bool { return v.ID == parts[2] })
				if index < 0 {
					return nil, missing("pricing override")
				}
				in = rows[index]
			}
			if method == http.MethodDelete {
				if index < 0 {
					return nil, missing("pricing override")
				}
				rows = slices.Delete(rows, index, index+1)
			} else {
				raw, err := mergeJSON(in, body)
				if err != nil {
					return nil, err
				}
				if err = json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(body, &fields); err != nil {
					return nil, err
				}
				patch := fields["patch"]
				if len(patch) == 0 {
					patch = []byte(`{}`)
				}
				var options modelcatalog.PricingOptions
				if err := json.Unmarshal(patch, &options); err != nil {
					return nil, fmt.Errorf("invalid pricing patch: %w", err)
				}
				in.PricingPatchJSON = string(patch)
				shape := modelcatalog.PricingOverride{Name: in.Name, ScopeKind: modelcatalog.ScopeKind(in.ScopeKind), UserID: in.UserID, VirtualKeyID: in.VirtualKeyID, ProviderID: in.ProviderID, ProviderKeyID: in.ProviderKeyID, MatchType: modelcatalog.MatchType(in.MatchType), Pattern: in.Pattern, RequestTypes: in.RequestTypes, Options: options}
				if err := shape.IsValid(); err != nil {
					return nil, err
				}

				if index < 0 {
					in.ID = uuid.NewString()
					rows = append(rows, in)
				} else {
					in.ID = parts[2]
					rows[index] = in
				}
			}
			raw, err := json.Marshal(rows)
			if err != nil {
				return nil, err
			}
			cfg.Settings["pricing-overrides"] = raw
			return map[string]any{"pricing_override": in}, nil
		}
	}
	return nil, missing("gateway API")
}
func validateProvider(name schemas.ModelProvider, pc schemas.ProviderConfig) error {
	base := name
	if pc.CustomProviderConfig != nil {
		base = pc.CustomProviderConfig.BaseProviderType
	}
	if (pc.CustomProviderConfig == nil && !slices.Contains(schemas.StandardProviders, base)) || (pc.CustomProviderConfig != nil && !slices.Contains(schemas.SupportedBaseProviders, base)) {
		return fmt.Errorf("unsupported provider type %q", base)
	}
	if pc.ConcurrencyAndBufferSize.BufferSize > 0 && pc.ConcurrencyAndBufferSize.Concurrency > pc.ConcurrencyAndBufferSize.BufferSize {
		return &APIError{StatusCode: http.StatusBadRequest, Message: "Concurrency must be less than or equal to buffer size"}
	}
	if pc.ConcurrencyAndBufferSize.Concurrency < 0 || pc.ConcurrencyAndBufferSize.BufferSize < 0 {
		return fmt.Errorf("invalid concurrency configuration")
	}
	if pc.NetworkConfig.MaxRetries < 0 || pc.NetworkConfig.DefaultRequestTimeoutInSeconds < 0 {
		return fmt.Errorf("invalid network configuration")
	}
	return nil
}

func usesUnsupportedBudgetVariable(expression string) bool {
	env, err := cel.NewEnv()
	if err != nil {
		return true
	}
	parsed, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return true
	}
	found := celast.MatchDescendants(celast.NavigateAST(parsed.NativeRep()), func(expr celast.NavigableExpr) bool {
		if expr.Kind() != celast.IdentKind {
			return false
		}
		return slices.Contains([]string{"budget_used", "tokens_used", "request"}, expr.AsIdent())
	})
	return len(found) > 0
}
