package types

import (
	"encoding/json"
)

// LLMAuditLog represents an audit log entry for LLM gateway calls.
type LLMAuditLog struct {
	ID        string `json:"id"`
	CreatedAt Time   `json:"createdAt"`
	Duration  int64  `json:"duration"`
	UserID    string `json:"userID"`
	APIKeyID  *uint  `json:"apiKeyID,omitempty"`
	// APIKeyRevoked reports the API key's current lifecycle state.
	APIKeyRevoked bool `json:"apiKeyRevoked,omitempty"`
	// APIKeyName is the event-time display value and is a masked identifier for unnamed keys.
	APIKeyName    string `json:"apiKeyName,omitempty"`
	ModelProvider string `json:"modelProvider"`
	ModelID       string `json:"modelID"`
	TargetModel   string `json:"targetModel"`
	// ServedProvider and ServedModel are the provider and model that served the request, which
	// routing rules and fallbacks can make differ from TargetModel. ProviderKey names the
	// gateway key it was served with, and Fallback is true when a fallback served it.
	// ClaudeSubscription is true when the caller's Claude subscription paid for it instead,
	// with no gateway key.
	ServedProvider     string          `json:"servedProvider,omitempty"`
	ServedModel        string          `json:"servedModel,omitempty"`
	ProviderKey        string          `json:"providerKey,omitempty"`
	Fallback           bool            `json:"fallback,omitempty"`
	ClaudeSubscription bool            `json:"claudeSubscription,omitempty"`
	ReasoningEffort    string          `json:"reasoningEffort"`
	RequestPath        string          `json:"requestPath"`
	RequestMethod      string          `json:"requestMethod"`
	RequestHeaders     json.RawMessage `json:"requestHeaders,omitempty"`
	RequestBody        json.RawMessage `json:"requestBody,omitempty"`
	ResponseHeaders    json.RawMessage `json:"responseHeaders,omitempty"`
	ResponseBody       json.RawMessage `json:"responseBody,omitempty"`
	ResponseID         string          `json:"responseID"`
	ResponseStatus     int             `json:"responseStatus"`
	Outcome            string          `json:"outcome"`
	Error              string          `json:"error,omitempty"`
	InputTokens        int             `json:"inputTokens"`
	OutputTokens       int             `json:"outputTokens"`
	// CacheReadTokens and CacheWriteTokens are the input tokens read from and written to the
	// provider's prompt cache, and ReasoningTokens the output tokens spent reasoning; each is
	// part of InputTokens or OutputTokens.
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  int `json:"reasoningTokens,omitempty"`
	// Spend is what the request cost in USD, as recorded in token usage; 0 when the model has
	// no price.
	Spend           float64 `json:"spend,omitempty"`
	RequestID       string  `json:"requestID"`
	UserAgent       string  `json:"userAgent"`
	ClientSessionID string  `json:"clientSessionID"`
	ClientIP        string  `json:"clientIP"`
	// GuardrailFindings is what the LLM gateway's guardrails did to the request, in the order
	// their guardrail policies ran.
	GuardrailFindings []LLMAuditGuardrailFinding `json:"guardrailFindings,omitempty"`
}

// LLMAuditGuardrailFinding is what one guardrail policy did to an LLM request: the guardrail that
// found something, or failed, and what the policy did about it. It never includes the content
// that was found.
type LLMAuditGuardrailFinding struct {
	// Stage is input (the request) or output (the response).
	Stage         string `json:"stage"`
	PolicyID      string `json:"policyID"`
	PolicyName    string `json:"policyName"`
	GuardrailName string `json:"guardrailName,omitempty"`
	GuardrailType string `json:"guardrailType,omitempty"`
	// Outcome is blocked, redacted, logged or error.
	Outcome string `json:"outcome"`
	// Categories are the kinds of content the guardrail found, such as "secret".
	Categories []string `json:"categories,omitempty"`
	// Error is why the guardrail could not be evaluated, for an error outcome.
	Error string `json:"error,omitempty"`
}

type LLMAuditLogResponse struct {
	LLMAuditLogList `json:",inline"`
	Total           int64 `json:"total"`
	Limit           int   `json:"limit"`
	Offset          int   `json:"offset"`
}

// LLMAuditLogList represents a list of LLM audit logs.
type LLMAuditLogList List[LLMAuditLog]
