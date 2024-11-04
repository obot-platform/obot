package types

import "time"

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
