package types

type TokenUsage struct {
	UserID           string `json:"userID,omitempty"`
	RunName          string `json:"runName,omitempty"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
	Date             Time   `json:"date,omitzero"`
}

type TokenUsageList List[TokenUsage]

type RemainingTokenUsage struct {
	UserID                    string `json:"userID,omitempty"`
	PromptTokens              int    `json:"promptTokens"`
	CompletionTokens          int    `json:"completionTokens"`
	UnlimitedPromptTokens     bool   `json:"unlimitedPromptTokens"`
	UnlimitedCompletionTokens bool   `json:"unlimitedCompletionTokens"`
}

type RemainingTokenUsageList List[RemainingTokenUsage]
