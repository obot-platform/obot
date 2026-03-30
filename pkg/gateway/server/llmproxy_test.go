package server

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/messagepolicy"
)

func TestModifyResponse_PathFiltering(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		statusCode  int
		wantWrapped bool
	}{
		{"chat completions", "/v1/chat/completions", http.StatusOK, true},
		{"anthropic messages", "/v1/messages", http.StatusOK, true},
		{"openai responses", "/v1/responses", http.StatusOK, true},
		{"unknown path", "/v1/embeddings", http.StatusOK, false},
		{"non-200 status", "/v1/chat/completions", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseModifier{}
			body := io.NopCloser(strings.NewReader("{}"))
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    &http.Request{URL: &url.URL{Path: tt.path}},
			}

			if err := r.modifyResponse(resp); err != nil {
				t.Fatal(err)
			}

			// If wrapped, resp.Body should be the responseModifier itself
			if tt.wantWrapped && resp.Body != r {
				t.Error("expected response body to be wrapped by responseModifier")
			}
			if !tt.wantWrapped && resp.Body != body {
				t.Error("expected response body to remain unwrapped")
			}
		})
	}
}

func TestResponseModifier_OpenAIChatCompletions(t *testing.T) {
	// Streaming chat completions format
	stream := "data: {\"model\":\"gpt-4o\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20,\"total_tokens\":30}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 10 {
		t.Errorf("promptTokens = %d, want 10", r.promptTokens)
	}
	if r.completionTokens != 20 {
		t.Errorf("completionTokens = %d, want 20", r.completionTokens)
	}
	if r.totalTokens != 30 {
		t.Errorf("totalTokens = %d, want 30", r.totalTokens)
	}
}

func TestResponseModifier_AnthropicMessages(t *testing.T) {
	// Anthropic streaming: message_start has usage under "message", message_delta has top-level usage
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	// Read message_start
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	// Read message_delta
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 25 {
		t.Errorf("promptTokens = %d, want 25", r.promptTokens)
	}
	// message_delta output_tokens is cumulative (15 total), not incremental,
	// so it supersedes the initial output_tokens (1) from message_start.
	if r.completionTokens != 15 {
		t.Errorf("completionTokens = %d, want 15 (cumulative from message_delta)", r.completionTokens)
	}
}

func TestResponseModifier_OpenAIResponsesAPI(t *testing.T) {
	// Responses API streaming: response.completed has usage nested under "response"
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-4o\",\"usage\":{\"input_tokens\":50,\"output_tokens\":100,\"total_tokens\":150}}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 50 {
		t.Errorf("promptTokens = %d, want 50", r.promptTokens)
	}
	if r.completionTokens != 100 {
		t.Errorf("completionTokens = %d, want 100", r.completionTokens)
	}
	if r.totalTokens != 150 {
		t.Errorf("totalTokens = %d, want 150", r.totalTokens)
	}
}

func TestResponseModifier_NonStreamingResponse(t *testing.T) {
	// Non-streaming: plain JSON body with top-level usage
	body := "{\"model\":\"gpt-4o\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":10,\"total_tokens\":15}}\n"

	r := &responseModifier{
		stream: false,
		b:      bufio.NewReader(strings.NewReader(body)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 5 {
		t.Errorf("promptTokens = %d, want 5", r.promptTokens)
	}
	if r.completionTokens != 10 {
		t.Errorf("completionTokens = %d, want 10", r.completionTokens)
	}
	if r.totalTokens != 15 {
		t.Errorf("totalTokens = %d, want 15", r.totalTokens)
	}
}

func TestResponseModifier_ModelFromRequestPreserved(t *testing.T) {
	// If model is already set from the request, don't overwrite from response
	stream := "data: {\"model\":\"gpt-4o-realmodel\",\"usage\":{\"prompt_tokens\":1}}\n"

	r := &responseModifier{
		stream: true,
		model:  "my-alias",
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.model != "my-alias" {
		t.Errorf("model = %q, want %q (should preserve original)", r.model, "my-alias")
	}
}

func TestResponseModifier_AnthropicCumulativeTokens(t *testing.T) {
	// Anthropic message_delta reports cumulative tokens that supersede earlier counts.
	// This mirrors the web search case where message_delta has higher input_tokens.
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-opus-4-6\",\"usage\":{\"input_tokens\":2679,\"output_tokens\":3}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":10682,\"output_tokens\":510}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 10682 {
		t.Errorf("promptTokens = %d, want 10682 (cumulative from message_delta)", r.promptTokens)
	}
	if r.completionTokens != 510 {
		t.Errorf("completionTokens = %d, want 510 (cumulative from message_delta)", r.completionTokens)
	}
	// totalTokens should be 0 since Anthropic doesn't provide it explicitly;
	// it gets derived in Close().
	if r.totalTokens != 0 {
		t.Errorf("totalTokens = %d, want 0 (derived at Close time)", r.totalTokens)
	}
}

func TestResponseModifier_TotalTokensDerivedAtClose(t *testing.T) {
	// When no explicit total_tokens is provided (e.g. Anthropic), it should
	// be derived from prompt + completion at Close time.
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	// Simulate Close() logic without needing a real DB client.
	totalTokens := r.totalTokens
	if totalTokens == 0 {
		totalTokens = r.promptTokens + r.completionTokens
	}
	if totalTokens != 40 {
		t.Errorf("derived totalTokens = %d, want 40 (25 prompt + 15 completion)", totalTokens)
	}
}

func TestResponseModifier_StreamNonDataLinesPassThrough(t *testing.T) {
	// Non-data lines (like "event: ..." lines) should pass through without parsing
	stream := "event: message_start\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf[:n]) != "event: message_start\n" {
		t.Errorf("got %q, want %q", string(buf[:n]), "event: message_start\n")
	}
	if r.promptTokens != 0 || r.completionTokens != 0 {
		t.Error("non-data lines should not affect token counts")
	}
}

func TestExtractModelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"top-level model (OpenAI/Anthropic request)",
			`{"model":"gpt-4o","messages":[]}`,
			"gpt-4o",
		},
		{
			"nested under message",
			`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`,
			"claude-sonnet-4-20250514",
		},
		{
			"nested under response",
			`{"type":"response.completed","response":{"model":"gpt-4o"}}`,
			"gpt-4o",
		},
		{
			"top-level takes precedence over nested",
			`{"model":"top-level","message":{"model":"nested"}}`,
			"top-level",
		},
		{
			"empty body",
			`{}`,
			"",
		},
		{
			"no model anywhere",
			`{"messages":[{"role":"user","content":"hello"}]}`,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModelFromBody([]byte(tt.body))
			if got != tt.want {
				t.Errorf("extractModelFromBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLLMTransformRequest_RemovesAcceptEncoding(t *testing.T) {
	u := mustParseURL("https://api.example.com/v1")
	director := llmTransformRequest(*u, nil)

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/v1/chat/completions", nil)
	req.SetPathValue("path", "chat/completions")
	req.Header.Set("Accept-Encoding", "gzip")

	director(req)

	if got := req.Header.Get("Accept-Encoding"); got != "" {
		t.Fatalf("Accept-Encoding = %q, want empty", got)
	}
}

func TestExtractContentString(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{"plain string", "Hello world", "Hello world"},
		{"nil", nil, ""},
		{"integer", 42, ""},
		{"empty string", "", ""},
		{
			"array with single text part",
			[]any{
				map[string]any{"type": "text", "text": "Hello"},
			},
			"Hello",
		},
		{
			"array with multiple text parts",
			[]any{
				map[string]any{"type": "text", "text": "Hello"},
				map[string]any{"type": "text", "text": "World"},
			},
			"Hello\nWorld",
		},
		{
			"array with mixed content types",
			[]any{
				map[string]any{"type": "text", "text": "Describe this image"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
			},
			"Describe this image",
		},
		{
			"array with no text parts",
			[]any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
			},
			"",
		},
		{"empty array", []any{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContentString(tt.content)
			if got != tt.want {
				t.Errorf("extractContentString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMessagesFromBody_SimpleConversation(t *testing.T) {
	raw := []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant."},
		map[string]any{"role": "user", "content": "Hello"},
		map[string]any{"role": "assistant", "content": "Hi there!"},
		map[string]any{"role": "user", "content": "Book me a flight"},
	}

	history, lastMsg, lastIdx := parseMessagesFromBody(raw)

	if len(history) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(history))
	}
	if lastMsg != "Book me a flight" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "Book me a flight")
	}
	if lastIdx != 3 {
		t.Errorf("lastUserIdx = %d, want 3", lastIdx)
	}
	if history[0].Role != "system" || history[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system message: %+v", history[0])
	}
}

func TestParseMessagesFromBody_WithToolCalls(t *testing.T) {
	raw := []any{
		map[string]any{"role": "user", "content": "Search for flights"},
		map[string]any{
			"role":    "assistant",
			"content": "Let me search.",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "search_flights",
						"arguments": `{"to":"NYC"}`,
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"content":      `{"flights": [{"price": 500}]}`,
			"tool_call_id": "call_1",
		},
		map[string]any{"role": "user", "content": "Book the cheapest one"},
	}

	history, lastMsg, lastIdx := parseMessagesFromBody(raw)

	if len(history) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(history))
	}

	// Check assistant tool calls parsed correctly
	assistant := history[1]
	if assistant.Content != "Let me search." {
		t.Errorf("assistant content = %q, want %q", assistant.Content, "Let me search.")
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].Name != "search_flights" {
		t.Errorf("tool call name = %q, want %q", assistant.ToolCalls[0].Name, "search_flights")
	}
	if assistant.ToolCalls[0].Arguments != `{"to":"NYC"}` {
		t.Errorf("tool call arguments = %q, want %q", assistant.ToolCalls[0].Arguments, `{"to":"NYC"}`)
	}

	// Check tool message
	toolMsg := history[2]
	if toolMsg.ToolCallID != "call_1" {
		t.Errorf("tool_call_id = %q, want %q", toolMsg.ToolCallID, "call_1")
	}

	if lastMsg != "Book the cheapest one" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "Book the cheapest one")
	}
	if lastIdx != 3 {
		t.Errorf("lastUserIdx = %d, want 3", lastIdx)
	}
}

func TestParseMessagesFromBody_NoUserMessage(t *testing.T) {
	raw := []any{
		map[string]any{"role": "system", "content": "System prompt"},
		map[string]any{"role": "assistant", "content": "Hello!"},
	}

	_, lastMsg, lastIdx := parseMessagesFromBody(raw)

	if lastIdx != -1 {
		t.Errorf("lastUserIdx = %d, want -1", lastIdx)
	}
	if lastMsg != "" {
		t.Errorf("lastUserMessage = %q, want empty", lastMsg)
	}
}

func TestParseMessagesFromBody_EmptyMessages(t *testing.T) {
	history, lastMsg, lastIdx := parseMessagesFromBody(nil)

	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}
	if lastIdx != -1 {
		t.Errorf("lastUserIdx = %d, want -1", lastIdx)
	}
	if lastMsg != "" {
		t.Errorf("lastUserMessage = %q, want empty", lastMsg)
	}
}

func TestParseMessagesFromBody_ArrayContent(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "What is in this image?"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
			},
		},
	}

	history, lastMsg, lastIdx := parseMessagesFromBody(raw)

	if len(history) != 1 {
		t.Fatalf("expected 1 message, got %d", len(history))
	}
	if history[0].Content != "What is in this image?" {
		t.Errorf("content = %q, want %q", history[0].Content, "What is in this image?")
	}
	if lastMsg != "What is in this image?" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "What is in this image?")
	}
	if lastIdx != 0 {
		t.Errorf("lastUserIdx = %d, want 0", lastIdx)
	}
}

func TestParseMessagesFromBody_MultipleToolCalls(t *testing.T) {
	raw := []any{
		map[string]any{
			"role": "assistant",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": `{"city":"NYC"}`,
					},
				},
				map[string]any{
					"id":   "call_2",
					"type": "function",
					"function": map[string]any{
						"name":      "get_time",
						"arguments": `{"tz":"EST"}`,
					},
				},
			},
		},
	}

	history, _, _ := parseMessagesFromBody(raw)

	if len(history) != 1 {
		t.Fatalf("expected 1 message, got %d", len(history))
	}
	if len(history[0].ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(history[0].ToolCalls))
	}
	if history[0].ToolCalls[0].Name != "get_weather" {
		t.Errorf("first tool call name = %q, want %q", history[0].ToolCalls[0].Name, "get_weather")
	}
	if history[0].ToolCalls[1].Name != "get_time" {
		t.Errorf("second tool call name = %q, want %q", history[0].ToolCalls[1].Name, "get_time")
	}
}

func TestParseMessagesFromBody_InvalidEntries(t *testing.T) {
	raw := []any{
		"not a map",
		42,
		map[string]any{"role": "user", "content": "Valid message"},
	}

	history, lastMsg, lastIdx := parseMessagesFromBody(raw)

	// Invalid entries should be skipped
	if len(history) != 1 {
		t.Fatalf("expected 1 message (skipping invalid), got %d", len(history))
	}
	if lastMsg != "Valid message" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "Valid message")
	}
	// lastIdx should point to the raw array index, not the history index
	if lastIdx != 2 {
		t.Errorf("lastUserIdx = %d, want 2", lastIdx)
	}
}

func TestParseMessagesFromBody_AnthropicToolResultNotLastUser(t *testing.T) {
	// In Anthropic format, tool_result messages have role "user" but contain
	// no text content. They should NOT be treated as the last user message.
	raw := []any{
		map[string]any{"role": "user", "content": "Create a bar chart"},
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "I'll create that chart."},
				map[string]any{"type": "tool_use", "id": "toolu_01S", "name": "create_chart", "input": map[string]any{"data": "test"}},
			},
		},
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_01S", "content": []any{
					map[string]any{"type": "text", "text": "Chart created"},
				}},
			},
		},
	}

	_, lastMsg, lastIdx := parseMessagesFromBody(raw)

	// The last user message should be the actual user text, not the tool_result.
	if lastMsg != "Create a bar chart" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "Create a bar chart")
	}
	if lastIdx != 0 {
		t.Errorf("lastUserIdx = %d, want 0", lastIdx)
	}
}

func TestBuildToolCallTargetMessage_SingleToolCall(t *testing.T) {
	toolCalls := []messagepolicy.ToolCallInfo{
		{Name: "search_flights", Arguments: `{"to":"NYC"}`},
	}
	result := buildToolCallTargetMessage(toolCalls)
	expected := `[called tool "search_flights" with args: {"to":"NYC"}]`
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBuildToolCallTargetMessage_MultipleToolCalls(t *testing.T) {
	toolCalls := []messagepolicy.ToolCallInfo{
		{Name: "tool_a", Arguments: "{}"},
		{Name: "tool_b", Arguments: "{}"},
	}
	result := buildToolCallTargetMessage(toolCalls)
	if !strings.Contains(result, `"tool_a"`) || !strings.Contains(result, `"tool_b"`) {
		t.Errorf("expected both tool calls, got %q", result)
	}
}

func TestBuildToolCallTargetMessage_Empty(t *testing.T) {
	result := buildToolCallTargetMessage(nil)
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}

func TestNoOutputPolicies_StreamsNormally(t *testing.T) {
	// When no output policies, Read should stream line-by-line (no pipe).
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"
	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	got := string(buf[:n])
	if !strings.Contains(got, "hi") {
		t.Errorf("expected streamed content, got %q", got)
	}
	if r.pipeReader != nil {
		t.Error("pipeReader should be nil when no policies")
	}
}

func TestStreamAndEvaluateToolCallsSSE_TextOnly_StreamsThrough(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\", world!\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	pr, pw := io.Pipe()
	r := &responseModifier{
		stream:         true,
		b:              bufio.NewReader(strings.NewReader(stream)),
		c:              io.NopCloser(strings.NewReader("")),
		outputPolicies: []types2.MessagePolicyManifest{{DisplayName: "test"}},
	}

	go r.streamAndEvaluateToolCalls(t.Context(), pw)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}

	got := string(result)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world!") {
		t.Errorf("expected streamed text content, got %q", got)
	}
}

func TestStreamAndEvaluateToolCallsJSON_NoToolCalls_PassThrough(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"Hello"}}]}` + "\n"

	pr, pw := io.Pipe()
	r := &responseModifier{
		stream:         false,
		b:              bufio.NewReader(strings.NewReader(body)),
		c:              io.NopCloser(strings.NewReader("")),
		outputPolicies: []types2.MessagePolicyManifest{{DisplayName: "test"}},
	}

	go r.streamAndEvaluateToolCalls(t.Context(), pw)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(result), "Hello") {
		t.Errorf("expected original response, got %q", string(result))
	}
}

func TestStreamAndEvaluateToolCallsSSE_AnthropicToolCall_Detected(t *testing.T) {
	// Simulate an Anthropic streaming response with a text block followed by a tool_use block.
	stream := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Let me check.\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_123\",\"name\":\"get_weather\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\":\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"NYC\\\"}\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":1}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":50}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	pr, pw := io.Pipe()
	r := &responseModifier{
		stream:              true,
		b:                   bufio.NewReader(strings.NewReader(stream)),
		c:                   io.NopCloser(strings.NewReader("")),
		messagePolicyHelper: &messagepolicy.Helper{},
	}

	go r.streamAndEvaluateToolCalls(t.Context(), pw)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}

	got := string(result)
	// Text before the tool call should be streamed through.
	if !strings.Contains(got, "Let me check.") {
		t.Errorf("expected text content to be streamed, got %q", got)
	}
	// Tool call events should also be present (buffered then flushed with no violations).
	if !strings.Contains(got, "get_weather") {
		t.Errorf("expected tool call to be present in output, got %q", got)
	}
}

func TestStreamAndEvaluateToolCallsSSE_AnthropicMultipleToolCalls(t *testing.T) {
	stream := "event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"get_weather\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\":\\\"NYC\\\"}\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_2\",\"name\":\"get_time\",\"input\":{}}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"tz\\\":\\\"EST\\\"}\"}}\n\n" +
		"event: content_block_stop\n" +
		"data: {\"type\":\"content_block_stop\",\"index\":1}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	pr, pw := io.Pipe()
	r := &responseModifier{
		stream:              true,
		b:                   bufio.NewReader(strings.NewReader(stream)),
		c:                   io.NopCloser(strings.NewReader("")),
		messagePolicyHelper: &messagepolicy.Helper{},
	}

	go r.streamAndEvaluateToolCalls(t.Context(), pw)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}

	got := string(result)
	if !strings.Contains(got, "get_weather") || !strings.Contains(got, "get_time") {
		t.Errorf("expected both tool calls in output, got %q", got)
	}
}

func TestIsAnthropicToolCallEvent(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			"content_block_start with tool_use",
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_123","name":"get_weather","input":{}}}`,
			true,
		},
		{
			"content_block_delta with input_json_delta",
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
			true,
		},
		{
			"content_block_start with text",
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			false,
		},
		{
			"content_block_delta with text_delta",
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			false,
		},
		{
			"OpenAI format",
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"get_weather"}}]}}]}`,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAnthropicToolCallEvent([]byte(tt.data))
			if got != tt.want {
				t.Errorf("isAnthropicToolCallEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccumulateAnthropicToolCallInfo(t *testing.T) {
	blockToTool := make(map[int]int)
	var toolCalls []messagepolicy.ToolCallInfo

	// First tool: content_block_start
	accumulateAnthropicToolCallInfo(
		[]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}`),
		&toolCalls, blockToTool,
	)
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("name = %q, want %q", toolCalls[0].Name, "get_weather")
	}

	// Partial arguments
	accumulateAnthropicToolCallInfo(
		[]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`),
		&toolCalls, blockToTool,
	)
	accumulateAnthropicToolCallInfo(
		[]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"NYC\"}"}}`),
		&toolCalls, blockToTool,
	)
	if toolCalls[0].Arguments != `{"city":"NYC"}` {
		t.Errorf("arguments = %q, want %q", toolCalls[0].Arguments, `{"city":"NYC"}`)
	}

	// Second tool at a different block index
	accumulateAnthropicToolCallInfo(
		[]byte(`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_2","name":"get_time","input":{}}}`),
		&toolCalls, blockToTool,
	)
	accumulateAnthropicToolCallInfo(
		[]byte(`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"tz\":\"EST\"}"}}`),
		&toolCalls, blockToTool,
	)
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	if toolCalls[1].Name != "get_time" {
		t.Errorf("name = %q, want %q", toolCalls[1].Name, "get_time")
	}
	if toolCalls[1].Arguments != `{"tz":"EST"}` {
		t.Errorf("arguments = %q, want %q", toolCalls[1].Arguments, `{"tz":"EST"}`)
	}
}

func TestStreamAndEvaluateToolCallsJSON_AnthropicToolCalls(t *testing.T) {
	// Non-streaming Anthropic response with a tool_use content block.
	body := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"NYC"}}],"stop_reason":"tool_use"}` + "\n"

	pr, pw := io.Pipe()
	r := &responseModifier{
		stream:              false,
		b:                   bufio.NewReader(strings.NewReader(body)),
		c:                   io.NopCloser(strings.NewReader("")),
		messagePolicyHelper: &messagepolicy.Helper{},
	}

	go r.streamAndEvaluateToolCalls(t.Context(), pw)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}

	got := string(result)
	if !strings.Contains(got, "get_weather") {
		t.Errorf("expected tool call in output, got %q", got)
	}
}

func TestParseMessagesFromBody_ConversationHistoryForPolicyEval(t *testing.T) {
	// Verify that parsed messages integrate correctly with BuildConversationContext
	raw := []any{
		map[string]any{"role": "system", "content": "You are a travel agent."},
		map[string]any{"role": "user", "content": "Find flights to NYC"},
		map[string]any{
			"role":    "assistant",
			"content": "Searching now.",
			"tool_calls": []any{
				map[string]any{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "search_flights",
						"arguments": `{"to":"NYC"}`,
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"content":      `[{"flight":"AA100","price":500}]`,
			"tool_call_id": "call_1",
		},
		map[string]any{"role": "assistant", "content": "Found a flight for $500."},
		map[string]any{"role": "user", "content": "Book it in first class"},
	}

	history, lastMsg, _ := parseMessagesFromBody(raw)

	// Verify the conversation context is built correctly
	ctx := messagepolicy.BuildConversationContext(history)

	if lastMsg != "Book it in first class" {
		t.Errorf("lastUserMessage = %q, want %q", lastMsg, "Book it in first class")
	}

	// System messages should be excluded
	if strings.Contains(ctx, "travel agent") {
		t.Error("conversation context should not contain system messages")
	}
	// Tool outputs should be redacted
	if strings.Contains(ctx, "AA100") {
		t.Error("conversation context should redact tool outputs")
	}
	if !strings.Contains(ctx, "[tool output redacted]") {
		t.Error("conversation context should contain redaction placeholder")
	}
	// Tool call info should be present
	if !strings.Contains(ctx, "search_flights") {
		t.Error("conversation context should contain tool call names")
	}
	// User messages should be present
	if !strings.Contains(ctx, "Find flights to NYC") {
		t.Error("conversation context should contain user messages")
	}
}
