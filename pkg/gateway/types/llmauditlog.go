package types

import (
	"time"

	"gorm.io/datatypes"
)

const (
	LLMAuditOutcomeSuccess  = "success"
	LLMAuditOutcomeCanceled = "canceled"
	LLMAuditOutcomeError    = "error"
)

type LLMAuditLog struct {
	ID                  string    `gorm:"primaryKey"`
	CreatedAt           time.Time `gorm:"primaryKey;index;index:idx_llm_audit_user_created,priority:2;index:idx_llm_audit_provider_created,priority:2;index:idx_llm_audit_client_session_created,priority:2;index:idx_llm_audit_response_created,priority:2"`
	Duration            int64
	UserID              string `gorm:"index:idx_llm_audit_user_created,priority:1"`
	ModelProvider       string `gorm:"index:idx_llm_audit_provider_created,priority:1"`
	ModelID             string
	TargetModel         string
	ReasoningEffort     string
	RequestPath         string
	RequestMethod       string
	RequestHeaders      datatypes.JSON
	RequestBody         string
	RedactedRequestBody string
	ResponseHeaders     datatypes.JSON
	ResponseBody        string
	ResponseID          string `gorm:"index:idx_llm_audit_response_created,priority:1"`
	ResponseStatus      int
	Outcome             string
	Error               string
	InputTokens         int
	OutputTokens        int
	RequestID           string
	Client              string
	ClientVersion       string
	ClientSessionID     string `gorm:"index:idx_llm_audit_client_session_created,priority:1"`
	ClientIP            string
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}
