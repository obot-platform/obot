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
			require.NoError(t, store.PutProvider(t.Context(), "anthropic", llmcoreclient.Provider{Config: schemas.ProviderConfig{
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
