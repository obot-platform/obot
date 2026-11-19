package types

const (
	LLMProviderTesterClientName     = "obot-llm-provider-tester"
	LLMProviderTesterDefaultMessage = "say hello"
	LLMProviderTesterMaxMessageSize = 4 * 1024
)

type LLMProviderTestRequest struct {
	Message  string `json:"message,omitempty"`
	Model    string `json:"model"`
	APIKeyID uint   `json:"apiKeyId"`
}

type LLMProviderTestEventType string

const (
	LLMProviderTestEventTextDelta LLMProviderTestEventType = "text_delta"
	LLMProviderTestEventReply     LLMProviderTestEventType = "reply"
	LLMProviderTestEventError     LLMProviderTestEventType = "error"
)

// LLMProviderTestEvent is one server-sent event of a provider test. A test
// ends with exactly one reply or error event, which carries the duration.
type LLMProviderTestEvent struct {
	Type LLMProviderTestEventType `json:"type"`
	// Delta is a chunk of streamed reply text.
	Delta string `json:"delta,omitempty"`
	// Reply is the complete reply text.
	Reply string `json:"reply,omitempty"`
	// Status is the HTTP status returned by the LLM proxy when the request failed.
	Status int `json:"status,omitempty"`
	// Error is the failure message. Upstream error bodies are returned as-is.
	Error string `json:"error,omitempty"`
	// Provider and ServedModel identify what actually served the request.
	Provider    string `json:"provider,omitempty"`
	ServedModel string `json:"servedModel,omitempty"`
	DurationMs  int64  `json:"durationMs,omitempty"`
}
