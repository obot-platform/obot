package mcp

import (
	"context"
	"encoding/json"
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
	payload := json.RawMessage(`{"jsonrpc":"2.0","method":"tools/call"}`)
	candidate := FilterCandidate{
		Target: "policy/validate",
		Server: wantServer,
	}

	output, err := runner.ExecuteFilter(t.Context(), candidate, payload)
	if err != nil {
		t.Fatal(err)
	}
	if output.Decision != FilterDecisionAccept || output.Reason != "allowed" {
		t.Fatalf("unexpected hook output: %#v", output)
	}
	data, err := json.Marshal(client.params.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if client.params == nil || client.params.Name != "validate" || input["accept"] != true || input["mutated"] != false || input["reason"] != "" || !reflect.DeepEqual(input["message"], map[string]any{"jsonrpc": "2.0", "method": "tools/call"}) {
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
	output, err := runner.ExecuteFilter(t.Context(), FilterCandidate{
		Target: "policy/check",
		Server: ServerConfig{MCPServerName: "sms1policy"},
	}, json.RawMessage(`{"method":"tools/call"}`))
	if err != nil {
		t.Fatal(err)
	}
	if output.Decision != FilterDecisionReject || output.Reason != "blocked" {
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

	_, err := runner.ExecuteFilter(t.Context(), FilterCandidate{
		Target: "policy/check",
		Server: ServerConfig{MCPServerName: "sms1policy"},
	}, json.RawMessage(`{}`))
	if err == nil || strings.Contains(err.Error(), "policy service denied the request") || !strings.Contains(err.Error(), "returned an error") {
		t.Fatalf("expected bounded hook tool error, got %v", err)
	}
}

func TestMCPHookRunnerValidatesTargetAndServer(t *testing.T) {
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return nil, errors.New("should not be called")
	}}

	if _, err := runner.ExecuteFilter(t.Context(), FilterCandidate{Target: "invalid"}, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "invalid Filter target") {
		t.Fatalf("expected invalid target error, got %v", err)
	}
	if _, err := runner.ExecuteFilter(t.Context(), FilterCandidate{Target: "policy/check"}, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected missing server error, got %v", err)
	}
}

func TestMCPHookRunnerRejectsContradictoryResponse(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		StructuredContent: map[string]any{
			"accept":  false,
			"mutated": true,
			"message": map[string]any{"secret": "must not appear in error"},
		},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}
	output, err := runner.ExecuteFilter(t.Context(), FilterCandidate{
		Target: "policy/check",
		Server: ServerConfig{MCPServerName: "sms1policy"},
	}, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "contradictory") || strings.Contains(err.Error(), "must not appear") {
		t.Fatalf("expected bounded contradictory-response error, got output=%#v err=%v", output, err)
	}
	if len(output.RawResponse) == 0 || !strings.Contains(string(output.RawResponse), "must not appear") {
		t.Fatalf("raw response was not retained for protected recording: %#v", output)
	}
}

func TestMCPHookRunnerRejectsMutationWithoutMessage(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		StructuredContent: map[string]any{
			"accept":  true,
			"mutated": true,
		},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}
	_, err := runner.ExecuteFilter(t.Context(), FilterCandidate{
		Target: "policy/check",
		Server: ServerConfig{MCPServerName: "sms1policy"},
	}, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "complete JSON message") {
		t.Fatalf("expected incomplete-mutation error, got %v", err)
	}
}
