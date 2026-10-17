package types

type TokenUsage struct {
	Date Time `json:"date,omitzero"`

	UserID     string `json:"userID,omitempty"`
	Model      string `json:"model,omitempty"`
	APIKeyID   *uint  `json:"apiKeyID,omitempty"`
	APIKeyName string `json:"apiKeyName,omitempty"`
	// Provider is the provider that served the request, which routing rules and fallbacks can
	// make differ from the requested model's.
	Provider string `json:"provider,omitempty"`
	// Outcome is success, error or canceled. Empty on records made before outcomes were
	// recorded, which were all successful requests.
	Outcome string `json:"outcome,omitempty"`
	// DurationMs is how long the request took, in milliseconds.
	DurationMs int64 `json:"durationMs,omitempty"`
	// Fallback is true when a fallback served the request.
	Fallback bool `json:"fallback,omitempty"`

	// InputTokens is the total input: CacheReadTokens + CacheWriteTokens + uncached input.
	InputTokens int `json:"inputTokens"`
	// CacheReadTokens is the cache-hit input tokens; a subset of InputTokens.
	CacheReadTokens int `json:"cacheReadTokens"`
	// CacheWriteTokens is the cache-write input tokens (5m + 1h); a subset of InputTokens
	// (Anthropic only).
	CacheWriteTokens int `json:"cacheWriteTokens"`
	// OutputTokens is the total output, including ThinkingTokens.
	OutputTokens int `json:"outputTokens"`
	// ThinkingTokens is the thinking/reasoning output tokens; a subset of OutputTokens.
	ThinkingTokens int `json:"thinkingTokens"`
	// TotalTokens is InputTokens + OutputTokens.
	TotalTokens int `json:"totalTokens"`

	// InputSpend is the total USD on InputTokens (each bucket at its own rate):
	// CacheReadSpend + CacheWriteSpend + uncached-input spend. 0 when unpriced.
	InputSpend float64 `json:"inputSpend"`
	// CacheReadSpend is the USD on CacheReadTokens; a subset of InputSpend.
	CacheReadSpend float64 `json:"cacheReadSpend"`
	// CacheWriteSpend is the USD on CacheWriteTokens; a subset of InputSpend.
	CacheWriteSpend float64 `json:"cacheWriteSpend"`
	// OutputSpend is the USD on OutputTokens.
	OutputSpend float64 `json:"outputSpend"`
	// TotalSpend is InputSpend + OutputSpend.
	TotalSpend float64 `json:"totalSpend"`
}

type TokenUsageList List[TokenUsage]

// TokenUsageStats summarizes LLM gateway requests over a time range.
type TokenUsageStats struct {
	Start Time `json:"start"`
	End   Time `json:"end"`
	// BucketSeconds is the width of each of Buckets.
	BucketSeconds int64                     `json:"bucketSeconds"`
	Totals        TokenUsageStatsTotals     `json:"totals"`
	Buckets       []TokenUsageStatsBucket   `json:"buckets"`
	Providers     []TokenUsageStatsProvider `json:"providers"`
	// TopModelsByRequests and TopModelsBySpend are the models that served the most requests and
	// spent the most.
	TopModelsByRequests []TokenUsageStatsModel `json:"topModelsByRequests"`
	TopModelsBySpend    []TokenUsageStatsModel `json:"topModelsBySpend"`
}

// TokenUsageStatsTotals counts requests and their usage.
type TokenUsageStatsTotals struct {
	Requests int64 `json:"requests"`
	// Successes are the requests that succeeded; the others failed or were canceled.
	Successes int64 `json:"successes"`
	// Fallbacks are the requests a fallback served.
	Fallbacks int64 `json:"fallbacks"`
	// AvgDurationMs is the average request duration over the requests with a recorded duration.
	AvgDurationMs float64 `json:"avgDurationMs"`
	InputTokens   int64   `json:"inputTokens"`
	OutputTokens  int64   `json:"outputTokens"`
	TotalTokens   int64   `json:"totalTokens"`
	TotalSpend    float64 `json:"totalSpend"`
}

// TokenUsageStatsBucket is the requests in one period of a time histogram.
type TokenUsageStatsBucket struct {
	Time Time `json:"time"`
	TokenUsageStatsTotals
}

// TokenUsageStatsProvider is the requests a provider served.
type TokenUsageStatsProvider struct {
	Provider string `json:"provider"`
	TokenUsageStatsTotals
}

// TokenUsageStatsModel is the requests a model served.
type TokenUsageStatsModel struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
	TokenUsageStatsTotals
}
