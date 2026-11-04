package mcpgateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/safehttp"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAuthorizationDecisions(t *testing.T) {
	tests := []struct {
		name         string
		response     string
		status       int
		wantHTTP     int
		wantDecision string
	}{
		{
			name:         "allow",
			response:     `{"decision":true}`,
			status:       200,
			wantHTTP:     200,
			wantDecision: "allow",
		},
		{
			name:         "deny",
			response:     `{"decision":false,"context":{"reason":"private policy detail"}}`,
			status:       200,
			wantHTTP:     403,
			wantDecision: "deny",
		},
		{
			name:         "missing decision",
			response:     `{"accept":true}`,
			status:       200,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "null decision",
			response:     `{"decision":null}`,
			status:       200,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "string decision",
			response:     `{"decision":"true"}`,
			status:       200,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "trailing JSON",
			response:     `{"decision":true}{}`,
			status:       200,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "PDP credential rejected",
			response:     `{"decision":true}`,
			status:       401,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "redirect",
			response:     `{"decision":true}`,
			status:       302,
			wantHTTP:     503,
			wantDecision: "error",
		},
		{
			name:         "oversize",
			response:     strings.Repeat(" ", maxAuthorizationResponse) + `{"decision":true}`,
			status:       200,
			wantHTTP:     503,
			wantDecision: "error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("unexpected bearer authentication")
				}
				if strings.Contains(string(body), "separate-secret") {
					t.Error("signing secret disclosed in payload")
				}
				signature := r.Header.Get("X-Obot-Signature-256")
				if !verifyAuthorizationSignature(body, signature, "separate-secret") {
					t.Error("receiver rejected authorization signature")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if verifyAuthorizationSignature(append(body, ' '), signature, "separate-secret") ||
					verifyAuthorizationSignature(body, signature, "wrong-secret") {
					t.Error("signature accepted altered bytes or wrong secret")
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()
			a := testAuthorizer(server)
			runner := new(scriptedMCPHookRunner)
			hooks := mcp.Hooks{
				{Name: "*", Targets: []mcp.HookTarget{{Target: "content/first"}}},
				{Name: "*", Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "filter-1", URL: server.URL}},
				{Name: "*", Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "filter-1", URL: server.URL}},
			}
			processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_customer","arguments":{"secret":"never disclose"}}}`), runner, hooks, nil, nil, nil, nil, nil, a)
			if err != nil {
				t.Fatal(err)
			}
			body, blocked, blockErr := processor.blockedRequest()
			if hookHTTPStatus(blockErr) != tt.wantHTTP || blocked != (tt.wantHTTP != 200) {
				t.Fatalf("blocked=%v status=%d error=%v", blocked, hookHTTPStatus(blockErr), blockErr)
			}
			if calls != 1 {
				t.Fatalf("evaluations=%d; overlapping selectors must deduplicate", calls)
			}
			if blocked {
				if runner.callCount() != 0 {
					t.Fatal("content ran before authorization")
				}
				var response mcp.Message
				if json.Unmarshal(body, &response) != nil || response.ID != float64(7) {
					t.Fatalf("lost request id: %s", body)
				}
				if strings.Contains(string(body), "private policy") || strings.Contains(string(body), "separate-secret") {
					t.Fatal("leaked policy detail")
				}
			} else if runner.callCount() != 1 {
				t.Fatal("content not invoked after allow")
			}
			// Verify the exact status that the existing audit adapter receives.
			status, _ := a.evaluate(t.Context(), hooks[1].Authorization, mcp.Message{Method: "tools/list"})
			audit := auditHookStatuses([]hookStatus{status})[0]
			if audit.Status != tt.wantDecision || audit.FilterID != "filter-1" || audit.EvaluationID == "" || audit.Mapping != authorizationMapping || audit.DurationMS < 1 {
				t.Fatalf("incomplete audit status: %+v", audit)
			}
		})
	}
}

func testAuthorizer(server *httptest.Server) *hookAuthorizer {
	return &hookAuthorizer{
		subject:   authorizationEntity{Type: "user", ID: "user-1"},
		serverID:  "crm-server",
		authority: "https://obot.example",
		client:    func(string) (*http.Client, error) { return server.Client(), nil },
		secret:    func(context.Context, string, string) (string, error) { return "separate-secret", nil },
	}
}

func TestAuthorizationEnvelopeAndMethodCoverage(t *testing.T) {
	tests := []struct {
		method       string
		params       string
		identifier   string
		resourceType string
		action       string
	}{
		{
			method:       "tools/call",
			params:       `{"name":"get_customer","arguments":{"private":"value"},"_meta":{"subject":"spoofed"}}`,
			identifier:   "get_customer",
			resourceType: "mcp-server",
			action:       "tools/call",
		},
		{
			method:       "resources/read",
			params:       `{"uri":"crm://customers"}`,
			identifier:   "crm://customers",
			resourceType: "mcp-server",
			action:       "resources/read",
		},
		{
			method:       "prompts/get",
			params:       `{"name":"summarize"}`,
			identifier:   "summarize",
			resourceType: "mcp-server",
			action:       "prompts/get",
		},
		{
			method:       "tools/list",
			params:       `{}`,
			resourceType: "mcp-server",
			action:       "tools/list",
		},
		{
			method:       "initialize",
			params:       `{}`,
			resourceType: "mcp-server",
			action:       "initialize",
		},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var envelope authorizationRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "private") || strings.Contains(string(body), "spoofed") {
					t.Error("unapproved content disclosed")
				}
				if err := json.Unmarshal(body, &envelope); err != nil {
					t.Error(err)
				}
				_, _ = io.WriteString(w, `{"decision":true}`)
			}))
			defer server.Close()
			a := testAuthorizer(server)
			a.catalogID = "default"
			a.catalogEntryID = "crm-entry"
			hook := &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: server.URL}
			p := &hookProcessor{ctx: t.Context(), authorizer: a, hooks: mcp.Hooks{{Name: tt.method, Params: map[string]string{"name": tt.identifier}, Authorization: hook}}}
			result := p.authorize(mcp.Message{ID: 1, Method: tt.method, Params: json.RawMessage(tt.params)}, hookOriginClient)
			if result.err != nil || len(result.statuses) != 1 {
				t.Fatalf("not authorized: %+v", result)
			}
			if envelope.Subject.ID != "user-1" || envelope.Resource.ID != "crm-server" || envelope.Resource.Type != tt.resourceType || envelope.Action.Name != tt.action {
				t.Fatalf("wrong envelope: %+v", envelope)
			}
			if envelope.Context.Obot.Version != 1 || envelope.Context.Obot.Authority != a.authority || envelope.Context.Obot.Mapping != "obot-mcp-requests-v2" || envelope.Context.Obot.CatalogID != "default" || envelope.Context.Obot.CatalogEntryID != "crm-entry" {
				t.Fatal("missing mapping authority")
			}
		})
	}
}

func TestAuthorizationScopeAndFailures(t *testing.T) {
	a := &hookAuthorizer{
		subject:  authorizationEntity{ID: "agent-1", Type: "agent"},
		serverID: "crm",
		secret: func(context.Context, string, string) (string, error) {
			return "", errors.New("private credential error")
		},
	}
	hook := &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: "https://example.com"}
	p := &hookProcessor{ctx: t.Context(), authorizer: a, hooks: mcp.Hooks{{Name: "tools/call", Params: map[string]string{"name": "get_customer"}, Authorization: hook}}}
	message := mcp.Message{ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"get_customer"}`)}
	if result := p.authorize(message, hookOriginClient); hookHTTPStatus(result.err) != 503 {
		t.Fatal("missing credential did not block")
	}
	if result := p.authorize(message, hookOriginServer); len(result.statuses) != 0 {
		t.Fatal("server message authorized")
	}
	message.ID = nil
	if result := p.authorize(message, hookOriginClient); len(result.statuses) != 0 {
		t.Fatal("notification authorized")
	}
	message.ID = 1
	message.Params = json.RawMessage(`{"name":"other"}`)
	if result := p.authorize(message, hookOriginClient); len(result.statuses) != 0 {
		t.Fatal("selector ignored")
	}
}

func TestAuthorizationCannotBeRetargetedByContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"decision":true}`) }))
	defer server.Close()
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		input.Mutated = true
		input.Message.Params = json.RawMessage(`{"name":"delete_customer"}`)
		return input, true, nil
	}}
	processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_customer"}}`), runner,
		mcp.Hooks{{Name: "*", Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: server.URL}, Targets: []mcp.HookTarget{{Target: "content/mutate"}}}}, nil, nil, nil, nil, nil, testAuthorizer(server))
	if err != nil {
		t.Fatal(err)
	}
	_, blocked, blockErr := processor.blockedRequest()
	if !blocked || hookHTTPStatus(blockErr) != 503 {
		t.Fatalf("retargeted request allowed: %v", blockErr)
	}
}

func TestAuthorizationDenialUsesRequestAudit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"decision":false}`) }))
	defer server.Close()
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":9,"method":"tools/list"}`)
	collector := new(recordingProxyAuditCollector)
	auditor, err := newProxyAudit(request, map[string]string{"mcpID": "crm-server", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	processor, err := newHookProcessor(request, new(scriptedMCPHookRunner),
		mcp.Hooks{{Name: "*", Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: server.URL}}}, nil, auditor, nil, nil, nil, testAuthorizer(server))
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	body, blocked, blockErr := processor.blockedRequest()
	if !blocked {
		t.Fatal("expected denial")
	}
	auditor.recordBlockedRequest(body, blockErr)
	if len(collector.entries) != 2 {
		t.Fatalf("audit records=%d", len(collector.entries))
	}
	statuses := collector.entries[0].WebhookStatuses
	if len(statuses) != 1 || statuses[0].FilterID != "f1" || statuses[0].Status != "deny" {
		t.Fatalf("lost evaluation: %+v", statuses)
	}
	if collector.entries[1].ResponseStatus != http.StatusForbidden {
		t.Fatalf("wrong audit HTTP status: %+v", collector.entries[1])
	}
}

func TestAuthorizationCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"decision":true}`) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	status, err := testAuthorizer(server).evaluate(ctx, &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: server.URL}, mcp.Message{Method: "tools/list"})
	if hookHTTPStatus(err) != 503 || status.status != "error" {
		t.Fatalf("cancellation permitted request: %v", err)
	}
}

func TestContentCannotRetargetIntoAnAuthorizationSelector(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		input.Mutated = true
		input.Message.Params = json.RawMessage(`{"name":"protected"}`)
		return input, true, nil
	}}
	hooks := mcp.Hooks{
		{Name: "tools/call", Params: map[string]string{"name": "protected"}, Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: "https://policy.example"}},
		{Name: "*", Targets: []mcp.HookTarget{{Target: "content/mutate"}}},
	}
	processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"public"}}`), runner, hooks, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, blocked, blockErr := processor.blockedRequest()
	if !blocked || hookHTTPStatus(blockErr) != 503 {
		t.Fatal("content bypassed authorization selector")
	}
}

func TestAuthorizationDoesNotDependOnContentRunner(t *testing.T) {
	hooks := mcp.Hooks{{Name: "*", Authorization: &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: "https://policy.example"}}}
	processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), nil, hooks, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, blocked, blockErr := processor.blockedRequest()
	if !blocked || hookHTTPStatus(blockErr) != 503 {
		t.Fatal("missing runner silently disabled authorization")
	}
}

// Use the receiver's raw-body verification contract, independently of the sender.
func verifyAuthorizationSignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	actual, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	expected := hmac.New(sha256.New, []byte(secret))
	_, _ = expected.Write(body)
	return hmac.Equal(actual, expected.Sum(nil))
}

func TestAuthorizationMissingSigningSecretNeverSends(t *testing.T) {
	a := &hookAuthorizer{
		subject:  authorizationEntity{Type: "user", ID: "user-1"},
		serverID: "crm",
		secret:   func(context.Context, string, string) (string, error) { return "", nil },
		client: func(string) (*http.Client, error) {
			t.Fatal("attempted unsigned authorization request")
			return nil, nil
		},
	}
	status, err := a.evaluate(t.Context(), &mcp.AuthorizationHook{SigningEnabled: true, FilterID: "f1", URL: "https://policy.example"}, mcp.Message{Method: "tools/list"})
	if hookHTTPStatus(err) != 503 || status.status != "error" {
		t.Fatal("missing signing secret did not fail closed")
	}
}

func TestUnsignedAuthorizationStillEnforcesDecisions(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(strconv.FormatBool(allowed), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Obot-Signature-256") != "" || r.Header.Get("Authorization") != "" {
					t.Error("unsigned callback included authentication headers")
				}
				_, _ = fmt.Fprintf(w, `{"decision":%t}`, allowed)
			}))
			defer server.Close()
			a := testAuthorizer(server)
			a.secret = func(context.Context, string, string) (string, error) {
				t.Fatal("unsigned callback attempted credential lookup")
				return "", nil
			}
			status, err := a.evaluate(t.Context(), &mcp.AuthorizationHook{FilterID: "f1", URL: server.URL}, mcp.Message{Method: "tools/list"})
			if allowed && (err != nil || status.status != "allow") {
				t.Fatalf("unsigned allow failed: %v", err)
			}
			if !allowed && (hookHTTPStatus(err) != 403 || status.status != "deny") {
				t.Fatalf("unsigned denial not enforced: %v", err)
			}
		})
	}
}

func TestAuthorizationRateLimitResponse(t *testing.T) {
	for _, tt := range []struct {
		name        string
		response    string
		wantHTTP    int
		wantMessage string
		wantRetry   string
	}{
		{
			name:        "rate limit with retry",
			response:    `{"decision":false,"context":{"obot":{"reason":"rate_limited","retry_after_seconds":30},"reason":"private detail"}}`,
			wantHTTP:    429,
			wantMessage: "Rate limit exceeded. Try again in 30 seconds.",
			wantRetry:   "30",
		},
		{
			name:        "rate limit without retry",
			response:    `{"decision":false,"context":{"obot":{"reason":"rate_limited"}}}`,
			wantHTTP:    429,
			wantMessage: "Rate limit exceeded. Try again later.",
		},
		{
			name:        "legacy deny",
			response:    `{"decision":false}`,
			wantHTTP:    403,
			wantMessage: "Request denied by authorization policy",
		},
		{
			name:        "unknown reason does not disclose details",
			response:    `{"decision":false,"context":{"obot":{"reason":"private detail","retry_after_seconds":30}}}`,
			wantHTTP:    403,
			wantMessage: "Request denied by authorization policy",
		},
		{
			name:        "invalid negative retry fails closed",
			response:    `{"decision":false,"context":{"obot":{"reason":"rate_limited","retry_after_seconds":-1}}}`,
			wantHTTP:    503,
			wantMessage: "Authorization evaluation unavailable",
		},
		{
			name:        "excessive retry fails closed",
			response:    `{"decision":false,"context":{"obot":{"reason":"rate_limited","retry_after_seconds":86401}}}`,
			wantHTTP:    503,
			wantMessage: "Authorization evaluation unavailable",
		},
		{
			name:     "allow ignores denial hint",
			response: `{"decision":true,"context":{"obot":{"reason":"rate_limited","retry_after_seconds":30}}}`,
			wantHTTP: 200,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()
			runner := new(scriptedMCPHookRunner)
			processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo"}}`), runner,
				mcp.Hooks{{Name: "*", Authorization: &mcp.AuthorizationHook{FilterID: "f1", URL: server.URL}}}, nil, nil, nil, nil, nil, testAuthorizer(server))
			if err != nil {
				t.Fatal(err)
			}
			body, blocked, blockErr := processor.blockedRequest()
			if hookHTTPStatus(blockErr) != tt.wantHTTP || blocked != (tt.wantHTTP != 200) {
				t.Fatalf("status=%d blocked=%v", hookHTTPStatus(blockErr), blocked)
			}
			headers := make(http.Header)
			setAuthorizationResponseHeaders(headers, blockErr)
			if headers.Get("Retry-After") != tt.wantRetry {
				t.Fatalf("headers=%v", headers)
			}
			if tt.wantHTTP == 429 && headers.Get("Cache-Control") != "no-store" {
				t.Fatal("missing cache control")
			}
			if !blocked {
				return
			}
			var message mcp.Message
			if err := json.Unmarshal(body, &message); err != nil {
				t.Fatal(err)
			}
			if message.ID != float64(7) || message.Error.Message != tt.wantMessage {
				t.Fatalf("body=%s", body)
			}
			if tt.wantHTTP == 429 {
				var details struct {
					Reason string `json:"reason"`
					Retry  int    `json:"retry_after_seconds"`
				}
				if err := json.Unmarshal(message.Error.Data, &details); err != nil {
					t.Fatal(err)
				}
				if details.Reason != "rate_limited" {
					t.Fatalf("details=%+v", details)
				}
				if tt.wantRetry != "" && strconv.Itoa(details.Retry) != tt.wantRetry {
					t.Fatalf("details=%+v", details)
				}
			} else if len(message.Error.Data) != 0 {
				t.Fatal("leaked decision context")
			}
		})
	}
}

func TestAuthorizationActionProperties(t *testing.T) {
	for _, tt := range []struct {
		method string
		params string
		want   map[string]any
	}{
		{
			method: "tools/call",
			params: `{"name":"echo","arguments":{"secret":"private"},"_meta":{"tool":"spoofed"}}`,
			want:   map[string]any{"tool": "echo"},
		},
		{
			method: "prompts/get",
			params: `{"name":"summarize","arguments":{"secret":"private"}}`,
			want:   map[string]any{"prompt": "summarize"},
		},
		{
			method: "resources/read",
			params: `{"uri":"crm://customer/1"}`,
			want:   map[string]any{"uri": "crm://customer/1"},
		},
		{
			method: "resources/subscribe",
			params: `{"uri":"crm://customer/1"}`,
			want:   map[string]any{"uri": "crm://customer/1"},
		},
		{
			method: "resources/unsubscribe",
			params: `{"uri":"crm://customer/1"}`,
			want:   map[string]any{"uri": "crm://customer/1"},
		},
		{
			method: "completion/complete",
			params: `{"ref":{"type":"ref/prompt","name":"summarize","uri":"private"},"argument":{"name":"style","value":"private"},"context":{"arguments":{"private":"value"}}}`,
			want:   map[string]any{"ref": map[string]string{"type": "ref/prompt", "name": "summarize"}, "argument": "style"},
		},
		{
			method: "completion/complete",
			params: `{"ref":{"type":"ref/resource","uri":"crm://customers/{id}","name":"private"},"argument":{"name":"id","value":"private"}}`,
			want:   map[string]any{"ref": map[string]string{"type": "ref/resource", "uri": "crm://customers/{id}"}, "argument": "id"},
		},
	} {
		t.Run(tt.method, func(t *testing.T) {
			got, err := mcpHookActionProperties(mcp.Message{Method: tt.method, Params: json.RawMessage(tt.params)})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
	for _, method := range []string{"tools/list", "prompts/list", "resources/list", "resources/templates/list", "initialize", "ping", "unknown/method"} {
		t.Run(method, func(t *testing.T) {
			var envelope authorizationRequest
			properties, err := mcpHookActionProperties(mcp.Message{Method: method, Params: json.RawMessage(`{"name":"private","uri":"private"}`)})
			if err != nil {
				t.Fatal(err)
			}
			envelope.Action.Properties = properties
			body, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "properties") || strings.Contains(string(body), "catalog_id") || strings.Contains(string(body), "catalog_entry_id") || strings.Contains(string(body), "request") {
				t.Fatalf("unexpected optional properties: %s", body)
			}
		})
	}
}

func TestAuthorizationCompletionMutationGuard(t *testing.T) {
	original := `{"ref":{"type":"ref/prompt","name":"summary"},"argument":{"name":"style","value":"private"}}`
	for _, tt := range []struct {
		name    string
		params  string
		blocked bool
	}{
		{
			name:    "reference name",
			params:  `{"ref":{"type":"ref/prompt","name":"other"},"argument":{"name":"style","value":"private"}}`,
			blocked: true,
		},
		{
			name:    "reference type and URI",
			params:  `{"ref":{"type":"ref/resource","uri":"crm://other"},"argument":{"name":"style","value":"private"}}`,
			blocked: true,
		},
		{
			name:    "argument name",
			params:  `{"ref":{"type":"ref/prompt","name":"summary"},"argument":{"name":"other","value":"private"}}`,
			blocked: true,
		},
		{
			name:   "argument value redaction",
			params: `{"ref":{"type":"ref/prompt","name":"summary"},"argument":{"name":"style","value":"[redacted]"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"decision":true}`) }))
			defer server.Close()
			runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
				input.Mutated = true
				input.Message.Params = json.RawMessage(tt.params)
				return input, true, nil
			}}
			req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"completion/complete","params":`+original+`}`)
			processor, err := newHookProcessor(req, runner, mcp.Hooks{{Name: "*", Authorization: &mcp.AuthorizationHook{FilterID: "f1", URL: server.URL}, Targets: []mcp.HookTarget{{Target: "content/mutate"}}}}, nil, nil, nil, nil, nil, testAuthorizer(server))
			if err != nil {
				t.Fatal(err)
			}
			_, blocked, blockErr := processor.blockedRequest()
			if blocked != tt.blocked {
				t.Fatalf("blocked=%v error=%v", blocked, blockErr)
			}
		})
	}
}

func TestAuthorizationIgnoresUnrelatedParams(t *testing.T) {
	for _, tt := range []struct {
		method     string
		params     string
		identifier string
	}{
		{
			method:     "tools/call",
			params:     `{"name":"blocked-tool","arguments":{},"uri":123,"ref":false,"argument":[]}`,
			identifier: "blocked-tool",
		},
		{
			method:     "resources/read",
			params:     `{"uri":"crm://blocked","name":123,"ref":false}`,
			identifier: "crm://blocked",
		},
	} {
		t.Run(tt.method, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var envelope authorizationRequest
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Error(err)
				}
				key := "tool"
				if tt.method == "resources/read" {
					key = "uri"
				}
				if envelope.Action.Properties[key] != tt.identifier {
					t.Errorf("lost selector: %+v", envelope.Action)
				}
				_, _ = io.WriteString(w, `{"decision":false}`)
			}))
			defer server.Close()
			message := mcp.Message{ID: 1, Method: tt.method, Params: json.RawMessage(tt.params)}
			processor := &hookProcessor{ctx: t.Context(), authorizer: testAuthorizer(server), hooks: mcp.Hooks{{Name: tt.method, Params: map[string]string{"name": tt.identifier}, Authorization: &mcp.AuthorizationHook{FilterID: "f1", URL: server.URL}}}}
			result := processor.authorize(message, hookOriginClient)
			if calls != 1 || hookHTTPStatus(result.err) != 403 {
				t.Fatalf("calls=%d error=%v", calls, result.err)
			}
		})
	}
}

func TestAuthorizationInvalidSelectorNeverSends(t *testing.T) {
	a := &hookAuthorizer{subject: authorizationEntity{Type: "user", ID: "user-1"}, serverID: "crm", client: func(string) (*http.Client, error) { t.Fatal("sent incomplete action to PDP"); return nil, nil }}
	for _, params := range []string{`{}`, `{"name":123}`, `{"name":null}`} {
		status, err := a.evaluate(t.Context(), &mcp.AuthorizationHook{FilterID: "f1", URL: "https://policy.example"}, mcp.Message{Method: "tools/call", Params: json.RawMessage(params)})
		if hookHTTPStatus(err) != 503 || status.status != "error" {
			t.Fatalf("params=%s status=%+v err=%v", params, status, err)
		}
	}
}

func TestAuthorizationTransportReuseAndShutdown(t *testing.T) {
	var connections atomic.Int32
	closed := make(chan struct{}, 4)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"decision":true}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	handler := &Handler{authorizationClient: safehttp.NewClient(safehttp.Options{})}
	defer handler.Close()
	for range 3 {
		// Each incoming MCP request creates an authorizer, but must reuse transport.
		authorizer := handler.authorizer(api.Context{User: &user.DefaultInfo{UID: "user-1"}}, mcp.ServerConfig{MCPServerName: "crm"})
		_, err := authorizer.evaluate(t.Context(), &mcp.AuthorizationHook{FilterID: "f1", URL: server.URL}, mcp.Message{Method: "tools/list"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("opened %d connections for 3 callbacks", got)
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("handler close retained idle authorization connection")
	}
}

// Tool search wraps the operation in call_tool. Authorization must match the
// resolved tool even when no content-filter runner is available.
func TestAuthorizationUsesResolvedGenericToolWithoutRunner(t *testing.T) {
	evaluations := 0
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evaluations++
		var envelope authorizationRequest
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
		}
		if envelope.Action.Name != "tools/call" || envelope.Action.Properties["tool"] != "get_customer" {
			t.Errorf("unexpected resolved action: %#v", envelope.Action)
		}
		_, _ = io.WriteString(w, `{"decision":false}`)
	}))
	defer policy.Close()
	for _, selector := range []string{"get_customer", "call_tool"} {
		evaluations = 0
		request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"get_customer","revision":"r1","arguments":{"id":"123"}}}}`)
		hooks := mcp.Hooks{{
			Name:          "tools/call",
			Params:        map[string]string{"name": selector},
			Authorization: &mcp.AuthorizationHook{FilterID: "policy", URL: policy.URL},
		}}
		resolved := &resolvedToolCall{name: "get_customer", revision: "r1", arguments: json.RawMessage(`{"id":"123"}`)}
		processor, err := newHookProcessor(request, nil, hooks, nil, nil, nil, resolved, nil, testAuthorizer(policy))
		if err != nil {
			t.Fatal(err)
		}
		_, blocked, blockErr := processor.blockedRequest()
		if !blocked || hookHTTPStatus(blockErr) != http.StatusForbidden || evaluations != 1 {
			t.Fatalf("blocked=%v error=%v evaluations=%d", blocked, blockErr, evaluations)
		}
	}
}
