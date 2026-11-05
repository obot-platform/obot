package llmcore_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulTokenCountsReportServingMetadata(t *testing.T) {
	for _, mode := range []string{"routed", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "fallback" && body.Model == "routed-count" {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"primary unavailable"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"input_tokens":23}`)
			}))
			defer upstream.Close()
			store := newTestStore(t)
			for _, name := range []string{"openai", "anthropic"} {
				require.NoError(t, store.CreateProvider(t.Context(), name, llmcoreclient.Provider{
					Config: schemas.ProviderConfig{
						NetworkConfig: schemas.NetworkConfig{
							BaseURL:             upstream.URL,
							AllowPrivateNetwork: true,
							MaxRetries:          1,
							RetryBackoffInitial: time.Millisecond,
							RetryBackoffMax:     time.Millisecond,
						},
					},
					Keys: []schemas.Key{{ID: "key", Name: "key", Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
				}))
			}
			provider, model := "anthropic", "routed-count"
			rule := tables.TableRoutingRule{
				ID:            "count-route",
				Name:          "count-route",
				Scope:         "global",
				CelExpression: "true",
				Targets:       []tables.TableRoutingTarget{{Provider: &provider, Model: &model, Weight: 1}},
			}
			if mode == "fallback" {
				rule.ParsedFallbacks = []tables.RoutingFallback{{Provider: schemas.OpenAI, Model: "fallback-count"}}
			}
			require.NoError(t, store.PutRule(t.Context(), rule))
			engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
			require.NoError(t, err)
			defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
			for _, api := range []struct {
				path string
				body string
			}{
				{
					path: "/v1/responses/input_tokens",
					body: `{"model":"openai/original","input":"hello"}`,
				},
				{
					path: "/v1/messages/count_tokens",
					body: `{"model":"openai/original","messages":[{"role":"user","content":"hello"}]}`,
				},
				{
					path: "/anthropic/v1/messages/count_tokens",
					body: `{"model":"openai/original","messages":[{"role":"user","content":"hello"}]}`,
				},
			} {
				t.Run(api.path, func(t *testing.T) {
					var observed []schemas.BifrostResponseExtraFields
					ctx := llmcore.WithResponseObserver(t.Context(), func(extra schemas.BifrostResponseExtraFields) { observed = append(observed, extra) })
					response := httptest.NewRecorder()
					engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(api.body)).WithContext(ctx))
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.Len(t, observed, 1, "successful counts must notify the response observer")
					wantProvider, wantModel := schemas.Anthropic, "routed-count"
					if mode == "fallback" {
						wantProvider, wantModel = schemas.OpenAI, "fallback-count"
					}
					require.Equal(t, schemas.CountTokensRequest, observed[0].RequestType)
					require.Equal(t, wantProvider, observed[0].RoutingInfo.Provider)
					require.Equal(t, wantModel, observed[0].RoutingInfo.Model)
					require.Equal(t, mode == "fallback", observed[0].RoutingInfo.IsFallback)
					require.Equal(t, string(wantProvider), response.Header().Get("X-Obot-LLM-Provider"))
					require.Equal(t, wantModel, response.Header().Get("X-Obot-LLM-Model"))
					require.NotContains(t, response.Body.String(), "extra_fields")
					if strings.Contains(api.path, "/messages/") {
						require.JSONEq(t, `{"input_tokens":23}`, response.Body.String())
					} else {
						var body map[string]any
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
						require.Equal(t, float64(23), body["input_tokens"])
					}
				})
			}
		})
	}
}
