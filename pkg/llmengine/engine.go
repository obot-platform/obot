// Package llmengine adapts llmcore to Obot's services and administrative API.
// It owns model catalog refresh, legacy imports, and admin writes.
// Inference, routing, and runtime configuration reconciliation belong to llmcore;
// the catalog and plugins are injected so llmcore does not depend on Obot's service lifecycle.
package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	bifrostcore "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	Version                    = "obot-core"
	defaultMaxRequestBodyBytes = 100 << 20
)

type Options struct {
	Store                   *llmcoreclient.Client
	TriggerModelSync        func(context.Context) error
	DataDir                 string
	LogLevel                string
	FirstPlugin, LastPlugin schemas.LLMPlugin
	ClaudeSubscriptions     func(context.Context) (bool, error)
	ModelDataURL            string
}

type syncFailure struct{ err error }

type Engine struct {
	metrics          *gatewayMetrics
	syncFailure      atomic.Pointer[syncFailure]
	discovery        azureDiscovery
	availability     *modelAvailability
	core             atomic.Pointer[llmcore.Engine]
	store            *llmcoreclient.Client
	prices           *modelcatalog.ModelCatalog
	ready            chan struct{}
	startErr         error
	startOnce        sync.Once
	stopOnce         sync.Once
	shutdownDone     chan struct{}
	syncMu           sync.Mutex
	appliedPricing   json.RawMessage
	pricingApplied   bool
	opts             Options
	modelsLock       sync.Mutex
	lastModels       map[string][]listedModel
	lastHasProviders bool
	catalog          func() modelCatalog
}

func New() *Engine {
	e := &Engine{
		metrics:      newGatewayMetrics(),
		availability: &modelAvailability{unavailable: map[string]bool{}},
		ready:        make(chan struct{}),
	}

	e.availability.engine = e
	e.catalog = func() modelCatalog {
		if e.prices == nil {
			return nil
		}
		return e.prices
	}
	return e
}

func (e *Engine) Fail(err error) { e.startOnce.Do(func() { e.startErr = err; close(e.ready) }) }

func (e *Engine) Start(ctx context.Context, opts Options) error {
	e.startOnce.Do(func() {
		defer close(e.ready)
		e.opts, e.store = opts, opts.Store
		if opts.Store == nil {
			e.startErr = errors.New("LLM gateway requires Obot's configuration store")
			return
		}
		dir, err := filepath.Abs(opts.DataDir)
		if err != nil {
			e.startErr = err
			return
		}
		if err = modeldata.Prepare(ctx, dir, opts.ModelDataURL); err != nil {
			e.startErr = err
			return
		}
		pricing, params := modeldata.Paths(dir)
		pricingURL, paramsURL := modeldata.FileURL(pricing), modeldata.FileURL(params)
		interval, disabled := int64(3600), int64(0)
		level := schemas.LogLevel(strings.ToLower(opts.LogLevel))
		if level == "" {
			level = schemas.LogLevelWarn
		}
		logger := newEngineLogger(bifrostcore.NewDefaultLogger(level))
		e.prices, err = modelcatalog.Init(ctx, &modelcatalog.Config{PricingURL: &pricingURL, ModelParametersURL: &paramsURL, PricingSyncInterval: &interval, MCPLibrarySyncInterval: &disabled}, nil, logger)
		if err != nil {
			e.startErr = err
			return
		}
		plugins := []schemas.LLMPlugin{}
		if opts.FirstPlugin != nil {
			plugins = append(plugins, opts.FirstPlugin)
		}
		plugins = append(plugins, e.availability)
		if opts.LastPlugin != nil {
			plugins = append(plugins, opts.LastPlugin)
		}
		// Response hooks run in reverse: record billing before output guardrails discard responses.
		plugins = append(plugins, e.metrics)
		// Plugins own their counters; expose collectors through the gateway's replica-local registry.
		for _, plugin := range plugins {
			if collector, ok := plugin.(prometheus.Collector); ok {
				e.metrics.registry.MustRegister(collector)
			}
		}
		engine, err := llmcore.New(ctx, opts.Store, llmcore.Options{Plugins: plugins, Logger: logger, Catalog: e.prices})
		if err != nil {
			_ = e.prices.Cleanup()
			e.startErr = err
			return
		}
		// Startup may serve healthy providers while others need reconciliation. Keep
		// those diagnostics visible before the first controller event, and serialize
		// pricing initialization with any concurrent model-discovery synchronization.
		e.syncMu.Lock()
		e.core.Store(engine)
		if err = errors.Join(engine.Err(), e.syncPricing()); err != nil {
			e.syncFailure.Store(&syncFailure{err: err})
			slog.Warn("LLM gateway configuration application will retry", "error", err)
		}
		e.syncMu.Unlock()
		if opts.ModelDataURL != "" {
			go modeldata.Refresh(ctx, dir, opts.ModelDataURL, modeldata.RefreshInterval)
		}
		// The application joins cleanup with its shared shutdown deadline. Cancellation
		// still starts cleanup for standalone callers, without a separate wait budget.
		context.AfterFunc(ctx, func() { _ = e.Shutdown(ctx) })
	})
	return e.startErr
}

func (e *Engine) Sync(ctx context.Context) (bool, error) {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	engine := e.core.Load()
	if engine == nil {
		return false, nil
	}
	changed, err := engine.Sync(ctx)
	err = errors.Join(err, e.syncPricing())
	if err != nil {
		e.syncFailure.Store(&syncFailure{err: err})
	} else {
		e.syncFailure.Store(nil)
	}
	return changed, err
}

func (e *Engine) syncPricing() error {
	engine := e.core.Load()
	raw := engine.Setting("pricing-overrides")
	if e.pricingApplied && bytes.Equal(raw, e.appliedPricing) {
		return nil
	}
	var rows []configPricingOverride
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
	}
	if err := e.prices.SetPricingOverrides(rows); err != nil {
		return err
	}
	e.appliedPricing = bytes.Clone(raw)
	e.pricingApplied = true
	return nil
}

// Shutdown bounds the caller's wait while core cleanup continues before the catalog stops.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.stopOnce.Do(func() {
		e.shutdownDone = make(chan struct{})
		go func() {
			defer close(e.shutdownDone)
			// Application shutdown can race background startup; join it before reading
			// the core/catalog, and leave failed-start cleanup to Start.
			<-e.ready
			if e.startErr != nil {
				return
			}
			if engine := e.core.Load(); engine != nil {
				_ = engine.Shutdown(context.Background())
			}
			if e.prices != nil {
				_ = e.prices.Cleanup()
			}
		}()
	})
	select {
	case <-e.shutdownDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) Wait(ctx context.Context) error {
	select {
	case <-e.ready:
		return e.startErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) Ready() bool {
	select {
	case <-e.ready:
		return e.startErr == nil
	default:
		return false
	}
}

func (e *Engine) Err() error {
	select {
	case <-e.ready:
		if e.startErr != nil {
			return e.startErr
		}
		if failure := e.syncFailure.Load(); failure != nil {
			return failure.err
		}
		if core := e.core.Load(); core != nil {
			return core.Err()
		}
		return nil
	default:
		return nil
	}
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !e.Ready() {
		http.Error(w, "LLM gateway unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/anthropic/v1/models") {
		e.serveModels(w, r)
		return
	}
	e.core.Load().ServeHTTP(w, r)
}

func (e *Engine) MaxRequestBodyBytes() int64 {
	if engine := e.core.Load(); engine != nil {
		return engine.MaxRequestBodyBytes()
	}
	return defaultMaxRequestBodyBytes
}

func (e *Engine) Client() *bifrostcore.Bifrost {
	if !e.Ready() {
		return nil
	}
	return e.core.Load().Client()
}

func (e *Engine) LLMPluginOrder() []string {
	if !e.Ready() {
		return nil
	}
	names := []string{}
	if e.opts.FirstPlugin != nil {
		names = append(names, e.opts.FirstPlugin.GetName())
	}
	names = append(names, "obot-routing")
	if e.opts.LastPlugin != nil {
		names = append(names, e.opts.LastPlugin.GetName())
	}
	return names
}

// ChatCompletion runs trusted message-policy inference through the configured core.
func (e *Engine) ChatCompletion(ctx context.Context, model string, messages []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
	if !e.Ready() {
		return nil, errors.New("LLM gateway unavailable")
	}
	return e.core.Load().ChatCompletion(ctx, model, messages)
}
