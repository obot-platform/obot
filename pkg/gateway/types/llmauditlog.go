package types

import (
	"time"
)

const (
	LLMAuditOutcomeSuccess  = "success"
	LLMAuditOutcomeCanceled = "canceled"
	LLMAuditOutcomeError    = "error"
)

type LLMAuditLog struct {
	ID                  string    `gorm:"primaryKey"`
	CreatedAt           time.Time `gorm:"primaryKey;index;index:idx_llm_audit_user_created,priority:2;index:idx_llm_audit_provider_created,priority:2;index:idx_llm_audit_client_session_created,priority:2;index:idx_llm_audit_response_created,priority:2;index:idx_llm_audit_target_model_created,priority:2;index:idx_llm_audit_request_path_created,priority:2;index:idx_llm_audit_response_status_created,priority:2;index:idx_llm_audit_outcome_created,priority:2;index:idx_llm_audit_client_created,priority:2"`
	Duration            int64
	UserID              string `gorm:"index:idx_llm_audit_user_created,priority:1"`
	ModelProvider       string `gorm:"index:idx_llm_audit_provider_created,priority:1"`
	ModelID             string
	TargetModel         string `gorm:"index:idx_llm_audit_target_model_created,priority:1"`
	ReasoningEffort     string
	RequestPath         string `gorm:"index:idx_llm_audit_request_path_created,priority:1"`
	RequestMethod       string
	RequestHeaders      string
	RequestBody         string
	RedactedRequestBody string
	ResponseHeaders     string
	ResponseBody        string
	ResponseID          string `gorm:"index:idx_llm_audit_response_created,priority:1"`
	ResponseStatus      int    `gorm:"index:idx_llm_audit_response_status_created,priority:1"`
	Outcome             string `gorm:"index:idx_llm_audit_outcome_created,priority:1"`
	Error               string
	InputTokens         int
	OutputTokens        int
	RequestID           string
	Client              string `gorm:"index:idx_llm_audit_client_created,priority:1"`
	ClientVersion       string
	ClientSessionID     string `gorm:"index:idx_llm_audit_client_session_created,priority:1"`
	ClientIP            string
	Encrypted           bool
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}
