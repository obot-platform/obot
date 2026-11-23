package llmcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

type blockedSubscriptionWriter struct {
	*httptest.ResponseRecorder
	flushing chan struct{}
	expired  chan struct{}
	once     sync.Once
}

func (w *blockedSubscriptionWriter) FlushError() error {
	close(w.flushing)
	<-w.expired
	return os.ErrDeadlineExceeded
}

func (w *blockedSubscriptionWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		w.once.Do(func() { close(w.expired) })
	}
	return nil
}

func TestSubscriptionRetirementInterruptsBlockedOutput(t *testing.T) {
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI} {
		t.Run(string(provider), func(t *testing.T) {
			reply, transport := subscriptionReply, subscriptionTransport
			r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
				`{"model":"anthropic/test","messages":[{"role":"user","content":"hello"}],"max_tokens":16,"stream":true}`))
			r.Header.Set("User-Agent", "claude-cli/2.1.168")
			r.Header.Set("Authorization", subscriptionFixture)
			if provider == schemas.OpenAI {
				reply, transport = codexReply, codexTransport
				r = codexHTTPRequest(t.Context(), "/v1/responses", `{"model":"gpt-5.4","input":"hi","stream":true}`, true)
			}
			upstream := httptest.NewTLSServer(http.HandlerFunc(reply))
			defer upstream.Close()
			store := subscriptionStore(t)
			require.NoError(t, store.CreateProvider(t.Context(), string(provider), subscriptionProvider("old-key")))
			e, err := New(t.Context(), store, Options{})
			require.NoError(t, err)
			defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
			transport(t, e, upstream)
			w := &blockedSubscriptionWriter{
				ResponseRecorder: httptest.NewRecorder(),
				flushing:         make(chan struct{}),
				expired:          make(chan struct{}),
			}
			defer w.once.Do(func() { close(w.expired) })
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				e.ServeHTTP(w, r)
			}()
			select {
			case <-w.flushing:
			case <-time.After(5 * time.Second):
				t.Fatal("subscription stream did not reach the response transport")
			}
			var keysMu sync.Mutex
			var keys []string
			replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					key := r.Header.Get("X-Api-Key")
					if provider == schemas.OpenAI {
						key = r.Header.Get("Authorization")
					}
					keysMu.Lock()
					keys = append(keys, key)
					keysMu.Unlock()
				}
				reply(w, r)
			}))
			defer replacement.Close()
			require.NoError(t, store.MutateProvider(t.Context(), string(provider), func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.BaseURL = replacement.URL
				p.Config.NetworkConfig.AllowPrivateNetwork = true
				p.Keys[0].Value = *schemas.NewSecretVar("new-key")
				return nil
			}))
			// A short-lived Sync cannot cancel the retirement's grace timer.
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			_, err = e.Sync(ctx)
			cancel()
			require.ErrorContains(t, err, "is still draining")
			select {
			case <-returned:
			case <-time.After(5 * time.Second):
				t.Fatal("retirement did not release the blocked response")
			}
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			_, transitioning := e.account.transitioning.Load(provider)
			require.False(t, transitioning)
			require.Equal(t, "new-key", e.account.config.Load().Keys[provider][0].Value.GetValue())
			expectedKey := "new-key"
			if provider == schemas.OpenAI {
				result := codexCall(t.Context(), e, "/v1/responses", `{"model":"openai/test","input":"hi"}`, false, nil)
				require.Equal(t, http.StatusOK, result.Code, result.Body.String())
				expectedKey = "Bearer new-key"
			} else {
				result := subscriptionCall(t.Context(), e, "/v1/messages", "anthropic/test", false, false, nil, nil)
				require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			}
			keysMu.Lock()
			receivedKeys := append([]string(nil), keys...)
			keysMu.Unlock()
			require.Equal(t, []string{expectedKey}, receivedKeys)
			changed, err := e.Sync(t.Context())
			require.NoError(t, err)
			require.False(t, changed)
		})
	}
}
