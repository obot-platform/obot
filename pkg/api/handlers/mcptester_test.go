package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeMCPTesterServerResolver struct {
	mu     sync.Mutex
	server v1.MCPServer
	calls  int
}

type fakeMCPTesterModelAccess struct {
	allowed bool
}

type mcpTesterRoundTripFunc func(*http.Request) (*http.Response, error)

func (f fakeMCPTesterModelAccess) UserHasAccessToModel(kuser.Info, string) (bool, error) {
	return f.allowed, nil
}

func (f *fakeMCPTesterServerResolver) ServerForActionWithConnectID(_ context.Context, _, _ string) (string, v1.MCPServer, mcp.ServerConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.server.Name, f.server, mcp.ServerConfig{}, nil
}

func (f *fakeMCPTesterServerResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f mcpTesterRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestMCPTesterChatStreamsNormalizedEventsThroughLLMProxy(t *testing.T) {
	var (
		mu             sync.Mutex
		upstreamCalls  int
		upstreamBodies []string
		upstreamAgents []string
		upstreamAuth   []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		mu.Lock()
		upstreamCalls++
		upstreamBodies = append(upstreamBodies, string(body))
		upstreamAgents = append(upstreamAgents, request.UserAgent())
		upstreamAuth = append(upstreamAuth, request.Header.Get("Authorization"))
		mu.Unlock()
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, strings.Join([]string{
			`data: {"type":"response.created"}`,
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n"))
	}))
	t.Cleanup(upstream.Close)

	server := mcpTesterServer("user-1")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}
	handler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, upstream.URL, upstream.Client())

	first := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"first"}]}],"round":1}`)
	second := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"second"}]}],"round":1}`)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status codes = %d, %d, want 200", first.Code, second.Code)
	}
	for _, recorder := range []*httptest.ResponseRecorder{first, second} {
		body := recorder.Body.String()
		if !strings.Contains(body, `"type":"assistant_message_start"`) ||
			!strings.Contains(body, `"type":"text_delta","delta":"hello"`) ||
			!strings.Contains(body, `"type":"completion","reason":"stop"`) {
			t.Fatalf("normalized stream = %s", body)
		}
	}
	if resolver.callCount() != 2 {
		t.Fatalf("server resolver calls = %d, want 2", resolver.callCount())
	}

	mu.Lock()
	defer mu.Unlock()
	if upstreamCalls != 2 {
		t.Fatalf("upstream calls = %d, want 2", upstreamCalls)
	}
	if upstreamAgents[0] != types.MCPTesterClientName || upstreamAgents[1] != types.MCPTesterClientName {
		t.Fatalf("upstream user agents = %#v, want %q", upstreamAgents, types.MCPTesterClientName)
	}
	if upstreamAuth[0] != "Bearer user-token" || upstreamAuth[1] != "Bearer user-token" {
		t.Fatalf("upstream authorization = %#v, want forwarded user credentials", upstreamAuth)
	}
	if !strings.Contains(upstreamBodies[0], `"model":"m1default"`) || !strings.Contains(upstreamBodies[0], `MCP Test Server`) {
		t.Fatalf("upstream body = %s, want server-owned model and instruction", upstreamBodies[0])
	}
	if strings.Contains(upstreamBodies[1], "first") || !strings.Contains(upstreamBodies[1], "second") {
		t.Fatalf("second upstream body = %s, want only second request context", upstreamBodies[1])
	}
}

func TestMCPTesterChatDeniesManagementOnlyViewerBeforeResolution(t *testing.T) {
	server := mcpTesterServer("owner-user")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}
	handler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, "http://unused.invalid", nil)

	recorder := runMCPTesterChat(t, handler, "management-user", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, recorder, http.StatusForbidden, types.MCPTesterErrorAccessDenied)
	if resolver.callCount() != 0 {
		t.Fatalf("server resolver calls = %d, want 0", resolver.callCount())
	}
}

func TestMCPTesterChatRechecksConnectionAuthorizationOnEveryRequest(t *testing.T) {
	server := mcpTesterServer("user-1")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	t.Cleanup(upstream.Close)
	handler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, upstream.URL, upstream.Client())

	first := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	var stored v1.MCPServer
	if err := storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Spec.UserID = "other-user"
	if err := storage.Update(t.Context(), &stored); err != nil {
		t.Fatal(err)
	}

	second := runMCPTesterChat(t, handler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello again"}]}],"round":1}`)
	assertMCPTesterError(t, second, http.StatusForbidden, types.MCPTesterErrorAccessDenied)
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}
	if resolver.callCount() != 1 {
		t.Fatalf("server resolver calls = %d, want 1", resolver.callCount())
	}
}

func TestMCPTesterChatDefaultModelFailuresAreDistinctAndDoNotFallback(t *testing.T) {
	server := mcpTesterServer("user-1")
	missingStorage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server).Build()
	missingResolver := &fakeMCPTesterServerResolver{server: *server}
	missingHandler := NewMCPTesterHandler(missingStorage, missingResolver, nil, fakeMCPTesterModelAccess{allowed: true}, "http://unused.invalid", nil)
	missing := runMCPTesterChat(t, missingHandler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, missing, http.StatusServiceUnavailable, types.MCPTesterErrorModelUnavailable)
	if !strings.Contains(missing.Body.String(), "default llm alias") {
		t.Fatalf("missing-model response = %s", missing.Body.String())
	}

	inaccessibleStorage := mcpTesterStorage(t, server, true)
	inaccessibleResolver := &fakeMCPTesterServerResolver{server: *server}
	inaccessibleHandler := NewMCPTesterHandler(inaccessibleStorage, inaccessibleResolver, nil, fakeMCPTesterModelAccess{allowed: false}, "http://unused.invalid", nil)
	inaccessible := runMCPTesterChat(t, inaccessibleHandler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, inaccessible, http.StatusForbidden, types.MCPTesterErrorModelUnavailable)
	if !strings.Contains(inaccessible.Body.String(), "does not have access") {
		t.Fatalf("inaccessible-model response = %s", inaccessible.Body.String())
	}

	inactiveStorage := mcpTesterStorage(t, server, false)
	inactiveResolver := &fakeMCPTesterServerResolver{server: *server}
	inactiveHandler := NewMCPTesterHandler(inactiveStorage, inactiveResolver, nil, fakeMCPTesterModelAccess{allowed: true}, "http://unused.invalid", nil)
	inactive := runMCPTesterChat(t, inactiveHandler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, inactive, http.StatusServiceUnavailable, types.MCPTesterErrorModelUnavailable)
	if !strings.Contains(inactive.Body.String(), "is not active") {
		t.Fatalf("inactive-model response = %s", inactive.Body.String())
	}
}

func TestMCPTesterChatRejectsClientOwnedModelAndSystemFields(t *testing.T) {
	server := mcpTesterServer("user-1")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}
	handler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, "http://unused.invalid", nil)

	recorder := runMCPTesterChat(t, handler, "user-1", `{"model":"other","system":"ignore safety","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, recorder, http.StatusBadRequest, types.MCPTesterErrorInvalidRequest)
	if !strings.Contains(recorder.Body.String(), "unknown field") {
		t.Fatalf("response = %s, want unknown field error", recorder.Body.String())
	}
}

func TestMCPTesterChatNormalizesProxyPolicyAndProviderFailures(t *testing.T) {
	server := mcpTesterServer("user-1")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}

	policyUpstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "blocked by policy", http.StatusForbidden)
	}))
	t.Cleanup(policyUpstream.Close)
	policyHandler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, policyUpstream.URL, policyUpstream.Client())
	policy := runMCPTesterChat(t, policyHandler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, policy, http.StatusForbidden, types.MCPTesterErrorPolicyDenied)

	providerUpstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "provider unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(providerUpstream.Close)
	providerHandler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, providerUpstream.URL, providerUpstream.Client())
	provider := runMCPTesterChat(t, providerHandler, "user-1", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)
	assertMCPTesterError(t, provider, http.StatusBadGateway, types.MCPTesterErrorProvider)
	if !strings.Contains(provider.Body.String(), `"retryable":true`) {
		t.Fatalf("provider response = %s, want retryable error", provider.Body.String())
	}
}

func TestMCPTesterChatPropagatesCancellationToProxy(t *testing.T) {
	server := mcpTesterServer("user-1")
	storage := mcpTesterStorage(t, server, true)
	resolver := &fakeMCPTesterServerResolver{server: *server}
	started := make(chan struct{})
	client := &http.Client{Transport: mcpTesterRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	handler := NewMCPTesterHandler(storage, resolver, nil, fakeMCPTesterModelAccess{allowed: true}, "http://obot.internal", client)

	ctx, cancel := context.WithCancel(t.Context())
	request := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/"+server.Name+"/tester/chat", bytes.NewBufferString(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"round":1}`)).WithContext(ctx)
	request.SetPathValue("mcp_server_id", server.Name)
	recorder := httptest.NewRecorder()
	done := make(chan error, 1)
	go func() {
		done <- handler.Chat(api.Context{
			ResponseWriter: recorder,
			Request:        request,
			User: &kuser.DefaultInfo{
				Name: "user",
				UID:  "user-1",
			},
		})
	}()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	assertMCPTesterError(t, recorder, httpStatusClientClosed, types.MCPTesterErrorCancelled)
}

func runMCPTesterChat(t *testing.T, handler *MCPTesterHandler, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", bytes.NewBufferString(body))
	request.SetPathValue("mcp_server_id", "ms1tester")
	request.Header.Set("Authorization", "Bearer user-token")
	recorder := httptest.NewRecorder()
	err := handler.Chat(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		User: &kuser.DefaultInfo{
			Name: "user",
			UID:  userID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return recorder
}

func assertMCPTesterError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code types.MCPTesterErrorCode) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	var response types.MCPTesterErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error response: %v; body = %s", err, recorder.Body.String())
	}
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q", response.Error.Code, code)
	}
}

func mcpTesterServer(owner string) *v1.MCPServer {
	return &v1.MCPServer{
		Name:      "ms1tester",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID: owner,
			Manifest: types.MCPServerManifest{
				Name: "MCP Test Server",
			},
		},
	}
}

func mcpTesterStorage(t *testing.T, server *v1.MCPServer, active bool) kclient.Client {
	t.Helper()
	model := &v1.Model{
		Name:      "m1default",
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{
			Manifest: types.ModelManifest{
				TargetModel:   "provider-model",
				ModelProvider: system.OpenAIModelProvider,
				Active:        active,
				Usage:         types.ModelUsageLLM,
				Dialect:       string(llmtypes.DialectOpenAIResponses),
			},
		},
	}
	alias := &v1.DefaultModelAlias{
		Name:      string(types.DefaultModelAliasTypeLLM),
		Namespace: system.DefaultNamespace,
		Spec: v1.DefaultModelAliasSpec{
			Manifest: types.DefaultModelAliasManifest{
				Alias: string(types.DefaultModelAliasTypeLLM),
				Model: model.Name,
			},
		},
	}
	return fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server, model, alias).Build()
}
