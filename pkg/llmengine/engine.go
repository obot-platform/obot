// Package llmengine integrates the Bifrost core gateway with Obot's lifecycle and models.
package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bifrostcore "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata"
)

const (
	Version                    = "obot-core"
	defaultMaxRequestBodyBytes = 100 << 20
)

type Options struct {
	Store                   *llmcoreclient.Client
	DataDir                 string
	LogLevel                string
	FirstPlugin, LastPlugin schemas.LLMPlugin
	ClaudeSubscriptions     func(context.Context) (bool, error)
	ModelDataURL            string
}

type syncFailure struct{ err error }

type Engine struct {
	syncFailure      atomic.Pointer[syncFailure]
	metrics          *gatewayMetrics
	availability     *modelAvailability
	core             atomic.Pointer[llmcore.Engine]
	store            *llmcoreclient.Client
	prices           *modelcatalog.ModelCatalog
	client           *http.Client
	baseURL          *url.URL
	ready            chan struct{}
	startErr         error
	startOnce        sync.Once
	stopOnce         sync.Once
	syncMu           sync.Mutex
	appliedPricing   json.RawMessage
	pricingApplied   bool
	opts             Options
	modelsLock       sync.Mutex
	lastModels       map[string][]listedModel
	lastHasProviders bool
	catalog          func() modelCatalog
}

type APIError struct {
	StatusCode int
	Message    string
}

func New() *Engine {
	e := &Engine{metrics: newGatewayMetrics(), availability: &modelAvailability{unavailable: map[string]bool{}}, ready: make(chan struct{}), baseURL: &url.URL{Scheme: "http", Host: "obot.internal"}}
	e.catalog = func() modelCatalog {
		if e.prices == nil {
			return nil
		}
		return e.prices
	}
	e.client = &http.Client{Transport: handlerTransport{handler: e}}
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
		plugins = append(plugins, e.metrics, e.availability)
		if opts.LastPlugin != nil {
			plugins = append(plugins, opts.LastPlugin)
		}
		engine, err := llmcore.New(ctx, opts.Store, llmcore.Options{Plugins: plugins, Logger: logger, Catalog: e.prices})
		if err != nil {
			_ = e.prices.Cleanup()
			e.startErr = err
			return
		}
		e.core.Store(engine)
		if err = e.syncPricing(ctx); err != nil {
			e.syncFailure.Store(&syncFailure{err: err})
			slog.Warn("LLM gateway pricing application will retry", "error", err)
		}
		if opts.ModelDataURL != "" {
			go modeldata.Refresh(ctx, dir, opts.ModelDataURL, modeldata.RefreshInterval)
		}
		go func() { <-ctx.Done(); e.Shutdown() }()
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
	err = errors.Join(err, e.syncPricing(ctx))
	if err != nil {
		e.syncFailure.Store(&syncFailure{err: err})
	} else {
		e.syncFailure.Store(nil)
	}
	return changed, err
}
func (e *Engine) syncPricing(_ context.Context) error {
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
func (e *Engine) Shutdown() {
	e.stopOnce.Do(func() {
		if engine := e.core.Load(); engine != nil {
			engine.Shutdown()
		}
		if e.prices != nil {
			_ = e.prices.Cleanup()
		}
	})
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
	if strings.HasPrefix(r.URL.Path, "/api/") {
		e.serveAdmin(w, r)
		return
	}
	if r.URL.Path == "/metrics" {
		e.serveMetrics(w, r)
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

func (e *Engine) Transport() http.RoundTripper { return e.client.Transport }
func (e *Engine) BaseURL() url.URL             { return *e.baseURL }
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
	if e.opts.FirstPlugin != nil {
		names = append(names, "obot-routing", "obot-metrics")
	} else {
		names = append(names, "obot-metrics", "obot-routing")
	}
	if e.opts.LastPlugin != nil {
		names = append(names, e.opts.LastPlugin.GetName())
	}
	return names
}

// Do sends a request to the embedded server's API and decodes a JSON response into out. A
// write the store refused because it was busy is sent again, for up to 30 seconds (see
// IsStoreBusy).
func (e *Engine) Do(ctx context.Context, method, path string, body any, out any) error {
	deadline := time.Now().Add(30 * time.Second)
	for delay := 100 * time.Millisecond; ; delay = min(delay*2, 2*time.Second) {
		err := e.do(ctx, method, path, body, out)
		// Internal import/configuration callers may use a provider immediately after a write.
		// Public HTTP writes acknowledge persistence; every-replica controllers apply them.
		if err == nil && method != http.MethodGet && strings.HasPrefix(path, "/api/") && ctx.Value(configContextKey{}) == nil {
			if _, syncErr := e.Sync(ctx); syncErr != nil {
				slog.Warn("gateway configuration saved; reconciliation will retry", "error", syncErr)
			}
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) || !IsStoreBusy(apiErr.StatusCode, []byte(apiErr.Message)) || time.Now().Add(delay).After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
	}
}

func (e *Engine) do(ctx context.Context, method, path string, body any, out any) error {
	if !e.Ready() {
		if err := e.Err(); err != nil {
			return fmt.Errorf("LLM gateway failed to start: %w", err)
		}
		return errors.New("LLM gateway is starting")
	}

	var reader *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(data))
	} else {
		reader = strings.NewReader("")
	}

	u := e.BaseURL()
	u.Path = path
	if p, q, ok := strings.Cut(path, "?"); ok {
		u.Path = p
		u.RawQuery = q
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		msg := errBody.Error.Message
		if msg == "" {
			msg = errBody.Message
		}
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (e *APIError) Error() string {
	return fmt.Sprintf("LLM gateway returned %d: %s", e.StatusCode, e.Message)
}
