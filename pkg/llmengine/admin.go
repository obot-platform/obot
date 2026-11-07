package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
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

// ProviderResponse adds Obot's admin identifiers to Bifrost's redacted configuration.
type Key struct {
	schemas.Key
	llmcoreclient.AzureAccount
}

type ProviderResponse struct {
	Keys []Key `json:"keys"`
	*configstore.ProviderConfig
	Name           schemas.ModelProvider `json:"name"`
	Provider       schemas.ModelProvider `json:"provider"`
	ProviderStatus string                `json:"provider_status"`
}

func (e *Engine) config(ctx context.Context) (llmcoreclient.Config, error) {
	if cfg, ok := ctx.Value(configContextKey{}).(*llmcoreclient.Config); ok {
		return *cfg, nil
	}
	if !e.Ready() {
		return llmcoreclient.Config{}, apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	// Keep healthy aggregates readable; provider diagnostics are exposed by engine status.
	loaded, err := e.store.Load(ctx)
	if err != nil {
		return loaded.Config, apierrors.NewServiceUnavailable(err.Error())
	}
	return loaded.Config, nil
}

func requireAdminIdentifier(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return missing("gateway API")
	}
	return nil
}

func missing(what string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{Code: http.StatusNotFound, Message: what + " not found"}}
}

// Administration returns desired configuration directly. HTTP routing and envelopes
// belong to Obot's API; only inference uses Engine.ServeHTTP and Transport.
// Mutations acknowledge persistence, leaving runtime application to Sync.
func (e *Engine) mutation(ctx context.Context, persist func(context.Context) error) error {
	if !e.Ready() {
		return apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	// Once accepted, finish the durable write even if the caller disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	return persist(ctx)
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*apierrors.StatusError](err); ok {
		return err
	}
	status := http.StatusServiceUnavailable
	if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
		status = http.StatusConflict
	}
	if apierrors.IsNotFound(err) {
		status = http.StatusNotFound
	}
	return &apierrors.StatusError{ErrStatus: metav1.Status{Code: int32(status), Message: err.Error()}}
}

// changeProvider runs guards against the owning resource on every mutation attempt.
// The scoped snapshot also keeps masked secret merges and removeKeys atomic.
func changeProvider[T any](ctx context.Context, e *Engine, name string, method string, body []byte, removeKeys bool, apply func(context.Context, *llmcoreclient.Config, []byte) (T, error)) (T, error) {
	var out T
	var rejected error
	err := e.mutation(ctx, func(ctx context.Context) error {
		cfg := llmcoreclient.Config{AzureAccounts: map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{}, Providers: map[schemas.ModelProvider]schemas.ProviderConfig{}, Keys: map[schemas.ModelProvider][]schemas.Key{}}
		change := func() error {
			rejected = nil
			scoped := context.WithValue(ctx, configContextKey{}, &cfg)
			path := "providers"
			if name != "" {
				path += "/" + name
			}
			guarded, err := e.GuardProviderChange(scoped, method, path, body, removeKeys)
			if err != nil {
				rejected = err
				return err
			}
			if len(guarded.RemoveKeys) > 0 {
				cfg.Keys[schemas.ModelProvider(name)] = nil
				cfg.AzureAccounts[schemas.ModelProvider(name)] = nil
			}
			out, err = apply(ctx, &cfg, guarded.Body)
			rejected = err
			return err
		}
		if name == "" {
			if err := change(); err != nil {
				return err
			}
			for provider, pc := range cfg.Providers {
				return e.store.CreateProvider(ctx, string(provider), llmcoreclient.Provider{Config: pc, Keys: cfg.Keys[provider]})
			}
			return fmt.Errorf("provider name required")
		}
		provider := schemas.ModelProvider(name)
		return e.store.MutateProvider(ctx, name, func(current *llmcoreclient.Provider) error {
			cfg.Providers[provider], cfg.Keys[provider] = current.Config, current.Keys
			cfg.AzureAccounts[provider] = current.AzureAccounts
			if err := change(); err != nil {
				return err
			}
			current.Config, current.Keys = cfg.Providers[provider], cfg.Keys[provider]
			current.AzureAccounts = cfg.AzureAccounts[provider]

			return nil
		})
	})
	if rejected != nil {
		return out, rejected
	}
	return out, storageError(err)
}

func (e *Engine) changeKey(ctx context.Context, provider, id, method string, body []byte) (Key, error) {
	if err := requireAdminIdentifier(provider); err != nil {
		return Key{}, err
	}

	return changeProvider(ctx, e, provider, method, nil, false, func(ctx context.Context, cfg *llmcoreclient.Config, _ []byte) (Key, error) {
		key, err := e.applyKey(ctx, cfg, provider, id, method, body)

		return Key{Key: key, AzureAccount: cfg.AzureAccounts[schemas.ModelProvider(provider)][key.ID]}, err
	})
}

func (e *Engine) applyKey(ctx context.Context, cfg *llmcoreclient.Config, provider, id, method string, body []byte) (schemas.Key, error) {
	// Key guards share the accepted mutation deadline and owning snapshot with the merge.
	scoped := context.WithValue(ctx, configContextKey{}, cfg)
	path := "providers/" + provider + "/keys"
	if id != "" {
		path += "/" + id
	}
	change, err := e.GuardProviderChange(scoped, method, path, body, false)
	if err != nil {
		return schemas.Key{}, err
	}
	name := schemas.ModelProvider(provider)
	pc := cfg.Providers[name]
	return updateKey(cfg, name, pc, id, method, change.Body)
}

func changeSettings[T any](ctx context.Context, e *Engine, apply func(*llmcoreclient.Config) (T, error)) (T, error) {
	var out T
	var rejected error
	err := e.mutation(ctx, func(ctx context.Context) error {
		return e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			var err error
			out, err = apply(&llmcoreclient.Config{Settings: settings})
			rejected = err
			return err
		})
	})
	if rejected != nil {
		return out, rejected
	}
	return out, storageError(err)
}

func (e *Engine) changeRule(ctx context.Context, id string, body []byte) (tables.TableRoutingRule, error) {
	var out tables.TableRoutingRule
	var rejected error
	err := e.mutation(ctx, func(ctx context.Context) error {
		cfg := llmcoreclient.Config{Rules: map[string]tables.TableRoutingRule{}}
		change := func() error {
			var err error
			out, err = updateRule(&cfg, id, body)
			rejected = err
			return err
		}
		if id == "" {
			if err := change(); err != nil {
				return err
			}
			for _, rule := range cfg.Rules {
				return e.store.PutRule(ctx, rule)
			}
		}
		return e.store.MutateRule(ctx, id, func(rule *tables.TableRoutingRule) error {
			cfg.Rules[rule.ID] = *rule
			if err := change(); err != nil {
				return err
			}
			*rule = cfg.Rules[rule.ID]
			return nil
		})
	})
	if rejected != nil {
		return tables.TableRoutingRule{}, rejected
	}
	return out, storageError(err)
}

func providerView(name schemas.ModelProvider, pc schemas.ProviderConfig, keys []schemas.Key) ProviderResponse {
	pc.CheckAndSetDefaults()
	provider := configstore.ProviderConfig{
		Keys:                     keys,
		NetworkConfig:            &pc.NetworkConfig,
		ConcurrencyAndBufferSize: &pc.ConcurrencyAndBufferSize,
		ProxyConfig:              pc.ProxyConfig,
		SendBackRawRequest:       pc.SendBackRawRequest,
		SendBackRawResponse:      pc.SendBackRawResponse,
		StoreRawRequestResponse:  pc.StoreRawRequestResponse,
		IgnoreProviderCost:       pc.IgnoreProviderCost,
		CustomProviderConfig:     pc.CustomProviderConfig,
		OpenAIConfig:             pc.OpenAIConfig,
		PromptCache:              pc.PromptCache,
		InjectedTools:            pc.InjectedTools,
	}
	return ProviderResponse{
		ProviderConfig: provider.Redacted(),
		Keys:           keyViews(provider.Redacted().Keys, nil),
		Name:           name,
		Provider:       name,
		ProviderStatus: "active",
	}
}

func object(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func (e *Engine) ListProviders(ctx context.Context) ([]ProviderResponse, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	out := []ProviderResponse{}
	for _, name := range slices.Sorted(maps.Keys(cfg.Providers)) {
		out = append(out, providerViewWithIDs(name, cfg.Providers[name], cfg.Keys[name], cfg.AzureAccounts[name]))
	}
	return out, nil
}

func (e *Engine) GetProvider(ctx context.Context, provider string) (ProviderResponse, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return ProviderResponse{}, err
	}
	name := schemas.ModelProvider(provider)
	pc, ok := cfg.Providers[name]
	if !ok {
		return ProviderResponse{}, missing("provider")
	}

	return providerViewWithIDs(name, pc, cfg.Keys[name], cfg.AzureAccounts[name]), nil
}

func (e *Engine) ListKeys(ctx context.Context) ([]Key, error) {
	providers, err := e.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	out := []Key{}

	for _, provider := range providers {
		out = append(out, provider.Keys...)
	}
	return out, nil
}

func (e *Engine) ListProviderKeys(ctx context.Context, provider string) ([]Key, error) {
	view, err := e.GetProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	return view.Keys, nil
}

func (e *Engine) GetKey(ctx context.Context, provider, id string) (Key, error) {
	keys, err := e.ListProviderKeys(ctx, provider)
	if err != nil {
		return Key{}, err
	}
	for _, key := range keys {
		if key.ID == id {
			return key, nil
		}
	}

	return Key{}, missing("key")
}

func (e *Engine) GetClientConfig(ctx context.Context) (map[string]any, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	client := map[string]any{"drop_excess_requests": false, "max_request_body_size_mb": 100}
	if raw := cfg.Settings["client"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &client)
	}
	pricing, params := modeldata.Paths(e.opts.DataDir)
	return map[string]any{"client_config": client, "framework_config": map[string]any{"pricing_url": modeldata.FileURL(pricing), "model_parameters_url": modeldata.FileURL(params)}}, nil
}

func (e *Engine) GetProxyConfig(ctx context.Context) (map[string]any, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
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
}

func (e *Engine) ListRoutingRules(ctx context.Context) ([]tables.TableRoutingRule, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	out := []tables.TableRoutingRule{}
	for _, rule := range cfg.Rules {
		out = append(out, rule)
	}
	slices.SortFunc(out, func(a, b tables.TableRoutingRule) int { return a.Priority - b.Priority })
	return out, nil
}

func (e *Engine) GetRoutingRule(ctx context.Context, id string) (tables.TableRoutingRule, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return tables.TableRoutingRule{}, err
	}
	rule, ok := cfg.Rules[id]
	if !ok {
		return tables.TableRoutingRule{}, missing("rule")
	}
	return rule, nil
}

func (e *Engine) ListPricingOverrides(ctx context.Context) ([]tables.TablePricingOverride, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	var rows []configPricingOverride
	_ = json.Unmarshal(cfg.Settings["pricing-overrides"], &rows)
	return rows, nil
}

func (e *Engine) ListCatalogModels(ctx context.Context, provider, query string) ([]map[string]any, error) {
	cfg, err := e.config(ctx)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for name := range cfg.Providers {
		if provider != "" && provider != string(name) {
			continue
		}

		names := e.prices.GetUnfilteredModelsForProvider(name)
		models, azure := e.azureDeploymentModels(cfg, name)
		if azure {
			names = nil

			for _, model := range models {
				if e.modelMetadataIsUnavailable(model.ID, model.Alias, azureBaseModel(model)) {
					continue
				}

				_, id, _ := strings.Cut(model.ID, "/")
				names = append(names, id)
			}
		}

		for _, model := range names {
			if !azure && e.modelWithKeysIsUnavailable(string(name)+"/"+model, cfg.Keys[name]) {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(model), strings.ToLower(query)) {
				continue
			}
			out = append(out, map[string]any{"name": model, "provider": name})
		}
	}
	return out, nil
}

func (e *Engine) RefreshPricing(ctx context.Context) error {
	if !e.Ready() {
		return apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	if err := e.prices.ForceReloadPricing(ctx); err != nil {
		return apierrors.NewServiceUnavailable(err.Error())
	}
	return nil
}

// RefreshProviderModels queues Azure discovery on the leader; other providers refresh synchronously.
func (e *Engine) RefreshProviderModels(ctx context.Context, name string) (pending bool, err error) {
	if _, err := e.Sync(ctx); err != nil {
		return false, apierrors.NewServiceUnavailable(err.Error())
	}
	cfg, err := e.config(ctx)
	if err != nil {
		return false, err
	}
	provider := schemas.ModelProvider(name)
	if _, ok := cfg.Providers[provider]; !ok {
		return false, missing("provider")
	}
	if isAzureProvider(cfg, provider) {
		if e.opts.TriggerModelSync == nil {
			return false, apierrors.NewServiceUnavailable("model-sync controller unavailable")
		}
		if err := e.opts.TriggerModelSync(ctx); err != nil {
			return false, err
		}
		return true, nil
	}

	if _, err := e.listGatewayModels(ctx, provider, true); err != nil {
		return false, &apierrors.StatusError{ErrStatus: metav1.Status{Code: http.StatusBadGateway, Message: err.Error()}}
	}
	return false, nil
}

func (e *Engine) CreateProvider(ctx context.Context, body json.RawMessage) (ProviderResponse, error) {
	return changeProvider(ctx, e, "", http.MethodPost, body, false, createProvider)
}

func (e *Engine) UpdateProvider(ctx context.Context, provider string, body json.RawMessage, removeKeys bool) (ProviderResponse, error) {
	if err := requireAdminIdentifier(provider); err != nil {
		return ProviderResponse{}, err
	}
	return changeProvider(ctx, e, provider, http.MethodPut, body, removeKeys, func(_ context.Context, cfg *llmcoreclient.Config, body []byte) (ProviderResponse, error) {
		name := schemas.ModelProvider(provider)
		return updateProvider(cfg, name, cfg.Providers[name], body)
	})
}

func (e *Engine) DeleteProvider(ctx context.Context, provider string) error {
	if err := requireAdminIdentifier(provider); err != nil {
		return err
	}
	return e.mutation(ctx, func(ctx context.Context) error { return storageError(e.store.DeleteProvider(ctx, provider)) })
}

func (e *Engine) CreateKey(ctx context.Context, provider string, body json.RawMessage) (Key, error) {
	return e.changeKey(ctx, provider, "", http.MethodPost, body)
}

func (e *Engine) UpdateKey(ctx context.Context, provider, id string, body json.RawMessage) (Key, error) {
	if err := requireAdminIdentifier(id); err != nil {
		return Key{}, err
	}
	return e.changeKey(ctx, provider, id, http.MethodPut, body)
}

func (e *Engine) DeleteKey(ctx context.Context, provider, id string) error {
	if err := requireAdminIdentifier(id); err != nil {
		return err
	}
	_, err := e.changeKey(ctx, provider, id, http.MethodDelete, nil)
	return err
}

func (e *Engine) UpdateClientConfig(ctx context.Context, body json.RawMessage) (map[string]any, error) {
	return changeSettings(ctx, e, func(cfg *llmcoreclient.Config) (map[string]any, error) { return updateClientConfig(cfg, body) })
}

func (e *Engine) UpdateProxyConfig(ctx context.Context, body json.RawMessage) error {
	_, err := changeSettings(ctx, e, func(cfg *llmcoreclient.Config) (struct{}, error) {
		return struct{}{}, updateProxyConfig(cfg, body)
	})
	return err
}

func (e *Engine) CreateRoutingRule(ctx context.Context, body json.RawMessage) (tables.TableRoutingRule, error) {
	return e.changeRule(ctx, "", body)
}

func (e *Engine) UpdateRoutingRule(ctx context.Context, id string, body json.RawMessage) (tables.TableRoutingRule, error) {
	if err := requireAdminIdentifier(id); err != nil {
		return tables.TableRoutingRule{}, err
	}
	return e.changeRule(ctx, id, body)
}

func (e *Engine) DeleteRoutingRule(ctx context.Context, id string) error {
	if err := requireAdminIdentifier(id); err != nil {
		return err
	}
	return e.mutation(ctx, func(ctx context.Context) error { return storageError(e.store.DeleteRule(ctx, id)) })
}

func (e *Engine) CreatePricingOverride(ctx context.Context, body json.RawMessage) (tables.TablePricingOverride, error) {
	return changeSettings(ctx, e, func(cfg *llmcoreclient.Config) (tables.TablePricingOverride, error) {
		return updatePricingOverride(cfg, "", http.MethodPost, body)
	})
}

func (e *Engine) UpdatePricingOverride(ctx context.Context, id string, body json.RawMessage) (tables.TablePricingOverride, error) {
	if err := requireAdminIdentifier(id); err != nil {
		return tables.TablePricingOverride{}, err
	}
	return changeSettings(ctx, e, func(cfg *llmcoreclient.Config) (tables.TablePricingOverride, error) {
		return updatePricingOverride(cfg, id, http.MethodPut, body)
	})
}

func (e *Engine) DeletePricingOverride(ctx context.Context, id string) (tables.TablePricingOverride, error) {
	if err := requireAdminIdentifier(id); err != nil {
		return tables.TablePricingOverride{}, err
	}
	return changeSettings(ctx, e, func(cfg *llmcoreclient.Config) (tables.TablePricingOverride, error) {
		return updatePricingOverride(cfg, id, http.MethodDelete, nil)
	})
}

// mergeJSON preserves omitted fields and masked secrets in partial admin updates.
// Maps retain field presence and explicit nulls, which decoding into Bifrost structs loses.
func mergeJSON(current any, body []byte) ([]byte, error) {
	base := object(current)
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return nil, err
	}
	// Declare before assigning so the closure can call itself recursively.
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

func createProvider(_ context.Context, cfg *llmcoreclient.Config, body []byte) (ProviderResponse, error) {
	var in struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return ProviderResponse{}, err
	}
	if in.Provider == "" || strings.ContainsAny(in.Provider, "/\\ ") {
		return ProviderResponse{}, fmt.Errorf("provider name required")
	}
	name := schemas.ModelProvider(in.Provider)
	if _, ok := cfg.Providers[name]; ok {
		return ProviderResponse{}, fmt.Errorf("provider already exists")
	}

	var pc schemas.ProviderConfig
	defaults := object(json.RawMessage(body))
	ApplyProviderDefaults(defaults)
	raw, _ := json.Marshal(defaults)
	if err := json.Unmarshal(raw, &pc); err != nil {
		return ProviderResponse{}, err
	}
	if err := validateProvider(name, pc); err != nil {
		return ProviderResponse{}, err
	}

	pc.CheckAndSetDefaults()
	if err := validateProvider(name, pc); err != nil {
		return ProviderResponse{}, err
	}

	cfg.Providers[name] = pc
	return providerView(name, pc, nil), nil
}

func updateProvider(cfg *llmcoreclient.Config, name schemas.ModelProvider, pc schemas.ProviderConfig, body []byte) (ProviderResponse, error) {
	// Nested provider settings use replacement semantics when explicitly supplied.
	current := object(pc)
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return ProviderResponse{}, err
	}
	for _, field := range []string{"network_config", "proxy_config", "concurrency_and_buffer_size", "custom_provider_config"} {
		if value, ok := patch[field]; ok {
			current[field] = value
		}
	}
	restoreMasked(current, object(pc))
	raw, err := mergeJSON(current, body)
	if err != nil {
		return ProviderResponse{}, err
	}
	var next schemas.ProviderConfig
	if err := json.Unmarshal(raw, &next); err != nil {
		return ProviderResponse{}, err
	}
	if err := validateProvider(name, next); err != nil {
		return ProviderResponse{}, err
	}
	next.CheckAndSetDefaults()
	if err := validateProvider(name, next); err != nil {
		return ProviderResponse{}, err
	}

	cfg.Providers[name] = next

	return providerViewWithIDs(name, next, cfg.Keys[name], cfg.AzureAccounts[name]), nil
}

func updateKey(cfg *llmcoreclient.Config, name schemas.ModelProvider, pc schemas.ProviderConfig, id, method string, body []byte) (schemas.Key, error) {
	keys := cfg.Keys[name]
	index := -1
	if id != "" {
		index = slices.IndexFunc(keys, func(k schemas.Key) bool { return k.ID == id })
		if index < 0 {
			return schemas.Key{}, missing("key")
		}
	}
	if method == http.MethodDelete {
		if index < 0 {
			return schemas.Key{}, missing("key")
		}

		delete(cfg.AzureAccounts[name], id)
		cfg.Keys[name] = slices.Delete(keys, index, index+1)
		return schemas.Key{}, nil
	}

	key := schemas.Key{ID: uuid.New().String(), Weight: 1, Models: schemas.WhiteList{"*"}}
	if index >= 0 {
		key = keys[index]
	}
	raw, err := mergeJSON(key, body)
	if err != nil {
		return schemas.Key{}, err
	}
	if err = json.Unmarshal(raw, &key); err != nil {
		return schemas.Key{}, err
	}

	// The URL (or a fresh create ID) owns metadata; a body ID never selects another key.
	if index >= 0 {
		key.ID = keys[index].ID
	} else {
		key.ID = uuid.New().String()
	}

	var discovery struct {
		SubscriptionID *string         `json:"azure_subscription_id"`
		ResourceGroup  *string         `json:"azure_resource_group"`
		AccountName    *string         `json:"azure_account_name"`
		ResourceID     json.RawMessage `json:"azure_resource_id"`
	}

	if err := json.Unmarshal(body, &discovery); err != nil {
		return schemas.Key{}, err
	}

	if len(discovery.ResourceID) > 0 {
		return schemas.Key{}, fmt.Errorf("use azure_subscription_id, azure_resource_group and azure_account_name instead of azure_resource_id")
	}

	if cfg.AzureAccounts == nil {
		cfg.AzureAccounts = map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{}
	}

	if cfg.AzureAccounts[name] == nil {
		cfg.AzureAccounts[name] = map[string]llmcoreclient.AzureAccount{}
	}

	account := cfg.AzureAccounts[name][key.ID]
	if discovery.SubscriptionID != nil {
		account.SubscriptionID = strings.TrimSpace(*discovery.SubscriptionID)
	}

	if discovery.ResourceGroup != nil {
		account.ResourceGroup = strings.TrimSpace(*discovery.ResourceGroup)
	}

	if discovery.AccountName != nil {
		account.AccountName = strings.TrimSpace(*discovery.AccountName)
	}

	if account != (llmcoreclient.AzureAccount{}) {
		if err := validateAzureDiscovery(key, account); err != nil {
			return schemas.Key{}, err
		}
	}

	// Validate the decoded key so alternate field spellings cannot disagree with persistence.
	effective, err := json.Marshal(key)
	if err != nil {
		return schemas.Key{}, err
	}
	// Partial credential clears can switch retained endpoints to server identity.
	if name == schemas.Azure || key.AzureKeyConfig != nil {
		if err := ValidateAzureKey(effective); err != nil {
			return schemas.Key{}, err
		}
	}
	if slices.ContainsFunc(keys, func(existing schemas.Key) bool { return existing.ID != key.ID && existing.Name == key.Name }) {
		return schemas.Key{}, fmt.Errorf("key name already exists")
	}

	if key.Name == "" || key.Weight < 0 {
		return schemas.Key{}, fmt.Errorf("key name and nonnegative weight required")
	}
	if err := key.Models.Validate(); err != nil {
		return schemas.Key{}, err
	}
	if err := key.BlacklistedModels.Validate(); err != nil {
		return schemas.Key{}, err
	}
	if err := key.Aliases.Validate(name); err != nil {
		return schemas.Key{}, err
	}
	if index >= 0 {
		keys[index] = key
	} else {
		keys = append(keys, key)
	}

	if account == (llmcoreclient.AzureAccount{}) {
		delete(cfg.AzureAccounts[name], key.ID)
	} else {
		cfg.AzureAccounts[name][key.ID] = account
	}

	cfg.Keys[name] = keys
	redacted := providerView(name, pc, []schemas.Key{key})

	return redacted.Keys[0].Key, nil
}

func updateClientConfig(cfg *llmcoreclient.Config, body []byte) (map[string]any, error) {
	var in struct {
		Client map[string]any `json:"client_config"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	prior := object(cfg.Settings["client"])
	maps.Copy(prior, in.Client)
	if v, ok := prior["max_request_body_size_mb"].(float64); ok && (v <= 0 || v > 1048576) {
		return nil, fmt.Errorf("invalid request body size")
	}
	raw, err := json.Marshal(prior)
	if err != nil {
		return nil, err
	}
	cfg.Settings["client"] = raw
	return prior, nil
}

func updateProxyConfig(cfg *llmcoreclient.Config, body []byte) error {
	var previous map[string]any
	if raw := cfg.Settings["proxy"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return err
		}
	}
	raw, err := mergeJSON(previous, body)
	if err != nil {
		return err
	}
	cfg.Settings["proxy"] = raw
	return nil
}

func updateRule(cfg *llmcoreclient.Config, id string, body []byte) (tables.TableRoutingRule, error) {
	var in tables.TableRoutingRule
	if id != "" {
		var ok bool
		in, ok = cfg.Rules[id]
		if !ok {
			return tables.TableRoutingRule{}, missing("rule")
		}
	}
	raw, err := mergeJSON(in, body)
	if err != nil {
		return tables.TableRoutingRule{}, err
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return tables.TableRoutingRule{}, err
	}
	if id != "" {
		in.ID = id
	} else {
		in.ID = uuid.New().String()
	}
	if in.Scope == "" {
		in.Scope = "global"
	}
	if in.Scope != "global" || in.ScopeID != nil {
		return tables.TableRoutingRule{}, fmt.Errorf("only global routing rules are supported")
	}
	if err := rules.ValidateCELExpression(in.CelExpression); err != nil {
		return tables.TableRoutingRule{}, err
	}
	if usesUnsupportedBudgetVariable(in.CelExpression) {
		return tables.TableRoutingRule{}, fmt.Errorf("routing budget variables are not supported")
	}
	if len(in.Targets) == 0 {
		return tables.TableRoutingRule{}, fmt.Errorf("routing targets required")
	}
	for i := range in.Targets {
		target := &in.Targets[i]
		target.RuleID = in.ID
		if target.Weight < 0 {
			return tables.TableRoutingRule{}, fmt.Errorf("target weight must be nonnegative")
		}
		if target.TTFTTimeoutMs != nil && (*target.TTFTTimeoutMs < 0 || *target.TTFTTimeoutMs > 300000) {
			return tables.TableRoutingRule{}, fmt.Errorf("invalid first-token timeout")
		}
	}

	cfg.Rules[in.ID] = in
	return in, nil
}

func updatePricingOverride(cfg *llmcoreclient.Config, id, method string, body []byte) (tables.TablePricingOverride, error) {
	var rows []configPricingOverride
	_ = json.Unmarshal(cfg.Settings["pricing-overrides"], &rows)
	var in configPricingOverride
	index := -1
	if id != "" {
		index = slices.IndexFunc(rows, func(v configPricingOverride) bool { return v.ID == id })
		if index < 0 {
			return tables.TablePricingOverride{}, missing("pricing override")
		}
		in = rows[index]
	}
	if method == http.MethodDelete {
		if index < 0 {
			return tables.TablePricingOverride{}, missing("pricing override")
		}
		rows = slices.Delete(rows, index, index+1)
	} else {
		raw, err := mergeJSON(in, body)
		if err != nil {
			return tables.TablePricingOverride{}, err
		}
		if err = json.Unmarshal(raw, &in); err != nil {
			return tables.TablePricingOverride{}, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return tables.TablePricingOverride{}, err
		}
		patch := fields["patch"]
		// An omitted patch preserves custom prices; an explicit patch replaces them.
		if len(patch) == 0 && index >= 0 {
			patch = []byte(rows[index].PricingPatchJSON)
		}
		if len(patch) == 0 {
			patch = []byte(`{}`)
		}
		var options modelcatalog.PricingOptions
		if err := json.Unmarshal(patch, &options); err != nil {
			return tables.TablePricingOverride{}, fmt.Errorf("invalid pricing patch: %w", err)
		}
		in.PricingPatchJSON = string(patch)
		shape := modelcatalog.PricingOverride{Name: in.Name, ScopeKind: modelcatalog.ScopeKind(in.ScopeKind), UserID: in.UserID, VirtualKeyID: in.VirtualKeyID, ProviderID: in.ProviderID, ProviderKeyID: in.ProviderKeyID, MatchType: modelcatalog.MatchType(in.MatchType), Pattern: in.Pattern, RequestTypes: in.RequestTypes, Options: options}
		if err := shape.IsValid(); err != nil {
			return tables.TablePricingOverride{}, err
		}

		if index < 0 {
			in.ID = uuid.New().String()
			rows = append(rows, in)
		} else {
			in.ID = id
			rows[index] = in
		}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return tables.TablePricingOverride{}, err
	}
	cfg.Settings["pricing-overrides"] = raw
	return in, nil
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
		return apierrors.NewBadRequest("Concurrency must be less than or equal to buffer size")
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
