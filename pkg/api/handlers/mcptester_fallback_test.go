package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/tidwall/gjson"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	fallbackChatBody = `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`
)

type fakeTesterProviders struct {
	configured bool
	err        error
	calls      int
}

type fakeTesterLicense struct {
	key string
	err error
}

func (p *fakeTesterProviders) HasModelProvider(context.Context) (bool, error) {
	p.calls++
	return p.configured, p.err
}

func (l *fakeTesterLicense) LicenseKey(context.Context) (string, error) {
	return l.key, l.err
}

func (*fakeTesterLicense) MachineFingerprint() string {
	return "persisted-machine"
}

func newFallbackTestHandler(t *testing.T, providers *fakeTesterProviders, licenseSource *fakeTesterLicense, upstream *httptest.Server) *MCPTesterHandler {
	t.Helper()

	server := mcpTesterServer("user-1")
	server.Spec.Manifest.Runtime = types.RuntimeNPX
	server.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{Package: "@org/test-server@1.0", Args: []string{"--secret=hidden"}}

	endpoint, err := mcptester.ParseModelProxyURL(upstream.URL, true)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewMCPTesterHandler(mcpTesterStorage(t, server, true), &fakeMCPTesterServerResolver{server: *server}, nil, fakeMCPTesterModelAccess{allowed: true}, "https://obot.invalid", nil, MCPTesterFallbackOptions{
		URL:       endpoint,
		Providers: providers,
		License:   licenseSource,
		Settings:  newHandlerTestGateway(t),
	})
	handler.httpClient = &http.Client{Transport: mcpTesterRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected gateway call")
		return nil, errors.New("unexpected gateway call")
	})}

	return handler
}

func TestTesterFallbackRechecksProvidersAndLicense(t *testing.T) {
	var auth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/responses" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Obot-Machine-Fingerprint") != "persisted-machine" {
			t.Errorf("unexpected outbound request: %v %v", r.URL, r.Header)
		}

		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "model").String() != mcptester.FallbackModel || gjson.GetBytes(body, "reasoning.effort").String() != "high" {
			t.Errorf("unexpected body: %s", body)
		}

		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\"}\n\n")
	}))
	defer upstream.Close()

	providers := &fakeTesterProviders{}
	licenseSource := &fakeTesterLicense{key: "license-one"}
	handler := newFallbackTestHandler(t, providers, licenseSource, upstream)
	for _, key := range []string{"license-one", "license-two"} {
		licenseSource.key = key
		response := runMCPTesterChat(t, handler, "user-1", fallbackChatBody)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"type":"completion"`) {
			t.Fatalf("response: %d %s", response.Code, response.Body)
		}
	}

	var gatewayCalls int
	handler.httpClient = &http.Client{Transport: mcpTesterRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		gatewayCalls++
		if !strings.HasPrefix(req.URL.Path, "/api/llm-proxy/") || req.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("incorrect configured-provider request: %v", req)
		}

		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))}, nil
	})}

	providers.configured = true
	if response := runMCPTesterChat(t, handler, "user-1", fallbackChatBody); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}

	// Configured-provider model access failures do not permit fallback.
	handler.modelResolver = fakeMCPTesterModelAccess{allowed: false}
	assertMCPTesterError(t, runMCPTesterChat(t, handler, "user-1", fallbackChatBody), http.StatusForbidden, types.MCPTesterErrorModelUnavailable)

	providers.configured = false
	if response := runMCPTesterChat(t, handler, "user-1", fallbackChatBody); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}

	providers.err = errors.New("pending configuration")
	assertMCPTesterError(t, runMCPTesterChat(t, handler, "user-1", fallbackChatBody), http.StatusServiceUnavailable, types.MCPTesterErrorModelUnavailable)

	providers.err = nil
	licenseSource.key = ""
	assertMCPTesterError(t, runMCPTesterChat(t, handler, "user-1", fallbackChatBody), http.StatusServiceUnavailable, types.MCPTesterErrorLicenseRequired)

	licenseSource.err = errors.New("database read failed")
	assertMCPTesterError(t, runMCPTesterChat(t, handler, "user-1", fallbackChatBody), http.StatusServiceUnavailable, types.MCPTesterErrorProvider)
	if strings.Join(auth, ",") != "Bearer license-one,Bearer license-two,Bearer license-two" || gatewayCalls != 1 || providers.calls != 8 {
		t.Fatalf("auth=%v gateway calls=%d resolver calls=%d", auth, gatewayCalls, providers.calls)
	}

	// Disabled fallback leaves the existing model error, regardless of absence.
	handler.fallback.URL = nil
	assertMCPTesterError(t, runMCPTesterChat(t, handler, "user-1", fallbackChatBody), http.StatusForbidden, types.MCPTesterErrorModelUnavailable)
}

func TestTesterFallbackPreservesMCPAuthorization(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()

	providers := &fakeTesterProviders{}
	handler := newFallbackTestHandler(t, providers, &fakeTesterLicense{key: "license"}, upstream)

	assertMCPTesterError(t, runMCPTesterChat(t, handler, "another-user", fallbackChatBody), http.StatusForbidden, types.MCPTesterErrorAccessDenied)
	if calls != 0 || providers.calls != 0 {
		t.Fatal("unauthorized request reached fallback resolution")
	}
}

func TestTesterFallbackSafeErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantCode   types.MCPTesterErrorCode
		retryable  bool
	}{
		{
			name:       "missing license",
			status:     http.StatusUnauthorized,
			body:       `{"error":{"message":"private detail"}}`,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   types.MCPTesterErrorLicenseRequired,
		},
		{
			name:       "invalid license",
			status:     http.StatusForbidden,
			body:       `{"error":{"message":"private detail"}}`,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   types.MCPTesterErrorLicenseRequired,
		},
		{
			name:       "daily quota",
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"code":"daily_token_quota_exceeded","quota":{"reset_at":"2026-09-10T00:00:00Z"}}}`,
			wantStatus: http.StatusTooManyRequests,
			wantCode:   types.MCPTesterErrorQuotaExceeded,
		},
		{
			name:       "validation rate limit",
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"code":"validation_rate_limit_exceeded"}}`,
			wantStatus: http.StatusBadGateway,
			wantCode:   types.MCPTesterErrorProvider,
			retryable:  true,
		},
		{
			name:       "unavailable",
			status:     http.StatusServiceUnavailable,
			body:       "private detail",
			wantStatus: http.StatusBadGateway,
			wantCode:   types.MCPTesterErrorProvider,
			retryable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer upstream.Close()

			handler := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "license"}, upstream)
			response := runMCPTesterChat(t, handler, "user-1", fallbackChatBody)
			assertMCPTesterError(t, response, tt.wantStatus, tt.wantCode)

			var payload types.MCPTesterErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}

			if payload.Error.Retryable != tt.retryable || strings.Contains(payload.Error.Message, "private detail") {
				t.Fatalf("unsafe error: %#v", payload)
			}

			if tt.wantCode == types.MCPTesterErrorQuotaExceeded && !strings.Contains(payload.Error.Message, "2026-09-10T00:00:00Z") {
				t.Fatal("missing quota reset time")
			}
		})
	}
}

func newTesterAuditClient(t *testing.T) (*gatewayclient.Client, *gatewaydb.DB) {
	t.Helper()

	storage, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}

	db, err := gatewaydb.New(storage.DB.DB, storage.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}

	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })

	return client, db
}

func TestTesterFallbackPersistsAuditWithoutMetering(t *testing.T) {
	client, db := newTesterAuditClient(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, key := range []string{"Cookie", "X-Request-Id", "X-User-Id", "X-Obot-MCP-URL"} {
			if r.Header.Get(key) != "" {
				t.Errorf("forwarded browser header %s", key)
			}
		}

		w.Header().Set("X-Obot-Machine-Fingerprint", "private-fingerprint")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-test\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":42,\"output_tokens\":7}}}\n\n")
	}))
	defer upstream.Close()

	handler := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "installation-secret"}, upstream)
	handler.fallback.GatewayClient = client

	request := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", strings.NewReader(fallbackChatBody))
	request.SetPathValue("mcp_server_id", "ms1tester")
	for key, value := range map[string]string{"Authorization": "Bearer browser-secret", "Cookie": "session=browser-cookie", "X-Obot-Machine-Fingerprint": "browser-fingerprint", "X-Obot-MCP-URL": "https://browser-chosen.example", "X-User-Id": "browser-identity"} {
		request.Header.Set(key, value)
	}

	response := httptest.NewRecorder()
	if err := handler.Chat(api.Context{Request: request, ResponseWriter: response, User: &kuser.DefaultInfo{UID: "user-1"}}); err != nil {
		t.Fatal(err)
	}

	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}

	var logs []gatewaytypes.LLMAuditLog
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		var err error
		logs, _, err = client.GetLLMAuditLogs(t.Context(), gatewayclient.LLMAuditLogOptions{WithSensitiveFields: true})
		if err != nil {
			t.Fatal(err)
		}

		if len(logs) > 0 {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	if len(logs) != 1 {
		t.Fatalf("audit count = %d", len(logs))
	}

	log := logs[0]
	if log.UserID != "user-1" || log.ModelProvider != "model-proxy" || log.TargetModel != mcptester.FallbackModel || log.ReasoningEffort != "high" || log.InputTokens != 42 || log.OutputTokens != 7 || log.Outcome != gatewaytypes.LLMAuditOutcomeSuccess || log.ResponseID != "resp-test" || log.MessagePolicyTriggered {
		t.Fatalf("unexpected audit: %#v", log)
	}

	raw, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}

	for _, secret := range []string{"installation-secret", "browser-secret", "browser-cookie", "browser-fingerprint", "private-fingerprint"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("audit leaked %s", secret)
		}
	}

	var count int64
	if err := db.WithContext(t.Context()).Model(&gatewaytypes.RunTokenActivity{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatal("fallback updated gateway usage")
	}
}

func TestVersionExposesProviderAndLicenseBooleans(t *testing.T) {
	client := newHandlerTestGateway(t)
	licenseProvider, err := license.NewProvider(t.Context(), client, license.Config{})
	if err != nil {
		t.Fatal(err)
	}

	providers := &fakeTesterProviders{}
	endpoint, err := mcptester.ParseModelProxyURL(mcptester.DefaultModelProxyURL, false)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewVersionHandler(VersionHandlerOptions{
		GatewayClient:         client,
		StorageClient:         fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(),
		LicenseProvider:       licenseProvider,
		ProviderConfiguration: providers,
		ModelProxyURL:         endpoint,
		ModelProxySettings:    client,
	})

	t.Setenv("OBOT_SERVER_VERSIONS", "hasModelProvider=bad,hasValidLicense=bad,mcpTesterFallbackAvailable=bad")

	for _, configured := range []bool{false, true} {
		providers.configured = configured
		response := httptest.NewRecorder()
		if err := handler.GetVersion(api.Context{Request: httptest.NewRequest(http.MethodGet, "/api/version", nil), ResponseWriter: response}); err != nil {
			t.Fatal(err)
		}

		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}

		if result["hasModelProvider"] != configured || result["hasValidLicense"] != false || result["mcpTesterFallbackAvailable"] != false {
			t.Fatalf("incorrect version: %v", result)
		}
	}

	providers.err = errors.New("unreadable configuration")
	result, err := handler.getVersionResponse(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if result["hasModelProvider"] != nil || result["mcpTesterFallbackAvailable"] != false || result["obot"] == nil || result["hasValidLicense"] != false {
		t.Fatalf("lost version fields or advertised fallback during provider lookup failure: %v", result)
	}

	providers.err = nil
	providers.configured = false
	handler.ModelProxySettings = failingModelProxySettings{}

	result, err = handler.getVersionResponse(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if result["hasModelProvider"] != false || result["mcpTesterFallbackAvailable"] != false || result["obot"] == nil {
		t.Fatalf("lost version fields or advertised fallback during settings lookup failure: %v", result)
	}
}

func TestTesterFallbackToolContinuation(t *testing.T) {
	var bodies [][]byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"echo\",\"arguments\":\"{}\"}}\n\ndata: {\"type\":\"response.completed\"}\n\n")
		} else {
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"tool worked\"}\n\ndata: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":20,\"output_tokens\":5}}}\n\n")
		}
	}))
	defer upstream.Close()

	handler := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "license"}, upstream)
	first := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"echo"}]}],"tools":[{"name":"echo","inputSchema":{}}],"round":1}`)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"type":"tool_calls"`) || !strings.Contains(first.Body.String(), `"id":"call-1"`) {
		t.Fatal(first.Body.String())
	}

	second := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"echo"}]},{"role":"assistant","toolCalls":[{"id":"call-1","name":"echo","arguments":{}}]},{"role":"tool","toolResult":{"callID":"call-1","status":"success","content":{"text":"result"}}}],"tools":[{"name":"echo","inputSchema":{}}],"round":2}`)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"reason":"max_tokens"`) {
		t.Fatal(second.Body.String())
	}

	if len(bodies) != 2 || !strings.Contains(string(bodies[1]), `"type":"function_call_output"`) || !strings.Contains(string(bodies[1]), `"call_id":"call-1"`) {
		t.Fatalf("continuation bodies: %s", bodies)
	}
}

func TestTesterFallbackAuditsStreamOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		want   string
		tokens int
	}{
		{
			name:   "incomplete with usage",
			stream: "data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}}\n\n",
			want:   gatewaytypes.LLMAuditOutcomeSuccess,
			tokens: 12,
		},
		{
			name:   "failed with usage",
			stream: "data: {\"type\":\"response.failed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":3}}}\n\n",
			want:   gatewaytypes.LLMAuditOutcomeError,
			tokens: 12,
		},
		{
			name:   "disconnected stream",
			stream: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
			want:   gatewaytypes.LLMAuditOutcomeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTesterAuditClient(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, tt.stream) }))
			defer upstream.Close()

			handler := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "license"}, upstream)
			handler.fallback.GatewayClient = client
			runMCPTesterChat(t, handler, "user-1", fallbackChatBody)

			log := waitForTesterAudit(t, client)
			if log.Outcome != tt.want || log.InputTokens != tt.tokens {
				t.Fatalf("audit outcome=%s tokens=%d", log.Outcome, log.InputTokens)
			}
		})
	}
}

func waitForTesterAudit(t *testing.T, client *gatewayclient.Client) gatewaytypes.LLMAuditLog {
	t.Helper()

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		logs, _, err := client.GetLLMAuditLogs(t.Context(), gatewayclient.LLMAuditLogOptions{WithSensitiveFields: true})
		if err != nil {
			t.Fatal(err)
		}

		if len(logs) == 1 {
			return logs[0]
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("audit entry was not persisted")
	return gatewaytypes.LLMAuditLog{}
}

func TestTesterFallbackAuditsCancellation(t *testing.T) {
	client, _ := newTesterAuditClient(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()

	handler := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "license"}, upstream)
	handler.fallback.GatewayClient = client

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	request := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", strings.NewReader(fallbackChatBody)).WithContext(ctx)
	request.SetPathValue("mcp_server_id", "ms1tester")

	done := make(chan error, 1)
	go func() {
		done <- handler.Chat(api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), User: &kuser.DefaultInfo{UID: "user-1"}})
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Tester did not stop after cancellation")
	}

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream context was not canceled")
	}

	if log := waitForTesterAudit(t, client); log.Outcome != gatewaytypes.LLMAuditOutcomeCanceled {
		t.Fatalf("audit outcome = %q", log.Outcome)
	}
}
