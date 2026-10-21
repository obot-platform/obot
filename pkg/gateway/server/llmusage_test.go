package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"k8s.io/apiserver/pkg/authentication/user"
)

func newUsageTestClient(t *testing.T) *client.Client {
	t.Helper()
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	// The LLM audit log is off: token usage is recorded without it.
	gatewayClient := client.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	return gatewayClient
}

func TestLLMUsageRecorderRecordsEveryRequestOnce(t *testing.T) {
	route := func() llmRoute {
		return llmRoute{Provider: "bedrock", Model: "claude-sonnet-4-5", Key: "east", Fallback: true}
	}
	for _, tt := range []struct {
		name         string
		status       int
		err          error
		usage        *types.TokenUsage
		route        func() llmRoute
		wantOutcome  string
		wantProvider string
		wantTokens   int64
	}{
		{
			name:         "success",
			status:       http.StatusOK,
			usage:        &types.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, TotalSpend: 0.1},
			route:        route,
			wantOutcome:  types.LLMAuditOutcomeSuccess,
			wantProvider: "bedrock",
			wantTokens:   15,
		},
		{
			name:         "error response",
			status:       http.StatusTooManyRequests,
			wantOutcome:  types.LLMAuditOutcomeError,
			wantProvider: "anthropic",
		},
		{
			name:         "canceled",
			err:          context.Canceled,
			wantOutcome:  types.LLMAuditOutcomeCanceled,
			wantProvider: "anthropic",
		},
		{
			name:         "gateway unreachable",
			status:       http.StatusBadGateway,
			err:          errors.New("connection refused"),
			route:        route,
			wantOutcome:  types.LLMAuditOutcomeError,
			wantProvider: "bedrock",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newUsageTestClient(t)
			r := newLLMUsageRecorder(&user.DefaultInfo{UID: "7"}, c, "anthropic/claude-sonnet-4-5", tt.route)
			r.recordResponseStatus(tt.status)
			if tt.usage != nil {
				r.setTokenUsage("bedrock/claude-sonnet-4-5", *tt.usage)
			}
			r.finish(tt.err)
			r.finish(nil)

			stats, err := c.TokenUsageStats(t.Context(), client.TokenUsageStatsOptions{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			if stats.Totals.Requests != 1 {
				t.Fatalf("recorded %d requests, want 1", stats.Totals.Requests)
			}
			if len(stats.Providers) != 1 || stats.Providers[0].Provider != tt.wantProvider {
				t.Errorf("providers = %+v, want %q", stats.Providers, tt.wantProvider)
			}
			if stats.Totals.TotalTokens != tt.wantTokens {
				t.Errorf("tokens = %d, want %d", stats.Totals.TotalTokens, tt.wantTokens)
			}
			wantSuccesses := int64(0)
			if tt.wantOutcome == types.LLMAuditOutcomeSuccess {
				wantSuccesses = 1
			}
			if stats.Totals.Successes != wantSuccesses {
				t.Errorf("successes = %d, want %d", stats.Totals.Successes, wantSuccesses)
			}
			wantFallbacks := int64(0)
			if tt.route != nil {
				wantFallbacks = 1
			}
			if stats.Totals.Fallbacks != wantFallbacks {
				t.Errorf("fallbacks = %d, want %d", stats.Totals.Fallbacks, wantFallbacks)
			}
			wantModel := "anthropic/claude-sonnet-4-5"
			if tt.usage != nil {
				wantModel = "bedrock/claude-sonnet-4-5"
			}
			if len(stats.TopModelsByRequests) != 1 || stats.TopModelsByRequests[0].Model != wantModel {
				t.Errorf("models = %+v, want %q", stats.TopModelsByRequests, wantModel)
			}
		})
	}
}

func TestRecordingResponseRecordsUnmeteredResponses(t *testing.T) {
	c := newUsageTestClient(t)
	usage := newLLMUsageRecorder(&user.DefaultInfo{UID: "7"}, c, "openai/gpt-5", nil)
	usage.recordResponseStatus(http.StatusInternalServerError)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"error":{"message":"upstream failed"}}`))}
	wrapRecordingResponse(resp, nil, usage, c)
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}

	stats, err := c.TokenUsageStats(t.Context(), client.TokenUsageStatsOptions{Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Totals.Requests != 1 || stats.Totals.Successes != 0 {
		t.Fatalf("totals = %+v, want one failed request", stats.Totals)
	}
}

func TestMeteredLLMEndpoint(t *testing.T) {
	for path, want := range map[string]bool{
		"v1/messages":               true,
		"v1/messages/count_tokens":  false,
		"v1/responses":              true,
		"v1/responses/input_tokens": false,
		"v1/chat/completions":       true,
		"/v1/embeddings/":           true,
		"v1/models":                 false,
	} {
		if got := meteredLLMEndpoint(path); got != want {
			t.Errorf("meteredLLMEndpoint(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestTokenCountRequestsAreNotLLMAudited(t *testing.T) {
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	audited := client.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, true)
	t.Cleanup(func() { _ = audited.Close() })
	notAudited := newUsageTestClient(t)

	for path, want := range map[string]bool{
		"v1/messages":              true,
		"v1/messages/count_tokens": false,
		"v1/chat/completions":      true,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+path, nil)
		req.SetPathValue("path", path)
		if got := newProxyLLMAuditRecorder(api.Context{Request: req, GatewayClient: audited}) != nil; got != want {
			t.Errorf("%s LLM audited = %v, want %v", path, got, want)
		}
		if newProxyLLMAuditRecorder(api.Context{Request: req, GatewayClient: notAudited}) != nil {
			t.Errorf("%s LLM audited with the LLM audit log off", path)
		}
	}
}

func TestServedRoute(t *testing.T) {
	reported := llmRoute{Provider: "anthropic", Model: "claude-opus-4-8", Key: "primary", Fallback: true}
	if got := servedRoute(nil, reported); got != reported {
		t.Errorf("without a gateway request, route = %+v, want %+v", got, reported)
	}

	attempts := &gatewayAttempts{budgets: map[string][]string{}}
	if got := servedRoute(attempts, reported); got != reported {
		t.Errorf("with no admitted attempt, route = %+v, want %+v", got, reported)
	}

	// The admitted model, as Obot names it, wins over the engine's wire name; the key still
	// belongs to the same provider.
	attempts.admitted = "anthropic/claude-opus-4-8-alias"
	if got := servedRoute(attempts, reported); got.Model != "claude-opus-4-8-alias" || got.Key != "primary" || !got.Fallback {
		t.Errorf("route = %+v", got)
	}

	attempts.admitted = "openai/gpt-5"
	if got := servedRoute(attempts, reported); got.Provider != "openai" || got.Model != "gpt-5" || got.Key != "" || got.Subscription {
		t.Errorf("route of another provider's attempt = %+v, want no key", got)
	}

	// The caller's Claude subscription paid for the attempt, with no gateway key.
	attempts.admitted, attempts.admittedSubscription = "anthropic/claude-opus-4-8", true
	if got := servedRoute(attempts, reported); !got.Subscription || got.Key != "" || got.Provider != "anthropic" {
		t.Errorf("route of a subscription attempt = %+v, want it paid by the subscription with no key", got)
	}
	// A refused last attempt served nothing, though the engine reports its route.
	attempts.admittedSubscription = false
	attempts.refused = true
	if got := servedRoute(attempts, reported); got != (llmRoute{}) {
		t.Errorf("route of a refused request = %+v, want none", got)
	}
}

func TestGatewayRoutingFromEngineResponses(t *testing.T) {
	t.Run("headers", func(t *testing.T) {
		routing := &gatewayRouting{}
		routing.recordHeaders(http.Header{
			"X-Bifrost-Provider":                 []string{"openai"},
			"X-Bifrost-Resolved-Model":           []string{"gpt-4.1-wire"},
			"X-Bifrost-Routing-Info-Provider":    []string{"azure"},
			"X-Bifrost-Routing-Info-Model":       []string{"gpt-4.1"},
			"X-Bifrost-Routing-Info-Key":         []string{"eastus"},
			"X-Bifrost-Routing-Info-Is-Fallback": []string{"true"},
		})
		if got, want := routing.get(), (llmRoute{Provider: "azure", Model: "gpt-4.1", Key: "eastus", Fallback: true}); got != want {
			t.Errorf("route = %+v, want %+v", got, want)
		}
	})

	t.Run("json and errors", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Bifrost-Provider": []string{"openai"}},
			Body:       io.NopCloser(strings.NewReader(`{"is_bifrost_error":true,"error":{"message":"rate limited"},"extra_fields":{"routing_info":{"provider":"anthropic","model":"claude-haiku-4-5","key":"backup","is_fallback":true}}}`)),
		}
		routing := &gatewayRouting{}
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = req.WithContext(withGatewayRouting(req.Context(), routing))
		out, err := sanitizingTransport{next: fakeRoundTripper{resp: resp}}.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(out.Body)
		if strings.Contains(string(body), "extra_fields") || strings.Contains(string(body), "backup") {
			t.Fatalf("engine fields reached the client: %s", body)
		}
		if got, want := routing.get(), (llmRoute{Provider: "anthropic", Model: "claude-haiku-4-5", Key: "backup", Fallback: true}); got != want {
			t.Errorf("route = %+v, want %+v", got, want)
		}
	})

	t.Run("stream", func(t *testing.T) {
		stream := "data: {\"id\":\"c1\",\"extra_fields\":{\"routing_info\":{\"provider\":\"openai\",\"model\":\"gpt-5\",\"key\":\"first\"}},\"choices\":[]}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"extra_fields\":{\"routing_info\":{\"provider\":\"openai\",\"model\":\"gpt-5\",\"key\":\"second\"}}}}\n\n" +
			"data: {\"id\":\"c1\",\"choices\":[]}\n\n" +
			"data: [DONE]\n\n"
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(stream)),
		}
		routing := &gatewayRouting{}
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = req.WithContext(withGatewayRouting(req.Context(), routing))
		out, err := sanitizingTransport{next: fakeRoundTripper{resp: resp}}.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(out.Body)
		if strings.Contains(string(body), "extra_fields") {
			t.Fatalf("engine fields reached the client: %s", body)
		}
		// The last event that names a route wins.
		if got := routing.get(); got.Key != "second" || got.Provider != "openai" || got.Fallback {
			t.Errorf("route = %+v", got)
		}
	})

	t.Run("no routing holder", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"extra_fields":{"routing_info":{"key":"k"}}}`)),
		}
		if _, err := (sanitizingTransport{next: fakeRoundTripper{resp: resp}}).RoundTrip(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestLLMAuditRecorderRecordsRoute(t *testing.T) {
	recorder := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", nil), &user.DefaultInfo{UID: "7"}, 1024)
	recorder.setModel("llm-gateway", "m1", "anthropic/claude-opus-4-8")
	recorder.setRoute(func() llmRoute {
		return llmRoute{Provider: "bedrock", Model: "claude-opus-4-8", Key: "us-east", Fallback: true}
	})
	recorder.recordResponseStatus(http.StatusOK)
	recorder.finish(newUsageTestClient(t), nil)

	log := recorder.log
	if log.TargetModel != "anthropic/claude-opus-4-8" || log.ServedProvider != "bedrock" || log.ServedModel != "claude-opus-4-8" || log.ProviderKey != "us-east" || !log.Fallback || log.ClaudeSubscription {
		t.Fatalf("audit log routing = target %q, served %q/%q, key %q, fallback %v, subscription %v", log.TargetModel, log.ServedProvider, log.ServedModel, log.ProviderKey, log.Fallback, log.ClaudeSubscription)
	}

	subscription := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", nil), &user.DefaultInfo{UID: "7"}, 1024)
	subscription.setRoute(func() llmRoute {
		return llmRoute{Provider: "anthropic", Model: "claude-opus-4-8", Subscription: true}
	})
	subscription.recordResponseStatus(http.StatusOK)
	subscription.finish(newUsageTestClient(t), nil)
	if !subscription.log.ClaudeSubscription || subscription.log.ProviderKey != "" {
		t.Fatalf("audit log of a subscription request: subscription %v, key %q", subscription.log.ClaudeSubscription, subscription.log.ProviderKey)
	}
}

// A response the caller's Claude subscription paid for cost Obot nothing: the audit log and
// token usage record its tokens without spend. One a gateway key paid for keeps its spend.
func TestSubscriptionResponsesRecordNoSpend(t *testing.T) {
	usage := types.TokenUsage{
		InputTokens:     100,
		CacheReadTokens: 40,
		OutputTokens:    10,
		TotalTokens:     110,
		InputSpend:      0.002,
		CacheReadSpend:  0.0004,
		OutputSpend:     0.001,
		TotalSpend:      0.003,
	}
	tests := []struct {
		name         string
		subscription bool
		wantSpend    float64
	}{
		{
			name:         "paid by the Claude subscription",
			subscription: true,
			wantSpend:    0,
		},
		{
			name:         "paid by a gateway key",
			subscription: false,
			wantSpend:    0.003,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audit := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", nil), &user.DefaultInfo{UID: "7"}, 1024)
			recorder := &llmUsageRecorder{}
			r := &responseModifier{
				user:   &user.DefaultInfo{UID: "7"},
				client: newUsageTestClient(t),
				gateway: &gatewayAttempts{
					budgets:              map[string][]string{},
					admitted:             "anthropic/claude-opus-4-8",
					admittedSubscription: tt.subscription,
				},
				audit: audit,
				usage: recorder,
			}
			r.recordUsage(usage)
			if audit.log.Spend != tt.wantSpend || recorder.usage.TotalSpend != tt.wantSpend {
				t.Fatalf("audit spend %v and usage spend %v, want %v", audit.log.Spend, recorder.usage.TotalSpend, tt.wantSpend)
			}
			if tt.subscription && (recorder.usage.InputSpend != 0 || recorder.usage.CacheReadSpend != 0 || recorder.usage.OutputSpend != 0) {
				t.Fatalf("usage of a subscription response has spend: %+v", recorder.usage)
			}
			if audit.log.InputTokens != 100 || audit.log.OutputTokens != 10 || recorder.usage.TotalTokens != 110 {
				t.Fatalf("token counts changed: audit %d in, %d out; usage %+v", audit.log.InputTokens, audit.log.OutputTokens, recorder.usage)
			}
		})
	}
}
