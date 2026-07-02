package types

// LLMAuditLog represents an audit log entry for LLM gateway calls.
type LLMAuditLog struct {
	ID                  string `json:"id"`
	CreatedAt           Time   `json:"createdAt"`
	Duration            int64  `json:"duration"`
	UserID              string `json:"userID"`
	ModelProvider       string `json:"modelProvider"`
	ModelID             string `json:"modelID"`
	TargetModel         string `json:"targetModel"`
	ReasoningEffort     string `json:"reasoningEffort"`
	RequestPath         string `json:"requestPath"`
	RequestMethod       string `json:"requestMethod"`
	RequestHeaders      string `json:"requestHeaders,omitempty"`
	RequestBody         string `json:"requestBody,omitempty"`
	RedactedRequestBody string `json:"redactedRequestBody,omitempty"`
	ResponseHeaders     string `json:"responseHeaders,omitempty"`
	ResponseBody        string `json:"responseBody,omitempty"`
	ResponseID          string `json:"responseID"`
	ResponseStatus      int    `json:"responseStatus"`
	Outcome             string `json:"outcome"`
	Error               string `json:"error,omitempty"`
	InputTokens         int    `json:"inputTokens"`
	OutputTokens        int    `json:"outputTokens"`
	RequestID           string `json:"requestID"`
	Client              string `json:"client"`
	ClientVersion       string `json:"clientVersion"`
	ClientSessionID     string `json:"clientSessionID"`
	ClientIP            string `json:"clientIP"`
}

type LLMAuditLogResponse struct {
	LLMAuditLogList `json:",inline"`
	Total           int64 `json:"total"`
	Limit           int   `json:"limit"`
	Offset          int   `json:"offset"`
}

// LLMAuditLogList represents a list of LLM audit logs.
type LLMAuditLogList List[LLMAuditLog]
