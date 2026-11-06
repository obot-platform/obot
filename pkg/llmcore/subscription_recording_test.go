package llmcore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/llm"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	subscriptionFixture = "Bearer sk-ant-oat-fixture"
	subscriptionMessage = `{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
)

type subscriptionPolicies struct{}

// A trusted plugin may rewrite a request, so the last gate must inspect it again.
type subscriptionRewrite struct {
	subscriptionPlugin
}

func (subscriptionPolicies) ListGuardrailPolicies(context.Context) ([]types.GuardrailPolicy, error) {
	return nil, nil
}
func (subscriptionPolicies) ListGuardrails(context.Context) ([]types.Guardrail, error) {
	return nil, nil
}
func (subscriptionPolicies) GuardrailSecrets(context.Context, string) (map[string]string, error) {
	return nil, nil
}

// Bifrost has no transport injection API. Before any requests, redirect only its
// two private HTTP clients to a local TLS recorder. Keep the production account
// and URL untouched, and verify the recorder certificate using its test identity.
// This pins the test to core v1.12.0 without adding a production policy bypass.
func subscriptionTransport(t *testing.T, e *Engine, server *httptest.Server) {
	t.Helper()
	provider, ok := e.client.GetProviderByKey(schemas.Anthropic).(*anthropic.AnthropicProvider)
	require.True(t, ok)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, name := range []string{"client", "streamingClient"} {
		field := reflect.ValueOf(provider).Elem().FieldByName(name)
		client := *(**fasthttp.Client)(unsafe.Pointer(field.UnsafeAddr()))
		client.TLSConfig = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
		client.Dial = func(addr string) (net.Conn, error) {
			if addr != "api.anthropic.com:443" {
				return nil, fmt.Errorf("unexpected test destination %s", addr)
			}
			return net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		}
	}
}

func subscriptionStore(t *testing.T) *llmcoreclient.Client {
	t.Helper()
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	return store
}

func subscriptionProvider(key string) llmcoreclient.Provider {
	return llmcoreclient.Provider{
		Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{
			MaxRetries:          1,
			RetryBackoffInitial: time.Millisecond,
			RetryBackoffMax:     time.Millisecond,
		}},
		Keys: []schemas.Key{{
			ID:     "configured",
			Name:   "configured",
			Value:  *schemas.NewSecretVar(key),
			Models: schemas.WhiteList{"*"},
			Weight: 1,
		}},
	}
}

func subscriptionCall(ctx context.Context, e *Engine, path, model string, stream, subscription bool, fallbacks []string, check guardrails.AttemptCheck) *httptest.ResponseRecorder {
	body := map[string]any{
		"model":      model,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": "hello"}},
		"input":      "hello",
		"stream":     stream,
		"fallbacks":  fallbacks,
	}
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data))).WithContext(ctx)
	r.Header.Set("User-Agent", "claude-cli/2.1.168")
	r.Header.Set("Anthropic-Beta", "test-beta")
	r.Header.Add("X-Stainless-Lang", "go")
	r.Header.Add("X-Stainless-Lang", "typescript")
	r.Header.Set("X-Unapproved", "private")
	if subscription {
		r.Header.Set("Authorization", subscriptionFixture)
	}
	token, release := guardrails.BindRequest(guardrails.Identity{}, guardrails.RequestHooks{AttemptCheck: check})
	defer release()
	r.Header.Set(guardrails.HeaderRequestToken, token)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	return w
}

func subscriptionReply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "count_tokens") {
		_, _ = io.WriteString(w, `{"input_tokens":23}`)
		return
	}
	var body struct {
		Stream bool `json:"stream"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		return
	}
	_, _ = io.WriteString(w, subscriptionMessage)
}

func TestSubscriptionRecordingHTTP(t *testing.T) {
	received := make(chan http.Header, 100)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		subscriptionReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("configured-key")))
	input, output := guardrails.New(subscriptionPolicies{}, nil).Plugins()
	e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{input, output}})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
	subscriptionTransport(t, e, server)

	for _, path := range []string{"/v1/messages", "/anthropic/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1/messages/count_tokens", "/anthropic/v1/messages/count_tokens", "/v1/responses/input_tokens"} {
		for _, stream := range []bool{false, true} {
			if stream && strings.Contains(path, "tokens") {
				continue
			}
			for _, subscription := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/stream=%t/subscription=%t", path, stream, subscription), func(t *testing.T) {
					var attempts []guardrails.Attempt
					result := subscriptionCall(t.Context(), e, path, "anthropic/test", stream, subscription, nil, func(_ context.Context, attempt guardrails.Attempt) error {
						attempts = append(attempts, attempt)
						return nil
					})
					require.Equal(t, http.StatusOK, result.Code, result.Body.String())
					require.Equal(t, []guardrails.Attempt{{Model: "anthropic/test", Subscription: subscription}}, attempts)
					headers := <-received
					if subscription {
						require.Equal(t, subscriptionFixture, headers.Get("Authorization"))
						require.Empty(t, headers.Get("X-Api-Key"))
					} else {
						require.Empty(t, headers.Get("Authorization"))
						require.Equal(t, "configured-key", headers.Get("X-Api-Key"))
					}
					require.Equal(t, []string{"go", "typescript"}, headers.Values("X-Stainless-Lang"))
					require.Empty(t, headers.Get("X-Unapproved"))
					require.Empty(t, headers.Get(guardrails.HeaderRequestToken))
				})
			}
		}
	}
	// The existing access hook sees subscription mode before making its decision.
	for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		for _, stream := range []bool{false, true} {
			result := subscriptionCall(t.Context(), e, path, "anthropic/test", stream, true, nil, func(_ context.Context, attempt guardrails.Attempt) error {
				require.True(t, attempt.Subscription)
				return &guardrails.AttemptError{Status: http.StatusForbidden, Type: "permission_denied", Message: "denied by access"}
			})
			require.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
			require.Empty(t, received)
		}
	}
}

func TestSubscriptionRecordingFallbackRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		stream bool
	}{
		{
			name: "unary",
			path: "/v1/messages",
		},
		{
			name:   "stream",
			path:   "/v1/messages",
			stream: true,
		},
		{
			name: "count",
			path: "/v1/messages/count_tokens",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, stream := tc.path, tc.stream
			var mu sync.Mutex
			var models []string
			var headers []http.Header
			failures := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				var body struct {
					Model string `json:"model"`
				}
				_ = json.Unmarshal(data, &body)
				mu.Lock()
				models = append(models, body.Model)
				headers = append(headers, r.Header.Clone())
				fail := body.Model == "fail" || (body.Model == "retry" && failures == 0)
				if fail {
					failures++
				}
				mu.Unlock()
				if fail {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"retry fixture"}}`)
					return
				}
				r.Body = io.NopCloser(strings.NewReader(string(data)))
				subscriptionReply(w, r)
			}))
			defer server.Close()
			foreignCalls := make(chan http.Header, 10)
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				foreignCalls <- r.Header.Clone()
				subscriptionReply(w, r)
			}))
			defer foreign.Close()
			store := subscriptionStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("configured-key")))
			custom := subscriptionProvider("foreign-key")
			custom.Config.NetworkConfig.BaseURL = foreign.URL
			custom.Config.NetworkConfig.AllowPrivateNetwork = true
			custom.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.Anthropic}
			require.NoError(t, store.CreateProvider(t.Context(), "foreign", custom))
			input, output := guardrails.New(subscriptionPolicies{}, nil).Plugins()
			e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{input, output}})
			require.NoError(t, err)
			defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
			subscriptionTransport(t, e, server)

			result := subscriptionCall(t.Context(), e, path, "anthropic/fail", stream, true, []string{"foreign/test", "anthropic/test"}, nil)
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			require.Empty(t, foreignCalls, "foreign fallback must be refused before sending anything")
			mu.Lock()
			require.Equal(t, "fail", models[0])
			require.Equal(t, "test", models[len(models)-1])
			for _, h := range headers {
				require.Equal(t, subscriptionFixture, h.Get("Authorization"))
				require.Empty(t, h.Get("X-Api-Key"))
			}
			mu.Unlock()

			// A routed foreign primary is also refused, while later Anthropic can recover.
			provider, model := "foreign", "test"
			require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{
				ID:            "subscription-route",
				Name:          "subscription-route",
				Scope:         "global",
				CelExpression: "true",
				Targets: []tables.TableRoutingTarget{{
					Provider: &provider,
					Model:    &model,
					Weight:   1,
				}},
				ParsedFallbacks: []tables.RoutingFallback{{
					Provider: schemas.Anthropic,
					Model:    "test",
				}},
			}))
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			result = subscriptionCall(t.Context(), e, path, "anthropic/test", stream, true, nil, nil)
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			require.Empty(t, foreignCalls)
			result = subscriptionCall(t.Context(), e, path, "anthropic/test", stream, false, nil, nil)
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			require.Equal(t, "foreign-key", (<-foreignCalls).Get("X-Api-Key"))

			require.NoError(t, store.DeleteRule(t.Context(), "subscription-route"))
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			mu.Lock()
			models, headers, failures = nil, nil, 0
			mu.Unlock()
			result = subscriptionCall(t.Context(), e, path, "anthropic/retry", stream, true, nil, nil)
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			mu.Lock()
			require.Equal(t, []string{"retry", "retry"}, models)
			for _, h := range headers {
				require.Equal(t, subscriptionFixture, h.Get("Authorization"))
				require.Empty(t, h.Get("X-Api-Key"))
			}
			mu.Unlock()

			// An unkeyed normal primary must not leave subscription mode set on fallback.
			require.NoError(t, store.CreateProvider(t.Context(), "unkeyed", llmcoreclient.Provider{Config: custom.Config}))
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			var attempts []guardrails.Attempt
			result = subscriptionCall(t.Context(), e, path, "unkeyed/test", stream, false, []string{"anthropic/test"}, func(_ context.Context, attempt guardrails.Attempt) error {
				attempts = append(attempts, attempt)
				return nil
			})
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			require.Equal(t, []guardrails.Attempt{{Model: "unkeyed/test"}, {Model: "anthropic/test"}}, attempts)
			require.Empty(t, foreignCalls)
			mu.Lock()
			require.Equal(t, "configured-key", headers[len(headers)-1].Get("X-Api-Key"))
			require.Empty(t, headers[len(headers)-1].Get("Authorization"))
			mu.Unlock()
		})
	}
}

func (*subscriptionRewrite) GetName() string { return "test-subscription-rewrite" }
func (*subscriptionRewrite) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	req.SetProvider("foreign")
	return req, nil, nil
}

func TestSubscriptionFinalGateAfterRewrite(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		subscriptionReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("configured-key")))
	e, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{&subscriptionRewrite{}}})
	require.NoError(t, err)
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
	subscriptionTransport(t, e, server)
	for _, stream := range []bool{false, true} {
		result := subscriptionCall(t.Context(), e, "/v1/messages", "anthropic/test", stream, true, nil, nil)
		require.Equal(t, http.StatusForbidden, result.Code, result.Body.String())
		require.Contains(t, result.Body.String(), "permission_denied")
		require.Empty(t, received)
	}
}

// A policy judge may run inside a subscription request, but its own inference
// must use configured credentials rather than inheriting the user's token mode.
func TestNativeJudgeDoesNotInheritSubscriptionCredentials(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		subscriptionReply(w, r)
	}))
	defer server.Close()
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("judge-configured-key")))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
	subscriptionTransport(t, engine, server)

	parent := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	defer parent.Cancel()
	parent.SetValue(subscriptionTokenKey{}, subscriptionCredentialValue{token: subscriptionFixture})
	parent.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
	parent.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{})
	parent.SetValue(schemas.BifrostContextKeyUserAgent, "claude-cli/2")
	llm.SetSubscriptionAttempt(parent, true)

	_, err = engine.ChatCompletion(parent, "anthropic/test", []schemas.ChatMessage{{
		Role: schemas.ChatMessageRoleUser,
		Content: &schemas.ChatMessageContent{
			ContentStr: new("Judge this message"),
		},
	}})
	require.NoError(t, err)
	headers := <-received
	require.Equal(t, "judge-configured-key", headers.Get("X-Api-Key"))
	require.Empty(t, headers.Get("Authorization"))
	require.NoError(t, parent.Err(), "the judge cancels only its own inference context")
}

func TestNativeJudgeDoesNotInheritKeyPins(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		subscriptionReply(w, r)
	}))
	t.Cleanup(server.Close)
	store := subscriptionStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", subscriptionProvider("judge-configured-key")))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	subscriptionTransport(t, engine, server)

	for _, key := range []schemas.BifrostContextKey{
		schemas.BifrostContextKeyRoutingPinnedAPIKeyID,
		schemas.BifrostContextKeyAPIKeyID,
		schemas.BifrostContextKeyAPIKeyName,
	} {
		t.Run(string(key), func(t *testing.T) {
			parent := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
			defer parent.Cancel()
			parent.SetValue(key, "outer-provider-key")
			_, err := engine.ChatCompletion(parent, "anthropic/test", []schemas.ChatMessage{{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentStr: new("Judge this message"),
				},
			}})
			require.NoError(t, err)
			require.Equal(t, "judge-configured-key", (<-received).Get("X-Api-Key"))
			require.Equal(t, "outer-provider-key", parent.Value(key), "child key selection must not change the parent's pin")
			require.NoError(t, parent.Err())
		})
	}
}
