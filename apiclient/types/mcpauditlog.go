package types

import (
	"encoding/json"
)

// Audit log source types.
const (
	AuditLogSourceTypeMCP        = "mcp"
	AuditLogSourceTypeLocalAgent = "local_agent"
)

// Generic audit event types.
const (
	AuditLogEventTypeToolCall     = "tool_call"
	AuditLogEventTypeResourceRead = "resource_read"
	AuditLogEventTypePromptGet    = "prompt_get"
	AuditLogEventTypeMCPRequest   = "mcp_request"
)

// Audit log outcomes.
const (
	AuditLogOutcomeSuccess = "success"
	AuditLogOutcomeError   = "error"
)

// MCPAuditLog represents an audit log entry. Despite the name (kept for API
// compatibility), it can represent generic audit events distinguished by
// SourceType; MCP-specific fields are empty for non-MCP rows.
type MCPAuditLog struct {
	ID                        uint                        `json:"id"`
	EventID                   string                      `json:"eventID,omitempty"`
	SourceType                string                      `json:"sourceType,omitempty"`
	EventType                 string                      `json:"eventType,omitempty"`
	CreatedAt                 Time                        `json:"createdAt"`
	ReceivedAt                *Time                       `json:"receivedAt,omitempty"`
	UserID                    string                      `json:"userID"`
	DeviceID                  string                      `json:"deviceID,omitempty"`
	Outcome                   string                      `json:"outcome,omitempty"`
	MCPID                     string                      `json:"mcpID"`
	APIKey                    string                      `json:"apiKey,omitempty"`
	PowerUserWorkspaceID      string                      `json:"powerUserWorkspaceID,omitempty"`
	MCPServerDisplayName      string                      `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string                      `json:"mcpServerCatalogEntryName"`
	ClientInfo                ClientInfo                  `json:"client"`
	ClientIP                  string                      `json:"clientIP"`
	CallType                  string                      `json:"callType"`
	CallIdentifier            string                      `json:"callIdentifier,omitempty"`
	RequestMutated            bool                        `json:"requestMutated"`
	RequestBody               json.RawMessage             `json:"requestBody,omitempty"`
	MutatedRequestBody        json.RawMessage             `json:"mutatedRequestBody,omitempty"`
	ResponseMutated           bool                        `json:"responseMutated"`
	ResponseBody              json.RawMessage             `json:"responseBody,omitempty"`
	OriginalResponseBody      json.RawMessage             `json:"originalResponseBody,omitempty"`
	ResponseStatus            int                         `json:"responseStatus"`
	WebhookStatuses           []WebhookStatus             `json:"webhookStatuses,omitempty"`
	Error                     string                      `json:"error,omitempty"`
	ErrorDetail               string                      `json:"errorDetail,omitempty"`
	RawEvent                  json.RawMessage             `json:"rawEvent,omitempty"`
	Context                   *AuditLogContext            `json:"context,omitempty"`
	PayloadMeta               map[string]PayloadFieldMeta `json:"payloadMeta,omitempty"`
	ProcessingTimeMs          int64                       `json:"processingTimeMs"`
	SessionID                 string                      `json:"sessionID,omitempty"`
	RequestID                 string                      `json:"requestID,omitempty"`
	UserAgent                 string                      `json:"userAgent,omitempty"`
	RequestHeaders            json.RawMessage             `json:"requestHeaders,omitempty"`
	ResponseHeaders           json.RawMessage             `json:"responseHeaders,omitempty"`
}

// AuditEvent is the canonical generic audit event shape used for ingestion of
// non-MCP audit logs (e.g. local agent tool calls submitted by the CLI).
type AuditEvent struct {
	EventID    string `json:"eventID"`
	SourceType string `json:"sourceType"`
	EventType  string `json:"eventType"`
	CreatedAt  Time   `json:"createdAt"`
	// ReceivedAt and UserID are assigned by the server on ingestion;
	// client-provided values are ignored.
	ReceivedAt  *Time                       `json:"receivedAt,omitempty"`
	UserID      string                      `json:"userID,omitempty"`
	DeviceID    string                      `json:"deviceID,omitempty"`
	Client      ClientInfo                  `json:"client"`
	Tool        ToolInfo                    `json:"tool"`
	Outcome     string                      `json:"outcome"`
	DurationMs  int64                       `json:"durationMs"`
	SessionID   string                      `json:"sessionID,omitempty"`
	Request     json.RawMessage             `json:"request,omitempty"`
	Response    json.RawMessage             `json:"response,omitempty"`
	Error       string                      `json:"error,omitempty"`
	RawEvent    json.RawMessage             `json:"rawEvent,omitempty"`
	Context     *AuditLogContext            `json:"context,omitempty"`
	PayloadMeta map[string]PayloadFieldMeta `json:"payloadMeta,omitempty"`
}

const (
	AuditEventSubmitStatusAccepted  = "accepted"
	AuditEventSubmitStatusDuplicate = "duplicate"
	AuditEventSubmitStatusError     = "error"
)

type AuditEventSubmitResponse struct {
	Items []AuditEventSubmitStatus `json:"items"`
}

type AuditEventSubmitStatus struct {
	EventID string `json:"eventID"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

// ToolInfo identifies what was invoked and what kind of invocation it was.
type ToolInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// AuditLogContext holds source-specific, non-indexed audit event metadata.
type AuditLogContext struct {
	ConversationID  string `json:"conversationID,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	GitRemote       string `json:"gitRemote,omitempty"`
	GitBranch       string `json:"gitBranch,omitempty"`
	SourceHookEvent string `json:"sourceHookEvent,omitempty"`
	ClientEventID   string `json:"clientEventID,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	OS              string `json:"os,omitempty"`
	Arch            string `json:"arch,omitempty"`
	Username        string `json:"username,omitempty"`
}

// PayloadFieldMeta records truncation info for a single payload field.
type PayloadFieldMeta struct {
	Truncated     bool  `json:"truncated,omitempty"`
	OriginalBytes int64 `json:"originalBytes,omitempty"`
	StoredBytes   int64 `json:"storedBytes,omitempty"`
}

type MCPAuditLogResponse struct {
	MCPAuditLogList `json:",inline"`
	Total           int64 `json:"total"`
	Limit           int   `json:"limit"`
	Offset          int   `json:"offset"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type WebhookStatus struct {
	Type    string `json:"type,omitempty"`
	Method  string `json:"method,omitempty"`
	URL     string `json:"url,omitempty"`
	Name    string `json:"name,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message"`
}

// MCPAuditLogList represents a list of MCP audit logs
type MCPAuditLogList List[MCPAuditLog]

// MCPUsageStatItem represents usage statistics for MCP servers
type MCPUsageStatItem struct {
	MCPID                     string                 `json:"mcpID"`
	MCPServerDisplayName      string                 `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string                 `json:"mcpServerCatalogEntryName"`
	ToolCalls                 []MCPToolCallStats     `json:"toolCalls,omitempty"`
	ResourceReads             []MCPResourceReadStats `json:"resourceReads,omitempty"`
	PromptReads               []MCPPromptReadStats   `json:"promptReads,omitempty"`
}

type MCPUsageStats struct {
	TotalCalls  int64              `json:"totalCalls"`
	UniqueUsers int64              `json:"uniqueUsers"`
	TimeStart   Time               `json:"timeStart"`
	TimeEnd     Time               `json:"timeEnd"`
	Items       []MCPUsageStatItem `json:"items"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStatsItem struct {
	CreatedAt        Time   `json:"createdAt"`
	UserID           string `json:"userID"`
	ProcessingTimeMs int64  `json:"processingTimeMs"`
	ResponseStatus   int    `json:"responseStatus"`
	Error            string `json:"error"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStats struct {
	ToolName  string                 `json:"toolName"`
	CallCount int64                  `json:"callCount"`
	Items     []MCPToolCallStatsItem `json:"items"`
}

// MCPResourceReadStats represents statistics for individual resource reads
type MCPResourceReadStats struct {
	ResourceURI string `json:"resourceURI"`
	ReadCount   int64  `json:"readCount"`
}

// MCPPromptReadStats represents statistics for individual prompt reads
type MCPPromptReadStats struct {
	PromptName string `json:"promptName"`
	ReadCount  int64  `json:"readCount"`
}

// MCPUsageStatsList represents a list of MCP usage statistics
type MCPUsageStatsList List[MCPUsageStatItem]
