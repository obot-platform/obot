package llmcore_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestProviderHeaders(t *testing.T) {
	for _, configuredAuth := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured auth headers=%t", configuredAuth), func(t *testing.T) {
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
			pc := schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true}}
			expectedAuth := ""
			if configuredAuth {
				// Admin-configured authentication remains available to ordinary key traffic.
				// Subscription requests alone reject this additional auth source.
				expectedAuth = "Bearer admin-fixture"
				pc.NetworkConfig.ExtraHeaders = map[string]string{"Authorization": expectedAuth}
			}
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{Config: pc}))
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
			require.Equal(t, "configured-key", headers.Get("X-Api-Key"))
			require.Equal(t, expectedAuth, headers.Get("Authorization"))
		})
	}
}

// These are real HTTP requests to the engine, with recording destinations. No
// successful subscription transport fixture may replace Anthropic with loopback.
func TestCallerCredentialsHTTPRejection(t *testing.T) {
	for _, keyed := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured-key=%t", keyed), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected outbound", http.StatusInternalServerError)
			}))
			defer upstream.Close()
			store := newTestStore(t)
			provider := llmcoreclient.Provider{Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true}}}
			if keyed {
				provider.Keys = []schemas.Key{{ID: "configured", Name: "configured", Value: *schemas.NewSecretVar("configured-key"), Models: schemas.WhiteList{"*"}, Weight: 1}}
			}
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", provider))
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
			require.NoError(t, err)
			defer engine.Shutdown()

			for _, mode := range []string{"unsafe destination", "missing UA", "other UA", "api key", "mixed", "duplicate", "wrong bearer"} {
				t.Run(mode, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"anthropic/test","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
					req.Header.Set("User-Agent", "claude-cli/2.1.168")
					req.Header.Set("Authorization", "Bearer sk-ant-oat-fixture")
					switch mode {
					case "missing UA":
						req.Header.Del("User-Agent")
					case "other UA":
						req.Header.Set("User-Agent", "other")
					case "api key":
						req.Header.Del("Authorization")
						req.Header.Set("X-Api-Key", "fixture")
					case "mixed":
						req.Header.Set("X-Api-Key", "fixture")
					case "duplicate":
						req.Header.Add("Authorization", "Bearer sk-ant-oat-other")
					case "wrong bearer":
						req.Header.Set("Authorization", "Bearer other")
					}
					out := httptest.NewRecorder()
					engine.ServeHTTP(out, req)
					require.Equal(t, http.StatusForbidden, out.Code, out.Body.String())
					require.Contains(t, out.Body.String(), "permission_denied")
					require.Zero(t, calls.Load(), "neither subscription nor configured credentials may be sent")
				})
			}
		})
	}
}

// Pause after admission and queue capture, but before credential enforcement.
// Retirement must stay pending while the request holds its old context.
type pausedCredentialAttempt struct {
	entered chan struct{}
	resume  chan struct{}
}

func (*pausedCredentialAttempt) GetName() string { return "pause-credential-attempt" }
func (*pausedCredentialAttempt) Cleanup() error  { return nil }
func (*pausedCredentialAttempt) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}
func (p *pausedCredentialAttempt) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	close(p.entered)
	<-p.resume
	return req, nil, nil
}
func (*pausedCredentialAttempt) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func TestSubscriptionAdmissionSyncAttemptRace(t *testing.T) {
	var calls atomic.Int32
	alternate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected outbound", http.StatusInternalServerError)
	}))
	defer alternate.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
	pause := &pausedCredentialAttempt{entered: make(chan struct{}), resume: make(chan struct{})}
	var resume sync.Once
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{pause}})
	require.NoError(t, err)
	// Release before waiting for Shutdown, including assertion failures.
	defer func() { resume.Do(func() { close(pause.resume) }); engine.Shutdown() }()
	out := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"anthropic/test","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("User-Agent", "claude-cli/2.1.168")
		req.Header.Set("Authorization", "Bearer sk-ant-oat-fixture")
		engine.ServeHTTP(out, req)
	}()
	waitTransition(t, pause.entered)
	require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = alternate.URL
		p.Config.NetworkConfig.AllowPrivateNetwork = true
		return nil
	}))
	// The lease keeps the old queue alive, but Sync still has a bounded wait.
	_, err = engine.Sync(t.Context())
	require.ErrorContains(t, err, "anthropic is still draining")
	resume.Do(func() { close(pause.resume) })
	waitTransition(t, done)
	require.Equal(t, http.StatusServiceUnavailable, out.Code, out.Body.String())
	require.Contains(t, out.Body.String(), "api_error")
	require.Zero(t, calls.Load())
	_, err = engine.Sync(t.Context())
	require.NoError(t, err, "released HTTP context allows replacement on retry")
}
