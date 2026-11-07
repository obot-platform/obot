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
	"math"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/framework/streaming"
	"github.com/maximhq/bifrost/framework/tracing"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// Options configure an Engine.
type Options struct {
	// Plugins are registered in order, with routing inserted after Plugins[0]. The first
	// plugin sees the original request in PreRequestHook; the remaining plugins
	// see the routed request. All PreLLMHook calls run per attempt after routing.
	Plugins []schemas.LLMPlugin
	Logger  schemas.Logger
	// Catalog supplies request pricing and model metadata to Obot plugins.
	Catalog schemas.ModelInfoProvider
}

// Engine is one replica's gateway.
type Engine struct {
	accumulator *streaming.Accumulator
	traceStore  *tracing.TraceStore
	tracer      *tracing.Tracer
	stopOnce    sync.Once
	catalog     schemas.ModelInfoProvider
	store       *llmcoreclient.Client
	client      *bifrost.Bifrost
	mux         *http.ServeMux
	account     *account
	routing     *routingPlugin

	shutdownDone chan struct{}

	// Anthropic drains after its admitted subscription requests finish.
	subscriptionLifetime sync.RWMutex

	// syncLock serializes Sync, so two syncs don't apply provider changes out of order.
	syncLock sync.Mutex
	// retirements retain unfinished removals across Sync retries. Both fields
	// are protected by syncLock; Shutdown joins removals before stopping core.
	retirements map[schemas.ModelProvider]*providerRetirement
	stopped     bool
	// syncErr includes load/application failures and non-retryable routing diagnostics,
	// even when healthy providers can serve requests.
	syncErr error
	// applied is the configuration last applied.
	applied llmcoreclient.Config
	// A proxy change may apply to only some providers before Sync fails. Track
	// each success so retrying reconciliation only restarts providers still behind.
	appliedProxy map[schemas.ModelProvider]*schemas.ProxyConfig
}

// account publishes credentials only after affected runtimes have drained. Bifrost
// reads keys per request, so key-only rotation does not require a runtime restart.
type account struct {
	config atomic.Pointer[llmcoreclient.Config]
	// transitioning gates requests while a provider is drained/replaced. The bool
	// permits config reads only while reconciliation constructs the new runtime;
	// credentials and attempts remain blocked until construction succeeds.
	transitioning sync.Map // schemas.ModelProvider -> bool
}

func (a *account) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return slices.Collect(maps.Keys(a.config.Load().Providers)), nil
}

func (a *account) GetKeysForProvider(_ context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	if _, blocked := a.transitioning.Load(provider); blocked {
		return nil, fmt.Errorf("provider %s is being reconfigured", provider)
	}

	cfg := a.config.Load()
	keys := cfg.Keys[provider]
	pc := cfg.Providers[provider]
	azure := provider == schemas.Azure || (pc.CustomProviderConfig != nil && pc.CustomProviderConfig.BaseProviderType == schemas.Azure)
	if !azure || len(cfg.AzureAccounts[provider]) == 0 {
		return keys, nil
	}

	// Bifrost filters on the requested name before resolving its per-key alias.
	// Only the runtime copies are narrowed; saved policies remain authoritative.
	keys = slices.Clone(keys)
	for i := range keys {
		key := &keys[i]
		account := cfg.AzureAccounts[provider][key.ID]
		if account == (llmcoreclient.AzureAccount{}) {
			continue
		}

		snapshot, ok := cfg.AzureDiscoveries[provider][key.ID]
		pipeline := providerUtils.ListModelsPipeline{
			AllowedModels:     key.Models,
			BlacklistedModels: key.BlacklistedModels,
			Aliases:           key.Aliases,
			MatchFns:          providerUtils.DefaultMatchFns(),
		}
		key.Models = nil
		if !ok || snapshot.Fingerprint != llmcoreclient.AzureDiscoveryFingerprint(*key, account) || pipeline.ShouldEarlyExit() {
			continue
		}

		for _, deployment := range snapshot.Names {
			for _, match := range pipeline.FilterModel(deployment) {
				// An alias can shadow a discovered name; authorize its actual wire target only.
				if key.Aliases.Resolve(match.ResolvedID) != deployment {
					continue
				}

				key.Models = append(key.Models, match.ResolvedID)
			}
		}
	}

	return keys, nil
}

func (a *account) GetConfigForProvider(provider schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	if allowConfig, blocked := a.transitioning.Load(provider); blocked && !allowConfig.(bool) {
		return nil, fmt.Errorf("provider %s is being reconfigured", provider)
	}
	cfg := a.config.Load()
	pc, ok := cfg.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider %s is not configured", provider)
	}

	if err := validateProviderQueue(provider, pc); err != nil {
		return nil, err
	}

	// Bifrost stamps CustomProviderKey during construction. Keep that runtime
	// mutation out of the immutable account/applied snapshot, or every retry
	// treats a healthy custom provider as changed and restarts it.
	if pc.CustomProviderConfig != nil {
		custom := *pc.CustomProviderConfig
		pc.CustomProviderConfig = &custom
	}
	pc.CheckAndSetDefaults()

	pc.ProxyConfig = ProviderProxy(pc, cfg.Settings["proxy"])
	return &pc, nil
}

// Bifrost defaults only zero values; negative sizes can panic or start no workers.
func validateProviderQueue(provider schemas.ModelProvider, pc schemas.ProviderConfig) error {
	if pc.ConcurrencyAndBufferSize.Concurrency < 0 {
		return fmt.Errorf("provider %s: concurrency must not be negative", provider)
	}

	if pc.ConcurrencyAndBufferSize.BufferSize < 0 {
		return fmt.Errorf("provider %s: buffer_size must not be negative", provider)
	}

	return nil
}

// ProviderProxy returns the effective inference proxy, including provider overrides.
// Compare this value during reconciliation so unrelated or cosmetic global edits
// do not restart providers whose network behavior has not changed.
func ProviderProxy(pc schemas.ProviderConfig, raw json.RawMessage) *schemas.ProxyConfig {
	pc.CheckAndSetDefaults()
	// An empty proxy object, like NoProxy, is unconfigured and must inherit
	// an enabled global inference proxy.
	if pc.ProxyConfig != nil && pc.ProxyConfig.Type != "" && pc.ProxyConfig.Type != schemas.NoProxy {
		return pc.ProxyConfig
	}
	var proxy tables.GlobalProxyConfig
	_ = json.Unmarshal(raw, &proxy)
	if proxy.Type == "" {
		proxy.Type = "http"
	}
	globalProxyEnabled := proxy.Enabled && proxy.EnableForInference && strings.TrimSpace(proxy.URL) != ""
	supportedProxyType := proxy.Type == "http" || proxy.Type == "socks5"
	if !globalProxyEnabled || !supportedProxyType {
		return pc.ProxyConfig
	}
	return &schemas.ProxyConfig{
		Type:          schemas.ProxyType(proxy.Type),
		URL:           &schemas.SecretVar{Val: proxy.URL},
		Username:      &schemas.SecretVar{Val: proxy.Username},
		Password:      &schemas.SecretVar{Val: proxy.Password},
		NoProxy:       proxy.NoProxy,
		SkipTLSVerify: proxy.SkipTLSVerify,
	}
}

// New starts an engine with the configuration in the store. Individual provider
// failures leave a degraded engine available; Err reports them until Sync recovers.
func New(ctx context.Context, store *llmcoreclient.Client, opts Options) (*Engine, error) {
	if store == nil {
		return nil, errors.New("a store is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load the gateway configuration: %w", err)
	}

	cfg := loaded.Config

	// A nil config store gives an empty rule set, which Sync fills from Obot's tables.
	ruleStore, err := rules.NewLocalStore(ctx, logger, nil)
	if err != nil {
		return nil, err
	}

	a := &account{}
	a.config.Store(&cfg)
	routing, err := newRoutingPlugin(ruleStore, logger, a)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		store:        store,
		catalog:      opts.Catalog,
		account:      a,
		routing:      routing,
		appliedProxy: make(map[schemas.ModelProvider]*schemas.ProxyConfig),
	}
	e.account.config.Store(&cfg)
	for _, rule := range cfg.Rules {
		if err := routing.UpsertRule(ctx, &rule); err != nil {
			return nil, fmt.Errorf("routing rule %s: %w", rule.ID, err)
		}
	}

	e.mux = e.newMux()

	// The tracer also owns stream pause/resume and buffered chunks. Output
	// guardrails need it to hold and rewrite a stream before releasing content;
	// Bifrost's default no-op tracer cannot provide that behavior.
	e.traceStore = tracing.NewTraceStore(5*time.Minute, logger)
	e.tracer = tracing.NewTracer(e.traceStore, nil, logger)
	// Classify subscription attempts before access/budget hooks; enforce their
	// destination again at the last gate after any trusted plugin rewrites.
	plugins := append([]schemas.LLMPlugin{&subscriptionPlugin{account: e.account}}, orderedPlugins(routing, opts.Plugins)...)
	plugins = append(plugins, &providerTransitionPlugin{account: e.account})
	e.client, err = bifrost.Init(ctx, schemas.BifrostConfig{
		Account:      e.account,
		LLMPlugins:   plugins,
		Logger:       logger,
		ModelCatalog: opts.Catalog,
		Tracer:       e.tracer,
	})
	if err != nil {
		e.tracer.Stop()
		e.traceStore.Stop()
		return nil, err
	}
	// Audit accumulation is separate from provider hooks, which see output before guardrail replay.
	e.accumulator = streaming.NewAccumulator(nil, logger)
	e.applySettings(cfg)
	e.applied = cfg
	// Init logs individual constructor failures but still returns a client. Its
	// provider instances, not the desired configuration, tell us what started.
	// Clone before filtering: the account must retain the complete desired view.
	e.applied.Providers = maps.Clone(cfg.Providers)
	startupErrors := []error{providerLoadErrors(loaded.ProviderErrors), routingDependencyWarnings(cfg)}
	for name := range cfg.Providers {
		// GetProviderByKey can lazily start standard providers with defaults on
		// account errors. Do not let that probe hide a rejected queue config.
		queueErr := validateProviderQueue(name, cfg.Providers[name])
		if queueErr != nil || e.client.GetProviderByKey(name) == nil {
			delete(e.applied.Providers, name)
			e.account.transitioning.Store(name, false)
			// Init only logs account errors. Retain queue diagnostics in Err as well.
			if queueErr != nil {
				startupErrors = append(startupErrors, queueErr)
			} else {
				startupErrors = append(startupErrors, fmt.Errorf("provider %s failed to initialize; reconciliation will retry", name))
			}
			// prepareProvider publishes its queue before constructing the runtime.
			// Clean up the workerless queue and prohibit lazy recreation until Sync.
			if err := e.client.RemoveProvider(name); err != nil {
				startupErrors = append(startupErrors, err)
			}
			continue
		}
		e.appliedProxy[name] = ProviderProxy(cfg.Providers[name], cfg.Settings["proxy"])
	}
	e.syncErr = errors.Join(startupErrors...)
	return e, nil
}

// Sync brings the engine up to the store's configuration, if it changed. Only what changed is
// touched: a changed provider is gated and drained before its replacement starts;
// key-only changes take effect on the next request, and rules are replaced one
// by one. Waiting for drains has a shared one-second budget; unfinished removals
// stay gated for retry while unrelated configuration advances.
func (e *Engine) Sync(ctx context.Context) (bool, error) {
	e.syncLock.Lock()
	defer e.syncLock.Unlock()
	if e.stopped {
		return false, errors.New("gateway engine is shut down")
	}

	loaded, err := e.store.Load(ctx)
	if err != nil {
		// A blocking read cannot safely replace any part of the running configuration.
		e.syncErr = err
		return false, err
	}
	// Unreadable aggregates are omitted from Config. Applying that view removes
	// their runtime providers and keys while still advancing healthy changes.
	changed, applyErr := e.apply(ctx, loaded.Config)
	retryErr := errors.Join(providerLoadErrors(loaded.ProviderErrors), applyErr)
	// Dangling references are already applied policy. Retrying cannot repair
	// them; a provider/rule resource event will reconcile the eventual repair.
	e.syncErr = errors.Join(retryErr, routingDependencyWarnings(loaded.Config))
	return changed, retryErr
}

// Err reports replica-local load/application failures and routing diagnostics,
// including degraded startup. A missing routing dependency alone does not fail Sync.
func (e *Engine) Err() error {
	e.syncLock.Lock()
	defer e.syncLock.Unlock()
	return e.syncErr
}

func providerLoadErrors(diagnostics []llmcoreclient.ProviderLoadError) error {
	errs := make([]error, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		errs = append(errs, diagnostic)
	}
	return errors.Join(errs...)
}

func routingDependencyWarnings(cfg llmcoreclient.Config) error {
	// Keep rules live even when their primary provider is absent: dropping them
	// would bypass routing policy. Missing optional fallbacks remain in the
	// chain for request-time handling and do not invalidate a healthy primary.
	var errs []error
	for id, rule := range cfg.Rules {
		for _, target := range rule.Targets {
			// Both omitted and empty providers inherit the request's provider.
			if target.Provider != nil && *target.Provider != "" {
				if _, ok := cfg.Providers[schemas.ModelProvider(*target.Provider)]; !ok {
					errs = append(errs, fmt.Errorf("routing rule %s: provider %s is not configured", id, *target.Provider))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// apply updates the live Bifrost providers and routing rules after desired resources
// change; controller writes alone do not update Bifrost's in-memory state.
// The caller must hold syncLock to serialize reconciliation and applied-state updates.
func (e *Engine) apply(ctx context.Context, cfg llmcoreclient.Config) (bool, error) {
	changed := !reflect.DeepEqual(e.applied, cfg)
	// Start from the last known applied state. Only successful items advance;
	// failed providers and rules retain their previous values for the next retry.
	old := e.applied
	old.Providers = maps.Clone(old.Providers)
	old.Rules = maps.Clone(old.Rules)
	old.Settings = cfg.Settings
	// Workers read keys separately from their captured provider. Pending drains
	// must retain their old account values while unrelated providers advance.
	previous := e.account.config.Load()
	updates := make(map[schemas.ModelProvider]*schemas.ProxyConfig)
	retiring := make(map[schemas.ModelProvider]struct{})
	for name, pc := range cfg.Providers {
		proxy := ProviderProxy(pc, cfg.Settings["proxy"])
		_, pending := e.account.transitioning.Load(name)
		if prev, ok := old.Providers[name]; !ok || pending || !reflect.DeepEqual(prev, pc) || !reflect.DeepEqual(e.appliedProxy[name], proxy) {
			updates[name] = proxy
			retiring[name] = struct{}{}
		}
	}
	for name := range previous.Providers {
		if _, exists := cfg.Providers[name]; !exists {
			retiring[name] = struct{}{}
		}
	}
	for name := range retiring {
		e.account.transitioning.Store(name, false)
	}
	toDrain := make(map[schemas.ModelProvider]struct{})
	for name := range retiring {
		if e.client.GetProviderByKey(name) != nil || e.retirements[name] != nil {
			toDrain[name] = struct{}{}
		}
	}
	blocked, errs := e.retireProviders(ctx, toDrain, e.client.RemoveProvider)

	live := cfg
	live.Providers = maps.Clone(cfg.Providers)
	live.Keys = maps.Clone(cfg.Keys)
	for name := range blocked {
		if pc, exists := previous.Providers[name]; exists {
			live.Providers[name] = pc
		} else {
			delete(live.Providers, name)
		}
		if keys, exists := previous.Keys[name]; exists {
			live.Keys[name] = keys
		} else {
			delete(live.Keys, name)
		}
	}
	e.account.config.Store(&live)
	e.applySettings(live)
	old.Keys = live.Keys
	for name, proxy := range updates {
		if _, exists := blocked[name]; exists {
			continue
		}
		// Bifrost's constructor reads provider config through the same Account as
		// lazy creation. Both may now see only the new destination, while request
		// hooks and key reads remain gated until successful construction.
		e.account.transitioning.Store(name, true)
		if err := e.client.UpdateProvider(name); err != nil {
			e.account.transitioning.Store(name, false)
			errs = append(errs, err)
			// prepareProvider can leave a queue with no workers on failure. Remove
			// it so late callers are failed rather than left waiting on that queue.
			if cleanupErr := e.client.RemoveProvider(name); cleanupErr != nil {
				errs = append(errs, cleanupErr)
			}
		} else {
			old.Providers[name] = cfg.Providers[name]
			e.appliedProxy[name] = proxy
			e.account.transitioning.Delete(name)
		}
	}
	for name := range retiring {
		_, isBlocked := blocked[name]
		if _, exists := cfg.Providers[name]; !exists && !isBlocked {
			delete(old.Providers, name)
			delete(e.appliedProxy, name)
			e.account.transitioning.Delete(name)
		}
	}

	for id, rule := range cfg.Rules {
		if prev, ok := old.Rules[id]; !ok || !reflect.DeepEqual(prev, rule) {
			if err := e.routing.UpsertRule(ctx, &rule); err != nil {
				errs = append(errs, err)
			} else {
				old.Rules[id] = rule
			}
		}
	}
	for id := range old.Rules {
		if _, ok := cfg.Rules[id]; !ok {
			if err := e.routing.DeleteRule(ctx, id); err != nil {
				errs = append(errs, err)
			} else {
				delete(old.Rules, id)
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

// Shutdown starts shutdown and waits for completion or ctx cancellation.
// Cancellation bounds only the caller's wait: cleanup continues in order because
// Bifrost provider removal is not cancellable. Later calls join the same shutdown.
// The caller must stop serving requests before shutting down the engine.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.stopOnce.Do(func() {
		e.shutdownDone = make(chan struct{})
		go func() {
			defer close(e.shutdownDone)
			e.syncLock.Lock()
			defer e.syncLock.Unlock()
			e.stopped = true
			// Join removals before core shutdown touches the same queues and workers.
			for _, drain := range e.retirements {
				<-drain.done
			}
			e.client.Shutdown()
			e.accumulator.Cleanup()
			e.tracer.Stop()
			e.traceStore.Stop()
		}()
	})
	select {
	case <-e.shutdownDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	// Saturate before shifting so an oversized admin value cannot wrap to a
	// negative or unexpectedly small request limit.
	if settings.Max > math.MaxInt64>>20 {
		return math.MaxInt64
	}
	return settings.Max << 20
}

// Setting returns an immutable copy of a setting from the loaded configuration.
func (e *Engine) Setting(name string) json.RawMessage {
	return slices.Clone(e.account.config.Load().Settings[name])
}

// Input hooks see the original request before routing; output hooks run last in the list.
func orderedPlugins(routing schemas.LLMPlugin, plugins []schemas.LLMPlugin) []schemas.LLMPlugin {
	if len(plugins) == 0 {
		return []schemas.LLMPlugin{routing}
	}
	return append([]schemas.LLMPlugin{plugins[0], routing}, plugins[1:]...)
}
