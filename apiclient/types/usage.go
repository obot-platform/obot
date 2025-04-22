package types

type TokenUsage struct {
	UserID           string `json:"userID,omitempty"`
	RunCount         int    `json:"runCount,omitempty"`
	RunName          string `json:"runName,omitempty"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
	Date             Time   `json:"date,omitzero"`
}

type TokenUsageList List[TokenUsage]
