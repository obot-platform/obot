package llmcore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI} {
		t.Run(string(provider), func(t *testing.T) { testSubscriptionReplacementAfterHooks(t, provider) })
	}
}

func testSubscriptionReplacementAfterHooks(t *testing.T, provider schemas.ModelProvider) {
	t.Helper()
	reply := subscriptionReply
	transport := subscriptionTransport
	credential := subscriptionFixture
	other := schemas.OpenAI
	if provider == schemas.OpenAI {
		reply = codexReply
		transport = codexTransport
		credential = codexFixture
		other = schemas.Anthropic
	}
	call := func(ctx context.Context, e *Engine, path string, stream bool) *httptest.ResponseRecorder {
		if provider == schemas.Anthropic {
			return subscriptionCall(ctx, e, path, "anthropic/test", stream, true, nil, nil)
		}
		codexPath := "/v1/responses"
		if strings.Contains(path, "count_tokens") {
			codexPath = "/v1/models?client_version=0.162.1"
		}
		return codexCall(ctx, e, codexPath, fmt.Sprintf(`{"model":"gpt-5.4","input":"hi","stream":%t}`, stream), true, nil)
	}
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
				reply(w, r)
			}))
			defer oldServer.Close()
			var replacementCalls atomic.Int32
			replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				replacementCalls.Add(1)
				reply(w, r)
			}))
			defer replacement.Close()
			store := subscriptionStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), string(provider), subscriptionProvider("old-key")))
			require.NoError(t, store.CreateProvider(t.Context(), string(other), subscriptionProvider("other-old-key")))
			e, err := New(t.Context(), store, Options{})
			require.NoError(t, err)
			transport(t, e, oldServer)
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
			go func() { response <- call(traced, e, tc.path, tc.stream) }()
			select {
			case <-tracer.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach the post-hook/pre-enqueue boundary")
			}
			original := e.client.GetProviderByKey(provider)
			otherOriginal := e.client.GetProviderByKey(other)
			require.NoError(t, store.MutateProvider(t.Context(), string(other), func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = 20
				p.Keys[0].Value = *schemas.NewSecretVar("other-new-key")
				return nil
			}))
			if tc.remove {
				require.NoError(t, store.DeleteProvider(t.Context(), string(provider)))
			} else {
				require.NoError(t, store.MutateProvider(t.Context(), string(provider), func(p *llmcoreclient.Provider) error {
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
			require.ErrorContains(t, err, string(provider)+" is still draining")
			require.Less(t, time.Since(start), 3*time.Second)
			require.Same(t, original, e.client.GetProviderByKey(provider))
			require.NotNil(t, e.client.GetProviderByKey("healthy"))
			require.NotSame(t, otherOriginal, e.client.GetProviderByKey(other))
			require.Equal(t, "other-new-key", e.account.config.Load().Keys[other][0].Value.GetValue())
			require.Equal(t, int64(7<<20), e.MaxRequestBodyBytes())
			require.Equal(t, "old-key", e.account.config.Load().Keys[provider][0].Value.GetValue())
			require.Empty(t, oldHeaders, "the held request has not been enqueued")
			require.Zero(t, replacementCalls.Load())
			blocked := call(t.Context(), e, tc.path, tc.stream)
			require.Equal(t, http.StatusServiceUnavailable, blocked.Code, blocked.Body.String())

			// Canceled reconciliation retains the same writer job; it never duplicates removals.
			pending := e.retirements[provider]
			canceled, stop := context.WithCancel(t.Context())
			stop()
			_, err = e.Sync(canceled)
			require.Error(t, err)
			require.Same(t, pending, e.retirements[provider])
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
				require.Equal(t, credential, h.Get("Authorization"))
				require.Empty(t, h.Get("X-Api-Key"))
			}
			refused := call(t.Context(), e, tc.path, tc.stream)
			require.GreaterOrEqual(t, refused.Code, http.StatusBadRequest)
			require.Zero(t, replacementCalls.Load())
			changed, err := e.Sync(t.Context())
			require.NoError(t, err)
			require.False(t, changed)
		})
	}
}

func TestSubscriptionKeyOnlyRotationDoesNotRetire(t *testing.T) {
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI} {
		t.Run(string(provider), func(t *testing.T) { testSubscriptionKeyOnlyRotation(t, provider) })
	}
}

func testSubscriptionKeyOnlyRotation(t *testing.T, provider schemas.ModelProvider) {
	t.Helper()
	reply := subscriptionReply
	transport := subscriptionTransport
	if provider == schemas.OpenAI {
		reply = codexReply
		transport = codexTransport
	}
	server := httptest.NewTLSServer(http.HandlerFunc(reply))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), string(provider), subscriptionProvider("old-key")))
	e, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
	transport(t, e, server)
	original := e.client.GetProviderByKey(provider)
	e.subscriptionLock(provider).RLock()
	require.NoError(t, store.MutateProvider(t.Context(), string(provider), func(p *llmcoreclient.Provider) error {
		p.Keys[0].Value = *schemas.NewSecretVar("new-key")
		return nil
	}))
	_, err = e.Sync(t.Context())
	e.subscriptionLock(provider).RUnlock()
	require.NoError(t, err)
	require.Empty(t, e.retirements)
	require.Same(t, original, e.client.GetProviderByKey(provider))
	require.Equal(t, "new-key", e.account.config.Load().Keys[provider][0].Value.GetValue())
	var result *httptest.ResponseRecorder
	if provider == schemas.OpenAI {
		result = codexCall(t.Context(), e, "/v1/responses", `{"model":"gpt-5.4","input":"hi"}`, true, nil)
	} else {
		result = subscriptionCall(t.Context(), e, "/v1/messages", "anthropic/test", false, true, nil, nil)
	}
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
}

type canceledSubscriptionStream struct {
	subscriptionPlugin
	entered chan struct{}
	release chan struct{}
	done    chan struct{}
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
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP cancellation waited for producer cleanup")
	}
	lifetime := engine.subscriptionLock(schemas.Anthropic)
	if lifetime.TryLock() {
		lifetime.Unlock()
		t.Fatal("provider retirement can replace a subscription destination before its producer drained")
	}
	release.Do(func() { close(plugin.release) })
	require.Eventually(t, func() bool {
		if lifetime.TryLock() {
			lifetime.Unlock()
			return true
		}
		return false
	}, 3*time.Second, time.Millisecond, "draining must release subscription admission")
}
