package llmengine

// Provider network defaults. Retries matter beyond transient errors: the engine moves to a
// provider's next key only when it retries, so without them a provider with several keys
// returns a dead or rate-limited key's error instead of failing over to a working key.
const (
	defaultMaxRetries          = 2
	defaultRetryBackoffInitial = 500  // milliseconds
	defaultRetryBackoffMax     = 5000 // milliseconds
	defaultRequestTimeout      = 300  // seconds
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
