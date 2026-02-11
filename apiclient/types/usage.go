package types

type TokenUsage struct {
	UserID           string `json:"userID,omitempty"`
	RunName          string `json:"runName,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
	Date             Time   `json:"date,omitzero"`
	PersonalToken    bool   `json:"personalToken"`
}

type TokenUsageList List[TokenUsage]

type TokenUsageByDate struct {
	Date  string       `json:"date"`
	Items []TokenUsage `json:"items"`
}

type TokenUsageSeries []TokenUsageByDate

type RemainingTokenUsage struct {
	UserID                    string `json:"userID,omitempty"`
	PromptTokens              int    `json:"promptTokens"`
	CompletionTokens          int    `json:"completionTokens"`
	UnlimitedPromptTokens     bool   `json:"unlimitedPromptTokens"`
	UnlimitedCompletionTokens bool   `json:"unlimitedCompletionTokens"`
}

type RemainingTokenUsageList List[RemainingTokenUsage]
