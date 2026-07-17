package messagepolicy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/gateway/azure"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
)

func TestCallLLMBedrockAnthropicMessages(t *testing.T) {
	var (
		gotPath    string
		gotBody    map[string]any
		gotHeaders http.Header
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"yes\"}}\n\n")
	}))
	defer server.Close()

	result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
		targetModel:  "anthropic.claude-haiku-4-5",
		providerName: system.AmazonBedrockModelProvider,
		providerURL:  server.URL + "/anthropic/v1",
		dialect:      string(nanobottypes.DialectAnthropicMessages),
	}, []chatMessage{{Role: "system", Content: "Check policy"}, {Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "yes", result)
	assert.Equal(t, "/anthropic/v1/messages", gotPath)
	assert.Equal(t, "anthropic.claude-haiku-4-5", gotBody["model"])
	assert.Equal(t, "2023-06-01", gotHeaders.Get("anthropic-version"))
	assert.NotContains(t, gotBody, "anthropic_version")
	assert.Equal(t, "Check policy", gotBody["system"])
	assert.Equal(t, true, gotBody["stream"])
	assert.Equal(t, []any{map[string]any{"role": "user", "content": "Hello"}}, gotBody["messages"])
}

func TestCallLLMBedrockOpenAIResponses(t *testing.T) {
	var (
		gotPath string
		gotBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"COMPLIANT\"}\n\n")
	}))
	defer server.Close()

	for _, dialect := range []nanobottypes.Dialect{nanobottypes.DialectOpenAIResponses, nanobottypes.DialectOpenResponses} {
		result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
			targetModel:  "openai.gpt-oss-120b-1:0",
			providerName: system.AmazonBedrockAPIKeyModelProvider,
			providerURL:  server.URL + "/openai/v1",
			dialect:      string(dialect),
		}, []chatMessage{{Role: "system", Content: "Review policy"}, {Role: "user", Content: "Hello"}})
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, "COMPLIANT", result)
		assert.Equal(t, "/openai/v1/responses", gotPath)
		assert.Equal(t, "openai.gpt-oss-120b-1:0", gotBody["model"])
		assert.Equal(t, "Review policy", gotBody["instructions"])
		assert.Equal(t, true, gotBody["stream"])
		assert.Equal(t, []any{map[string]any{"role": "user", "content": "Hello"}}, gotBody["input"])
	}
}

func TestCallLLMGenericResponses(t *testing.T) {
	var (
		gotPath string
		gotBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"COMPLIANT\"}\n\n")
	}))
	defer server.Close()

	result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
		targetModel:  "open-model",
		providerName: system.GenericResponsesModelProvider,
		providerURL:  server.URL + "/v1",
		dialect:      string(nanobottypes.DialectOpenResponses),
	}, []chatMessage{{Role: "system", Content: "Review policy"}, {Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "COMPLIANT", result)
	assert.Equal(t, "/v1/responses", gotPath)
	assert.Equal(t, "open-model", gotBody["model"])
	assert.Equal(t, "Review policy", gotBody["instructions"])
	assert.Equal(t, true, gotBody["stream"])
	assert.Equal(t, []any{map[string]any{"role": "user", "content": "Hello"}}, gotBody["input"])
}

func TestCallLLMAzureAnthropicMessages(t *testing.T) {
	var (
		gotPath    string
		gotBody    map[string]any
		gotHeaders http.Header
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"text\":\"yes\"}}\n\n")
	}))
	defer server.Close()

	transport, err := azure.Transport(system.AzureModelProvider, map[string]string{
		azure.APIKeyEnv: "azure-key",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
		targetModel:  "claude-sonnet",
		providerName: system.AzureModelProvider,
		providerURL:  server.URL + "/anthropic/v1",
		dialect:      string(nanobottypes.DialectAnthropicMessages),
		httpClient:   &http.Client{Transport: transport},
	}, []chatMessage{{Role: "system", Content: "Check policy"}, {Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "yes", result)
	assert.Equal(t, "/anthropic/v1/messages", gotPath)
	assert.Equal(t, "2023-06-01", gotHeaders.Get("anthropic-version"))
	assert.Equal(t, "Bearer azure-key", gotHeaders.Get("Authorization"))
	assert.Empty(t, gotHeaders.Get("api-key"))
	assert.NotContains(t, gotBody, "anthropic_version")
}

func TestCallLLMAzureOpenAIResponses(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"COMPLIANT\"}\n\n")
	}))
	defer server.Close()

	result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
		targetModel:  "gpt-5",
		providerName: system.AzureEntraModelProvider,
		providerURL:  server.URL + "/openai/v1",
		dialect:      string(nanobottypes.DialectOpenAIResponses),
	}, []chatMessage{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "COMPLIANT", result)
	assert.Equal(t, "/openai/v1/responses", gotPath)
}

func TestCallLLMNonBedrockAnthropicUsesChatCompletions(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"yes\"}}]}\n\n")
	}))
	defer server.Close()

	result, err := (&Helper{}).callLLM(context.Background(), &resolvedModel{
		targetModel:  "claude-haiku-4-5",
		providerName: system.AnthropicModelProvider,
		providerURL:  server.URL + "/v1",
		dialect:      string(nanobottypes.DialectAnthropicMessages),
	}, []chatMessage{{Role: "user", Content: "Hello"}})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "yes", result)
	assert.Equal(t, "/v1/chat/completions", gotPath)
}

func TestBuildConversationContextUserMessage(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "Book me a first class flight to Paris"},
	}

	result := BuildConversationContext(messages)
	assert.Equal(t, "User: Book me a first class flight to Paris", result)
}

func TestBuildConversationContextAssistantText(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there! How can I help?"},
	}

	result := BuildConversationContext(messages)
	assert.Equal(t, "User: Hello\nAssistant: Hi there! How can I help?", result)
}

func TestBuildConversationContextAssistantToolCalls(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "Book a flight"},
		{Role: "assistant", ToolCalls: []ToolCallInfo{
			{Name: "book_flight", Arguments: `{"destination": "Paris", "class": "first"}`},
		}},
	}

	result := BuildConversationContext(messages)
	expected := "User: Book a flight\nAssistant: [called tool \"book_flight\" with args: {\"destination\": \"Paris\", \"class\": \"first\"}]"
	assert.Equal(t, expected, result)
}

func TestBuildConversationContextAssistantTextAndToolCalls(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "assistant", Content: "Let me book that for you.", ToolCalls: []ToolCallInfo{
			{Name: "book_flight", Arguments: `{"destination": "Paris"}`},
		}},
	}

	result := BuildConversationContext(messages)
	expected := "Assistant: Let me book that for you.\nAssistant: [called tool \"book_flight\" with args: {\"destination\": \"Paris\"}]"
	assert.Equal(t, expected, result)
}

func TestBuildConversationContextToolOutputRedacted(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "assistant", ToolCalls: []ToolCallInfo{
			{Name: "search", Arguments: `{"query": "flights"}`},
		}},
		{Role: "tool", Content: "Here are 5 flights...", ToolCallID: "call_123"},
	}

	result := BuildConversationContext(messages)
	expected := "Assistant: [called tool \"search\" with args: {\"query\": \"flights\"}]\nTool: [tool output redacted]"
	assert.Equal(t, expected, result)
}

func TestBuildConversationContextSystemMessagesExcluded(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}

	result := BuildConversationContext(messages)
	assert.Equal(t, "User: Hello", result)
}

func TestBuildConversationContextEmpty(t *testing.T) {
	result := BuildConversationContext(nil)
	assert.Equal(t, "", result)
}

func TestBuildConversationContextFullConversation(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "system", Content: "You are a travel agent."},
		{Role: "user", Content: "Find me a flight to NYC"},
		{Role: "assistant", Content: "I'll search for flights.", ToolCalls: []ToolCallInfo{
			{Name: "search_flights", Arguments: `{"to": "NYC"}`},
		}},
		{Role: "tool", Content: "<big json response>", ToolCallID: "call_1"},
		{Role: "assistant", Content: "I found several options."},
		{Role: "user", Content: "Book the cheapest one in first class"},
	}

	result := BuildConversationContext(messages)
	expected := "User: Find me a flight to NYC\n" +
		"Assistant: I'll search for flights.\n" +
		"Assistant: [called tool \"search_flights\" with args: {\"to\": \"NYC\"}]\n" +
		"Tool: [tool output redacted]\n" +
		"Assistant: I found several options.\n" +
		"User: Book the cheapest one in first class"
	assert.Equal(t, expected, result)
}
