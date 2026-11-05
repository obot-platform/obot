package llmcore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	"github.com/stretchr/testify/require"
)

// Capture after HTTP conversion, then stop each attempt before any network call.
// This exercises every handler without depending on upstream dialect support.
type conversionRecorder struct {
	provider  schemas.ModelProvider
	model     string
	fallbacks []schemas.Fallback
	calls     int
}

func (*conversionRecorder) GetName() string { return "conversion-recorder" }
func (*conversionRecorder) Cleanup() error  { return nil }
func (p *conversionRecorder) PreRequestHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	p.provider, p.model, p.fallbacks = req.GetRequestFields()
	p.calls++
	return nil
}
func (*conversionRecorder) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{
		StatusCode: schemas.Ptr(http.StatusServiceUnavailable),
		Error:      &schemas.ErrorField{Message: "test attempt stopped"},
	}}, nil
}
func (*conversionRecorder) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func TestHTTPConversionUsesConfiguredProviders(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "conversion-custom", transitionProvider("http://unused.example", "secret")))
	require.NoError(t, store.CreateProvider(t.Context(), "openai", transitionProvider("http://unused.example", "secret")))
	capture := &conversionRecorder{}
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{capture}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })

	// Another engine can retire this name globally while this engine retains it.
	schemas.UnregisterKnownProvider("conversion-custom")
	schemas.RegisterKnownProvider("foreign-engine")
	t.Cleanup(func() { schemas.UnregisterKnownProvider("foreign-engine") })
	for _, path := range []string{
		"/v1/chat/completions",
		"/v1/responses",
		"/v1/messages",
		"/anthropic/v1/messages",
		"/v1/messages/count_tokens",
		"/anthropic/v1/messages/count_tokens",
		"/v1/responses/input_tokens",
	} {
		for _, stream := range []bool{false, true} {
			if stream && (strings.Contains(path, "count_tokens") || strings.Contains(path, "input_tokens")) {
				continue
			}
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				for _, tt := range []struct {
					raw      string
					provider schemas.ModelProvider
					model    string
				}{
					{
						raw:      "conversion-custom/namespace/model",
						provider: "conversion-custom",
						model:    "namespace/model",
					},
					{
						raw:      "openai/namespace/model",
						provider: schemas.OpenAI,
						model:    "namespace/model",
					},
					{
						raw: "unknown/model",
					},
					{
						raw: "foreign-engine/model",
					},
					{
						raw: "bare-model",
					},
					{
						raw: "",
					},
				} {
					schemas.UnregisterKnownProvider("conversion-custom")
					body, err := json.Marshal(map[string]any{
						"model":      tt.raw,
						"messages":   []map[string]string{{"role": "user", "content": "hello"}},
						"input":      "hello",
						"max_tokens": 16,
						"stream":     stream,
						"fallbacks":  []string{"", "bare", "unknown/model", "foreign-engine/model", "conversion-custom/", "conversion-custom/namespace/fallback", "openai/backup"},
					})
					require.NoError(t, err)

					before := capture.calls
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body))))

					if tt.provider == "" {
						require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
						require.Equal(t, before, capture.calls)
						continue
					}

					require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
					require.Equal(t, before+1, capture.calls)
					require.Equal(t, tt.provider, capture.provider)
					require.Equal(t, tt.model, capture.model)
					require.Equal(t, []schemas.Fallback{
						{
							Provider: "conversion-custom",
							Model:    "namespace/fallback",
						},
						{
							Provider: schemas.OpenAI,
							Model:    "backup",
						},
					}, capture.fallbacks)
				}
			})
		}
	}
}

func TestUnavailableCustomProviderHTTPGate(t *testing.T) {
	store := newTestStore(t)
	provider := transitionProvider("http://unused.example", "secret")
	provider.Config.CustomProviderConfig.BaseProviderType = "unsupported-base"
	require.NoError(t, store.CreateProvider(t.Context(), "unavailable-custom", provider))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.ErrorContains(t, engine.Err(), "unavailable-custom")

	for _, path := range []string{
		"/v1/chat/completions",
		"/v1/responses",
		"/v1/messages",
		"/anthropic/v1/messages",
		"/v1/messages/count_tokens",
		"/anthropic/v1/messages/count_tokens",
		"/v1/responses/input_tokens",
	} {
		for _, stream := range []bool{false, true} {
			if stream && (strings.Contains(path, "count_tokens") || strings.Contains(path, "input_tokens")) {
				continue
			}
			body, err := json.Marshal(map[string]any{
				"model":      "unavailable-custom/test",
				"input":      "hello",
				"messages":   []map[string]string{{"role": "user", "content": "hello"}},
				"max_tokens": 16,
				"stream":     stream,
			})
			require.NoError(t, err)

			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body))))

			require.Equal(t, http.StatusServiceUnavailable, response.Code, "%s stream=%t: %s", path, stream, response.Body.String())
			require.Contains(t, response.Body.String(), "being reconfigured")
			require.NotContains(t, response.Body.String(), "invalid_request_error")
		}
	}
}

func TestUpstreamBadRequestKeepsItsStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"failed to get config for provider upstream-input: provider upstream-input is being reconfigured"}}`))
	}))
	defer upstream.Close()
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "upstream-input", transitionProvider(upstream.URL, "secret")))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"upstream-input/test","messages":[{"role":"user","content":"hello"}]}`)))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "invalid_request_error")
}
