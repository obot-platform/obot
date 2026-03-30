package messagepolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
