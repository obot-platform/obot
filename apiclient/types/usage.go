package types

type TokenUsage struct {
	Date Time `json:"date,omitzero"`

	UserID string `json:"userID,omitempty"`
	Model  string `json:"model,omitempty"`

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

type RemainingTokenUsage struct {
	UserID                string `json:"userID,omitempty"`
	InputTokens           int    `json:"inputTokens"`
	OutputTokens          int    `json:"outputTokens"`
	UnlimitedInputTokens  bool   `json:"unlimitedInputTokens"`
	UnlimitedOutputTokens bool   `json:"unlimitedOutputTokens"`
}
