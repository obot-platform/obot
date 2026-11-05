package llmcore_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

type credentialRecorder struct {
	mu   sync.Mutex
	keys []string
}

// transitionLogger pauses construction after reconciliation publishes the desired
// account view. On the old implementation, the old worker is still live here.
type transitionLogger struct {
	schemas.Logger
	updateStarted chan struct{}
	releaseUpdate chan struct{}
	removing      chan struct{}
	armed         atomic.Bool
}

func (r *credentialRecorder) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.keys = append(r.keys, req.Header.Get("Authorization"))
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"id":"test","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
}

func (r *credentialRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.keys...)
}

func transitionProvider(url, key string) llmcoreclient.Provider {
	return llmcoreclient.Provider{
		Config: schemas.ProviderConfig{
			NetworkConfig:            schemas.NetworkConfig{BaseURL: url, AllowPrivateNetwork: true},
			CustomProviderConfig:     &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
			ConcurrencyAndBufferSize: schemas.ConcurrencyAndBufferSize{Concurrency: 1, BufferSize: 1},
		},
		Keys: []schemas.Key{{ID: "key", Name: "key", Value: *schemas.NewSecretVar(key), Models: schemas.WhiteList{"*"}, Weight: 1}},
	}
}

func callTransitionProvider(ctx context.Context, engine *llmcore.Engine) *schemas.BifrostError {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	bfCtx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	_, err := engine.Client().ChatCompletionRequest(bfCtx, &schemas.BifrostChatRequest{
		Provider: "paired",
		Model:    "test",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
	})
	return err
}

func TestFailedProviderReplacementBlocksNewCredentials(t *testing.T) {
	oldRecorder, newRecorder := &credentialRecorder{}, &credentialRecorder{}
	oldServer := httptest.NewServer(http.HandlerFunc(oldRecorder.serve))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(newRecorder.serve))
	defer newServer.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "paired", transitionProvider(oldServer.URL, "old-secret")))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
	require.Nil(t, callTransitionProvider(t.Context(), engine))
	require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
		*p = transitionProvider(newServer.URL, "new-secret")
		p.Config.CustomProviderConfig.BaseProviderType = "unsupported-base"
		return nil
	}))
	for range 2 {
		_, err = engine.Sync(t.Context())
		require.Error(t, err, "failed replacements retry without another resource write")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"paired/test","messages":[{"role":"user","content":"hello"}]}`)))
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "being reconfigured")

	attemptErr := callTransitionProvider(t.Context(), engine)
	require.Equal(t, []string{"Bearer old-secret"}, oldRecorder.snapshot(), "failed replacement must not send a new credential to the old destination")
	require.Empty(t, newRecorder.snapshot())
	require.NotNil(t, attemptErr, "the failed provider must remain blocked")
	// A later successful retry reopens the provider with the same desired credential.
	require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
		p.Config.CustomProviderConfig.BaseProviderType = schemas.OpenAI
		return nil
	}))
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	require.Nil(t, callTransitionProvider(t.Context(), engine))
	require.Equal(t, []string{"Bearer new-secret"}, newRecorder.snapshot())
}

func (l *transitionLogger) Info(msg string, args ...any) {
	if l.armed.Load() {
		if msg == "Removing provider %s" && len(args) > 0 && args[0] == schemas.ModelProvider("paired") {
			select {
			case l.removing <- struct{}{}:
			default:
			}
		}
		if msg == "Updating provider configuration for provider paired" {
			l.updateStarted <- struct{}{}
			<-l.releaseUpdate
		}
	}
	l.Logger.Info(msg, args...)
}

func waitTransition(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("provider transition did not reach the expected phase")
	}
}

func TestProviderReplacementGatesHTTPAndDirectCalls(t *testing.T) {
	oldRecorder, newRecorder := &credentialRecorder{}, &credentialRecorder{}
	oldServer := httptest.NewServer(http.HandlerFunc(oldRecorder.serve))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(newRecorder.serve))
	defer newServer.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "paired", transitionProvider(oldServer.URL, "old-secret")))
	logger := &transitionLogger{
		Logger:        bifrost.NewDefaultLogger(schemas.LogLevelWarn),
		updateStarted: make(chan struct{}, 1),
		releaseUpdate: make(chan struct{}),
		removing:      make(chan struct{}, 1),
	}
	// Release before Shutdown even if an assertion aborts while Sync is paused.
	var release sync.Once
	defer release.Do(func() { close(logger.releaseUpdate) })
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Logger: logger})
	require.NoError(t, err)
	defer func() {
		release.Do(func() { close(logger.releaseUpdate) })
		require.NoError(t, engine.Shutdown(context.Background()))
	}()
	require.Nil(t, callTransitionProvider(t.Context(), engine))
	require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
		*p = transitionProvider(newServer.URL, "new-secret")
		return nil
	}))
	logger.armed.Store(true)
	synced := make(chan error, 1)
	go func() { _, err := engine.Sync(t.Context()); synced <- err }()
	waitTransition(t, logger.updateStarted)

	// HTTP must run before a direct call can recreate global provider registration.
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"paired/test","messages":[{"role":"user","content":"hello"}]}`)))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "being reconfigured")

	attemptErr := callTransitionProvider(t.Context(), engine)
	require.NotNil(t, attemptErr, "direct core admission must be gated")
	require.Equal(t, http.StatusServiceUnavailable, attemptErr.EffectiveHTTPStatus())
	require.Contains(t, attemptErr.Error.Message, "being reconfigured")
	require.Equal(t, []string{"Bearer old-secret"}, oldRecorder.snapshot())
	require.Empty(t, newRecorder.snapshot())
	release.Do(func() { close(logger.releaseUpdate) })
	require.NoError(t, <-synced)
	logger.armed.Store(false)
	require.Nil(t, callTransitionProvider(t.Context(), engine))
	require.Equal(t, []string{"Bearer new-secret"}, newRecorder.snapshot())
}

func TestProviderReplacementDrainsInflightRequests(t *testing.T) {
	oldRecorder, newRecorder := &credentialRecorder{}, &credentialRecorder{}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		<-release
		oldRecorder.serve(w, req)
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(newRecorder.serve))
	defer newServer.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "paired", transitionProvider(oldServer.URL, "old-secret")))
	logger := &transitionLogger{
		Logger:        bifrost.NewDefaultLogger(schemas.LogLevelWarn),
		updateStarted: make(chan struct{}, 1),
		releaseUpdate: make(chan struct{}),
		removing:      make(chan struct{}, 1),
	}
	close(logger.releaseUpdate)
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Logger: logger})
	require.NoError(t, err)
	defer func() {
		releaseOnce.Do(func() { close(release) })
		require.NoError(t, engine.Shutdown(context.Background()))
	}()
	inflight := make(chan *schemas.BifrostError, 1)
	go func() { inflight <- callTransitionProvider(t.Context(), engine) }()
	waitTransition(t, entered)
	engine.Client().UpdateDropExcessRequests(true)
	// With one busy worker and one buffer slot, two more producers guarantee
	// one queued request and one queue-full result without timing assumptions.
	queued := make(chan *schemas.BifrostError, 2)
	for range 2 {
		go func() { queued <- callTransitionProvider(t.Context(), engine) }()
	}
	queueFull := <-queued
	require.NotNil(t, queueFull)
	require.Contains(t, queueFull.Error.Message, "queue is full")
	require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
		*p = transitionProvider(newServer.URL, "new-secret")
		return nil
	}))
	logger.armed.Store(true)
	synced := make(chan error, 1)
	go func() { _, err := engine.Sync(t.Context()); synced <- err }()
	waitTransition(t, logger.removing)
	select {
	case err := <-synced:
		t.Fatalf("replacement finished before the old request drained: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	require.Nil(t, <-inflight)
	require.NotNil(t, <-queued, "queued work must fail during retirement")
	require.NoError(t, <-synced)
	logger.armed.Store(false)
	require.Nil(t, callTransitionProvider(t.Context(), engine))
	require.Equal(t, []string{"Bearer old-secret"}, oldRecorder.snapshot())
	require.Equal(t, []string{"Bearer new-secret"}, newRecorder.snapshot())
}
