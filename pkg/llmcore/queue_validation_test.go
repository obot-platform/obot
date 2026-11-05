package llmcore_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestInvalidProviderQueuesRecover(t *testing.T) {
	for _, field := range []string{"concurrency", "buffer_size"} {
		for _, phase := range []string{"startup", "replacement"} {
			t.Run(field+"/"+phase, func(t *testing.T) {
				oldRecorder, newRecorder := &credentialRecorder{}, &credentialRecorder{}
				oldServer := httptest.NewServer(http.HandlerFunc(oldRecorder.serve))
				defer oldServer.Close()
				newServer := httptest.NewServer(http.HandlerFunc(newRecorder.serve))
				defer newServer.Close()
				store := newTestStore(t)
				require.NoError(t, store.CreateProvider(t.Context(), "queue-healthy", transitionProvider(oldServer.URL, "healthy-secret")))
				provider := transitionProvider(oldServer.URL, "old-secret")
				invalidate := func(p *llmcoreclient.Provider) {
					if field == "concurrency" {
						p.Config.ConcurrencyAndBufferSize.Concurrency = -1
					} else {
						p.Config.ConcurrencyAndBufferSize.BufferSize = -1
					}
				}

				if phase == "startup" {
					invalidate(&provider)
				}
				require.NoError(t, store.CreateProvider(t.Context(), "queue-invalid", provider))

				engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
				require.NoError(t, err)
				defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
				healthy := engine.Client().GetProviderByKey("queue-healthy")
				require.NotNil(t, healthy)

				call := func(name string) *httptest.ResponseRecorder {
					t.Helper()
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+name+`/test","messages":[{"role":"user","content":"hello"}]}`)))
					return response
				}

				if phase == "replacement" {
					require.Equal(t, http.StatusOK, call("queue-invalid").Code)
					require.NoError(t, store.MutateProvider(t.Context(), "queue-invalid", func(p *llmcoreclient.Provider) error {
						*p = transitionProvider(newServer.URL, "new-secret")
						invalidate(p)
						return nil
					}))
				} else {
					require.ErrorContains(t, engine.Err(), "queue-invalid")
					require.ErrorContains(t, engine.Err(), field)
					require.Nil(t, engine.Client().GetProviderByKey("queue-invalid"))
				}

				for range 2 {
					changed, err := engine.Sync(t.Context())
					require.True(t, changed)
					require.ErrorContains(t, err, "queue-invalid")
					require.ErrorContains(t, err, field)
					response := call("queue-invalid")
					require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
					require.Equal(t, http.StatusOK, call("queue-healthy").Code)
					require.Same(t, healthy, engine.Client().GetProviderByKey("queue-healthy"))
				}

				expected := []string{"Bearer healthy-secret", "Bearer healthy-secret"}
				if phase == "replacement" {
					expected = append([]string{"Bearer old-secret"}, expected...)
				}
				require.Equal(t, expected, oldRecorder.snapshot(), "no desired credential may reach the retired destination")
				require.Empty(t, newRecorder.snapshot())

				require.NoError(t, store.MutateProvider(t.Context(), "queue-invalid", func(p *llmcoreclient.Provider) error {
					*p = transitionProvider(newServer.URL, "new-secret")
					p.Config.ConcurrencyAndBufferSize = schemas.ConcurrencyAndBufferSize{}
					return nil
				}))
				changed, err := engine.Sync(t.Context())
				require.NoError(t, err)
				require.True(t, changed)
				require.NoError(t, engine.Err())
				require.Equal(t, http.StatusOK, call("queue-invalid").Code, "zero queue sizes use upstream defaults")
				require.Equal(t, []string{"Bearer new-secret"}, newRecorder.snapshot())
				require.Same(t, healthy, engine.Client().GetProviderByKey("queue-healthy"))

				changed, err = engine.Sync(t.Context())
				require.NoError(t, err)
				require.False(t, changed)
			})
		}
	}
}

func TestInvalidStandardProviderQueueRemainsPending(t *testing.T) {
	for _, field := range []string{"concurrency", "buffer_size"} {
		t.Run(field, func(t *testing.T) {
			store := newTestStore(t)
			provider := transitionProvider("http://unused.example", "secret")
			if field == "concurrency" {
				provider.Config.ConcurrencyAndBufferSize.Concurrency = -1
			} else {
				provider.Config.ConcurrencyAndBufferSize.BufferSize = -1
			}
			require.NoError(t, store.CreateProvider(t.Context(), "openai", provider))

			engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
			require.NoError(t, err)
			defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
			require.ErrorContains(t, engine.Err(), "openai")
			require.ErrorContains(t, engine.Err(), field)

			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/test","messages":[{"role":"user","content":"hello"}]}`)))
			require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		})
	}
}
