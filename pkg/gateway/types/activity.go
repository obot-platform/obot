package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type LLMProxyActivity struct {
	ID             uint
	CreatedAt      time.Time
	WorkflowID     string
	WorkflowStepID string
	AgentID        string
	ThreadID       string
	RunID          string
	Username       string
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
