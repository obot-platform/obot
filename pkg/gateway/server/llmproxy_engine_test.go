package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type fakeRoundTripper struct {
	resp *http.Response
}

func TestResolveBifrostRoute(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		header      map[string]string
		wantBase    string
		wantDialect llmtypes.Dialect
		wantErr     bool
	}{
		{
			name:        "anthropic messages",
			path:        "v1/messages",
			wantBase:    "/anthropic",
			wantDialect: llmtypes.DialectAnthropicMessages,
		},
		{
			name:        "anthropic token counting",
			path:        "v1/messages/count_tokens",
			wantBase:    "/anthropic",
			wantDialect: llmtypes.DialectAnthropicMessages,
		},
		{
			name:        "openai responses",
			path:        "v1/responses",
			wantBase:    "/",
			wantDialect: llmtypes.DialectOpenAIResponses,
		},
		{
			name:        "chat completions",
			path:        "v1/chat/completions",
			wantBase:    "/",
			wantDialect: llmtypes.DialectOpenAIChatCompletions,
		},
		{
			name:        "embeddings",
			path:        "v1/embeddings",
			wantBase:    "/",
			wantDialect: llmtypes.DialectOpenAIChatCompletions,
		},
		{
			name:    "legacy completions are not exposed",
			path:    "v1/completions",
			wantErr: true,
		},
		{
			name:    "image generation is not exposed",
			path:    "v1/images/generations",
			wantErr: true,
		},
		{
			name:     "openai model listing",
			path:     "v1/models",
			wantBase: "/",
		},
		{
			name:        "anthropic model listing",
			path:        "v1/models",
			header:      map[string]string{"anthropic-version": "2023-06-01"},
			wantBase:    "/anthropic",
			wantDialect: llmtypes.DialectAnthropicMessages,
		},
		{
			name:    "files are not exposed",
			path:    "v1/files",
			wantErr: true,
		},
		{
			name:    "responses retrieval is not exposed",
			path:    "v1/responses/resp_123",
			wantErr: true,
		},
		{
			name:    "bifrost management API is not reachable",
			path:    "api/providers",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/llm-proxy/"+tt.path, nil)
			req.SetPathValue("path", tt.path)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			route, err := resolveBifrostRoute(req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got route %+v", route)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if route.base != tt.wantBase || route.dialect != tt.wantDialect {
				t.Fatalf("got base %q dialect %q, want base %q dialect %q", route.base, route.dialect, tt.wantBase, tt.wantDialect)
			}
		})
	}
}

func TestChatCompletionsTokenUsageTracker(t *testing.T) {
	tracker := &chatCompletionsTokenUsageTracker{}
	tracker.addTokenUsage([]byte(`{"choices":[{"delta":{"content":"hi"}}],"usage":null}`))
	tracker.addTokenUsage([]byte(`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}}`))

	usage := tracker.getTokenUsage()
	if usage.InputTokens != 100 || usage.OutputTokens != 20 || usage.CacheReadTokens != 40 || usage.ThinkingTokens != 5 || usage.TotalTokens != 120 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestSanitizingTransport(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":                []string{"application/json"},
				"X-Bifrost-Provider":          []string{"openai"},
				"X-Bifrost-Resolved-Model":    []string{"gpt-4.1"},
				"X-Bifrost-Routing-Info-Key":  []string{"primary"},
				"Server":                      []string{"fasthttp"},
				"Access-Control-Allow-Origin": []string{"*"},
				"X-Request-Id":                []string{"abc"},
			},
			Body: io.NopCloser(strings.NewReader(`{"id":"c1","is_bifrost_error":false,"extra_fields":{"routing_info":{"key":"primary"}},"choices":[]}`)),
		}
		out, err := sanitizingTransport{next: fakeRoundTripper{resp: resp}}.RoundTrip(httptest.NewRequest(http.MethodPost, "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(out.Body)
		if strings.Contains(string(body), "extra_fields") || strings.Contains(string(body), "bifrost") || !strings.Contains(string(body), `"id":"c1"`) {
			t.Fatalf("unexpected body %s", body)
		}
		for name := range out.Header {
			if strings.Contains(strings.ToLower(name), "bifrost") || name == "Server" || strings.HasPrefix(name, "Access-Control-") {
				t.Fatalf("header %s was not removed", name)
			}
		}
		if out.Header.Get("X-Obot-LLM-Provider") != "openai" || out.Header.Get("X-Obot-LLM-Model") != "gpt-4.1" || out.Header.Get("X-Request-Id") != "abc" {
			t.Fatalf("unexpected headers %v", out.Header)
		}
	})

	t.Run("model list", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"openai/gpt-4.1"}],"key_statuses":[{"key_id":"k1","provider":"openai","status":"success"}]}`)),
		}
		out, err := sanitizingTransport{next: fakeRoundTripper{resp: resp}}.RoundTrip(httptest.NewRequest(http.MethodGet, "/v1/models", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(out.Body)
		if string(body) != `{"object":"list","data":[{"id":"openai/gpt-4.1"}]}` {
			t.Fatalf("got %s, want the list without the provider keys' statuses", body)
		}
	})

	t.Run("stream", func(t *testing.T) {
		stream := "data: {\"id\":\"c1\",\"extra_fields\":{\"routing_info\":{}},\"choices\":[]}\n\n" +
			"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"r1\",\"extra_fields\":{\"latency\":0}}}\n\n" +
			"event: ping\ndata: [DONE]\n\n"
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(stream)),
		}
		out, err := sanitizingTransport{next: fakeRoundTripper{resp: resp}}.RoundTrip(httptest.NewRequest(http.MethodPost, "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(out.Body)
		want := "data: {\"id\":\"c1\",\"choices\":[]}\n\n" +
			"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"r1\"}}\n\n" +
			"event: ping\ndata: [DONE]\n\n"
		if string(body) != want {
			t.Fatalf("got %q, want %q", body, want)
		}
	})
}

func (f fakeRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return f.resp, nil
}

func TestBudgetExceededResponse(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	exceeded := &client.LLMGatewayBudgetExceeded{
		Budget:  gatewaytypes.LLMGatewayBudget{Name: "Daily", PerUser: true},
		Limit:   gatewaytypes.LLMGatewayBudgetLimit{Metric: gatewaytypes.LLMGatewayBudgetMetricRequests, Amount: 3, Window: "1h"},
		Used:    3,
		ResetAt: now.Add(90 * time.Second),
	}
	tests := []struct {
		name     string
		path     string
		wantType string
	}{
		{name: "anthropic messages", path: "/api/llm-proxy/v1/messages", wantType: "rate_limit_error"},
		{name: "openai chat completions", path: "/api/llm-proxy/v1/chat/completions", wantType: "budget_exceeded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, data := budgetExceededResponse(tt.path, exceeded, now)
			if header.Get("X-Should-Retry") != "false" || header.Get("Retry-After") != "90" || header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected headers %v", header)
			}
			var body struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(data, &body); err != nil || body.Error.Type != tt.wantType || !strings.Contains(body.Error.Message, `"Daily"`) {
				t.Fatalf("unexpected body %s", data)
			}
		})
	}
}

func TestRewriteBudgetExceeded(t *testing.T) {
	exceeded := &client.LLMGatewayBudgetExceeded{
		Budget:  gatewaytypes.LLMGatewayBudget{Name: "Daily", PerUser: true},
		Limit:   gatewaytypes.LLMGatewayBudgetLimit{Metric: gatewaytypes.LLMGatewayBudgetMetricRequests, Amount: 3, Window: "1h"},
		Used:    3,
		ResetAt: time.Now().Add(time.Hour),
	}
	message, _ := json.Marshal(exceeded.Error())
	tests := []struct {
		name      string
		path      string
		body      string
		rewritten bool
	}{
		{
			name:      "chat completions refusal",
			path:      "/api/llm-proxy/v1/chat/completions",
			body:      `{"error":{"type":"budget_exceeded","message":` + string(message) + `}}`,
			rewritten: true,
		},
		{
			// The gateway writes Anthropic errors with the type for the status.
			name:      "messages refusal",
			path:      "/api/llm-proxy/v1/messages",
			body:      `{"type":"error","error":{"type":"rate_limit_error","message":` + string(message) + `}}`,
			rewritten: true,
		},
		{
			name: "provider's own rate limit on messages",
			path: "/api/llm-proxy/v1/messages",
			body: `{"type":"error","error":{"type":"rate_limit_error","message":"Number of requests has exceeded your rate limit"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Obot's API server set its per-second rate limit headers on the client's response.
			clientHeader := http.Header{}
			clientHeader.Set("X-RateLimit-Limit", "200")
			clientHeader.Set("X-RateLimit-Remaining", "199")
			clientHeader.Set("X-RateLimit-Reset", time.Now().UTC().Format(time.RFC1123))
			r := &responseModifier{gateway: &gatewayAttempts{budgets: map[string][]string{}, exceeded: exceeded}, clientHeader: clientHeader}
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(tt.body)),
				Request:    httptest.NewRequest(http.MethodPost, tt.path, nil),
			}
			r.rewriteBudgetExceeded(resp)
			body, _ := io.ReadAll(resp.Body)
			if got := resp.Header.Get("X-Should-Retry") == "false" && resp.Header.Get("Retry-After") != ""; got != tt.rewritten {
				t.Fatalf("refusal headers set = %v, want %v: %v", got, tt.rewritten, resp.Header)
			}
			if kept := len(clientHeader) > 0; kept == tt.rewritten {
				t.Fatalf("rate limit headers kept = %v on the client's response, want %v: %v", kept, !tt.rewritten, clientHeader)
			}
			if !tt.rewritten && string(body) != tt.body {
				t.Fatalf("a provider's rate limit was changed: %s", body)
			}
			if tt.rewritten && !strings.Contains(string(body), `\"Daily\"`) {
				t.Fatalf("the refusal doesn't name the budget: %s", body)
			}
		})
	}
}

func TestStripCallerCredentials(t *testing.T) {
	const oauth = "Bearer sk-ant-oat01-test"
	tests := []struct {
		name        string
		passthrough bool
		target      string
		auth        string
		wantAuth    string
	}{
		{
			name:        "subscription token kept for an Anthropic model",
			passthrough: true,
			target:      "anthropic/claude-haiku-5-5",
			auth:        oauth,
			wantAuth:    oauth,
		},
		{
			name:        "subscription token removed when passthrough is off",
			passthrough: false,
			target:      "anthropic/claude-haiku-5-5",
			auth:        oauth,
		},
		{
			name:        "subscription token never sent to another provider",
			passthrough: true,
			target:      "openai/gpt-5-mini",
			auth:        oauth,
		},
		{
			name:        "Obot API key removed",
			passthrough: true,
			target:      "anthropic/claude-haiku-5-5",
			auth:        "Bearer ok1-1-2-secret",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			req.Header.Set("Authorization", tt.auth)
			req.Header.Set(HeaderObotAPIKey, "ok1-1-2-secret")
			req.Header.Set("X-Api-Key", "ok1-1-2-secret")
			req.Header.Set("X-Bf-Vk", "sk-bf-other")
			model := &v1.Model{}
			model.Spec.Manifest.TargetModel = tt.target

			kept := stripCallerCredentials(req, model, tt.passthrough)

			if kept != (tt.wantAuth != "") {
				t.Errorf("reported the subscription token kept = %v, want %v", kept, tt.wantAuth != "")
			}
			if got := req.Header.Get("Authorization"); got != tt.wantAuth {
				t.Errorf("Authorization = %q, want %q", got, tt.wantAuth)
			}
			for _, h := range []string{HeaderObotAPIKey, "X-Api-Key", "X-Bf-Vk"} {
				if got := req.Header.Get(h); got != "" {
					t.Errorf("%s = %q, want it removed", h, got)
				}
			}
		})
	}
}

// An attempt the caller's Claude subscription pays for spends no budget, but only when Obot
// kept the subscription token for the gateway to forward.
func TestSubscriptionAttemptsSpendNoBudget(t *testing.T) {
	sonnet := &v1.Model{}
	sonnet.Spec.Manifest.TargetModel = "anthropic/claude-sonnet-4-5"
	attempts := &gatewayAttempts{budgets: map[string][]string{"stale": {"b1"}}, subscription: true}
	// The budget lookup isn't reached: the backend and proxy are nil.
	check := attempts.check(nil, nil, api.Context{}, sonnet)
	if err := check(context.Background(), guardrails.Attempt{Model: sonnet.Spec.Manifest.TargetModel, Subscription: true}); err != nil {
		t.Fatal(err)
	}
	if model, budgets := attempts.served(); model != sonnet.Spec.Manifest.TargetModel || len(budgets) != 0 {
		t.Fatalf("served %q with budgets %v, want %q with none", model, budgets, sonnet.Spec.Manifest.TargetModel)
	}
	if !attempts.paidBySubscription() {
		t.Fatal("the attempt isn't recorded as paid by the subscription")
	}
}

// A request carrying a caller's subscription token can't be routed to a provider other than
// Anthropic, which would receive the token.
func TestSubscriptionRequestsStayOnAnthropic(t *testing.T) {
	sonnet := &v1.Model{}
	sonnet.Spec.Manifest.TargetModel = "anthropic/claude-sonnet-4-5"
	attempts := &gatewayAttempts{budgets: map[string][]string{}, subscription: true}
	check := attempts.check(nil, nil, api.Context{}, sonnet)
	for _, routed := range []string{"my-anthropic/claude-sonnet-4-5", "openai/gpt-5-mini"} {
		err := check(context.Background(), guardrails.Attempt{Model: routed})
		var refused *guardrails.AttemptError
		if !errors.As(err, &refused) || refused.Status != http.StatusForbidden {
			t.Errorf("%s: got %v, want a 403 refusal", routed, err)
		}
	}
	if model, _ := attempts.served(); model != "" {
		t.Fatalf("admitted %q", model)
	}
	if !attempts.lastRefused() {
		t.Fatal("the refused attempt isn't reported as refused")
	}
	// An admitted attempt after a refused one served the request.
	if err := check(context.Background(), guardrails.Attempt{Model: sonnet.Spec.Manifest.TargetModel, Subscription: true}); err != nil {
		t.Fatal(err)
	}
	if attempts.lastRefused() {
		t.Fatal("the admitted attempt is reported as refused")
	}
}

func TestServedModelChange(t *testing.T) {
	opus := &v1.Model{}
	opus.Name = "m1-opus"
	opus.Spec.Manifest.TargetModel = "anthropic/claude-opus-4-8"
	resolve := func(served string) (*v1.Model, error) {
		if served == opus.Spec.Manifest.TargetModel {
			return opus, nil
		}
		return nil, fmt.Errorf("model %q not found", served)
	}
	served := func(provider, model string) http.Header {
		h := http.Header{}
		h.Set(headerServedProvider, provider)
		h.Set(headerServedModel, model)
		return h
	}

	tests := []struct {
		name   string
		header http.Header
		want   *v1.Model
	}{
		{
			name:   "served by the requested model",
			header: served("openai", "gpt-5-mini"),
		},
		{
			name:   "routed to another model",
			header: served("anthropic", "claude-opus-4-8"),
			want:   opus,
		},
		{
			name:   "served by a model Obot doesn't know",
			header: served("anthropic", "claude-unknown"),
		},
		{
			name:   "no served model reported",
			header: http.Header{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := servedModelChange(tt.header, "openai/gpt-5-mini", resolve); got != tt.want {
				t.Errorf("servedModelChange = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenUsageTrackerSetCost(t *testing.T) {
	cheap := types.ModelCost{Input: 0.25, Output: 2}
	expensive := types.ModelCost{Input: 5, Output: 25}
	tracker := &threadSafeTokenUsageTracker{inner: &chatCompletionsTokenUsageTracker{cost: cheap}}
	tracker.addTokenUsage([]byte(`{"usage":{"prompt_tokens":1000000,"completion_tokens":1000000}}`))

	// A response served by another model is costed at that model's prices.
	var served v1.Model
	served.Status.Cost = expensive
	tracker.setModelCost(served)
	if got := tracker.getTokenUsage().TotalSpend; got != 30 {
		t.Errorf("spend = %v, want 30 at the serving model's prices", got)
	}
	if tracker.costOverridden() {
		t.Error("the serving model's synced prices are not an override")
	}

	// An administrator's prices for the serving model are an override.
	served.Spec.Manifest.OverrideCost = &cheap
	tracker.setModelCost(served)
	if !tracker.costOverridden() {
		t.Error("the serving model's prices are overridden")
	}
}

func TestGatewayCost(t *testing.T) {
	attempts := &gatewayAttempts{budgets: map[string][]string{}}
	if _, ok := attempts.cost(); ok {
		t.Fatal("a response the gateway didn't price has no cost")
	}

	// A stream is priced as its usage grows; the whole response costs the most.
	attempts.recordCost(guardrails.Cost{Input: 0.03, CacheRead: 0.01})
	attempts.recordCost(guardrails.Cost{Input: 0.03, CacheRead: 0.01, CacheWrite: 0.005, Output: 0.2})
	attempts.recordCost(guardrails.Cost{Input: 0.03})
	cost, ok := attempts.cost()
	if !ok || cost.Output != 0.2 {
		t.Fatalf("cost = %+v, %v; want the whole response's", cost, ok)
	}

	// For a model without a price, the report with the most tokens is the whole response's.
	unpriced := &gatewayAttempts{budgets: map[string][]string{}}
	unpriced.recordCost(guardrails.Cost{InputTokens: 10})
	unpriced.recordCost(guardrails.Cost{InputTokens: 10, OutputTokens: 5})
	unpriced.recordCost(guardrails.Cost{InputTokens: 10, OutputTokens: 1})
	if c, _ := unpriced.cost(); c.OutputTokens != 5 {
		t.Errorf("unpriced usage = %+v, want the whole response's", c)
	}

	usage := withGatewayCost(gatewaytypes.TokenUsage{InputTokens: 100, OutputTokens: 10, TotalSpend: 99}, cost)
	want := gatewaytypes.TokenUsage{
		InputTokens:     100,
		OutputTokens:    10,
		InputSpend:      0.03,
		CacheReadSpend:  0.01,
		CacheWriteSpend: 0.005,
		OutputSpend:     0.2,
		TotalSpend:      0.23,
	}
	if usage != want {
		t.Errorf("usage = %+v, want %+v", usage, want)
	}
}

// The audit log records what the guardrails did to a request. The gateway checks the input
// again for every model it tries, so only the last input check is kept.
func TestGatewayAttemptsGuardrailFindings(t *testing.T) {
	attempts := &gatewayAttempts{budgets: map[string][]string{}}
	if findings := attempts.guardrailFindings(); len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}

	logged := guardrails.Detection{
		Stage:         guardrails.StageInput,
		PolicyID:      "r1",
		PolicyName:    "log secrets",
		GuardrailName: "secrets",
		GuardrailType: "secrets",
		Outcome:       "logged",
		Categories:    []string{"secret"},
	}
	attempts.recordDetections(guardrails.StageOutput, []guardrails.Detection{{
		Stage:         guardrails.StageOutput,
		PolicyID:      "r2",
		PolicyName:    "block pii",
		GuardrailName: "presidio",
		GuardrailType: "presidio",
		Outcome:       "blocked",
		Categories:    []string{"EMAIL_ADDRESS", "PHONE_NUMBER"},
	}})
	attempts.recordDetections(guardrails.StageInput, []guardrails.Detection{logged, logged})
	attempts.recordDetections(guardrails.StageInput, []guardrails.Detection{logged})

	want := []types.LLMAuditGuardrailFinding{
		{
			Stage:         guardrails.StageInput,
			PolicyID:      "r1",
			PolicyName:    "log secrets",
			GuardrailName: "secrets",
			GuardrailType: "secrets",
			Outcome:       "logged",
			Categories:    []string{"secret"},
		},
		{
			Stage:         guardrails.StageOutput,
			PolicyID:      "r2",
			PolicyName:    "block pii",
			GuardrailName: "presidio",
			GuardrailType: "presidio",
			Outcome:       "blocked",
			Categories:    []string{"EMAIL_ADDRESS", "PHONE_NUMBER"},
		},
	}
	got, _ := json.Marshal(attempts.guardrailFindings())
	wantJSON, _ := json.Marshal(want)
	if string(got) != string(wantJSON) {
		t.Fatalf("findings = %s, want %s", got, wantJSON)
	}
}

func TestWithGatewayUsage(t *testing.T) {
	cost := guardrails.Cost{
		Input:        0.02,
		Output:       0.1,
		InputTokens:  1000,
		OutputTokens: 200,
	}
	parsed := gatewaytypes.TokenUsage{InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200, TotalSpend: 99}
	override := types.ModelCost{Input: 1, Output: 10}

	tests := []struct {
		name       string
		usage      gatewaytypes.TokenUsage
		overridden bool
		want       gatewaytypes.TokenUsage
	}{
		{
			name:  "the gateway's price replaces Obot's",
			usage: parsed,
			want:  gatewaytypes.TokenUsage{InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200, InputSpend: 0.02, OutputSpend: 0.1, TotalSpend: 0.12},
		},
		{
			name:       "an administrator's cost override wins",
			usage:      parsed,
			overridden: true,
			want:       parsed,
		},
		{
			name: "a response the caller saw no usage for, such as one a guardrail blocked, uses the gateway's",
			want: gatewaytypes.TokenUsage{InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200, InputSpend: 0.02, OutputSpend: 0.1, TotalSpend: 0.12},
		},
		{
			name:       "and is priced at an override",
			overridden: true,
			want:       gatewaytypes.TokenUsage{InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200, InputSpend: 0.001, OutputSpend: 0.002, TotalSpend: 0.003},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model v1.Model
			if tt.overridden {
				model.Spec.Manifest.OverrideCost = &override
			}
			r := &responseModifier{tokenUsageTracker: newTokenUsageTrackerForDialect(model, llmtypes.DialectOpenAIChatCompletions)}
			got := r.withGatewayUsage(tt.usage, cost)
			const eps = 1e-12
			if got.InputTokens != tt.want.InputTokens || got.OutputTokens != tt.want.OutputTokens || got.TotalTokens != tt.want.TotalTokens ||
				math.Abs(got.InputSpend-tt.want.InputSpend) > eps || math.Abs(got.OutputSpend-tt.want.OutputSpend) > eps || math.Abs(got.TotalSpend-tt.want.TotalSpend) > eps {
				t.Errorf("usage = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Model listings include the Anthropic models listed for Claude subscriptions, which the
// gateway's own listing leaves out while the Anthropic provider has no enabled keys.
func TestAddListedModels(t *testing.T) {
	subscriptionModels := []llmengine.ListedModel{
		{ID: "anthropic/claude-haiku-4-5", Name: "Claude Haiku 4.5"},
		{ID: "anthropic/claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
	}
	tests := []struct {
		name      string
		anthropic bool
		status    int
		body      string
		wantIDs   []string
		wantBody  string
	}{
		{
			name:      "Anthropic format, nothing listed",
			anthropic: true,
			status:    http.StatusOK,
			body:      `{"data":[],"has_more":false}`,
			wantIDs:   []string{"claude-haiku-4-5", "claude-sonnet-4-5"},
		},
		{
			name:      "Anthropic format keeps listed models and repoints the cursors",
			anthropic: true,
			status:    http.StatusOK,
			body:      `{"data":[{"type":"model","id":"claude-haiku-4-5","display_name":"Haiku","created_at":"2025-10-01T00:00:00Z"}],"has_more":false,"first_id":"claude-haiku-4-5","last_id":"claude-haiku-4-5"}`,
			wantIDs:   []string{"claude-haiku-4-5", "claude-sonnet-4-5"},
		},
		{
			name:    "OpenAI format",
			status:  http.StatusOK,
			body:    `{"object":"list","data":[{"id":"openai/gpt-5","object":"model","created":1,"owned_by":"openai"}]}`,
			wantIDs: []string{"openai/gpt-5", "anthropic/claude-haiku-4-5", "anthropic/claude-sonnet-4-5"},
		},
		{
			name:      "not on a page that has more after it",
			anthropic: true,
			status:    http.StatusOK,
			body:      `{"data":[],"has_more":true}`,
			wantBody:  `{"data":[],"has_more":true}`,
		},
		{
			name:     "not on an error",
			status:   http.StatusInternalServerError,
			body:     `{"error":{"message":"boom"}}`,
			wantBody: `{"error":{"message":"boom"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}
			if err := addListedModels(resp, subscriptionModels, tt.anthropic); err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantBody != "" {
				if string(body) != tt.wantBody {
					t.Fatalf("body = %s, want it unchanged: %s", body, tt.wantBody)
				}
				return
			}
			var list struct {
				Data []struct {
					ID          string `json:"id"`
					Type        string `json:"type"`
					DisplayName string `json:"display_name"`
					Object      string `json:"object"`
					OwnedBy     string `json:"owned_by"`
				} `json:"data"`
				FirstID string `json:"first_id"`
				LastID  string `json:"last_id"`
			}
			if err := json.Unmarshal(body, &list); err != nil {
				t.Fatalf("decode %s: %v", body, err)
			}
			var ids []string
			for _, m := range list.Data {
				ids = append(ids, m.ID)
				if tt.anthropic && (m.Type != "model" || m.DisplayName == "") {
					t.Errorf("Anthropic-format entry %+v", m)
				}
				if !tt.anthropic && (m.Object != "model" || m.OwnedBy == "") {
					t.Errorf("OpenAI-format entry %+v", m)
				}
			}
			if strings.Join(ids, ",") != strings.Join(tt.wantIDs, ",") {
				t.Fatalf("listed %v, want %v", ids, tt.wantIDs)
			}
			if strings.Contains(tt.body, "last_id") && (list.FirstID != ids[0] || list.LastID != ids[len(ids)-1]) {
				t.Errorf("cursors %q..%q, want %q..%q", list.FirstID, list.LastID, ids[0], ids[len(ids)-1])
			}
			if resp.ContentLength != int64(len(body)) {
				t.Errorf("Content-Length %d, body %d bytes", resp.ContentLength, len(body))
			}
		})
	}
}
