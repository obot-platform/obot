// Package bifrost embeds the Bifrost AI gateway (github.com/maximhq/bifrost) inside the
// Obot process and exposes it to the rest of Obot as an http.Handler.
//
// Bifrost runs its full HTTP transport (inference routes, governance, logging, telemetry,
// routing and caching plugins) in-process. It never listens on a network socket: it is
// served over an in-memory listener, so the only way to reach it is through Obot, which
// authenticates callers and applies its own policies before forwarding requests.
package llmengine

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/obot-platform/obot/pkg/modeldata"

	bifrostcore "github.com/maximhq/bifrost/core"
	providerutils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/transports/bifrost-http/handlers"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
	bifrostserver "github.com/maximhq/bifrost/transports/bifrost-http/server"
	"github.com/valyala/fasthttp/fasthttputil"
)

const (
	// retireGracePeriod is how long a gateway replaced by a reload keeps serving the
	// requests it already had, such as long streams, before it stops.
	retireGracePeriod = 15 * time.Minute

	// Version is reported to Bifrost as its runtime version.
	Version = "obot-embedded"

	// internalHost is the synthetic host used for requests to the embedded server. It is
	// never resolved; the transport dials the in-memory listener directly.
	internalHost = "bifrost.internal"

	// pricingSyncIntervalSeconds is how often Bifrost re-reads the model data files (see
	// pkg/modeldata): its minimum.
	pricingSyncIntervalSeconds = 3600
	// setupTokenHeader is the header Bifrost reads its setup token from.
	setupTokenHeader = "X-Bifrost-Setup-Token"
	// defaultMaxRequestBodyBytes is the engine's request size limit when none is configured.
	defaultMaxRequestBodyBytes = 100 << 20
)

// Options configures the embedded engine.
type Options struct {
	// DataDir holds the generated config.json and, when no Postgres DSN is supplied, the
	// SQLite config and log stores.
	DataDir string
	// PostgresDSN, when set, is used for Bifrost's config and log stores instead of SQLite.
	// It must point to a database that is not shared with Obot,
	// because Bifrost's table names overlap with Obot's.
	PostgresDSN string
	// EncryptionKey encrypts provider keys at rest. When empty, the key comes from
	// SharedKeys if set, and is otherwise generated and persisted in DataDir.
	EncryptionKey string
	// SharedKeys stores the encryption key where every replica can read it. Set it whenever
	// the stores are shared (Postgres), so all replicas encrypt with the same key.
	SharedKeys SharedKeyStore
	// LogLevel is the Bifrost logger level (debug, info, warn, error).
	LogLevel string
	// FirstPlugin and LastPlugin, when set, are registered before the gateway serves any
	// request (see registerLLMPlugins). The gateway fails to start if they can't be, so it
	// never serves traffic without them.
	FirstPlugin, LastPlugin schemas.LLMPlugin
	// CurrentRevision, when set, returns the shared configuration revision. The engine reads
	// it before it boots, so it knows it already has everything up to that revision and
	// doesn't boot a second time to catch up.
	CurrentRevision func(context.Context) (int64, error)
	// WithLock, when set, runs fn while holding the lock every replica takes to change the
	// gateway's configuration (see gateway.Client.WithLLMGatewayLock). The engine boots while
	// holding it: booting writes every provider it read back to the shared stores, and on
	// empty stores the engine configures providers from the environment on its own, which
	// Obot then removes (see removeAutoDetectedKeys). Without the lock, those writes could
	// undo or remove what another replica configures meanwhile.
	WithLock func(ctx context.Context, fn func() error) error
	// ClaudeSubscriptions, when set, reports whether Claude Code users may use their own
	// Claude subscriptions through the gateway. While they may, an Anthropic provider without
	// keys lists its models from the gateway's model catalog (see ListModels).
	ClaudeSubscriptions func(context.Context) (bool, error)
	// ModelDataURL is where model pricing and capabilities are refreshed from (see
	// modeldata.go). Empty leaves the snapshot bundled with Obot.
	ModelDataURL string
}

// SharedKeyStore stores the gateway's encryption key for every replica.
type SharedKeyStore interface {
	// EnsureEncryptionKey stores key unless a key is already stored, and returns the
	// stored key.
	EnsureEncryptionKey(key string) (string, error)
}

// Engine is a running, embedded Bifrost gateway.
type Engine struct {
	// inst is the running gateway. A reload replaces it with a new one built from the shared
	// stores; the old one finishes its in-flight requests and then stops.
	inst      atomic.Pointer[instance]
	transport *http.Transport
	client    *http.Client
	proxy     *httputil.ReverseProxy
	baseURL   *url.URL
	// setupToken unlocks Bifrost's management API, which is locked until its own admin login
	// is set up; Obot never sets that login up. Only Obot's in-process calls send it.
	setupToken string

	ready     chan struct{}
	startErr  error
	startOnce sync.Once

	// runCtx and opts are what the engine was started with, for reloads.
	runCtx context.Context
	opts   Options
	// reloadLock serializes reloads; appliedRevision is the latest configuration revision
	// this engine has loaded, and pendingRevision one that arrived while it was starting.
	// configLock serializes reloads with configuration changes made through this engine,
	// so a reload never boots from the stores while a change is half-written.
	configLock      sync.Mutex
	appliedRevision atomic.Int64
	// pendingLock guards the hand-off of a revision that arrives while the engine starts.
	pendingLock     sync.Mutex
	pendingRevision int64

	// Last successful model listing per upstream provider, and the last provider check.
	modelsLock       sync.Mutex
	lastModels       map[string][]listedModel
	lastHasProviders bool

	// catalog returns the running gateway's model catalog; tests replace it.
	catalog func() modelCatalog
}

// setupTokenTransport sends the setup token that unlocks Bifrost's management API with every
// request Obot makes to the embedded server. It replaces any token a caller sent.
type setupTokenTransport struct {
	base  http.RoundTripper
	token string
}

// APIError is returned by Do for non-2xx responses from Bifrost.
type APIError struct {
	StatusCode int
	Message    string
}

// instance is one running Bifrost server and the in-memory listener it serves.
type instance struct {
	server   *bifrostserver.BifrostHTTPServer
	listener *fasthttputil.InmemoryListener
	// cancel ends the context the server was bootstrapped with, which stops the background
	// work tied to it (model refresh, sweeps).
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (t setupTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(setupTokenHeader, t.token)
	return t.base.RoundTrip(req)
}

// New creates an engine. Call Start to boot it.
func New() *Engine {
	e := &Engine{
		ready: make(chan struct{}),
	}
	e.catalog = func() modelCatalog {
		if cur := e.inst.Load(); cur != nil && cur.server.Config != nil && cur.server.Config.ModelCatalog != nil {
			return cur.server.Config.ModelCatalog
		}
		return nil
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// The in-memory dial blocks until the server accepts, so refuse to dial a server
			// that isn't serving rather than hang the caller.
			if !e.Ready() {
				if err := e.Err(); err != nil {
					return nil, fmt.Errorf("LLM gateway failed to start: %w", err)
				}
				return nil, errors.New("LLM gateway is starting")
			}
			type result struct {
				conn net.Conn
				err  error
			}
			ln := e.inst.Load().listener
			done := make(chan result, 1)
			go func() {
				conn, err := ln.DialWithLocalAddr(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})
				done <- result{conn, err}
			}()
			select {
			case r := <-done:
				return r.conn, r.err
			case <-ctx.Done():
				go func() {
					if r := <-done; r.conn != nil {
						_ = r.conn.Close()
					}
				}()
				return nil, ctx.Err()
			}
		},
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		// Streaming responses must not be buffered or transparently decompressed.
		DisableCompression: true,
		ForceAttemptHTTP2:  false,
	}

	baseURL := &url.URL{Scheme: "http", Host: internalHost}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		panic(fmt.Sprintf("failed to generate the LLM gateway setup token: %v", err))
	}
	e.setupToken = hex.EncodeToString(token)
	authenticated := setupTokenTransport{base: transport, token: e.setupToken}
	e.transport = transport
	e.client = &http.Client{Transport: authenticated}
	e.baseURL = baseURL
	e.proxy = &httputil.ReverseProxy{
		Transport:     authenticated,
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = baseURL.Scheme
			r.Out.URL.Host = baseURL.Host
			r.Out.Host = baseURL.Host
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			slog.Warn("LLM gateway proxy error", "error", err)
			http.Error(w, "LLM gateway unavailable: "+err.Error(), http.StatusBadGateway)
		},
	}
	return e
}

// Fail marks the engine as failed to start, with err as the reason, when it can't be started
// at all. The gateway then answers with that error instead of serving.
func (e *Engine) Fail(err error) {
	e.startOnce.Do(func() {
		e.startErr = err
		slog.Error("failed to start the LLM gateway", "error", err)
		close(e.ready)
	})
}

// Start boots Bifrost in the background. It returns once bootstrap completes or fails.
func (e *Engine) Start(ctx context.Context, opts Options) error {
	e.startOnce.Do(func() {
		e.runCtx, e.opts = ctx, opts
		if opts.ModelDataURL != "" {
			go modeldata.Refresh(ctx, opts.DataDir, opts.ModelDataURL, modeldata.RefreshInterval)
		}
		started := false
		start := func() error {
			started = true
			e.start(ctx, opts)
			return nil
		}
		if opts.WithLock == nil {
			_ = start()
		} else if err := opts.WithLock(ctx, start); err != nil && !started {
			// Starting matters more than the lock: start anyway, as before replicas shared it.
			slog.Warn("failed to take the LLM gateway configuration lock; starting the LLM gateway without it", "error", err)
			_ = start()
		}
		if e.startErr == nil {
			go func() {
				<-ctx.Done()
				e.Shutdown()
			}()
		}
	})
	return e.startErr
}

// start boots the gateway and marks the engine ready, or failed. It runs while holding
// opts.WithLock, when set.
func (e *Engine) start(ctx context.Context, opts Options) {
	// Read before booting: the stores then hold at least this revision's configuration.
	var booted int64
	if opts.CurrentRevision != nil {
		revCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var err error
		if booted, err = opts.CurrentRevision(revCtx); err != nil {
			// Without it, the first reload catches up instead.
			slog.Warn("failed to read the LLM gateway configuration revision before starting", "error", err)
			booted = 0
		}
		cancel()
	}
	inst, err := e.boot(ctx, opts)
	if err != nil {
		e.startErr = err
		slog.Error("failed to start the LLM gateway", "error", err)
		close(e.ready)
		return
	}
	e.raiseApplied(booted)
	e.pendingLock.Lock()
	e.inst.Store(inst)
	close(e.ready)
	rev := e.pendingRevision
	e.pendingLock.Unlock()
	// The stores are exactly what this engine booted from, since every replica changes them
	// only while holding the lock this runs under.
	e.removeAutoDetectedKeys(ctx)
	// A configuration change made elsewhere while this engine was starting may not be
	// in what it loaded.
	if rev > 0 {
		if _, err := e.Reload(rev); err != nil {
			slog.Error("failed to reload the LLM gateway configuration", "revision", rev, "error", err)
		}
	}
}

// Change makes a configuration change through this engine and tells the other replicas.
// change applies it and reports whether anything changed, and whether this engine is sure to
// have the change in memory (false when, say, it failed partway); announce then advances the shared
// configuration revision, and returns it. Reloads wait while a change is made, so this
// engine has exactly the configuration up to the new revision when it had the one before;
// otherwise it reloads to pick up what it missed. An error from announce is returned: the
// change is saved, but other replicas only see it after a restart or a later change.
func (e *Engine) Change(ctx context.Context, change func() (changed, applied bool), announce func(context.Context) (int64, error)) error {
	e.configLock.Lock()
	defer e.configLock.Unlock()
	changed, applied := change()
	if !changed || announce == nil {
		return nil
	}
	// The change is saved; announce it even if the caller has gone away, so other replicas
	// don't keep serving the old configuration.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var (
		revision int64
		err      error
	)
	for attempt := range 3 {
		if revision, err = announce(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	if err != nil {
		return fmt.Errorf("the change was saved, but other Obot replicas could not be told to reload it: %w", err)
	}
	if applied {
		e.appliedRevision.CompareAndSwap(revision-1, revision)
	}
	return nil
}

// Reload rebuilds the gateway from its stores when revision is newer than what it has
// loaded. Bifrost loads its configuration into memory at startup and only updates that
// copy for changes made through it, so with several replicas each must reload when another
// changes the configuration. New requests go to the rebuilt gateway at once; requests
// already in flight finish on the old one. It reports whether it reloaded. Callers hold the
// lock every replica takes to change the gateway's configuration (see Options.WithLock),
// since booting writes the configuration it read back to the shared stores.
func (e *Engine) Reload(revision int64) (bool, error) {
	if revision <= e.appliedRevision.Load() {
		return false, nil
	}
	e.pendingLock.Lock()
	if !e.Ready() {
		if e.Err() == nil {
			// Still starting: reload once started.
			e.pendingRevision = max(e.pendingRevision, revision)
		}
		e.pendingLock.Unlock()
		return false, nil
	}
	e.pendingLock.Unlock()

	e.configLock.Lock()
	defer e.configLock.Unlock()
	if revision <= e.appliedRevision.Load() {
		return false, nil
	}
	start := time.Now()
	next, err := e.boot(e.runCtx, e.opts)
	if err != nil {
		// A failed boot clears Bifrost's process-wide capability lookup while releasing
		// what it started; the running gateway still needs it.
		e.restoreCapabilityResolver()
		return false, err
	}
	old := e.inst.Swap(next)
	// Pooled connections lead to the old gateway; new requests must open new ones.
	e.transport.CloseIdleConnections()
	// A gateway that booted with no providers configured some from the environment.
	e.removeAutoDetectedKeys(e.runCtx)
	// The rebuilt gateway has everything in the stores, so everything up to revision.
	e.raiseApplied(revision)
	go func() {
		retire(old)
		e.restoreCapabilityResolver()
	}()
	slog.Info("reloaded the LLM gateway configuration", "revision", revision, "duration", time.Since(start))
	return true, nil
}

func (e *Engine) raiseApplied(revision int64) {
	for {
		cur := e.appliedRevision.Load()
		if cur >= revision || e.appliedRevision.CompareAndSwap(cur, revision) {
			return
		}
	}
}

// retire stops a replaced gateway once its in-flight requests finish, or after a grace
// period for long-running streams, and then releases everything it holds.
func retire(old *instance) {
	if old == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), retireGracePeriod)
	defer cancel()
	if old.server.Server != nil {
		_ = old.server.Server.ShutdownWithContext(ctx)
	}
	_ = old.listener.Close()
	old.close()
}

// restoreCapabilityResolver re-installs the current gateway's model capability lookup.
// Bifrost keeps it in a process-wide variable that each gateway sets when it starts and
// clears when it stops, so stopping a replaced gateway would otherwise clear the lookup the
// running one installed.
func (e *Engine) restoreCapabilityResolver() {
	if cur := e.inst.Load(); cur != nil && cur.server.Config != nil && cur.server.Config.ModelCatalog != nil {
		providerutils.SetCapabilityResolver(cur.server.Config.ModelCatalog.GetModelCapabilities)
	}
}

// close releases a server's background work and stores, following the shutdown sequence
// Bifrost's own Start runs on a termination signal. Bifrost has no exported way to run it
// without a signal, so it is repeated here; the server context's cancellation stops the
// workers whose stop functions aren't exported.
func (i *instance) close() {
	i.closeOnce.Do(i.release)
}

func (i *instance) release() {
	s := i.server
	if s.IntegrationHandler != nil {
		s.IntegrationHandler.Close()
	}
	i.cancel()
	// Warp's indexer workers use the client, so it stops first, as in Bifrost's own shutdown.
	if s.WarpHandler != nil {
		s.WarpHandler.Shutdown()
	}
	s.CloseAgentGateway()
	if s.Client != nil {
		s.Client.Shutdown()
	}
	if s.GovernanceHandler != nil {
		s.GovernanceHandler.StopExpiryCleanupScheduler()
	}
	// TODO: release Bifrost's upstream WebSocket connection pool too. Its eviction and
	// heartbeat loops (2 goroutines per reload) only stop in Bifrost's signal-driven
	// shutdown, through an unexported field (BifrostHTTPServer.wsPool), so every reload
	// leaks them until Bifrost exposes a way to close it.
	if s.LogsCleaner != nil {
		s.LogsCleaner.StopCleanupRoutine()
	}
	if s.AsyncJobCleaner != nil {
		s.AsyncJobCleaner.StopCleanupRoutine()
	}
	if s.WebhookDispatcher != nil {
		s.WebhookDispatcher.Stop()
	}
	if s.WSTicketStore != nil {
		s.WSTicketStore.Stop()
	}
	if s.TempTokenSweepWorker != nil {
		s.TempTokenSweepWorker.Stop()
	}
	if s.SidekiqDispatcherStop != nil {
		s.SidekiqDispatcherStop()
	}
	if s.SidekiqRunner != nil {
		s.SidekiqRunner.Shutdown()
	}
	if s.Config != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.Config.Close(ctx)
	}
}

// boot starts a Bifrost server from the stores and the generated configuration file.
func (e *Engine) boot(ctx context.Context, opts Options) (*instance, error) {
	ctx, cancel := context.WithCancel(ctx)
	inst, err := e.bootServer(ctx, opts)
	if err != nil {
		if inst != nil {
			inst.cancel = cancel
			_ = inst.listener.Close()
			inst.close()
		} else {
			cancel()
		}
		return nil, err
	}
	inst.cancel = cancel
	return inst, nil
}

func (e *Engine) bootServer(ctx context.Context, opts Options) (*instance, error) {
	if opts.DataDir == "" {
		return nil, errors.New("LLM gateway data dir is required")
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create the LLM gateway data dir: %w", err)
	}

	encryptionKey, err := resolveEncryptionKey(opts)
	if err != nil {
		return nil, err
	}
	if err := modeldata.Prepare(ctx, opts.DataDir, opts.ModelDataURL); err != nil {
		return nil, err
	}

	// The file is the same every time for the same options, so Bifrost's hash-based
	// reconciliation keeps settings saved through its API rather than reapplying the file.
	cfg, err := buildConfigFile(opts, encryptionKey, e.setupToken)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(opts.DataDir, "config.json"), cfg, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write the LLM gateway config: %w", err)
	}

	level := schemas.LogLevel(strings.ToLower(opts.LogLevel))
	if level == "" {
		level = schemas.LogLevelWarn
	}
	base := bifrostcore.NewDefaultLogger(level)
	base.SetOutputType(schemas.LoggerOutputTypeJSON)
	logger := newEngineLogger(base)
	lib.SetLogger(logger)
	bifrostserver.SetLogger(logger)
	handlers.SetLogger(logger)

	server := bifrostserver.NewBifrostHTTPServer(Version, embed.FS{})
	server.AppDir = opts.DataDir
	server.Host = "127.0.0.1"
	server.Port = "0"
	server.LogLevel = string(level)
	server.LogOutputStyle = string(schemas.LoggerOutputTypeJSON)

	start := time.Now()
	ln := fasthttputil.NewInmemoryListener()
	ln.SetLocalAddr(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80})
	if err := server.Bootstrap(ctx); err != nil {
		// Bootstrap may have started work before failing; release it.
		return &instance{server: server, listener: ln}, fmt.Errorf("failed to start the LLM gateway: %w", err)
	}
	inst := &instance{server: server, listener: ln}
	// Obot enforces the request size limit itself, from the live setting (see
	// MaxRequestBodyBytes), before a request reaches the engine. The engine's HTTP server
	// takes the setting only when it starts, so after the limit is raised it would refuse a
	// body Obot let through, with a plain-text error, until the next reload.
	server.Server.MaxRequestBodySize = math.MaxInt

	if opts.FirstPlugin != nil && opts.LastPlugin != nil {
		if err := registerLLMPlugins(server, opts.FirstPlugin, opts.LastPlugin); err != nil {
			return inst, err
		}
	}

	go func() {
		if err := server.Server.Serve(ln); err != nil && !errors.Is(err, fasthttputil.ErrInmemoryListenerClosed) {
			slog.Error("LLM gateway server stopped", "error", err)
		}
	}()

	slog.Info("LLM gateway started", "dataDir", opts.DataDir, "duration", time.Since(start))
	return inst, nil
}

// Shutdown stops the embedded server.
func (e *Engine) Shutdown() {
	inst := e.inst.Load()
	if inst == nil {
		return
	}
	if inst.server.Server != nil {
		_ = inst.server.Server.Shutdown()
	}
	_ = inst.listener.Close()
	inst.close()
}

// Wait blocks until the engine has finished starting (successfully or not).
func (e *Engine) Wait(ctx context.Context) error {
	select {
	case <-e.ready:
		return e.startErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Ready reports whether the engine has started successfully.
func (e *Engine) Ready() bool {
	select {
	case <-e.ready:
		return e.startErr == nil
	default:
		return false
	}
}

// Err returns the startup error, if any.
func (e *Engine) Err() error {
	select {
	case <-e.ready:
		return e.startErr
	default:
		return nil
	}
}

// ServeHTTP forwards the request to the embedded Bifrost server unchanged. Callers are
// responsible for authenticating the request and rewriting the path to a Bifrost route.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !e.Ready() {
		msg := "LLM gateway is starting"
		if err := e.Err(); err != nil {
			msg = "LLM gateway failed to start: " + err.Error()
		}
		http.Error(w, msg, http.StatusServiceUnavailable)
		return
	}
	e.proxy.ServeHTTP(w, r)
}

// MaxRequestBodyBytes is the largest request body the gateway accepts, as configured in its
// settings (max_request_body_size_mb). Obot enforces it before forwarding a request, so a
// request over it gets a proper error, and a change takes effect at once.
func (e *Engine) MaxRequestBodyBytes() int64 {
	if !e.Ready() {
		return defaultMaxRequestBodyBytes
	}
	cfg := e.inst.Load().server.Config
	if cfg == nil {
		return defaultMaxRequestBodyBytes
	}
	cfg.Mu.RLock()
	defer cfg.Mu.RUnlock()
	if cfg.ClientConfig == nil || cfg.ClientConfig.MaxRequestBodySizeMB <= 0 {
		return defaultMaxRequestBodyBytes
	}
	return int64(cfg.ClientConfig.MaxRequestBodySizeMB) << 20
}

// Transport returns a RoundTripper that sends requests to the embedded server regardless of
// the request's host.
func (e *Engine) Transport() http.RoundTripper {
	return e.client.Transport
}

// BaseURL is the URL requests to the embedded server should be addressed to.
func (e *Engine) BaseURL() url.URL {
	return *e.baseURL
}

// Client exposes the embedded Bifrost core client for direct, in-process calls.
func (e *Engine) Client() *bifrostcore.Bifrost {
	if !e.Ready() {
		return nil
	}
	return e.inst.Load().server.Client
}

// Do sends a request to the embedded server's API and decodes a JSON response into out. A
// write the store refused because it was busy is sent again, for up to 30 seconds (see
// IsStoreBusy).
func (e *Engine) Do(ctx context.Context, method, path string, body any, out any) error {
	deadline := time.Now().Add(30 * time.Second)
	for delay := 100 * time.Millisecond; ; delay = min(delay*2, 2*time.Second) {
		err := e.do(ctx, method, path, body, out)
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

func resolveEncryptionKey(opts Options) (string, error) {
	if opts.EncryptionKey != "" {
		return opts.EncryptionKey, nil
	}
	if opts.SharedKeys != nil {
		return sharedEncryptionKey(opts)
	}
	key, err := dataDirEncryptionKey(opts.DataDir)
	if err != nil {
		return "", err
	}
	if key == "" {
		if key, err = newEncryptionKey(); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(opts.DataDir, "encryption.key"), []byte(key), 0o600); err != nil {
		return "", fmt.Errorf("failed to persist the LLM gateway encryption key: %w", err)
	}
	return key, nil
}

// sharedEncryptionKey returns the key every replica shares, stored in Obot's database. The
// first replica to start stores its key: one already in its data dir, so provider keys
// encrypted before the key was shared stay readable, or else a new one.
func sharedEncryptionKey(opts Options) (string, error) {
	candidate, err := dataDirEncryptionKey(opts.DataDir)
	if err != nil {
		return "", err
	}
	if candidate == "" {
		if candidate, err = newEncryptionKey(); err != nil {
			return "", err
		}
	}
	key, err := opts.SharedKeys.EnsureEncryptionKey(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to get the shared LLM gateway encryption key: %w", err)
	}
	if local, _ := dataDirEncryptionKey(opts.DataDir); local != "" && local != key {
		slog.Warn("this replica's LLM gateway encryption key differs from the shared one; provider keys it encrypted can't be read and must be re-entered")
	}
	return key, nil
}

func dataDirEncryptionKey(dataDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "encryption.key"))
	if os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("failed to read the LLM gateway encryption key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func newEncryptionKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildConfigFile(opts Options, encryptionKey, setupToken string) ([]byte, error) {
	storeConfig := func(file string) (map[string]any, error) {
		if opts.PostgresDSN == "" {
			return map[string]any{
				"enabled": true,
				"type":    "sqlite",
				"config":  map[string]any{"path": filepath.Join(opts.DataDir, file)},
			}, nil
		}
		pg, err := postgresConfigFromDSN(opts.PostgresDSN)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"enabled": true,
			"type":    "postgres",
			"config":  pg,
		}, nil
	}

	configStore, err := storeConfig("config.db")
	if err != nil {
		return nil, err
	}
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return nil, err
	}
	pricingPath, paramsPath := modeldata.Paths(dataDir)
	cfg := map[string]any{
		"$schema":        "https://www.getbifrost.ai/schema",
		"encryption_key": encryptionKey,
		// Bifrost's own admin login is never set up (its session API is not exposed through
		// Obot), so its management API stays locked to this token, which only Obot's
		// in-process calls send.
		"setup_token": setupToken,
		"client": map[string]any{
			// Obot keeps its own record of each request (the LLM audit log and token usage),
			// so Bifrost's request log and its store are not used.
			"enable_logging": false,
			// Obot authenticates every request before it reaches Bifrost, and enforces budgets
			// itself, so Bifrost's own inference auth and virtual keys are not used.
			"enforce_auth_on_inference": false,
			"allowed_origins":           []string{"*"},
			"max_request_body_size_mb":  100,
			// The engine drops every request parameter its model-parameters file doesn't list
			// for the model. Obot's file (see modeldata.go) lists models' capabilities but not
			// their parameters, so this would drop max_tokens, temperature, stop and the rest
			// from every request to a known model. Providers reject what a model can't take,
			// and the engine still strips the sampling parameters it knows a model rejects.
			"compat": map[string]any{"should_drop_params": false},
		},
		"config_store": configStore,
		"framework": map[string]any{
			// Model prices and capabilities come from files Obot maintains (see
			// modeldata.go), never from Bifrost's vendor. Bifrost's MCP server library isn't
			// used, so it isn't synced.
			"pricing": map[string]any{
				"pricing_url":               modeldata.FileURL(pricingPath),
				"model_parameters_url":      modeldata.FileURL(paramsPath),
				"pricing_sync_interval":     pricingSyncIntervalSeconds,
				"mcp_library_sync_interval": 0,
			},
		},
		// Warp, Bifrost's dashboard agent, is on by default. Obot doesn't serve it, so turn it
		// off; a flag set here can't be changed through Bifrost's API.
		"feature_flags": map[string]any{
			"flags": map[string]any{
				"warp": map[string]any{"enabled": false},
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func postgresConfigFromDSN(dsn string) (map[string]any, error) {
	u, err := url.Parse(strings.Replace(dsn, "postgresql://", "postgres://", 1))
	if err != nil {
		return nil, fmt.Errorf("invalid LLM gateway postgres DSN: %w", err)
	}
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "prefer"
	}
	return map[string]any{
		"host":           u.Hostname(),
		"port":           port,
		"user":           u.User.Username(),
		"password":       password,
		"db_name":        strings.TrimPrefix(u.Path, "/"),
		"ssl_mode":       sslMode,
		"max_idle_conns": 5,
		"max_open_conns": 20,
	}, nil
}

// registerLLMPlugins adds Obot's own plugins to the gateway. first runs before every other
// plugin, so its request hook sees requests before anything (including request logging)
// records them; last runs after every other plugin, so its response hook runs first.
func registerLLMPlugins(server *bifrostserver.BifrostHTTPServer, first, last schemas.LLMPlugin) error {
	cfg := server.Config
	pre, post := schemas.PluginPlacementPreBuiltin, schemas.PluginPlacementPostBuiltin
	firstOrder, lastOrder := -1_000_000, 1_000_000
	for _, reg := range []struct {
		plugin    schemas.LLMPlugin
		placement *schemas.PluginPlacement
		order     *int
	}{{first, &pre, &firstOrder}, {last, &post, &lastOrder}} {
		cfg.SetPluginOrderInfo(reg.plugin.GetName(), reg.placement, reg.order)
		if err := cfg.ReloadPlugin(reg.plugin); err != nil {
			return fmt.Errorf("failed to register plugin %s: %w", reg.plugin.GetName(), err)
		}
		if err := server.Client.ReloadPlugin(reg.plugin, []schemas.PluginType{schemas.PluginTypeLLM}); err != nil {
			return fmt.Errorf("failed to register plugin %s: %w", reg.plugin.GetName(), err)
		}
	}
	cfg.SortAndRebuildPlugins()
	server.Client.ReorderPlugins(cfg.GetPluginOrder())
	return nil
}

// LLMPluginOrder returns the names of the gateway's LLM plugins in execution order.
func (e *Engine) LLMPluginOrder() []string {
	if !e.Ready() {
		return nil
	}
	return e.inst.Load().server.Config.GetPluginOrder()
}
