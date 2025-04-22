package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type LLMProxyActivity struct {
	ID               uint
	UserID           string
	CreatedAt        time.Time
	WorkflowID       string
	WorkflowStepID   string
	AgentID          string
	ThreadID         string
	RunID            string
	Path             string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
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
}

type TokenActivity struct {
	RunTokenActivity
	RunCount int
}

func ConvertTokenActivity(a TokenActivity) types2.TokenUsage {
	return types2.TokenUsage{
		UserID:           a.UserID,
		RunName:          a.Name,
		Date:             *types2.NewTime(a.CreatedAt),
		RunCount:         a.RunCount,
		PromptTokens:     a.PromptTokens,
		CompletionTokens: a.CompletionTokens,
		TotalTokens:      a.TotalTokens,
	}
}
