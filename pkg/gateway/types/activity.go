package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type LLMProxyActivity struct {
	ID             uint
	UserID         string
	CreatedAt      time.Time
	WorkflowID     string
	WorkflowStepID string
	AgentID        string
	ProjectID      string
	ThreadID       string
	RunID          string
	Path           string
}

type APIActivity struct {
	ID     uint
	UserID string
	Date   time.Time
}

func ConvertAPIActivity(a APIActivity) types2.APIActivity {
	return types2.APIActivity{
		UserID: a.UserID,
		Date:   *types2.NewTime(a.Date),
	}
}

type RunTokenActivity struct {
	ID               uint
	CreatedAt        time.Time
	Name             string
	UserID           string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	PersonalToken    bool
}

func ConvertTokenActivity(a RunTokenActivity) types2.TokenUsage {
	return types2.TokenUsage{
		UserID:           a.UserID,
		RunName:          a.Name,
		Date:             *types2.NewTime(a.CreatedAt),
		PromptTokens:     a.PromptTokens,
		CompletionTokens: a.CompletionTokens,
		TotalTokens:      a.TotalTokens,
		PersonalToken:    a.PersonalToken,
	}
}

type RemainingTokenUsage struct {
	PromptTokens              int
	CompletionTokens          int
	UnlimitedPromptTokens     bool
	UnlimitedCompletionTokens bool
}

func ConvertRemainingTokenUsage(userID string, r *RemainingTokenUsage) types2.RemainingTokenUsage {
	return types2.RemainingTokenUsage{
		UserID:                    userID,
		PromptTokens:              r.PromptTokens,
		CompletionTokens:          r.CompletionTokens,
		UnlimitedPromptTokens:     r.UnlimitedPromptTokens,
		UnlimitedCompletionTokens: r.UnlimitedCompletionTokens,
	}
}
