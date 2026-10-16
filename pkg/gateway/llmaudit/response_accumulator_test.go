package llmaudit

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestResponseFormatForRequestPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want responseFormat
	}{
		{
			path: "/v1/messages",
			want: responseFormatAnthropicMessages,
		},
		{
			path: "/anthropic/v1/messages",
			want: responseFormatAnthropicMessages,
		},
		{
			path: "/api/llm-proxy/aws-bedrock/anthropic/v1/messages",
			want: responseFormatAnthropicMessages,
		},
		{
			path: "/v1/responses",
			want: responseFormatOpenAIResponses,
		},
		{
			path: "/openai/v1/responses",
			want: responseFormatOpenAIResponses,
		},
		{
			path: "/api/llm-proxy/aws-bedrock/openai/v1/responses",
			want: responseFormatOpenAIResponses,
		},
		{
			path: "/api/llm-proxy/v1/chat/completions",
			want: responseFormatOpenAIChatCompletions,
		},
		{
			path: "/v1/models",
			want: responseFormatUnknown,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := responseFormatForRequestPath(tc.path); got != tc.want {
				t.Fatalf("responseFormatForRequestPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestChatCompletionsStreamIsAccumulated(t *testing.T) {
	a := NewResponseAccumulator("", "/api/llm-proxy/v1/chat/completions")
	for _, chunk := range []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"content":"Hel"}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"ci"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ty\":\"Paris\"}"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-x","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`,
	} {
		a.Write([]byte("data: " + chunk + "\n\n"))
	}
	a.Write([]byte("data: [DONE]\n\n"))

	if got := a.ResponseID(); got != "chatcmpl-1" {
		t.Fatalf("ResponseID() = %q, want chatcmpl-1", got)
	}
	var got map[string]any
	if err := json.Unmarshal(a.JSON(), &got); err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal([]byte(`{
		"id": "chatcmpl-1",
		"object": "chat.completion",
		"created": 1,
		"model": "gpt-x",
		"choices": [{
			"index": 0,
			"finish_reason": "tool_calls",
			"message": {
				"role": "assistant",
				"content": "Hello",
				"tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Paris\"}"}}]
			}
		}],
		"usage": {"prompt_tokens": 5, "completion_tokens": 7, "total_tokens": 12}
	}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON() = %v\nwant %v", got, want)
	}
}
