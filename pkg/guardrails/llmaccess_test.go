package guardrails

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/llmengine"
)

// TestGatewayChecksRoutedModelAccess checks that a caller is only served by models they may
// use, whichever model the gateway's routing rules or fallbacks chose.
func TestGatewayChecksRoutedModelAccess(t *testing.T) {
	gw, up := startGateway(t, &fakeStore{})
	ctx := context.Background()
	for _, rule := range []map[string]any{
		{
			"name":           "premium to opus",
			"enabled":        true,
			"cel_expression": `"x-tier" in headers && headers["x-tier"] == "premium"`,
			"priority":       1,
			"scope":          "global",
			"targets":        []map[string]any{{"provider": "anthropic", "model": "claude-opus-4-8", "weight": 1}},
		},
		{
			"name":           "trial to opus, falling back",
			"enabled":        true,
			"cel_expression": `"x-tier" in headers && headers["x-tier"] == "trial"`,
			"priority":       2,
			"scope":          "global",
			"targets":        []map[string]any{{"provider": "anthropic", "model": "claude-opus-4-8", "weight": 1}},
			"fallbacks":      []string{"openai/gpt-4o-mini"},
		},
	} {
		if err := gw.Do(ctx, http.MethodPost, "/api/governance/routing-rules", rule, nil); err != nil {
			t.Fatal(err)
		}
	}

	// The caller may use OpenAI models only.
	openAIOnly := func(_ context.Context, attempt Attempt) error {
		if strings.HasPrefix(attempt.Model, "openai/") {
			return nil
		}
		return &AttemptError{Status: http.StatusForbidden, Type: "permission_denied", Message: fmt.Sprintf("user does not have permission to use model %q", attempt.Model)}
	}
	body := `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`

	tests := []struct {
		name       string
		tier       string
		access     AttemptCheck
		wantStatus int
		wantModel  string
		wantError  string
	}{
		{
			name:       "requested model",
			access:     openAIOnly,
			wantStatus: http.StatusOK,
			wantModel:  "gpt-4o-mini",
		},
		{
			name:       "routed to a model the caller may not use",
			tier:       "premium",
			access:     openAIOnly,
			wantStatus: http.StatusForbidden,
			wantError:  `user does not have permission to use model \"anthropic/claude-opus-4-8\"`,
		},
		{
			name:       "routed to a forbidden model, served by an allowed fallback",
			tier:       "trial",
			access:     openAIOnly,
			wantStatus: http.StatusOK,
			wantModel:  "gpt-4o-mini",
		},
		{
			name:       "routed to a model whose budget is spent",
			tier:       "premium",
			access:     budgetSpentFor("anthropic/"),
			wantStatus: http.StatusTooManyRequests,
			wantError:  `"type":"budget_exceeded"`,
		},
		{
			name:       "routed to a model whose budget is spent, served by a fallback with budget",
			tier:       "trial",
			access:     budgetSpentFor("anthropic/"),
			wantStatus: http.StatusOK,
			wantModel:  "gpt-4o-mini",
		},
		{
			name:       "routed to a model the caller may use",
			tier:       "premium",
			access:     func(context.Context, Attempt) error { return nil },
			wantStatus: http.StatusOK,
			wantModel:  "claude-opus-4-8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(up.prompts)
			code, out := callWithAccess(t, gw, body, tt.tier, tt.access)
			if code != tt.wantStatus {
				t.Fatalf("status %d, want %d: %s", code, tt.wantStatus, out)
			}
			if tt.wantError != "" && !strings.Contains(out, tt.wantError) {
				t.Fatalf("response %s, want it to explain %s", out, tt.wantError)
			}
			up.lock.Lock()
			sent := strings.Join(up.prompts[before:], "\n")
			up.lock.Unlock()
			if strings.Contains(sent, "claude-opus-4-8") && tt.wantModel != "claude-opus-4-8" {
				t.Fatalf("a forbidden model reached the provider: %s", sent)
			}
			if tt.wantModel != "" && !strings.Contains(sent, `"`+tt.wantModel+`"`) {
				t.Fatalf("want the request served by %s, the provider got: %s", tt.wantModel, sent)
			}
		})
	}
}

// budgetSpentFor refuses models with prefix as an exhausted budget would.
func budgetSpentFor(prefix string) AttemptCheck {
	return func(_ context.Context, attempt Attempt) error {
		if strings.HasPrefix(attempt.Model, prefix) {
			return &AttemptError{Status: http.StatusTooManyRequests, Type: "budget_exceeded", Message: "budget \"Opus\" exhausted"}
		}
		return nil
	}
}

// An unexpected failure while checking an attempt is an internal error, not a refusal.
func TestGatewayAttemptCheckFailure(t *testing.T) {
	gw, _ := startGateway(t, &fakeStore{})
	failing := func(context.Context, Attempt) error { return fmt.Errorf("informer not synced: secret detail") }
	code, out := callWithAccess(t, gw, `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, "", failing)
	if code != http.StatusInternalServerError || strings.Contains(out, "secret detail") {
		t.Fatalf("got %d %s, want a 500 without the internal error", code, out)
	}
}

func callWithAccess(t *testing.T, gw *llmengine.Engine, body, tier string, access AttemptCheck) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if tier != "" {
		req.Header.Set("X-Tier", tier)
	}
	token, release := BindRequest(Identity{UserID: "alice"}, RequestHooks{AttemptCheck: access})
	defer release()
	req.Header.Set(HeaderRequestToken, token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b)
}

// The gateway reports an attempt as paid by the caller's Claude subscription only when it
// forwards the caller's OAuth token instead of a configured key: Claude Code calling an
// Anthropic model with a subscription token. Any other attempt, including a fallback to
// another provider, uses a configured key.
func TestGatewayFlagsSubscriptionAttempts(t *testing.T) {
	gw, _ := startGateway(t, &fakeStore{})
	const (
		claudeCLI = "claude-cli/2.1.0 (external, cli)"
		oauth     = "Bearer sk-ant-oat01-test"
	)
	messages := `{"model":"anthropic/claude-sonnet-4-5","max_tokens":100,"messages":[{"role":"user","content":"hello"}]%s}`

	tests := []struct {
		name         string
		headers      map[string]string
		extra        string
		refuse       string
		wantAttempts []Attempt
	}{
		{
			name:         "Claude Code with a subscription token",
			headers:      map[string]string{"User-Agent": claudeCLI, "Authorization": oauth},
			wantAttempts: []Attempt{{Model: "anthropic/claude-sonnet-4-5", Subscription: true}},
		},
		{
			name:         "Claude Code with a configured key",
			headers:      map[string]string{"User-Agent": claudeCLI},
			wantAttempts: []Attempt{{Model: "anthropic/claude-sonnet-4-5"}},
		},
		{
			name:         "another client with a subscription token",
			headers:      map[string]string{"User-Agent": "curl/8.0", "Authorization": oauth},
			wantAttempts: []Attempt{{Model: "anthropic/claude-sonnet-4-5"}},
		},
		{
			name:    "a fallback to another provider",
			headers: map[string]string{"User-Agent": claudeCLI, "Authorization": oauth},
			extra:   `,"fallbacks":["openai/gpt-4o-mini"]`,
			refuse:  "anthropic/",
			wantAttempts: []Attempt{
				{Model: "anthropic/claude-sonnet-4-5", Subscription: true},
				{Model: "openai/gpt-4o-mini"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts []Attempt
			record := func(_ context.Context, attempt Attempt) error {
				attempts = append(attempts, attempt)
				if tt.refuse != "" && strings.HasPrefix(attempt.Model, tt.refuse) {
					return &AttemptError{Status: http.StatusTooManyRequests, Type: "budget_exceeded", Message: "budget exhausted"}
				}
				return nil
			}
			req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(fmt.Sprintf(messages, tt.extra)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			token, release := BindRequest(Identity{UserID: "alice"}, RequestHooks{AttemptCheck: record})
			defer release()
			req.Header.Set(HeaderRequestToken, token)
			gw.ServeHTTP(httptest.NewRecorder(), req)
			if fmt.Sprint(attempts) != fmt.Sprint(tt.wantAttempts) {
				t.Fatalf("attempts %+v, want %+v", attempts, tt.wantAttempts)
			}
		})
	}
}

// A request the provider rejects as invalid isn't sent on to fallbacks, which would reject it
// too: the caller gets the provider's 400, and no other provider receives the request.
func TestGatewayDoesNotFallBackAfterInvalidRequest(t *testing.T) {
	gw, up := startGateway(t, &fakeStore{})
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"` + invalidRequest + `"}],"fallbacks":["anthropic/claude-sonnet-4-5"]}`,
		},
		{
			name: "anthropic messages",
			path: "/anthropic/v1/messages",
			body: `{"model":"anthropic/claude-sonnet-4-5","max_tokens":100,"messages":[{"role":"user","content":"` + invalidRequest + `"}],"fallbacks":["openai/gpt-4o-mini"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up.lock.Lock()
			before := len(up.prompts)
			up.lock.Unlock()
			var attempts []string
			record := func(_ context.Context, attempt Attempt) error {
				attempts = append(attempts, attempt.Model)
				return nil
			}
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			token, release := BindRequest(Identity{UserID: "alice"}, RequestHooks{AttemptCheck: record})
			defer release()
			req.Header.Set(HeaderRequestToken, token)
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want the provider's 400: %s", rec.Code, rec.Body)
			}
			up.lock.Lock()
			sent := len(up.prompts) - before
			up.lock.Unlock()
			if sent != 1 || len(attempts) != 1 {
				t.Fatalf("the upstream got %d requests for attempts %v, want only the primary's", sent, attempts)
			}
		})
	}
}

// A model no key serves is refused by the gateway before any provider sees the request, so a
// fallback can still serve it.
func TestGatewayFallsBackWhenNoKeySupportsModel(t *testing.T) {
	gw, up := startGateway(t, &fakeStore{})
	ctx := context.Background()
	if err := gw.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
		"provider":       "mistral",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := gw.Do(ctx, http.MethodPost, "/api/providers/mistral/keys", map[string]any{"name": "km", "value": "sk-m-test", "models": []string{"mistral-large-latest"}, "weight": 1}, nil); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chat completions",
			path: "/v1/chat/completions",
			body: `{"model":"mistral/mistral-small-latest","messages":[{"role":"user","content":"hi"}],"fallbacks":["openai/gpt-4o-mini"]}`,
		},
		{
			name: "anthropic messages",
			path: "/anthropic/v1/messages",
			body: `{"model":"mistral/mistral-small-latest","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"fallbacks":["anthropic/claude-sonnet-4-5"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts []string
			record := func(_ context.Context, attempt Attempt) error {
				attempts = append(attempts, attempt.Model)
				return nil
			}
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			token, release := BindRequest(Identity{UserID: "alice"}, RequestHooks{AttemptCheck: record})
			defer release()
			req.Header.Set(HeaderRequestToken, token)
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, want the fallback's 200 (attempts %v): %s", rec.Code, attempts, rec.Body)
			}
		})
	}
}
