// Package llm contains shared types for interacting with language model APIs.
package llm

const (
	// DialectAnthropicMessages is the Anthropic Messages API format.
	DialectAnthropicMessages Dialect = "AnthropicMessages"
	// DialectOpenAIResponses is the OpenAI Responses API format.
	DialectOpenAIResponses Dialect = "OpenAIResponses"
	// DialectOpenResponses is the provider-neutral OpenResponses API format.
	DialectOpenResponses Dialect = "OpenResponses"
	// DialectOpenAIChatCompletions is the OpenAI Chat Completions API format.
	DialectOpenAIChatCompletions Dialect = "OpenAIChatCompletions"
	// DialectDefault is the default API format used by Obot.
	DialectDefault = DialectOpenAIResponses
)

// Dialect identifies the API format used by an LLM provider.
type Dialect string
