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
