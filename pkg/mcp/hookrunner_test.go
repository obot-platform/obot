package mcp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeHookMCPClient struct {
	params *gomcp.CallToolParams
	result *gomcp.CallToolResult
	err    error
}

func (c *fakeHookMCPClient) CallTool(_ context.Context, params *gomcp.CallToolParams) (*gomcp.CallToolResult, error) {
	c.params = params
	return c.result, c.err
}

func TestMCPHookRunnerCallsConfiguredServerTool(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		StructuredContent: map[string]any{"accept": true, "reason": "allowed"},
	}}
	wantServer := ServerConfig{MCPServerName: "sms1policy", UserID: "user-1"}
	runner := &SessionManagerHookRunner{clientForServer: func(_ context.Context, server ServerConfig) (hookMCPClient, error) {
		if server.MCPServerName != wantServer.MCPServerName || server.UserID != wantServer.UserID {
			t.Fatalf("got hook server %#v, want %#v", server, wantServer)
		}
		return client, nil
	}}
	input := SessionMessageHook{Accept: true, Message: &Message{Method: "tools/call"}}

	output, err := runner.RunHook(t.Context(), HookServerConfigs{"policy": wantServer}, input, "policy/validate")
	if err != nil {
		t.Fatal(err)
	}
	if output == nil || !output.Accept || output.Reason != "allowed" {
		t.Fatalf("unexpected hook output: %#v", output)
	}
	if client.params == nil || client.params.Name != "validate" || !reflect.DeepEqual(client.params.Arguments, input) {
		t.Fatalf("unexpected tool call params: %#v", client.params)
	}
}

func TestMCPHookRunnerDecodesJSONTextContent(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: `{"accept":false,"reason":"blocked"}`}},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}
	output, err := runner.RunHook(
		t.Context(), HookServerConfigs{"policy": {MCPServerName: "sms1policy"}},
		SessionMessageHook{Message: &Message{Method: "tools/call"}}, "policy/check",
	)
	if err != nil {
		t.Fatal(err)
	}
	if output == nil || output.Accept || output.Reason != "blocked" {
		t.Fatalf("unexpected JSON text hook output: %#v", output)
	}
}

func TestMCPHookRunnerReturnsToolErrorMessage(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{&gomcp.TextContent{Text: "policy service denied the request"}},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}

	_, err := runner.RunHook(
		t.Context(), HookServerConfigs{"policy": {MCPServerName: "sms1policy"}},
		SessionMessageHook{}, "policy/check",
	)
	if err == nil || !strings.Contains(err.Error(), "policy service denied the request") {
		t.Fatalf("expected hook tool error message, got %v", err)
	}
}

func TestMCPHookRunnerValidatesTargetAndServer(t *testing.T) {
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return nil, errors.New("should not be called")
	}}

	if _, err := runner.RunHook(t.Context(), nil, SessionMessageHook{}, "invalid"); err == nil || !strings.Contains(err.Error(), "expected server/tool") {
		t.Fatalf("expected invalid target error, got %v", err)
	}
	if _, err := runner.RunHook(t.Context(), nil, SessionMessageHook{}, "policy/check"); err == nil || !strings.Contains(err.Error(), "is not configured") {
		t.Fatalf("expected missing server error, got %v", err)
	}
}
