package llmengine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	"github.com/stretchr/testify/require"
)

func metricTotal(t *testing.T, metrics *gatewayMetrics, name string) float64 {
	t.Helper()
	families, err := metrics.registry.Gather()
	require.NoError(t, err)
	var total float64
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if metric.Counter != nil {
				total += metric.Counter.GetValue()
			} else if metric.Histogram != nil {
				total += float64(metric.Histogram.GetSampleCount())
			}
		}
	}
	return total
}

func TestGatewayMetricsInferenceAndRetries(t *testing.T) {
	e, ctx := startTestEngine(t)
	modeldatatest.Write(t, e.opts.DataDir)
	require.NoError(t, e.RefreshPricing(ctx))
	mock := mockOpenAI(t)
	t.Cleanup(mock.Close)
	var retries atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			raw, err := io.ReadAll(r.Body)
			var body struct {
				Model string `json:"model"`
			}
			if err != nil || json.Unmarshal(raw, &body) != nil {
				http.Error(w, "invalid fixture request", http.StatusBadRequest)
				return
			}
			if body.Model == "broken" || (body.Model == "retry" && retries.Add(1) == 1) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"message":"temporary failure"}}`))
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(raw)))
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{
		"provider": "openai",
		"network_config": map[string]any{
			"base_url":              up.URL,
			"allow_private_network": true,
			"max_retries":           1,
			"retry_backoff_initial": 1,
			"retry_backoff_max":     1,
		},
	}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{
		"name":  "metrics-key",
		"value": "secret",
	}))))
	_, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Zero(t, metricTotal(t, e.metrics, "obot_llm_gateway_requests_total"), "discovery is not inference")
	for _, body := range []string{
		`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"stream":true}`,
		`{"model":"openai/retry","messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"openai/broken","fallbacks":["openai/gpt-4o-mini"],"messages":[{"role":"user","content":"hello"}]}`,
	} {
		code, response := do(t, e, http.MethodPost, "/v1/chat/completions", body)
		require.Equal(t, http.StatusOK, code, response)
	}
	_, err = e.ChatCompletion(ctx, "openai/gpt-4o-mini", []schemas.ChatMessage{{
		Role:    schemas.ChatMessageRoleUser,
		Content: &schemas.ChatMessageContent{ContentStr: new("judge")},
	}})
	require.NoError(t, err)
	require.Equal(t, float64(6), metricTotal(t, e.metrics, "obot_llm_gateway_requests_total"))
	require.Equal(t, float64(6), metricTotal(t, e.metrics, "obot_llm_gateway_attempt_duration_seconds"))
	require.Equal(t, float64(39), metricTotal(t, e.metrics, "obot_llm_gateway_tokens_total"))
	require.Positive(t, metricTotal(t, e.metrics, "obot_llm_gateway_cost_total"))
	scrape := httptest.NewRecorder()
	e.ServeMetrics(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, scrape.Code)
	require.Contains(t, scrape.Body.String(), `provider="openai"`)
	require.Contains(t, scrape.Body.String(), `key="metrics-key"`)
	require.NotContains(t, scrape.Body.String(), "bifrost_")
}

func TestGatewayMetricsNestedCallsHaveIndependentState(t *testing.T) {
	m := newGatewayMetrics()
	parent := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	defer parent.Cancel()
	req := &schemas.BifrostRequest{ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.OpenAI, Model: "outer"}}
	_, _, err := m.PreLLMHook(parent, req)
	require.NoError(t, err)
	child := schemas.NewBifrostContext(parent, schemas.NoDeadline)
	defer child.Cancel()
	_, _, err = m.PreLLMHook(child, req)
	require.NoError(t, err)
	resp := &schemas.BifrostResponse{ChatResponse: &schemas.BifrostChatResponse{
		Usage: &schemas.BifrostLLMUsage{PromptTokens: 2, CompletionTokens: 1},
	}}
	_, _, err = m.PostLLMHook(child, resp, nil)
	require.NoError(t, err)
	_, _, err = m.PostLLMHook(parent, resp, nil)
	require.NoError(t, err)
	require.Equal(t, float64(2), metricTotal(t, m, "obot_llm_gateway_requests_total"))
	require.Equal(t, float64(6), metricTotal(t, m, "obot_llm_gateway_tokens_total"))
	_, _, err = m.PreLLMHook(child, &schemas.BifrostRequest{ListModelsRequest: &schemas.BifrostListModelsRequest{}})
	require.NoError(t, err)
	_, _, err = m.PostLLMHook(child, resp, nil)
	require.NoError(t, err)
	require.Equal(t, float64(2), metricTotal(t, m, "obot_llm_gateway_requests_total"))
}
