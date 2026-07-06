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
	ID                  string    `gorm:"primaryKey;type:text"`
	CreatedAt           time.Time `gorm:"index;index:idx_llm_audit_user_created,priority:2;index:idx_llm_audit_provider_created,priority:2;index:idx_llm_audit_client_session_created,priority:2;index:idx_llm_audit_response_created,priority:2;index:idx_llm_audit_target_model_created,priority:2;index:idx_llm_audit_request_path_created,priority:2;index:idx_llm_audit_response_status_created,priority:2;index:idx_llm_audit_outcome_created,priority:2;index:idx_llm_audit_client_created,priority:2"`
	Duration            int64
	UserID              string `gorm:"type:text;index:idx_llm_audit_user_created,priority:1"`
	ModelProvider       string `gorm:"type:text;index:idx_llm_audit_provider_created,priority:1"`
	ModelID             string `gorm:"type:text"`
	TargetModel         string `gorm:"type:text;index:idx_llm_audit_target_model_created,priority:1"`
	ReasoningEffort     string `gorm:"type:text"`
	RequestPath         string `gorm:"type:text;index:idx_llm_audit_request_path_created,priority:1"`
	RequestMethod       string `gorm:"type:text"`
	RequestHeaders      string `gorm:"type:text"`
	RequestBody         string `gorm:"type:text"`
	RedactedRequestBody string `gorm:"type:text"`
	ResponseHeaders     string `gorm:"type:text"`
	ResponseBody        string `gorm:"type:text"`
	ResponseID          string `gorm:"type:text;index:idx_llm_audit_response_created,priority:1"`
	ResponseStatus      int    `gorm:"index:idx_llm_audit_response_status_created,priority:1"`
	Outcome             string `gorm:"type:text;index:idx_llm_audit_outcome_created,priority:1"`
	Error               string `gorm:"type:text"`
	InputTokens         int
	OutputTokens        int
	RequestID           string `gorm:"type:text"`
	Client              string `gorm:"type:text;index:idx_llm_audit_client_created,priority:1"`
	ClientVersion       string `gorm:"type:text"`
	ClientSessionID     string `gorm:"type:text;index:idx_llm_audit_client_session_created,priority:1"`
	ClientIP            string `gorm:"type:text"`
	Encrypted           bool
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}
