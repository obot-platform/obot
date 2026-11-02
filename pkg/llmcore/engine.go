// Package llmcore implements Obot's LLM gateway using the Bifrost core library
// (github.com/maximhq/bifrost/core) instead of Bifrost's HTTP transport.
//
// Providers, keys, routing rules and settings live in Obot resources. Each replica's
// engine reconciles their desired configuration against its successfully applied state.
// Resource watches trigger Sync without rebuilding the engine.
package llmcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/framework/tracing"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// Options configure an Engine.
type Options struct {
	Store *llmcoreclient.Client
	// Plugins run around every attempt the engine makes, after routing rules have picked the
	// model: Obot's access, budget and guardrail plugins.
	Plugins []schemas.LLMPlugin
	Logger  schemas.Logger
	// Catalog supplies request pricing and model metadata to Obot plugins.
	Catalog schemas.ModelInfoProvider
}

// Engine is one replica's gateway.
type Engine struct {
	traceStore *tracing.TraceStore
	tracer     *tracing.Tracer
	stopOnce   sync.Once
	catalog    schemas.ModelInfoProvider
	store      *llmcoreclient.Client
	client     *bifrost.Bifrost
	account    *account
	rules      *rules.LocalStore

	// syncLock serializes Sync, so two syncs don't apply provider changes out of order.
	syncLock sync.Mutex
	// applied is the configuration last applied.
	applied llmcoreclient.Config
	// A proxy change may apply to only some providers before Sync fails. Track
	// each success so retrying reconciliation only restarts providers still behind.
	appliedProxy map[schemas.ModelProvider]json.RawMessage
}

// account serves the engine the configuration last loaded. Bifrost reads keys on every
// request, so a key change applies to the next request without touching the engine.
type account struct {
	config atomic.Pointer[llmcoreclient.Config]
}

func (a *account) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return slices.Collect(maps.Keys(a.config.Load().Providers)), nil
}

func (a *account) GetKeysForProvider(_ context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	return a.config.Load().Keys[provider], nil
}

func (a *account) GetConfigForProvider(provider schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	cfg := a.config.Load()
	pc, ok := cfg.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider %s is not configured", provider)
	}

	pc.CheckAndSetDefaults()

	var proxy tables.GlobalProxyConfig
	_ = json.Unmarshal(cfg.Settings["proxy"], &proxy)
	if proxy.Type == "" {
		proxy.Type = "http"
	}
	if proxy.Enabled && proxy.EnableForInference && strings.TrimSpace(proxy.URL) != "" && (proxy.Type == "http" || proxy.Type == "socks5") && (pc.ProxyConfig == nil || pc.ProxyConfig.Type == schemas.NoProxy) {
		pc.ProxyConfig = &schemas.ProxyConfig{Type: schemas.ProxyType(proxy.Type), URL: &schemas.SecretVar{Val: proxy.URL}, Username: &schemas.SecretVar{Val: proxy.Username}, Password: &schemas.SecretVar{Val: proxy.Password}, NoProxy: proxy.NoProxy, SkipTLSVerify: proxy.SkipTLSVerify}
	}
	return &pc, nil
}

// New starts an engine with the configuration in the store.
func New(ctx context.Context, opts Options) (*Engine, error) {
	if opts.Store == nil {
		return nil, errors.New("a store is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	}

	cfg, err := opts.Store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load the gateway configuration: %w", err)
	}

	// A nil config store gives an empty rule set, which Sync fills from Obot's tables.
	ruleStore, err := rules.NewLocalStore(ctx, logger, nil)
	if err != nil {
		return nil, err
	}

	routing, err := newRoutingPlugin(ruleStore, logger)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		store:        opts.Store,
		catalog:      opts.Catalog,
		account:      &account{},
		rules:        ruleStore,
		appliedProxy: make(map[schemas.ModelProvider]json.RawMessage),
	}
	e.account.config.Store(&cfg)
	for _, rule := range cfg.Rules {
		if err := ruleStore.UpsertRule(ctx, &rule); err != nil {
			return nil, fmt.Errorf("routing rule %s: %w", rule.ID, err)
		}
	}

	e.traceStore = tracing.NewTraceStore(5*time.Minute, logger)
	e.tracer = tracing.NewTracer(e.traceStore, nil, logger)
	e.client, err = bifrost.Init(ctx, schemas.BifrostConfig{
		Account:      e.account,
		LLMPlugins:   orderedPlugins(routing, opts.Plugins),
		Logger:       logger,
		ModelCatalog: opts.Catalog,
		Tracer:       e.tracer,
	})
	if err != nil {
		e.tracer.Stop()
		e.traceStore.Stop()
		return nil, err
	}
	e.applySettings(cfg)
	e.applied = cfg
	for name := range cfg.Providers {
		e.appliedProxy[name] = cfg.Settings["proxy"]
	}
	return e, nil
}

// Sync brings the engine up to the store's configuration, if it changed. Only what changed is
// touched: a changed or added provider is restarted by Bifrost with its new settings while its
// in-flight requests finish, keys take effect on the next request, and rules are replaced one
// by one. Nothing else is rebuilt.
func (e *Engine) Sync(ctx context.Context) (bool, error) {
	e.syncLock.Lock()
	defer e.syncLock.Unlock()

	cfg, err := e.store.Load(ctx)
	if err != nil {
		return false, err
	}
	return e.apply(ctx, cfg)
}

// Apply reconciles independently versioned desired resources. Failed updates remain
// retryable with unchanged desired state; successful providers are not restarted again.
func (e *Engine) Apply(ctx context.Context, cfg llmcoreclient.Config) (bool, error) {
	e.syncLock.Lock()
	defer e.syncLock.Unlock()
	return e.apply(ctx, cfg)
}

func (e *Engine) apply(ctx context.Context, cfg llmcoreclient.Config) (bool, error) {
	changed := !reflect.DeepEqual(e.applied, cfg)
	e.applySettings(cfg)
	old := e.applied
	old.Providers = maps.Clone(old.Providers)
	old.Rules = maps.Clone(old.Rules)
	old.Keys = cfg.Keys
	old.Settings = cfg.Settings
	e.account.config.Store(&cfg)

	var errs []error
	for name, pc := range cfg.Providers {
		if prev, ok := old.Providers[name]; !ok || !reflect.DeepEqual(prev, pc) || !reflect.DeepEqual(e.appliedProxy[name], cfg.Settings["proxy"]) {
			if err := e.client.UpdateProvider(name); err != nil {
				errs = append(errs, err)
			} else {
				old.Providers[name] = pc
				e.appliedProxy[name] = cfg.Settings["proxy"]
			}
		}
	}
	for name := range old.Providers {
		if _, ok := cfg.Providers[name]; !ok {
			if err := e.client.RemoveProvider(name); err != nil {
				errs = append(errs, err)
			} else {
				delete(old.Providers, name)
				delete(e.appliedProxy, name)
			}
		}
	}
	for id, rule := range cfg.Rules {
		if prev, ok := old.Rules[id]; !ok || !reflect.DeepEqual(prev, rule) {
			if err := e.rules.UpsertRule(ctx, &rule); err != nil {
				errs = append(errs, err)
			} else {
				old.Rules[id] = rule
			}
		}
	}
	for id := range old.Rules {
		if _, ok := cfg.Rules[id]; !ok {
			if err := e.rules.DeleteRule(ctx, id); err != nil {
				errs = append(errs, err)
			} else {
				delete(old.Rules, id)
			}
		}
	}
	for id, rule := range cfg.Rules {
		for _, target := range rule.Targets {
			if target.Provider != nil {
				if _, ok := cfg.Providers[schemas.ModelProvider(*target.Provider)]; !ok {
					errs = append(errs, fmt.Errorf("routing rule %s: provider %s is not configured", id, *target.Provider))
				}
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		e.applied = old
		return true, err
	}
	e.applied = cfg
	return changed, nil
}

// Shutdown stops the engine, letting in-flight requests finish.
func (e *Engine) Shutdown() {
	e.stopOnce.Do(func() { e.client.Shutdown(); e.tracer.Stop(); e.traceStore.Stop() })
}

func (e *Engine) applySettings(cfg llmcoreclient.Config) {
	var settings struct {
		Drop bool `json:"drop_excess_requests"`
	}
	_ = json.Unmarshal(cfg.Settings["client"], &settings)
	e.client.UpdateDropExcessRequests(settings.Drop)
	if catalog, ok := e.catalog.(interface {
		ReplaceKeyConfig(map[schemas.ModelProvider][]schemas.Key)
	}); ok {
		catalog.ReplaceKeyConfig(cfg.Keys)
	}
}
func (e *Engine) MaxRequestBodyBytes() int64 {
	var settings struct {
		Max int64 `json:"max_request_body_size_mb"`
	}
	_ = json.Unmarshal(e.account.config.Load().Settings["client"], &settings)
	if settings.Max <= 0 {
		return maxRequestBodyBytes
	}
	return settings.Max << 20
}

// Setting returns an immutable copy of a setting from the loaded configuration.
func (e *Engine) Setting(name string) json.RawMessage {
	return append(json.RawMessage{}, e.account.config.Load().Settings[name]...)
}

// Input hooks see the original request before routing; output hooks run last in the list.
func orderedPlugins(routing schemas.LLMPlugin, plugins []schemas.LLMPlugin) []schemas.LLMPlugin {
	if len(plugins) == 0 {
		return []schemas.LLMPlugin{routing}
	}
	return append([]schemas.LLMPlugin{plugins[0], routing}, plugins[1:]...)
}
