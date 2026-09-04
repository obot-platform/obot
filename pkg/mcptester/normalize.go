package mcptester

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

type EventEmitter func(types.MCPTesterStreamEvent) error

type pendingToolCall struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

type streamNormalizer struct {
	dialect   llmtypes.Dialect
	emit      EventEmitter
	started   bool
	finished  bool
	calls     map[int]*pendingToolCall
	itemIndex map[string]int
	toolNames map[string]struct{}
}

// NormalizeStream converts provider SSE into the MCP tester event contract.
// Provider event names and partial argument encoding never cross this boundary.
func NormalizeStream(ctx context.Context, dialect llmtypes.Dialect, tools []types.MCPTesterTool, input io.Reader, emit EventEmitter) error {
	toolNames := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		toolNames[tool.Name] = struct{}{}
	}
	n := &streamNormalizer{
		dialect:   dialect,
		emit:      emit,
		calls:     map[int]*pendingToolCall{},
		itemIndex: map[string]int{},
		toolNames: toolNames,
	}

	if err := n.checkContext(ctx); err != nil {
		return err
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)
	for scanner.Scan() {
		if err := n.checkContext(ctx); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			if !n.finished {
				return n.finish(types.MCPTesterCompletionReasonStop)
			}
			return nil
		}
		if !json.Valid([]byte(data)) {
			return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned malformed streaming JSON", false)
		}
		if err := n.consume([]byte(data)); err != nil {
			return err
		}
		if n.finished {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return n.checkContext(ctx)
		}
		return n.fail(types.MCPTesterErrorProvider, fmt.Sprintf("model stream failed: %v", err), true)
	}
	if !n.finished {
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model stream ended without a completion event", true)
	}
	return nil
}

func (n *streamNormalizer) consume(data []byte) error {
	var policyEvent struct {
		Violation string `json:"obot_tool_call_policy_violation"`
	}
	if err := json.Unmarshal(data, &policyEvent); err == nil && policyEvent.Violation != "" {
		return n.fail(types.MCPTesterErrorPolicyDenied, policyEvent.Violation, false)
	}

	switch n.dialect {
	case llmtypes.DialectOpenAIResponses, llmtypes.DialectOpenResponses:
		return n.consumeResponses(data)
	case llmtypes.DialectOpenAIChatCompletions:
		return n.consumeChatCompletions(data)
	case llmtypes.DialectAnthropicMessages:
		return n.consumeAnthropic(data)
	default:
		return n.fail(types.MCPTesterErrorUnsupportedResponse, fmt.Sprintf("unsupported model dialect %q", n.dialect), false)
	}
}

func (n *streamNormalizer) consumeResponses(data []byte) error {
	var event struct {
		Type        string `json:"type"`
		Delta       string `json:"delta"`
		OutputIndex int    `json:"output_index"`
		ItemID      string `json:"item_id"`
		Item        struct {
			ID        string `json:"id"`
			CallID    string `json:"call_id"`
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"item"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Response struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned an unsupported Responses event", false)
	}

	switch event.Type {
	case "response.created", "response.in_progress":
		return n.start()
	case "response.output_text.delta", "response.refusal.delta":
		if err := n.start(); err != nil {
			return err
		}
		if event.Delta != "" {
			return n.emit(types.MCPTesterStreamEvent{
				Type:  types.MCPTesterStreamEventTextDelta,
				Delta: event.Delta,
			})
		}
	case "response.output_item.added", "response.output_item.done":
		if event.Item.Type == "function_call" {
			call := n.call(event.OutputIndex)
			call.id = firstNonEmpty(event.Item.CallID, event.Item.ID, call.id)
			call.name = firstNonEmpty(event.Item.Name, call.name)
			n.itemIndex[event.Item.ID] = event.OutputIndex
			n.itemIndex[event.Item.CallID] = event.OutputIndex
			if event.Type == "response.output_item.done" && event.Item.Arguments != "" {
				call.arguments.Reset()
				call.arguments.WriteString(event.Item.Arguments)
			}
		}
	case "response.function_call_arguments.delta":
		index := event.OutputIndex
		if mapped, ok := n.itemIndex[event.ItemID]; ok {
			index = mapped
		}
		n.call(index).arguments.WriteString(event.Delta)
	case "response.completed":
		return n.finish(types.MCPTesterCompletionReasonStop)
	case "response.incomplete":
		return n.finish(types.MCPTesterCompletionReasonMaxTokens)
	case "response.failed", "error":
		message := firstNonEmpty(event.Error.Message, "model provider returned an error")
		return n.fail(types.MCPTesterErrorProvider, message, true)
	case "":
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned a Responses event without a type", false)
	}
	return nil
}

func (n *streamNormalizer) consumeChatCompletions(data []byte) error {
	var event struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				Refusal   string `json:"refusal"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned an unsupported Chat Completions event", false)
	}
	if event.Error.Message != "" {
		return n.fail(types.MCPTesterErrorProvider, event.Error.Message, true)
	}
	for _, choice := range event.Choices {
		if text := firstNonEmpty(choice.Delta.Content, choice.Delta.Refusal); text != "" {
			if err := n.start(); err != nil {
				return err
			}
			if err := n.emit(types.MCPTesterStreamEvent{
				Type:  types.MCPTesterStreamEventTextDelta,
				Delta: text,
			}); err != nil {
				return err
			}
		}
		for _, delta := range choice.Delta.ToolCalls {
			call := n.call(delta.Index)
			call.id = firstNonEmpty(delta.ID, call.id)
			call.name = firstNonEmpty(delta.Function.Name, call.name)
			call.arguments.WriteString(delta.Function.Arguments)
		}
		switch choice.FinishReason {
		case "stop":
			return n.finish(types.MCPTesterCompletionReasonStop)
		case "tool_calls", "function_call":
			return n.finish(types.MCPTesterCompletionReasonToolCalls)
		case "length":
			return n.finish(types.MCPTesterCompletionReasonMaxTokens)
		}
	}
	return nil
}

func (n *streamNormalizer) consumeAnthropic(data []byte) error {
	var event struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
			StopReason  string `json:"stop_reason"`
		} `json:"delta"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned an unsupported Anthropic event", false)
	}

	switch event.Type {
	case "message_start":
		return n.start()
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			call := n.call(event.Index)
			call.id = event.ContentBlock.ID
			call.name = event.ContentBlock.Name
			if len(event.ContentBlock.Input) > 0 && string(event.ContentBlock.Input) != "{}" {
				call.arguments.Write(event.ContentBlock.Input)
			}
		}
	case "content_block_delta":
		switch event.Delta.Type {
		case "text_delta":
			if err := n.start(); err != nil {
				return err
			}
			if event.Delta.Text != "" {
				return n.emit(types.MCPTesterStreamEvent{
					Type:  types.MCPTesterStreamEventTextDelta,
					Delta: event.Delta.Text,
				})
			}
		case "input_json_delta":
			n.call(event.Index).arguments.WriteString(event.Delta.PartialJSON)
		}
	case "message_delta":
		switch event.Delta.StopReason {
		case "end_turn", "stop_sequence":
			return n.finish(types.MCPTesterCompletionReasonStop)
		case "tool_use":
			return n.finish(types.MCPTesterCompletionReasonToolCalls)
		case "max_tokens":
			return n.finish(types.MCPTesterCompletionReasonMaxTokens)
		}
	case "message_stop":
		return n.finish(types.MCPTesterCompletionReasonStop)
	case "error":
		return n.fail(types.MCPTesterErrorProvider, firstNonEmpty(event.Error.Message, "model provider returned an error"), true)
	case "":
		return n.fail(types.MCPTesterErrorUnsupportedResponse, "model returned an Anthropic event without a type", false)
	}
	return nil
}

func (n *streamNormalizer) start() error {
	if n.started {
		return nil
	}
	n.started = true
	return n.emit(types.MCPTesterStreamEvent{Type: types.MCPTesterStreamEventAssistantMessageStart})
}

func (n *streamNormalizer) call(index int) *pendingToolCall {
	call := n.calls[index]
	if call == nil {
		call = &pendingToolCall{index: index}
		n.calls[index] = call
	}
	return call
}

func (n *streamNormalizer) finish(reason types.MCPTesterCompletionReason) error {
	if n.finished {
		return nil
	}
	if err := n.start(); err != nil {
		return err
	}
	calls, err := n.normalizedCalls()
	if err != nil {
		return n.fail(types.MCPTesterErrorUnsupportedResponse, err.Error(), false)
	}
	if len(calls) > 0 {
		reason = types.MCPTesterCompletionReasonToolCalls
		if err := n.emit(types.MCPTesterStreamEvent{
			Type:  types.MCPTesterStreamEventToolCalls,
			Calls: calls,
		}); err != nil {
			return err
		}
	}
	n.finished = true
	return n.emit(types.MCPTesterStreamEvent{
		Type:   types.MCPTesterStreamEventCompletion,
		Reason: reason,
	})
}

func (n *streamNormalizer) normalizedCalls() ([]types.MCPTesterToolCall, error) {
	indexes := make([]int, 0, len(n.calls))
	for index := range n.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]types.MCPTesterToolCall, 0, len(indexes))
	seen := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		call := n.calls[index]
		arguments := strings.TrimSpace(call.arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		if call.id == "" || call.name == "" || !json.Valid([]byte(arguments)) {
			return nil, fmt.Errorf("model returned an invalid tool call at index %d", index)
		}
		if _, advertised := n.toolNames[call.name]; !advertised {
			return nil, fmt.Errorf("model returned a call to tool %q, which was not provided in this request", call.name)
		}
		if _, duplicate := seen[call.id]; duplicate {
			return nil, fmt.Errorf("model returned duplicate tool call ID %q", call.id)
		}
		seen[call.id] = struct{}{}
		result = append(result, types.MCPTesterToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: json.RawMessage(arguments),
		})
	}
	return result, nil
}

func (n *streamNormalizer) fail(code types.MCPTesterErrorCode, message string, retryable bool) error {
	if n.finished {
		return nil
	}
	n.finished = true
	return n.emit(types.MCPTesterStreamEvent{
		Type: types.MCPTesterStreamEventError,
		Error: &types.MCPTesterStreamError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
}

func (n *streamNormalizer) checkContext(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return n.fail(types.MCPTesterErrorCancelled, "request cancelled", false)
}

// TODO(g-linville): see if there is an existing function for this
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
