package llmcore_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestProviderHeaders(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		name := "configured key"
		if subscription {
			name = "Claude subscription"
		}
		t.Run(name, func(t *testing.T) {
			received := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.NotFound(w, r)
					return
				}
				received <- r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer upstream.Close()
			store := newTestStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{Config: schemas.ProviderConfig{
				NetworkConfig: schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true},
			}}))
			require.NoError(t, store.PutKey(t.Context(), "anthropic", schemas.Key{
				ID: "configured", Name: "configured", Value: *schemas.NewSecretVar("configured-key"),
				Models: schemas.WhiteList{"*"}, Weight: 1,
			}))
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
			require.NoError(t, err)
			defer engine.Shutdown()
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"anthropic/claude-test","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Anthropic-Version", "2023-06-01")
			req.Header.Set("Anthropic-Beta", "test-beta")
			req.Header.Add("X-Stainless-Lang", "go")
			req.Header.Add("X-Stainless-Lang", "typescript")
			req.Header.Set("X-Unapproved", "must-not-reach-provider")
			if subscription {
				// This models headers admitted by the proxy, which already validated the
				// subscription policy, token type, and direct Anthropic destination.
				req.Header.Set("User-Agent", "claude-cli/2.1.168")
				req.Header.Set("Authorization", "Bearer sk-ant-oat-test")
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var headers http.Header
			select {
			case headers = <-received:
			default:
				t.Fatal("provider received no request")
			}
			require.Equal(t, "2023-06-01", headers.Get("Anthropic-Version"))
			require.Contains(t, headers.Get("Anthropic-Beta"), "test-beta")
			require.Equal(t, []string{"go", "typescript"}, headers.Values("X-Stainless-Lang"))
			require.Empty(t, headers.Get("X-Unapproved"))
			if subscription {
				require.Equal(t, "Bearer sk-ant-oat-test", headers.Get("Authorization"))
				require.Empty(t, headers.Get("X-Api-Key"), "subscription must not select the configured key")
			} else {
				require.Equal(t, "configured-key", headers.Get("X-Api-Key"))
				require.Empty(t, headers.Get("Authorization"))
			}
		})
	}
}

// A core request has no proxy AttemptCheck; the engine must protect its credentials itself.
func TestSubscriptionFallbackCannotForwardCredentialsToAnotherProvider(t *testing.T) {
	for _, target := range []string{"openai", "custom-anthropic"} {
		t.Run(target, func(t *testing.T) {
			primaryHeaders := make(chan http.Header, 8)
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				primaryHeaders <- r.Header.Clone()
				body, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(string(body), `"model":"recovered"`) {
					_, _ = io.WriteString(w, `{"id":"msg_recovered","type":"message","role":"assistant","model":"recovered","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"primary unavailable"}}`)
			}))
			defer primary.Close()
			fallbackHeaders := make(chan http.Header, 8)
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackHeaders <- r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fallback","object":"chat.completion","model":"fallback","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			defer fallback.Close()
			store := newTestStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{Config: schemas.ProviderConfig{
				NetworkConfig: schemas.NetworkConfig{BaseURL: primary.URL, AllowPrivateNetwork: true, MaxRetries: 1},
			}}))
			pc := schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: fallback.URL, AllowPrivateNetwork: true, MaxRetries: 1}}
			if target == "custom-anthropic" {
				pc.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.Anthropic}
			}
			require.NoError(t, store.CreateProvider(t.Context(), target, llmcoreclient.Provider{Config: pc}))
			require.NoError(t, store.PutKey(t.Context(), target, schemas.Key{
				ID: "configured", Name: "configured", Value: *schemas.NewSecretVar("configured-key"),
				Models: schemas.WhiteList{"*"}, Weight: 1,
			}))
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
			require.NoError(t, err)
			defer engine.Shutdown()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"anthropic/claude-test","messages":[{"role":"user","content":"hi"}],"fallbacks":["`+target+`/fallback"]}`))
			req.Header.Set("User-Agent", "claude-cli/2.1.168")
			req.Header.Set("Authorization", "Bearer sk-ant-oat-test")
			req.Header.Set("X-Api-Key", "caller-key")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.NotEmpty(t, primaryHeaders, "exercise an actual failed Anthropic attempt")
			require.Equal(t, "Bearer sk-ant-oat-test", (<-primaryHeaders).Get("Authorization"))
			require.Empty(t, fallbackHeaders, "credentials must never reach another provider, including a custom Anthropic provider")
			// Bifrost preserves the primary error when all fallbacks fail.
			require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "primary unavailable")

			// A direct attempt on the other provider fails with the boundary's 403.
			for _, credential := range []string{"Authorization", "X-Api-Key"} {
				direct := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+target+`/fallback","messages":[{"role":"user","content":"hi"}]}`))
				direct.Header.Set("User-Agent", req.UserAgent())
				direct.Header.Set(credential, req.Header.Get(credential))
				denied := httptest.NewRecorder()
				engine.ServeHTTP(denied, direct)
				require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
				require.Contains(t, denied.Body.String(), "Anthropic")
				require.Empty(t, fallbackHeaders)
			}

			// Refusing a foreign provider must not discard the token for a later
			// canonical Anthropic fallback in the same request.
			recovery := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"anthropic/claude-test","messages":[{"role":"user","content":"hi"}],"fallbacks":["`+target+`/fallback","anthropic/recovered"]}`))
			recovery.Header = req.Header.Clone()
			recovered := httptest.NewRecorder()
			engine.ServeHTTP(recovered, recovery)
			require.Equal(t, http.StatusOK, recovered.Code, recovered.Body.String())
			require.Empty(t, fallbackHeaders)
			require.NotEmpty(t, primaryHeaders)
			for len(primaryHeaders) > 0 {
				headers := <-primaryHeaders
				require.Equal(t, "Bearer sk-ant-oat-test", headers.Get("Authorization"))
				require.Equal(t, "caller-key", headers.Get("X-Api-Key"))
			}
		})
	}
}
