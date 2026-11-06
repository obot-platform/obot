package llmcore_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

type drainLogger struct {
	schemas.Logger
	removals atomic.Int32
}

func (l *drainLogger) Info(msg string, args ...any) {
	if msg == "Removing provider %s" && len(args) > 0 && args[0] == schemas.ModelProvider("paired") {
		l.removals.Add(1)
	}
	l.Logger.Info(msg, args...)
}

func TestLongStreamDoesNotBlockUnrelatedConfiguration(t *testing.T) {
	for _, action := range []string{"replace", "delete"} {
		t.Run(action, func(t *testing.T) { testLongStreamRetirement(t, action) })
	}
}

func testLongStreamRetirement(t *testing.T, action string) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	oldKeys := make(chan string, 1)
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldKeys <- r.Header.Get("Authorization")
		// Stream establishment occupies the worker. Once headers arrive,
		// Bifrost hands off the stream and removal no longer waits for it.
		close(entered)
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"held\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"}}]}\n\n")
		_ = http.NewResponseController(w).Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer oldServer.Close()
	defer releaseOnce.Do(func() { close(release) })
	newRecorder := &credentialRecorder{}
	newServer := httptest.NewServer(http.HandlerFunc(newRecorder.serve))
	defer newServer.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "paired", transitionProvider(oldServer.URL, "old-secret")))
	logger := &drainLogger{Logger: bifrost.NewDefaultLogger(schemas.LogLevelWarn)}
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Logger: logger})
	require.NoError(t, err)
	defer func() {
		releaseOnce.Do(func() { close(release) })
		require.NoError(t, engine.Shutdown(context.Background()))
	}()
	ctx, cancel := schemas.NewBifrostContextWithCancel(t.Context())
	defer cancel()
	streamDone := make(chan struct{})
	streamErrors := make(chan *schemas.BifrostError, 1)
	go func() {
		defer close(streamDone)
		chunks, streamErr := engine.Client().ChatCompletionStreamRequest(ctx, &schemas.BifrostChatRequest{
			Provider: "paired",
			Model:    "test",
			Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
		})
		streamErrors <- streamErr
		if chunks != nil {
			for range chunks { //nolint:revive // Drain producer output before signaling stream completion.
			}
		}
	}()
	waitTransition(t, entered)
	require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
		*p = transitionProvider(newServer.URL, "intermediate-secret")
		return nil
	}))
	require.NoError(t, store.CreateProvider(t.Context(), "unrelated", transitionProvider(newServer.URL, "unrelated-secret")))
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{"max_request_body_size_mb":7}`)
		return nil
	}))
	synced, syncDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(syncDone)
		_, err := engine.Sync(t.Context())
		synced <- err
	}()
	defer func() { releaseOnce.Do(func() { close(release) }); <-syncDone }()
	select {
	case err := <-synced:
		require.ErrorContains(t, err, "paired")
	case <-time.After(3 * time.Second):
		t.Fatal("Sync waited indefinitely for the held-open stream")
	}
	require.Equal(t, int64(7<<20), engine.MaxRequestBodyBytes())
	unrelatedCtx, unrelatedCancel := schemas.NewBifrostContextWithCancel(t.Context())
	defer unrelatedCancel()
	_, unrelatedErr := engine.Client().ChatCompletionRequest(unrelatedCtx, &schemas.BifrostChatRequest{
		Provider: "unrelated",
		Model:    "test",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
	})
	require.Nil(t, unrelatedErr, "healthy changes must apply while paired drains")
	require.Equal(t, []string{"Bearer unrelated-secret"}, newRecorder.snapshot())
	// A new desired value must not start a second removal or publish keys to the old runtime.
	if action == "delete" {
		require.NoError(t, store.DeleteProvider(t.Context(), "paired"))
	} else {
		require.NoError(t, store.MutateProvider(t.Context(), "paired", func(p *llmcoreclient.Provider) error {
			*p = transitionProvider(newServer.URL, "latest-secret")
			return nil
		}))
	}
	retryCtx, retryCancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer retryCancel()
	_, err = engine.Sync(retryCtx)
	require.Error(t, err)
	require.EqualValues(t, 1, logger.removals.Load(), "retries reuse the pending removal")
	require.Equal(t, "Bearer old-secret", <-oldKeys)
	releaseOnce.Do(func() { close(release) })
	waitTransition(t, streamDone)
	require.Nil(t, <-streamErrors)
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	if action == "delete" {
		require.NotNil(t, callTransitionProvider(t.Context(), engine))
		require.Equal(t, []string{"Bearer unrelated-secret"}, newRecorder.snapshot())
	} else {
		require.Nil(t, callTransitionProvider(t.Context(), engine))
		require.Equal(t, []string{"Bearer unrelated-secret", "Bearer latest-secret"}, newRecorder.snapshot())
	}
	require.EqualValues(t, 1, logger.removals.Load())
	changed, err := engine.Sync(t.Context())
	require.NoError(t, err)
	require.False(t, changed, "reconciliation converges after the drain completes")
}
