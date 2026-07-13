package types

import (
	"encoding/json"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

const (
	LLMAuditOutcomeSuccess  = "success"
	LLMAuditOutcomeCanceled = "canceled"
	LLMAuditOutcomeError    = "error"
)

type LLMAuditLog struct {
	ID                        string    `gorm:"primaryKey;type:text"`
	CreatedAt                 time.Time `gorm:"index;index:idx_llm_audit_user_created,priority:2;index:idx_llm_audit_provider_created,priority:2;index:idx_llm_audit_client_session_created,priority:2;index:idx_llm_audit_response_created,priority:2;index:idx_llm_audit_target_model_created,priority:2;index:idx_llm_audit_request_path_created,priority:2;index:idx_llm_audit_response_status_created,priority:2;index:idx_llm_audit_outcome_created,priority:2;index:idx_llm_audit_client_created,priority:2;index:idx_llm_audit_message_policy_triggered_created,priority:2"`
	Duration                  int64
	UserID                    string `gorm:"type:text;index:idx_llm_audit_user_created,priority:1"`
	ModelProvider             string `gorm:"type:text;index:idx_llm_audit_provider_created,priority:1"`
	ModelID                   string `gorm:"type:text"`
	TargetModel               string `gorm:"type:text;index:idx_llm_audit_target_model_created,priority:1"`
	ReasoningEffort           string `gorm:"type:text"`
	RequestPath               string `gorm:"type:text;index:idx_llm_audit_request_path_created,priority:1"`
	RequestMethod             string `gorm:"type:text"`
	RequestHeaders            json.RawMessage
	RequestBody               json.RawMessage
	PolicyModifiedRequestBody json.RawMessage
	MessagePolicyTriggered    bool `gorm:"index:idx_llm_audit_message_policy_triggered_created,priority:1"`
	ResponseHeaders           json.RawMessage
	ResponseBody              json.RawMessage
	ResponseID                string `gorm:"type:text;index:idx_llm_audit_response_created,priority:1"`
	ResponseStatus            int    `gorm:"index:idx_llm_audit_response_status_created,priority:1"`
	Outcome                   string `gorm:"type:text;index:idx_llm_audit_outcome_created,priority:1"`
	Error                     string `gorm:"type:text"`
	InputTokens               int
	OutputTokens              int
	RequestID                 string `gorm:"type:text"`
	Client                    string `gorm:"type:text;index:idx_llm_audit_client_created,priority:1"`
	ClientVersion             string `gorm:"type:text"`
	ClientSessionID           string `gorm:"type:text;index:idx_llm_audit_client_session_created,priority:1"`
	ClientIP                  string `gorm:"type:text"`
	Encrypted                 bool
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}

func ConvertLLMAuditLog(a LLMAuditLog) types2.LLMAuditLog {
	return types2.LLMAuditLog{
		ID:                        a.ID,
		CreatedAt:                 *types2.NewTime(a.CreatedAt),
		Duration:                  a.Duration,
		UserID:                    a.UserID,
		ModelProvider:             a.ModelProvider,
		ModelID:                   a.ModelID,
		TargetModel:               a.TargetModel,
		ReasoningEffort:           a.ReasoningEffort,
		RequestPath:               a.RequestPath,
		RequestMethod:             a.RequestMethod,
		RequestHeaders:            a.RequestHeaders,
		RequestBody:               a.RequestBody,
		PolicyModifiedRequestBody: a.PolicyModifiedRequestBody,
		MessagePolicyTriggered:    a.MessagePolicyTriggered,
		ResponseHeaders:           a.ResponseHeaders,
		ResponseBody:              a.ResponseBody,
		ResponseID:                a.ResponseID,
		ResponseStatus:            a.ResponseStatus,
		Outcome:                   a.Outcome,
		Error:                     a.Error,
		InputTokens:               a.InputTokens,
		OutputTokens:              a.OutputTokens,
		RequestID:                 a.RequestID,
		Client:                    a.Client,
		ClientVersion:             a.ClientVersion,
		ClientSessionID:           a.ClientSessionID,
		ClientIP:                  a.ClientIP,
	}
}
