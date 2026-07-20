package types

import "encoding/json"

// LLMAuditLog represents an audit log entry for LLM gateway calls.
type LLMAuditLog struct {
	ID                        string          `json:"id"`
	CreatedAt                 Time            `json:"createdAt"`
	Duration                  int64           `json:"duration"`
	UserID                    string          `json:"userID"`
	ModelProvider             string          `json:"modelProvider"`
	ModelID                   string          `json:"modelID"`
	TargetModel               string          `json:"targetModel"`
	ReasoningEffort           string          `json:"reasoningEffort"`
	RequestPath               string          `json:"requestPath"`
	RequestMethod             string          `json:"requestMethod"`
	RequestHeaders            json.RawMessage `json:"requestHeaders,omitempty"`
	RequestBody               json.RawMessage `json:"requestBody,omitempty"`
	PolicyModifiedRequestBody json.RawMessage `json:"policyModifiedRequestBody,omitempty"`
	MessagePolicyTriggered    bool            `json:"messagePolicyTriggered"`
	ResponseHeaders           json.RawMessage `json:"responseHeaders,omitempty"`
	ResponseBody              json.RawMessage `json:"responseBody,omitempty"`
	ResponseID                string          `json:"responseID"`
	ResponseStatus            int             `json:"responseStatus"`
	Outcome                   string          `json:"outcome"`
	Error                     string          `json:"error,omitempty"`
	InputTokens               int             `json:"inputTokens"`
	OutputTokens              int             `json:"outputTokens"`
	RequestID                 string          `json:"requestID"`
	UserAgent                 string          `json:"userAgent"`
	ClientSessionID           string          `json:"clientSessionID"`
	ClientIP                  string          `json:"clientIP"`
}

type LLMAuditLogResponse struct {
	LLMAuditLogList `json:",inline"`
	Total           int64 `json:"total"`
	Limit           int   `json:"limit"`
	Offset          int   `json:"offset"`
}

// LLMAuditLogList represents a list of LLM audit logs.
type LLMAuditLogList List[LLMAuditLog]
