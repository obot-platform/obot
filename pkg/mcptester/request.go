package mcptester

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

const (
	anthropicMaxTokens = 4096
)

// BuildModelRequest validates the public request and converts it into the
// resolved model's native request dialect. The model and system instruction
// are server-owned and cannot be supplied by the browser.
func BuildModelRequest(request types.MCPTesterChatRequest, model ResolvedModel, systemInstruction string) ([]byte, error) {
	if err := ValidateChatRequest(request); err != nil {
		return nil, err
	}

	var payload any
	switch model.Dialect {
	case llmtypes.DialectOpenAIResponses, llmtypes.DialectOpenResponses:
		payload = buildResponsesRequest(request, model.ID, systemInstruction)
	case llmtypes.DialectOpenAIChatCompletions:
		payload = buildChatCompletionsRequest(request, model.ID, systemInstruction)
	case llmtypes.DialectAnthropicMessages:
		payload = buildAnthropicRequest(request, model.ID, systemInstruction)
	default:
		return nil, fmt.Errorf("unsupported model dialect %q", model.Dialect)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal model request: %w", err)
	}
	return body, nil
}

func ValidateChatRequest(request types.MCPTesterChatRequest) error {
	if request.Round < 1 || request.Round > types.MCPTesterMaxRounds {
		return fmt.Errorf("round must be between 1 and %d", types.MCPTesterMaxRounds)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("messages must not be empty")
	}

	toolNames := make(map[string]struct{}, len(request.Tools))
	for index, tool := range request.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return fmt.Errorf("tool %d has an empty name", index)
		}
		if _, exists := toolNames[tool.Name]; exists {
			return fmt.Errorf("tool name %q is duplicated", tool.Name)
		}
		toolNames[tool.Name] = struct{}{}
		if err := validateJSONObject(tool.InputSchema); err != nil {
			return fmt.Errorf("tool %q input schema: %w", tool.Name, err)
		}
		if len(tool.OutputSchema) > 0 {
			if err := validateJSONObject(tool.OutputSchema); err != nil {
				return fmt.Errorf("tool %q output schema: %w", tool.Name, err)
			}
		}
	}

	callIDs := map[string]struct{}{}
	resultIDs := map[string]struct{}{}
	for index, message := range request.Messages {
		if err := validateMessage(message, toolNames, callIDs, resultIDs); err != nil {
			return fmt.Errorf("message %d: %w", index, err)
		}
	}
	for callID := range callIDs {
		if _, exists := resultIDs[callID]; !exists {
			return fmt.Errorf("tool call %q does not have a result", callID)
		}
	}

	last := request.Messages[len(request.Messages)-1]
	if last.Role != types.MCPTesterChatRoleUser && last.Role != types.MCPTesterChatRoleTool {
		return fmt.Errorf("last message must have user or tool role")
	}
	return nil
}

func validateMessage(message types.MCPTesterChatMessage, toolNames, callIDs, resultIDs map[string]struct{}) error {
	switch message.Role {
	case types.MCPTesterChatRoleUser:
		if len(message.Content) == 0 || len(message.ToolCalls) > 0 || message.ToolResult != nil {
			return fmt.Errorf("user messages require content and cannot contain tool calls or results")
		}
		return validateContent(message.Content, true)
	case types.MCPTesterChatRoleAssistant:
		if len(message.Content) == 0 && len(message.ToolCalls) == 0 || message.ToolResult != nil {
			return fmt.Errorf("assistant messages require content or tool calls and cannot contain a tool result")
		}
		if err := validateContent(message.Content, false); err != nil {
			return err
		}
		for index, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" {
				return fmt.Errorf("tool call %d requires an ID and name", index)
			}
			if _, exists := callIDs[call.ID]; exists {
				return fmt.Errorf("tool call ID %q is duplicated", call.ID)
			}
			if _, exists := toolNames[call.Name]; !exists {
				return fmt.Errorf("tool call %q references unknown tool %q", call.ID, call.Name)
			}
			callIDs[call.ID] = struct{}{}
			if err := validateJSONObject(call.Arguments); err != nil {
				return fmt.Errorf("tool call %q arguments: %w", call.ID, err)
			}
		}
		return nil
	case types.MCPTesterChatRoleTool:
		if len(message.Content) > 0 || len(message.ToolCalls) > 0 || message.ToolResult == nil {
			return fmt.Errorf("tool messages require one result and cannot contain content or calls")
		}
		result := message.ToolResult
		if _, exists := callIDs[result.CallID]; !exists {
			return fmt.Errorf("tool result references unknown call ID %q", result.CallID)
		}
		if _, exists := resultIDs[result.CallID]; exists {
			return fmt.Errorf("tool result for call ID %q is duplicated", result.CallID)
		}
		resultIDs[result.CallID] = struct{}{}
		switch result.Status {
		case types.MCPTesterToolResultStatusSuccess,
			types.MCPTesterToolResultStatusError,
			types.MCPTesterToolResultStatusRejected,
			types.MCPTesterToolResultStatusCancelled:
		default:
			return fmt.Errorf("tool result has invalid status %q", result.Status)
		}
		if len(result.Content) == 0 || !json.Valid(result.Content) {
			return fmt.Errorf("tool result content must be valid JSON")
		}
		if len(toolResultText(*result)) > types.MCPTesterMaxModelBoundContentSize {
			return fmt.Errorf("tool result content exceeds %d bytes", types.MCPTesterMaxModelBoundContentSize)
		}
		return nil
	default:
		return fmt.Errorf("invalid role %q", message.Role)
	}
}

func validateContent(contents []types.MCPTesterContent, allowResource bool) error {
	for index, content := range contents {
		if content.Text == "" {
			return fmt.Errorf("content %d has empty text", index)
		}
		switch content.Type {
		case types.MCPTesterContentTypeText:
			if content.URI != "" || content.MIMEType != "" {
				return fmt.Errorf("text content %d cannot contain resource metadata", index)
			}
		case types.MCPTesterContentTypeResource:
			if !allowResource {
				return fmt.Errorf("resource content is only valid in user messages")
			}
			if strings.TrimSpace(content.URI) == "" {
				return fmt.Errorf("resource content %d requires a URI", index)
			}
			if len(contentText([]types.MCPTesterContent{content})) > types.MCPTesterMaxModelBoundContentSize {
				return fmt.Errorf("resource content %d exceeds %d bytes", index, types.MCPTesterMaxModelBoundContentSize)
			}
		default:
			return fmt.Errorf("content %d has invalid type %q", index, content.Type)
		}
	}
	return nil
}

func validateJSONObject(raw json.RawMessage) error {
	if len(raw) == 0 || !json.Valid(raw) {
		return fmt.Errorf("must be valid JSON")
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("must be a JSON object")
	}
	return nil
}

func contentText(contents []types.MCPTesterContent) string {
	parts := make([]string, 0, len(contents))
	for _, content := range contents {
		if content.Type == types.MCPTesterContentTypeResource {
			metadata := "Resource: " + content.URI
			if content.MIMEType != "" {
				metadata += "\nMIME type: " + content.MIMEType
			}
			parts = append(parts, metadata+"\n\n"+content.Text)
		} else {
			parts = append(parts, content.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func toolResultText(result types.MCPTesterToolResult) string {
	value := struct {
		Status  types.MCPTesterToolResultStatus `json:"status"`
		Content json.RawMessage                 `json:"content"`
	}{
		Status:  result.Status,
		Content: result.Content,
	}
	body, _ := json.Marshal(value)
	return string(body)
}

func toolArguments(arguments json.RawMessage) string {
	return string(arguments)
}

func schemaValue(schema json.RawMessage) any {
	var result any
	_ = json.Unmarshal(schema, &result)
	return result
}

func buildResponsesRequest(request types.MCPTesterChatRequest, modelID, systemInstruction string) map[string]any {
	input := make([]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case types.MCPTesterChatRoleUser, types.MCPTesterChatRoleAssistant:
			if len(message.Content) > 0 {
				contentType := "input_text"
				if message.Role == types.MCPTesterChatRoleAssistant {
					contentType = "output_text"
				}
				input = append(input, map[string]any{
					"role": message.Role,
					"content": []any{map[string]any{
						"type": contentType,
						"text": contentText(message.Content),
					}},
				})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Name,
					"arguments": toolArguments(call.Arguments),
				})
			}
		case types.MCPTesterChatRoleTool:
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolResult.CallID,
				"output":  toolResultText(*message.ToolResult),
			})
		}
	}

	payload := map[string]any{
		"model":        modelID,
		"stream":       true,
		"instructions": systemInstruction,
		"input":        input,
	}
	if tools := responsesTools(request.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}
	return payload
}

func responsesTools(tools []types.MCPTesterTool) []any {
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  schemaValue(tool.InputSchema),
		})
	}
	return result
}

func buildChatCompletionsRequest(request types.MCPTesterChatRequest, modelID, systemInstruction string) map[string]any {
	messages := []any{map[string]any{
		"role":    "system",
		"content": systemInstruction,
	}}
	for _, message := range request.Messages {
		switch message.Role {
		case types.MCPTesterChatRoleUser:
			messages = append(messages, map[string]any{
				"role":    "user",
				"content": contentText(message.Content),
			})
		case types.MCPTesterChatRoleAssistant:
			converted := map[string]any{
				"role":    "assistant",
				"content": contentText(message.Content),
			}
			if len(message.ToolCalls) > 0 {
				calls := make([]any, 0, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					calls = append(calls, map[string]any{
						"id":   call.ID,
						"type": "function",
						"function": map[string]any{
							"name":      call.Name,
							"arguments": toolArguments(call.Arguments),
						},
					})
				}
				converted["tool_calls"] = calls
			}
			messages = append(messages, converted)
		case types.MCPTesterChatRoleTool:
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": message.ToolResult.CallID,
				"content":      toolResultText(*message.ToolResult),
			})
		}
	}

	payload := map[string]any{
		"model":    modelID,
		"stream":   true,
		"messages": messages,
	}
	if tools := chatCompletionsTools(request.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}
	return payload
}

func chatCompletionsTools(tools []types.MCPTesterTool) []any {
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  schemaValue(tool.InputSchema),
			},
		})
	}
	return result
}

func buildAnthropicRequest(request types.MCPTesterChatRequest, modelID, systemInstruction string) map[string]any {
	messages := make([]map[string]any, 0, len(request.Messages))
	for _, message := range request.Messages {
		var role string
		var blocks []any
		switch message.Role {
		case types.MCPTesterChatRoleUser:
			role = "user"
			blocks = []any{map[string]any{
				"type": "text",
				"text": contentText(message.Content),
			}}
		case types.MCPTesterChatRoleAssistant:
			role = "assistant"
			if len(message.Content) > 0 {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": contentText(message.Content),
				})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Name,
					"input": schemaValue(call.Arguments),
				})
			}
		case types.MCPTesterChatRoleTool:
			role = "user"
			blocks = []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": message.ToolResult.CallID,
				"content":     toolResultText(*message.ToolResult),
				"is_error":    message.ToolResult.Status != types.MCPTesterToolResultStatusSuccess,
			}}
		}

		if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
			messages[len(messages)-1]["content"] = append(messages[len(messages)-1]["content"].([]any), blocks...)
		} else {
			messages = append(messages, map[string]any{
				"role":    role,
				"content": blocks,
			})
		}
	}

	payload := map[string]any{
		"model":      modelID,
		"stream":     true,
		"max_tokens": anthropicMaxTokens,
		"system":     systemInstruction,
		"messages":   messages,
	}
	if tools := anthropicTools(request.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}
	return payload
}

func anthropicTools(tools []types.MCPTesterTool) []any {
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": schemaValue(tool.InputSchema),
		})
	}
	return result
}
