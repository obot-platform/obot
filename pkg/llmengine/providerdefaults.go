package llmengine

import (
	"strings"
)

// Provider network defaults. Retries matter beyond transient errors: the engine moves to a
// provider's next key only when it retries, so without them a provider with several keys
// returns a dead or rate-limited key's error instead of failing over to a working key.
const (
	defaultMaxRetries          = 2
	defaultRetryBackoffInitial = 500  // milliseconds
	defaultRetryBackoffMax     = 5000 // milliseconds
	defaultRequestTimeout      = 300  // seconds
)

var (
	// versionedPathProviders are the provider types whose API paths the engine appends to the
	// base URL with their version, such as "/v1/chat/completions", so their base URL must not end
	// in the version itself.
	versionedPathProviders = map[string]bool{
		"openai":     true,
		"anthropic":  true,
		"vllm":       true,
		"ollama":     true,
		"sgl":        true,
		"groq":       true,
		"xai":        true,
		"mistral":    true,
		"openrouter": true,
		"cerebras":   true,
		"fireworks":  true,
		"nebius":     true,
		"parasail":   true,
	}
)

// ApplyProviderDefaults fills in the network settings a new provider leaves unset: retries
// with backoff, which also let requests fail over between the provider's keys, and the
// request timeout.
func ApplyProviderDefaults(provider map[string]any) {
	nc, _ := provider["network_config"].(map[string]any)
	if nc == nil {
		nc = map[string]any{}
		provider["network_config"] = nc
	}
	for key, value := range map[string]any{
		"max_retries":                        defaultMaxRetries,
		"retry_backoff_initial":              defaultRetryBackoffInitial,
		"retry_backoff_max":                  defaultRetryBackoffMax,
		"default_request_timeout_in_seconds": defaultRequestTimeout,
	} {
		if _, set := nc[key]; !set {
			nc[key] = value
		}
	}
}

// TrimProviderBaseURLVersion removes a trailing "/v1" from the base URL of a provider of type
// providerType (the provider's own name for a built-in provider, or the type a custom provider
// is built on) when the engine adds the version to every path itself. OpenAI-compatible
// servers usually document their base URL with "/v1", which the engine would otherwise turn
// into "/v1/v1/chat/completions". It reports whether it changed the base URL.
func TrimProviderBaseURLVersion(provider map[string]any, providerType string) bool {
	if !versionedPathProviders[providerType] {
		return false
	}
	nc, _ := provider["network_config"].(map[string]any)
	baseURL, _ := nc["base_url"].(string)
	if trimmed, ok := strings.CutSuffix(strings.TrimRight(baseURL, "/"), "/v1"); ok && trimmed != "" {
		nc["base_url"] = trimmed
		return true
	}
	return false
}
