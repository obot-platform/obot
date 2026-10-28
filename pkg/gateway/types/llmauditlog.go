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
	ID        string    `gorm:"primaryKey;type:text"`
	CreatedAt time.Time `gorm:"index;index:idx_llm_audit_user_created,priority:2;index:idx_llm_audit_provider_created,priority:2;index:idx_llm_audit_client_session_created,priority:2;index:idx_llm_audit_response_created,priority:2;index:idx_llm_audit_target_model_created,priority:2;index:idx_llm_audit_request_path_created,priority:2;index:idx_llm_audit_response_status_created,priority:2;index:idx_llm_audit_outcome_created,priority:2;index:idx_llm_audit_user_agent_created,priority:2;index:idx_llm_audit_message_policy_triggered_created,priority:2;index:idx_llm_audit_api_key_created,priority:2;index:idx_llm_audit_served_provider_created,priority:2"`
	Duration  int64
	UserID    string `gorm:"type:text;index:idx_llm_audit_user_created,priority:1"`
	APIKeyID  *uint  `gorm:"index:idx_llm_audit_api_key_created,priority:1"`
	// APIKeyRevoked is current lifecycle metadata populated when audit logs are read.
	// It is not persisted as part of the historical event snapshot.
	APIKeyRevoked bool `gorm:"-"`
	// APIKeyName is the event-time display value. Unnamed keys snapshot their
	// non-secret masked identifier instead of requiring an API-key table join.
	APIKeyName    string `gorm:"type:text"`
	ModelProvider string `gorm:"type:text;index:idx_llm_audit_provider_created,priority:1"`
	ModelID       string `gorm:"type:text"`
	// TargetModel is the requested model.
	TargetModel string `gorm:"type:text;index:idx_llm_audit_target_model_created,priority:1"`
	// ServedProvider and ServedModel are the provider and model that served the request, which
	// routing rules and fallbacks can make differ from TargetModel. ProviderKey names the
	// gateway key it was served with, and Fallback is true when a fallback served it.
	// ClaudeSubscription is true when the caller's Claude subscription paid for it instead,
	// with no gateway key.
	ServedProvider            string `gorm:"type:text;index:idx_llm_audit_served_provider_created,priority:1"`
	ServedModel               string `gorm:"type:text"`
	ProviderKey               string `gorm:"type:text"`
	Fallback                  bool   `gorm:"not null;default:false"`
	ClaudeSubscription        bool   `gorm:"not null;default:false"`
	ReasoningEffort           string `gorm:"type:text"`
	RequestPath               string `gorm:"type:text;index:idx_llm_audit_request_path_created,priority:1"`
	RequestMethod             string `gorm:"type:text"`
	RequestHeaders            json.RawMessage
	RequestBody               json.RawMessage
	PolicyModifiedRequestBody json.RawMessage
	MessagePolicyTriggered    bool `gorm:"not null;default:false;index:idx_llm_audit_message_policy_triggered_created,priority:1"`
	ResponseHeaders           json.RawMessage
	ResponseBody              json.RawMessage
	ResponseID                string `gorm:"type:text;index:idx_llm_audit_response_created,priority:1"`
	ResponseStatus            int    `gorm:"index:idx_llm_audit_response_status_created,priority:1"`
	Outcome                   string `gorm:"type:text;index:idx_llm_audit_outcome_created,priority:1"`
	Error                     string `gorm:"type:text"`
	InputTokens               int
	OutputTokens              int
	// CacheReadTokens and CacheWriteTokens are the input tokens read from and written to the
	// provider's prompt cache, and ReasoningTokens the output tokens spent reasoning; each is
	// part of InputTokens or OutputTokens.
	CacheReadTokens  int `gorm:"not null;default:0"`
	CacheWriteTokens int `gorm:"not null;default:0"`
	ReasoningTokens  int `gorm:"not null;default:0"`
	// Spend is what the request cost in USD, as recorded in token usage; 0 when the model has
	// no price.
	Spend           float64 `gorm:"not null;default:0"`
	RequestID       string  `gorm:"type:text"`
	UserAgent       string  `gorm:"type:text;index:idx_llm_audit_user_agent_created,priority:1"`
	ClientSessionID string  `gorm:"type:text;index:idx_llm_audit_client_session_created,priority:1"`
	ClientIP        string  `gorm:"type:text"`
	// GuardrailFindings say which guardrail policies found something and what they did, but
	// not what they found, so they are stored unencrypted.
	GuardrailFindings []types2.LLMAuditGuardrailFinding `gorm:"serializer:json"`
	Encrypted         bool
}

func (LLMAuditLog) TableName() string {
	return "llm_audit_logs"
}

func ConvertLLMAuditLog(a LLMAuditLog) types2.LLMAuditLog {
	return types2.LLMAuditLog{
		ID:                 a.ID,
		CreatedAt:          *types2.NewTime(a.CreatedAt),
		Duration:           a.Duration,
		UserID:             a.UserID,
		APIKeyID:           a.APIKeyID,
		APIKeyRevoked:      a.APIKeyRevoked,
		APIKeyName:         a.APIKeyName,
		ModelProvider:      a.ModelProvider,
		ModelID:            a.ModelID,
		TargetModel:        a.TargetModel,
		ServedProvider:     a.ServedProvider,
		ServedModel:        a.ServedModel,
		ProviderKey:        a.ProviderKey,
		Fallback:           a.Fallback,
		ClaudeSubscription: a.ClaudeSubscription,
		ReasoningEffort:    a.ReasoningEffort,
		RequestPath:        a.RequestPath,
		RequestMethod:      a.RequestMethod,
		RequestHeaders:     a.RequestHeaders,
		RequestBody:        a.RequestBody,
		ResponseHeaders:    a.ResponseHeaders,
		ResponseBody:       a.ResponseBody,
		ResponseID:         a.ResponseID,
		ResponseStatus:     a.ResponseStatus,
		Outcome:            a.Outcome,
		Error:              a.Error,
		InputTokens:        a.InputTokens,
		OutputTokens:       a.OutputTokens,
		CacheReadTokens:    a.CacheReadTokens,
		CacheWriteTokens:   a.CacheWriteTokens,
		ReasoningTokens:    a.ReasoningTokens,
		Spend:              a.Spend,
		RequestID:          a.RequestID,
		UserAgent:          a.UserAgent,
		ClientSessionID:    a.ClientSessionID,
		ClientIP:           a.ClientIP,
		GuardrailFindings:  a.GuardrailFindings,
	}
}
