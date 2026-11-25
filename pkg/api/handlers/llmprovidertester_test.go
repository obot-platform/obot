package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gserver "github.com/obot-platform/obot/pkg/gateway/server"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// providerTestKeys holds the IDs of the API keys created by newProviderTestGateway.
type providerTestKeys struct {
	valid      uint
	revoked    uint
	expired    uint
	noProxy    uint
	otherUsers uint
}

type providerTestUpstreamCall struct {
	path          string
	body          string
	authorization string
	cookie        string
	userAgent     string
}

type providerTestUpstream struct {
	mu    sync.Mutex
	calls []providerTestUpstreamCall
}

func (u *providerTestUpstream) record(t *testing.T, request *http.Request) {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read upstream body: %v", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls = append(u.calls, providerTestUpstreamCall{
		path:          request.URL.Path,
		body:          string(body),
		authorization: request.Header.Get("Authorization"),
		cookie:        request.Header.Get("Cookie"),
		userAgent:     request.UserAgent(),
	})
}

func (u *providerTestUpstream) snapshot() []providerTestUpstreamCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]providerTestUpstreamCall(nil), u.calls...)
}

func TestLLMProviderTesterChatStreamsReplyThroughLLMProxy(t *testing.T) {
	upstream := &providerTestUpstream{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		upstream.record(t, request)
		response.Header().Set("Content-Type", "text/event-stream")
		response.Header().Set(gserver.HeaderServedProvider, "openai")
		response.Header().Set(gserver.HeaderServedModel, "gpt-test")
		_, _ = io.WriteString(response, strings.Join([]string{
			`data: {"type":"response.created"}`,
			`data: {"type":"response.output_text.delta","delta":"hel"}`,
			`data: {"type":"response.output_text.delta","delta":"lo"}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n"))
	}))
	t.Cleanup(server.Close)

	gateway, keys := newProviderTestGateway(t)
	handler := NewLLMProviderTesterHandler(providerTestStorage(t), server.URL, server.Client())
	recorder := runLLMProviderTest(t, handler, gateway, "openai", "7", fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.valid))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	events := providerTestEvents(t, recorder.Body.String())
	if len(events) != 3 {
		t.Fatalf("events = %#v, want two deltas and a reply", events)
	}
	if events[0].Delta != "hel" || events[1].Delta != "lo" {
		t.Fatalf("deltas = %q, %q", events[0].Delta, events[1].Delta)
	}
	reply := events[2]
	if reply.Type != types.LLMProviderTestEventReply || reply.Reply != "hello" || reply.DurationMs <= 0 ||
		reply.Provider != "openai" || reply.ServedModel != "gpt-test" {
		t.Fatalf("reply = %#v", reply)
	}
	calls := upstream.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.path != "/api/llm-proxy/v1/responses" {
		t.Fatalf("upstream path = %q", call.path)
	}
	if call.authorization != "" {
		t.Fatalf("upstream authorization = %q, want none", call.authorization)
	}
	if call.cookie != "obot_access_token=admin-session" {
		t.Fatalf("upstream cookie = %q, want the caller's session", call.cookie)
	}
	if call.userAgent != types.LLMProviderTesterClientName {
		t.Fatalf("upstream user agent = %q", call.userAgent)
	}
	if !strings.Contains(call.body, `"model":"m1openai"`) || !strings.Contains(call.body, types.LLMProviderTesterDefaultMessage) ||
		strings.Contains(call.body, `"instructions"`) {
		t.Fatalf("upstream body = %s, want the model and default message without instructions", call.body)
	}
}

func TestLLMProviderTesterChatReturnsUpstreamErrorsAsIs(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "invalid upstream credential",
			status: http.StatusUnauthorized,
			body:   `{"error":{"message":"Incorrect API key provided: sk-***","type":"invalid_request_error"}}`,
			want:   `{"error":{"message":"Incorrect API key provided: sk-***","type":"invalid_request_error"}}`,
		},
		{
			name:   "model not found",
			status: http.StatusNotFound,
			body:   "The model `gpt-missing` does not exist\n",
			want:   "The model `gpt-missing` does not exist",
		},
		{
			name:   "empty body",
			status: http.StatusBadGateway,
			body:   "",
			want:   http.StatusText(http.StatusBadGateway),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set(gserver.HeaderServedProvider, "openai")
				response.WriteHeader(tt.status)
				_, _ = io.WriteString(response, tt.body)
			}))
			t.Cleanup(server.Close)

			gateway, keys := newProviderTestGateway(t)
			handler := NewLLMProviderTesterHandler(providerTestStorage(t), server.URL, server.Client())
			recorder := runLLMProviderTest(t, handler, gateway, "openai", "7", fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d,"message":"hi"}`, keys.valid))

			events := providerTestEvents(t, recorder.Body.String())
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one error", events)
			}
			event := events[0]
			if event.Type != types.LLMProviderTestEventError || event.Status != tt.status ||
				event.Error != tt.want || event.Provider != "openai" || event.DurationMs <= 0 {
				t.Fatalf("event = %#v", event)
			}
		})
	}
}

func TestLLMProviderTesterChatRejectsInvalidRequests(t *testing.T) {
	gateway, keys := newProviderTestGateway(t)
	tests := []struct {
		name     string
		provider string
		body     string
		status   int
	}{
		{
			name:     "missing API key",
			provider: "openai",
			body:     `{"model":"m1openai"}`,
			status:   http.StatusBadRequest,
		},
		{
			name:     "raw API key instead of an ID",
			provider: "openai",
			body:     `{"model":"m1openai","apiKey":"ok1-7-42-secretvalue"}`,
			status:   http.StatusBadRequest,
		},
		{
			name:     "unknown API key",
			provider: "openai",
			body:     `{"model":"m1openai","apiKeyId":999999}`,
			status:   http.StatusNotFound,
		},
		{
			name:     "another user's API key",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.otherUsers),
			status:   http.StatusNotFound,
		},
		{
			name:     "revoked API key",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.revoked),
			status:   http.StatusBadRequest,
		},
		{
			name:     "expired API key",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.expired),
			status:   http.StatusBadRequest,
		},
		{
			name:     "API key without LLM proxy access",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.noProxy),
			status:   http.StatusForbidden,
		},
		{
			name:     "missing model",
			provider: "openai",
			body:     fmt.Sprintf(`{"apiKeyId":%d}`, keys.valid),
			status:   http.StatusBadRequest,
		},
		{
			name:     "unknown model",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1missing","apiKeyId":%d}`, keys.valid),
			status:   http.StatusNotFound,
		},
		{
			name:     "inactive model",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1inactive","apiKeyId":%d}`, keys.valid),
			status:   http.StatusBadRequest,
		},
		{
			name:     "model of another provider",
			provider: "anthropic",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d}`, keys.valid),
			status:   http.StatusBadRequest,
		},
		{
			name:     "unknown field",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d,"stream":false}`, keys.valid),
			status:   http.StatusBadRequest,
		},
		{
			name:     "message too long",
			provider: "openai",
			body:     fmt.Sprintf(`{"model":"m1openai","apiKeyId":%d,"message":"%s"}`, keys.valid, strings.Repeat("a", types.LLMProviderTesterMaxMessageSize+1)),
			status:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewLLMProviderTesterHandler(providerTestStorage(t), "http://unused.invalid", &http.Client{
				Transport: mcpTesterRoundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatal("the LLM proxy must not be called")
					return nil, nil
				}),
			})

			request := httptest.NewRequest(http.MethodPost, "/api/llm-gateway/admin/providers/"+tt.provider+"/tester/chat", bytes.NewBufferString(tt.body))
			request.SetPathValue("provider", tt.provider)
			err := handler.Chat(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        request,
				GatewayClient:  gateway,
				User:           &kuser.DefaultInfo{Name: "admin", UID: "7"},
			})
			httpErr, ok := err.(*types.ErrHTTP)
			if !ok {
				t.Fatalf("error = %v, want HTTP error", err)
			}
			if httpErr.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", httpErr.Code, tt.status, httpErr.Message)
			}
		})
	}
}

// newProviderTestGateway returns a gateway client holding API keys of user 7 in
// each state the tester checks, plus a key of user 8.
func newProviderTestGateway(t *testing.T) (*gatewayclient.Client, providerTestKeys) {
	t.Helper()
	gateway := newEnforcementTestGatewayClient(t)
	create := func(userID uint, name string, expiresAt *time.Time, scopes gatewaytypes.APIKeyScopes) uint {
		t.Helper()
		created, err := gateway.CreateAPIKey(t.Context(), userID, name, "", expiresAt, scopes)
		if err != nil {
			t.Fatalf("create API key %q: %v", name, err)
		}
		return created.ID
	}
	proxy := gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true}
	past := time.Now().Add(-time.Hour)
	keys := providerTestKeys{
		valid:      create(7, "valid", nil, proxy),
		revoked:    create(7, "revoked", nil, proxy),
		expired:    create(7, "expired", &past, proxy),
		noProxy:    create(7, "no proxy", nil, gatewaytypes.APIKeyScopes{CanAccessAPI: true}),
		otherUsers: create(8, "other user's", nil, proxy),
	}
	if err := gateway.RevokeAPIKey(t.Context(), 7, keys.revoked); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}
	return gateway, keys
}

func runLLMProviderTest(t *testing.T, handler *LLMProviderTesterHandler, gateway *gatewayclient.Client, provider, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/llm-gateway/admin/providers/"+provider+"/tester/chat", bytes.NewBufferString(body))
	request.SetPathValue("provider", provider)
	request.Header.Set("Cookie", "obot_access_token=admin-session")
	recorder := httptest.NewRecorder()
	if err := handler.Chat(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		GatewayClient:  gateway,
		User:           &kuser.DefaultInfo{Name: "admin", UID: userID},
	}); err != nil {
		t.Fatal(err)
	}
	return recorder
}

func providerTestEvents(t *testing.T, body string) []types.LLMProviderTestEvent {
	t.Helper()
	var events []types.LLMProviderTestEvent
	for frame := range strings.SplitSeq(body, "\n\n") {
		data, ok := strings.CutPrefix(strings.TrimSpace(frame), "data: ")
		if !ok {
			continue
		}
		var event types.LLMProviderTestEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("unmarshal event %q: %v", data, err)
		}
		events = append(events, event)
	}
	return events
}

func providerTestStorage(t *testing.T) kclient.Client {
	t.Helper()
	model := func(name, target string, active bool) kclient.Object {
		return &v1.Model{
			Name:      name,
			Namespace: system.DefaultNamespace,
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					TargetModel:   target,
					ModelProvider: system.LLMGatewayModelProvider,
					Active:        active,
					Usage:         types.ModelUsageLLM,
					Dialect:       string(llmtypes.DialectOpenAIResponses),
				},
			},
		}
	}
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			model("m1openai", "openai/gpt-test", true),
			model("m1inactive", "openai/gpt-old", false),
		).
		Build()
}
