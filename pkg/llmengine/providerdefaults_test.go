package llmengine

import (
	"testing"
)

func TestApplyProviderDefaults(t *testing.T) {
	p := map[string]any{"provider": "openai"}
	ApplyProviderDefaults(p)
	nc := p["network_config"].(map[string]any)
	if nc["max_retries"] != defaultMaxRetries || nc["retry_backoff_initial"] != defaultRetryBackoffInitial || nc["retry_backoff_max"] != defaultRetryBackoffMax || nc["default_request_timeout_in_seconds"] != defaultRequestTimeout {
		t.Fatalf("defaults not applied: %v", nc)
	}

	// Settings the caller chose are kept, including turning retries off.
	p = map[string]any{"network_config": map[string]any{"max_retries": 0, "base_url": "https://example.com"}}
	ApplyProviderDefaults(p)
	nc = p["network_config"].(map[string]any)
	if nc["max_retries"] != 0 || nc["base_url"] != "https://example.com" || nc["retry_backoff_initial"] != defaultRetryBackoffInitial {
		t.Fatalf("caller settings not kept: %v", nc)
	}
}

func TestTrimProviderBaseURLVersion(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		baseURL      string
		want         string
	}{
		{
			name:         "OpenAI-compatible with /v1",
			providerType: "openai",
			baseURL:      "http://vllm.internal:8000/v1",
			want:         "http://vllm.internal:8000",
		},
		{
			name:         "with /v1/",
			providerType: "anthropic",
			baseURL:      "https://llm.example.com/proxy/v1/",
			want:         "https://llm.example.com/proxy",
		},
		{
			name:         "without a version",
			providerType: "openai",
			baseURL:      "https://llm.example.com/openai",
			want:         "https://llm.example.com/openai",
		},
		{
			name:         "a provider whose paths don't carry the version",
			providerType: "azure",
			baseURL:      "https://example.openai.azure.com/v1",
			want:         "https://example.openai.azure.com/v1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := map[string]any{"network_config": map[string]any{"base_url": tt.baseURL}}
			TrimProviderBaseURLVersion(p, tt.providerType)
			if got := p["network_config"].(map[string]any)["base_url"]; got != tt.want {
				t.Fatalf("base_url = %v, want %s", got, tt.want)
			}
		})
	}

	// A provider without a base URL is left alone.
	p := map[string]any{"provider": "openai"}
	TrimProviderBaseURLVersion(p, "openai")
	if _, ok := p["network_config"]; ok {
		t.Fatalf("network_config added: %v", p)
	}
}
