package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/mmmcp/toolsearch"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
)

func TestGenericToolCallUsesUnderlyingPolicyAndAuditIdentity(t *testing.T) {
	var called atomic.Bool
	server := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "delete_file", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		called.Store(true)
		return &gomcp.CallToolResult{}, nil
	})
	upstream := httptest.NewServer(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil))
	defer upstream.Close()

	handler, err := NewHandler(t.Context(), nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	cfg := obotmcp.MMMCPConfig(obotmcp.ServerConfig{
		Runtime:    types.RuntimeVMCP,
		ToolSearch: true,
		Components: []obotmcp.ComponentServer{{
			DisplayName: "component",
			URL:         upstream.URL,
		}},
	}, nil)
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.composite.HTTPHandler().ServeHTTP(w, r.WithContext(mmmcp.ContextWithConfig(r.Context(), cfg)))
	}))
	defer frontend.Close()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: frontend.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	search, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: toolsearch.SearchToolName, Arguments: map[string]any{"query": "delete_file"}})
	if err != nil || search.IsError {
		t.Fatalf("search = %#v, error = %v", search, err)
	}
	var found toolsearch.Results
	if err := json.Unmarshal([]byte(search.Content[0].(*gomcp.TextContent).Text), &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Tools) != 1 {
		t.Fatalf("search results = %#v", found)
	}

	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": toolsearch.CallToolName, "arguments": map[string]any{
			"name": found.Tools[0].Reference.Name, "revision": found.Tools[0].Revision,
			"arguments": map[string]any{"path": "secret.txt"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(string(requestBody)))
	collector := new(recordingProxyAuditCollector)
	audit, err := newProxyAudit(req, map[string]string{"mcpID": "vmcp", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := handler.resolveGenericToolCall(req, cfg, "")
	if err != nil || resolved == nil || resolved.name != "delete_file" {
		t.Fatalf("resolved call = %#v, error = %v", resolved, err)
	}
	audit.entry.CallIdentifier = resolved.name
	runner := &scriptedMCPHookRunner{run: func(input obotmcp.SessionMessageHook, _ string) (obotmcp.SessionMessageHook, bool, error) {
		var params struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err := json.Unmarshal(input.Message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.Name != "delete_file" || params.Arguments["path"] != "secret.txt" {
			t.Fatalf("hook saw wrong operation: %#v", params)
		}
		return obotmcp.SessionMessageHook{Accept: false, Message: input.Message, Reason: "blocked by policy"}, true, nil
	}}
	hooks := obotmcp.Hooks{{Name: "tools/call", Params: map[string]string{"name": "delete_file", "direction": "request"}, Targets: []obotmcp.HookTarget{{Target: "policy/delete"}}}}
	processor, err := newHookProcessor(req, runner, hooks, nil, audit, nil, hookProcessorOptions{resolved: resolved})
	if err != nil {
		t.Fatal(err)
	}
	audit.recordRequest()
	body, blocked, hookErr := processor.blockedRequest()
	if !blocked || hookErr == nil || runner.callCount() != 1 || called.Load() {
		t.Fatalf("filter did not block: blocked=%t error=%v calls=%d downstream=%t", blocked, hookErr, runner.callCount(), called.Load())
	}
	audit.recordBlockedRequest(body, hookErr)
	if len(collector.entries) != 2 || collector.entries[0].CallIdentifier != "delete_file" || collector.entries[0].Subject != "user-1" {
		t.Fatalf("audit did not identify underlying operation and user: %#v", collector.entries)
	}

	stale := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"delete_file","revision":"stale","arguments":{"path":"secret.txt"}}}}`)
	if resolved, err := handler.resolveGenericToolCall(stale, cfg, ""); err == nil || resolved != nil {
		t.Fatalf("stale generic call was allowed to forward: resolved=%#v error=%v", resolved, err)
	}
}

func TestGenericToolCallHookCannotRetarget(t *testing.T) {
	resolved := &resolvedToolCall{name: "delete_file", revision: "revision", arguments: json.RawMessage(`{"path":"one"}`)}
	wire := obotmcp.Message{
		JSONRPC: "2.0", ID: 1, Method: "tools/call",
		Params: json.RawMessage(`{"name":"call_tool","arguments":{"name":"delete_file","revision":"revision","arguments":{"path":"one"}}}`),
	}
	hookMessage, err := resolved.hookMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	hookMessage.Params = json.RawMessage(`{"name":"other_tool","arguments":{"path":"two"}}`)
	if _, err := resolved.restoreHookMutation(wire, hookMessage); err == nil {
		t.Fatal("policy mutation changed the resolved target")
	}
}

func TestGenericToolCallHookMutationPreservesWrapper(t *testing.T) {
	resolved := &resolvedToolCall{name: "delete_file", revision: "revision", arguments: json.RawMessage(`{"path":"one"}`)}
	runner := &scriptedMCPHookRunner{run: func(input obotmcp.SessionMessageHook, _ string) (obotmcp.SessionMessageHook, bool, error) {
		message := *input.Message
		message.Params = json.RawMessage(`{"name":"delete_file","arguments":{"path":"two"}}`)
		return obotmcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message}, true, nil
	}}
	hooks := obotmcp.Hooks{{Name: "tools/call", Params: map[string]string{"name": "delete_file"}, Targets: []obotmcp.HookTarget{{Target: "policy/redact"}}}}
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"delete_file","revision":"revision","arguments":{"path":"one"}}}}`)
	processor, err := newHookProcessor(req, runner, hooks, nil, nil, nil, hookProcessorOptions{resolved: resolved})
	if err != nil {
		t.Fatal(err)
	}
	if _, blocked, err := processor.blockedRequest(); blocked || err != nil {
		t.Fatalf("mutated call was blocked: %v", err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Params struct {
			Name      string `json:"name"`
			Arguments struct {
				Name      string            `json:"name"`
				Revision  string            `json:"revision"`
				Arguments map[string]string `json:"arguments"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Params.Name != toolsearch.CallToolName || wire.Params.Arguments.Name != "delete_file" ||
		wire.Params.Arguments.Revision != "revision" || wire.Params.Arguments.Arguments["path"] != "two" {
		t.Fatalf("hook mutation did not preserve generic call wrapper: %s", body)
	}
}

func TestGenericToolCallResponseHookUsesUnderlyingName(t *testing.T) {
	resolved := &resolvedToolCall{name: "delete_file", revision: "revision", arguments: json.RawMessage(`{}`)}
	runner := new(scriptedMCPHookRunner)
	hooks := obotmcp.Hooks{{Name: "tools/call", Params: map[string]string{"name": "delete_file", "direction": "response"}, Targets: []obotmcp.HookTarget{{Target: "policy/response"}}}}
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"delete_file","revision":"revision","arguments":{}}}}`)
	processor, err := newHookProcessor(req, runner, hooks, nil, nil, nil, hookProcessorOptions{resolved: resolved})
	if err != nil {
		t.Fatal(err)
	}
	resp := mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{"content":[]}}`)
	if err := processor.filterResponse(resp); err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 1 || runner.calls[0].target != "policy/response" {
		t.Fatalf("response policy saw wrong operation: %#v", runner.calls)
	}
}

func TestGenericToolCallNamedSearchToolsMatchesCallToolHooksAcrossRequests(t *testing.T) {
	resolved := &resolvedToolCall{name: toolsearch.SearchToolName, revision: "revision", arguments: json.RawMessage(`{}`)}
	runner := new(scriptedMCPHookRunner)
	hooks := obotmcp.Hooks{
		{Name: "tools/call", Params: map[string]string{"name": toolsearch.CallToolName, "direction": "request"}, Targets: []obotmcp.HookTarget{{Target: "policy/generic-request"}}},
		{Name: "tools/call", Params: map[string]string{"name": toolsearch.CallToolName, "direction": "response"}, Targets: []obotmcp.HookTarget{{Target: "policy/generic-response"}}},
		{Name: "tools/call", Params: map[string]string{"name": toolsearch.SearchToolName, "direction": "request"}, Targets: []obotmcp.HookTarget{{Target: "policy/underlying-request"}}},
	}
	storage := newMCPProxyTestStorage()
	metadata := map[string]string{"mcpID": "vmcp", "userID": "user-1"}
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"search_tools","revision":"revision","arguments":{}}}}`)
	req.Header.Set(mcpSessionHeader, "session-1")
	_, err := newHookProcessor(req, runner, hooks, nil, nil, newHookCorrelationStore(storage, metadata), hookProcessorOptions{resolved: resolved})
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 2 || runner.calls[0].target != "policy/generic-request" || runner.calls[1].target != "policy/underlying-request" {
		t.Fatalf("generic request hooks did not match both identities: %#v", runner.calls)
	}

	responseReq := httptest.NewRequest(http.MethodGet, "http://obot.example/mcp", nil)
	responseReq.Header.Set(mcpSessionHeader, "session-1")
	responseProcessor, err := newHookProcessor(responseReq, runner, hooks, nil, nil, newHookCorrelationStore(storage, metadata))
	if err != nil {
		t.Fatal(err)
	}
	if err := responseProcessor.filterResponse(mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{"content":[]}}`)); err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 3 || runner.calls[2].target != "policy/generic-response" {
		t.Fatalf("correlated response lost generic call identity: %#v", runner.calls)
	}

	searchRunner := new(scriptedMCPHookRunner)
	searchReq := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_tools","arguments":{"query":"test"}}}`)
	if _, err := newHookProcessor(searchReq, searchRunner, hooks, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if searchRunner.callCount() != 1 || searchRunner.calls[0].target != "policy/underlying-request" {
		t.Fatalf("search request matched a generic-call hook: %#v", searchRunner.calls)
	}
}

func TestGenericToolCallDiscoveryAuthorizationReturnsObotChallenge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://upstream.example/secret"`)
		http.Error(w, "upstream authentication details", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	handler, err := NewHandler(t.Context(), nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	cfg := obotmcp.MMMCPConfig(obotmcp.ServerConfig{
		Runtime:    types.RuntimeVMCP,
		ToolSearch: true,
		Components: []obotmcp.ComponentServer{{
			DisplayName: "component",
			URL:         upstream.URL,
		}},
	}, nil)
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"call_tool","arguments":{"name":"delete_file","revision":"revision","arguments":{}}}}`)
	req.URL.Path = "/mcp-connect/vmcpi1test"
	req.SetPathValue("mcp_id", "vmcpi1test")
	_, err = handler.resolveGenericToolCall(req, cfg, "")
	if authErr, ok := errors.AsType[*mmmcp.AuthorizationError](err); !ok || authErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("discovery error = %v, want authorization error", err)
	}

	w := httptest.NewRecorder()
	ctx := api.Context{Request: req, ResponseWriter: w, APIBaseURL: "https://obot.example/api"}
	if !writeGenericToolCallError(ctx, err) {
		t.Fatal("authorization error was not handled")
	}
	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	wantChallenge := `Bearer realm="Obot MCP Gateway", resource_metadata="https://obot.example/.well-known/oauth-protected-resource/mcp-connect/vmcpi1test"`
	if challenge := resp.Header.Get("WWW-Authenticate"); challenge != wantChallenge {
		t.Fatalf("challenge = %q, want %q", challenge, wantChallenge)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "MCP server requires authentication\n" || strings.Contains(string(body), "upstream") {
		t.Fatalf("component authentication details leaked: %q", body)
	}
}
