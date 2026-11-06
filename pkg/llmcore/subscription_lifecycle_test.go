package llmcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

// End of pipeline-pre is after the final policy hook and before core checks its
// captured queue. Pausing a plugin itself would only exercise the final gate.
type subscriptionPauseTracer struct {
	schemas.Tracer
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

type canceledSubscriptionStream struct {
	subscriptionPlugin
	entered chan struct{}
	release chan struct{}
	done    chan struct{}
}

func (p *subscriptionPauseTracer) EndSpan(handle schemas.SpanHandle, status schemas.SpanStatus, message string) {
	span := p.SpanFromHandle(handle)
	pause := span != nil && span.Name == "pipeline-pre"
	p.Tracer.EndSpan(handle, status, message)
	if pause {
		p.once.Do(func() {
			close(p.entered)
			<-p.release
		})
	}
}

func TestSubscriptionReplacementAfterHooks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		stream bool
		cancel bool
		remove bool
	}{
		{
			name: "unary replacement",
			path: "/v1/messages",
		},
		{
			name:   "stream replacement",
			path:   "/v1/messages",
			stream: true,
		},
		{
			name: "count replacement",
			path: "/v1/messages/count_tokens",
		},
		{
			name:   "cancellation",
			path:   "/v1/messages",
			cancel: true,
		},
		{
			name:   "removal",
			path:   "/v1/messages",
			remove: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldHeaders := make(chan http.Header, 10)
			oldServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				oldHeaders <- r.Header.Clone()
				subscriptionReply(w, r)
			}))
			defer oldServer.Close()
			var replacementCalls atomic.Int32
			replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				replacementCalls.Add(1)
				subscriptionReply(w, r)
			}))
			defer replacement.Close()
			store := subscriptionStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("old-key")))
			e, err := New(t.Context(), store, Options{})
			require.NoError(t, err)
			subscriptionTransport(t, e, oldServer)
			tracer := &subscriptionPauseTracer{Tracer: e.tracer, entered: make(chan struct{}), release: make(chan struct{})}
			e.client.SetTracer(tracer)
			var release sync.Once
			defer func() {
				release.Do(func() { close(tracer.release) })
				require.NoError(t, e.Shutdown(context.Background()))
			}()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// Unary/count calls need an explicit trace for the core phase span.
			traceID := e.tracer.CreateTrace("")
			traced := context.WithValue(ctx, schemas.BifrostContextKeyTraceID, traceID)
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() { response <- subscriptionCall(traced, e, tc.path, "anthropic/test", tc.stream, true, nil, nil) }()
			select {
			case <-tracer.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the post-hook/pre-enqueue boundary")
			}
			original := e.client.GetProviderByKey(schemas.Anthropic)
			if tc.remove {
				require.NoError(t, store.DeleteProvider(t.Context(), "anthropic"))
			} else {
				require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
					p.Config.NetworkConfig.BaseURL = replacement.URL
					p.Config.NetworkConfig.AllowPrivateNetwork = true
					p.Keys[0].Value = *schemas.NewSecretVar("new-key")
					return nil
				}))
			}
			require.NoError(t, store.CreateProvider(t.Context(), "healthy", llmcoreclient.Provider{
				Config: schemas.ProviderConfig{CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}},
			}))
			require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
				settings["client"] = json.RawMessage(`{"max_request_body_size_mb":7}`)
				return nil
			}))
			start := time.Now()
			_, err = e.Sync(t.Context())
			require.ErrorContains(t, err, "anthropic is still draining")
			require.Less(t, time.Since(start), 3*time.Second)
			require.Same(t, original, e.client.GetProviderByKey(schemas.Anthropic))
			require.NotNil(t, e.client.GetProviderByKey("healthy"))
			require.Equal(t, int64(7<<20), e.MaxRequestBodyBytes())
			require.Equal(t, "old-key", e.account.config.Load().Keys[schemas.Anthropic][0].Value.GetValue())
			require.Empty(t, oldHeaders, "the held request has not been enqueued")
			require.Zero(t, replacementCalls.Load())
			blocked := subscriptionCall(t.Context(), e, tc.path, "anthropic/test", tc.stream, true, nil, nil)
			require.Equal(t, http.StatusServiceUnavailable, blocked.Code, blocked.Body.String())

			// Canceled reconciliation retains the same writer job; it never duplicates removals.
			pending := e.retirements[schemas.Anthropic]
			canceled, stop := context.WithCancel(t.Context())
			stop()
			_, err = e.Sync(canceled)
			require.Error(t, err)
			require.Same(t, pending, e.retirements[schemas.Anthropic])
			if tc.cancel {
				cancel()
			}
			release.Do(func() { close(tracer.release) })
			select {
			case result := <-response:
				// This test created the trace, so it also owns transport handoff.
				e.tracer.SignalTransportHandoff(traceID)
				if !tc.cancel {
					require.Equal(t, http.StatusOK, result.Code, result.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("HTTP handler did not return")
			}
			select {
			case <-pending.done:
			case <-time.After(5 * time.Second):
				t.Fatal("HTTP cancellation/return stranded retirement")
			}
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			require.Zero(t, replacementCalls.Load(), "a validated token must never reach the replacement")
			if !tc.cancel {
				h := <-oldHeaders
				require.Equal(t, subscriptionFixture, h.Get("Authorization"))
				require.Empty(t, h.Get("X-Api-Key"))
			}
			refused := subscriptionCall(t.Context(), e, tc.path, "anthropic/test", tc.stream, true, nil, nil)
			require.GreaterOrEqual(t, refused.Code, http.StatusBadRequest)
			require.Zero(t, replacementCalls.Load())
			changed, err := e.Sync(t.Context())
			require.NoError(t, err)
			require.False(t, changed)
		})
	}
}

func TestSubscriptionKeyOnlyRotationDoesNotRetire(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(subscriptionReply))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("old-key")))
	e, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
	subscriptionTransport(t, e, server)
	original := e.client.GetProviderByKey(schemas.Anthropic)
	e.subscriptionLifetime.RLock()
	require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Keys[0].Value = *schemas.NewSecretVar("new-key")
		return nil
	}))
	_, err = e.Sync(t.Context())
	e.subscriptionLifetime.RUnlock()
	require.NoError(t, err)
	require.Empty(t, e.retirements)
	require.Same(t, original, e.client.GetProviderByKey(schemas.Anthropic))
	require.Equal(t, "new-key", e.account.config.Load().Keys[schemas.Anthropic][0].Value.GetValue())
	result := subscriptionCall(t.Context(), e, "/v1/messages", "anthropic/test", false, true, nil, nil)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
}

func (*canceledSubscriptionStream) GetName() string { return "canceled-subscription" }

func (p *canceledSubscriptionStream) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	chunks := make(chan *schemas.BifrostStreamChunk)
	go func() {
		defer close(p.done)
		defer close(chunks)
		close(p.entered)
		<-ctx.Done()
		<-p.release
	}()
	return req, &schemas.LLMPluginShortCircuit{Stream: chunks}, nil
}

func TestCanceledSubscriptionRetainsAdmissionUntilDrained(t *testing.T) {
	plugin := &canceledSubscriptionStream{entered: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("configured")))
	engine, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{plugin}})
	require.NoError(t, err)
	var release sync.Once
	defer func() {
		release.Do(func() { close(plugin.release) })
		require.NoError(t, engine.Shutdown(context.Background()))
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	returned := make(chan struct{})
	go func() {
		subscriptionCall(ctx, engine, "/v1/messages", "anthropic/test", true, true, nil, nil)
		close(returned)
	}()
	select {
	case <-plugin.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-returned:
		t.Fatal("handler returned before producer cleanup")
	case <-time.After(50 * time.Millisecond):
	}
	lifetime := &engine.subscriptionLifetime
	if lifetime.TryLock() {
		lifetime.Unlock()
		t.Fatal("provider retirement can replace a subscription destination before its producer drained")
	}
	release.Do(func() { close(plugin.release) })
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after producer cleanup")
	}
	require.True(t, lifetime.TryLock(), "handler return must release subscription admission")
	lifetime.Unlock()
}
