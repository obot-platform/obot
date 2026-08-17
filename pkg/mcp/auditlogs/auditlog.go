package auditlogs

import (
	"encoding/json"
	"strings"
	"time"
)

// MCPAuditLog represents an audit log entry for an MCP API call.
type MCPAuditLog struct {
	// Metadata is additional information about this server that a user can provide for audit log tracking purposes.
	// For example, Obot uses this to track catalog information.
	Metadata             map[string]string  `json:"metadata,omitempty"`
	CreatedAt            time.Time          `json:"createdAt"`
	Subject              string             `json:"subject"`
	APIKey               string             `json:"apiKey,omitempty"`
	ClientName           string             `json:"clientName"`
	ClientVersion        string             `json:"clientVersion"`
	ClientIP             string             `json:"clientIP"`
	CallType             string             `json:"callType"`
	CallIdentifier       string             `json:"callIdentifier,omitempty"`
	RequestBody          json.RawMessage    `json:"requestBody,omitempty"`
	MutatedRequestBody   json.RawMessage    `json:"mutatedRequestBody,omitempty"`
	ResponseBody         json.RawMessage    `json:"responseBody,omitempty"`
	OriginalResponseBody json.RawMessage    `json:"originalResponseBody,omitempty"`
	ResponseStatus       int                `json:"responseStatus"`
	Error                string             `json:"error,omitempty"`
	ProcessingTimeMs     int64              `json:"processingTimeMs"`
	SessionID            string             `json:"sessionID,omitempty"`
	WebhookStatuses      []MCPWebhookStatus `json:"webhookStatuses,omitempty"`

	RequestID       string          `json:"requestID,omitempty"`
	UserAgent       string          `json:"userAgent,omitempty"`
	RequestHeaders  json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders json.RawMessage `json:"responseHeaders,omitempty"`
}

// MCPWebhookStatus describes the result of running an MCP webhook.
type MCPWebhookStatus struct {
	Type    string `json:"type,omitempty"`
	Method  string `json:"method,omitempty"`
	Name    string `json:"name"`
	Tool    string `json:"tool"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// SetProcessingTime records elapsed processing time in whole milliseconds.
// Sub-millisecond calls are rounded up so completed calls are not mistaken for
// missing durations.
func (m *MCPAuditLog) SetProcessingTime() {
	m.ProcessingTimeMs = processingTimeMilliseconds(time.Since(m.CreatedAt))
}

func processingTimeMilliseconds(elapsed time.Duration) int64 {
	return max(elapsed.Milliseconds(), 1)
}

// RedactAPIKey redacts an API key, retaining a stable prefix for attribution.
func RedactAPIKey(apiKey string) string {
	if len(apiKey) < 2 {
		return ""
	}

	parts := strings.SplitAfterN(apiKey, "-", 4)
	prefix := strings.Join(parts[:min(3, len(parts))], "")

	if len(apiKey) < 20 {
		half := apiKey[:len(apiKey)/2]
		if len(parts) >= 4 && len(prefix) > len(half) {
			return prefix
		}
		return half
	}

	if len(parts) < 4 || len(prefix) < 12 {
		return apiKey[:12]
	}
	return prefix
}
