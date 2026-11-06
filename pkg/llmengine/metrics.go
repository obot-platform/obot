package llmengine

import (
	"net/http"
	"sync"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type gatewayMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	cost     *prometheus.CounterVec
	tokens   *prometheus.CounterVec
}

type metricsStart struct{}
type metricsAttempt struct {
	sync.Mutex
	start         time.Time
	counted       bool
	input, output int
	cost          float64
}

func newGatewayMetrics() *gatewayMetrics {
	m := &gatewayMetrics{registry: prometheus.NewRegistry()}
	labels := []string{"provider", "model", "key"}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "obot_llm_gateway_requests_total", Help: "Gateway provider attempts, including fallbacks but excluding internal retries."}, labels)
	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "obot_llm_gateway_attempt_duration_seconds", Help: "Inference attempt time until its first response or error."}, labels)
	m.cost = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "obot_llm_gateway_cost_total", Help: "Provider response cost in dollars."}, labels)
	m.tokens = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "obot_llm_gateway_tokens_total", Help: "Provider response tokens."}, append(append([]string{}, labels...), "direction"))
	m.registry.MustRegister(m.requests, m.latency, m.cost, m.tokens)
	return m
}

func (m *gatewayMetrics) GetName() string { return "obot-metrics" }

func (m *gatewayMetrics) Cleanup() error { return nil }

func (m *gatewayMetrics) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	// Discovery calls must not produce inference metrics or inherit a parent's attempt.
	ctx.SetValue(metricsStart{}, (*metricsAttempt)(nil))
	if req.ListModelsRequest == nil {
		ctx.SetValue(metricsStart{}, &metricsAttempt{start: time.Now()})
	}
	return req, nil, nil
}

func (m *gatewayMetrics) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, engineErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	state, ok := ctx.Value(metricsStart{}).(*metricsAttempt)
	if !ok || state == nil {
		return resp, engineErr, nil
	}
	state.Lock()
	defer state.Unlock()
	var route schemas.RoutingInfo
	if resp != nil {
		if extra := resp.GetExtraFields(); extra != nil {
			route = extra.RoutingInfo
		}
	}
	if engineErr != nil {
		route = engineErr.ExtraFields.RoutingInfo
	}
	labels := []string{string(route.Provider), route.Model, route.Key}
	if !state.counted {
		m.requests.WithLabelValues(labels...).Inc()
		m.latency.WithLabelValues(labels...).Observe(time.Since(state.start).Seconds())
		state.counted = true
	}
	if resp != nil {
		// Stream hooks may repeat cumulative usage and cost; add only increases.
		var usage *schemas.BifrostLLMUsage
		switch {
		case resp.ChatResponse != nil:
			usage = resp.ChatResponse.Usage
		case resp.ResponsesResponse != nil && resp.ResponsesResponse.Usage != nil:
			usage = resp.ResponsesResponse.Usage.ToBifrostLLMUsage()
		case resp.ResponsesStreamResponse != nil && resp.ResponsesStreamResponse.Response != nil && resp.ResponsesStreamResponse.Response.Usage != nil:
			usage = resp.ResponsesStreamResponse.Response.Usage.ToBifrostLLMUsage()
		case resp.EmbeddingResponse != nil:
			usage = resp.EmbeddingResponse.Usage
		}
		if usage != nil {
			if usage.PromptTokens > state.input {
				m.tokens.WithLabelValues(append(labels, "input")...).Add(float64(usage.PromptTokens - state.input))
				state.input = usage.PromptTokens
			}
			if usage.CompletionTokens > state.output {
				m.tokens.WithLabelValues(append(labels, "output")...).Add(float64(usage.CompletionTokens - state.output))
				state.output = usage.CompletionTokens
			}
		}
		if cost := ctx.CalculateCostBreakdown(resp); cost != nil && cost.TotalCost > state.cost {
			m.cost.WithLabelValues(labels...).Add(cost.TotalCost - state.cost)
			state.cost = cost.TotalCost
		}
	}

	return resp, engineErr, nil
}

// ServeMetrics exposes this replica's gateway inference metrics.
func (e *Engine) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(e.metrics.registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

func (m *gatewayMetrics) PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}
