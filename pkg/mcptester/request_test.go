package mcptester

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

func TestBuildModelRequestUsesServerOwnedModelAndInstruction(t *testing.T) {
	request := validChatRequest()
	body, err := BuildModelRequest(request, ResolvedModel{
		ID:      "m1default",
		Dialect: llmtypes.DialectOpenAIResponses,
	}, "fixed tester instruction")
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["model"] != "m1default" {
		t.Fatalf("model = %v, want m1default", payload["model"])
	}
	if payload["instructions"] != "fixed tester instruction" {
		t.Fatalf("instructions = %v, want fixed tester instruction", payload["instructions"])
	}
	if payload["stream"] != true {
		t.Fatalf("stream = %v, want true", payload["stream"])
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v, want one message", payload["input"])
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", payload["tools"])
	}
}

func TestBuildModelRequestPreservesToolBatchAndResultsAcrossDialects(t *testing.T) {
	request := validChatRequest()
	request.Messages = append(request.Messages,
		types.MCPTesterChatMessage{
			Role: types.MCPTesterChatRoleAssistant,
			ToolCalls: []types.MCPTesterToolCall{
				{
					ID:        "call-1",
					Name:      "lookup",
					Arguments: json.RawMessage(`{"query":"one"}`),
				},
				{
					ID:        "call-2",
					Name:      "lookup",
					Arguments: json.RawMessage(`{"query":"two"}`),
				},
			},
		},
		types.MCPTesterChatMessage{
			Role: types.MCPTesterChatRoleTool,
			ToolResult: &types.MCPTesterToolResult{
				CallID:  "call-1",
				Status:  types.MCPTesterToolResultStatusSuccess,
				Content: json.RawMessage(`{"answer":1}`),
			},
		},
		types.MCPTesterChatMessage{
			Role: types.MCPTesterChatRoleTool,
			ToolResult: &types.MCPTesterToolResult{
				CallID:  "call-2",
				Status:  types.MCPTesterToolResultStatusRejected,
				Content: json.RawMessage(`"rejected by user"`),
			},
		},
	)

	assertDialectRequestContains(t, request, llmtypes.DialectOpenAIResponses, "function_call_output", 2)
	assertDialectRequestContains(t, request, llmtypes.DialectOpenAIChatCompletions, "tool_call_id", 2)
	assertDialectRequestContains(t, request, llmtypes.DialectAnthropicMessages, "tool_result", 2)
}

func TestValidateChatRequestRejectsInvalidAndOversizedModelContent(t *testing.T) {
	request := validChatRequest()
	request.Round = types.MCPTesterMaxRounds + 1
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "round") {
		t.Fatalf("ValidateChatRequest() error = %v, want round error", err)
	}

	request = validChatRequest()
	request.Messages[0].Content[0] = types.MCPTesterContent{
		Type: types.MCPTesterContentTypeResource,
		Text: strings.Repeat("x", types.MCPTesterMaxModelBoundContentSize+1),
		URI:  "file:///large.txt",
	}
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ValidateChatRequest() error = %v, want oversized resource error", err)
	}

	request = validChatRequest()
	request.Messages[0].Content[0] = types.MCPTesterContent{
		Type: types.MCPTesterContentTypeText,
		Text: strings.Repeat("x", types.MCPTesterMaxModelBoundContentSize+1),
	}
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ValidateChatRequest() error = %v, want oversized text error", err)
	}

	request = validChatRequest()
	half := types.MCPTesterMaxModelBoundContentSize/2 + 1
	request.Messages[0].Content = []types.MCPTesterContent{
		{
			Type: types.MCPTesterContentTypeText,
			Text: strings.Repeat("x", half),
		},
		{
			Type: types.MCPTesterContentTypeText,
			Text: strings.Repeat("y", half),
		},
	}
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ValidateChatRequest() error = %v, want oversized combined content error", err)
	}

	request = validChatRequest()
	request.Tools[0].InputSchema = json.RawMessage(`[]`)
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("ValidateChatRequest() error = %v, want schema object error", err)
	}
}

func TestValidateChatRequestRejectsUnknownToolResult(t *testing.T) {
	request := validChatRequest()
	request.Messages = append(request.Messages, types.MCPTesterChatMessage{
		Role: types.MCPTesterChatRoleTool,
		ToolResult: &types.MCPTesterToolResult{
			CallID:  "unknown-call",
			Status:  types.MCPTesterToolResultStatusSuccess,
			Content: json.RawMessage(`{}`),
		},
	})
	if err := ValidateChatRequest(request); err == nil || !strings.Contains(err.Error(), "unknown call ID") {
		t.Fatalf("ValidateChatRequest() error = %v, want unknown call error", err)
	}
}

func assertDialectRequestContains(t *testing.T, request types.MCPTesterChatRequest, dialect llmtypes.Dialect, marker string, count int) {
	t.Helper()
	body, err := BuildModelRequest(request, ResolvedModel{
		ID:      "m1default",
		Dialect: dialect,
	}, "fixed")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(body), marker); got != count {
		t.Fatalf("%s request contains %q %d times, want %d: %s", dialect, marker, got, count, body)
	}
}

func validChatRequest() types.MCPTesterChatRequest {
	return types.MCPTesterChatRequest{
		Messages: []types.MCPTesterChatMessage{
			{
				Role: types.MCPTesterChatRoleUser,
				Content: []types.MCPTesterContent{
					{
						Type: types.MCPTesterContentTypeText,
						Text: "hello",
					},
				},
			},
		},
		Tools: []types.MCPTesterTool{
			{
				Name:        "lookup",
				Description: "Look up a value",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
		Round: 1,
	}
}
