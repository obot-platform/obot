package types

import (
	"encoding/json"
)

type (
	AuditLogSourceType string
	AuditLogEventType  string
	AuditLogOutcome    string
)

// Audit log source types.
const (
	AuditLogSourceTypeMCP                AuditLogSourceType = "mcp"
	AuditLogSourceTypeLocalAgentToolCall AuditLogSourceType = "local_agent_tool_call"
)

// Generic audit event types.
const (
	AuditLogEventTypeToolCall     AuditLogEventType = "tool_call"
	AuditLogEventTypeResourceRead AuditLogEventType = "resource_read"
	AuditLogEventTypePromptGet    AuditLogEventType = "prompt_get"
	AuditLogEventTypeMCPRequest   AuditLogEventType = "mcp_request"
)

// Audit log outcomes.
const (
	AuditLogOutcomeSuccess AuditLogOutcome = "success"
	AuditLogOutcomeError   AuditLogOutcome = "error"
)

// AuditLog represents an audit log entry. It can represent generic audit
// events distinguished by SourceType. Source-specific fields are returned under
// MCP or LocalAgentToolCall.
type AuditLog struct {
	ID        uint `json:"id"`
	CreatedAt Time `json:"createdAt"`

	SourceType AuditLogSourceType `json:"sourceType,omitempty"`
	EventType  AuditLogEventType  `json:"eventType,omitempty"`
	ReceivedAt *Time              `json:"receivedAt,omitempty"`
	Outcome    AuditLogOutcome    `json:"outcome,omitempty"`
	UserID     string             `json:"userID"`

	ClientInfo       ClientInfo      `json:"client"`
	CallType         string          `json:"callType"`
	CallIdentifier   string          `json:"callIdentifier,omitempty"`
	RequestBody      json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody     json.RawMessage `json:"responseBody,omitempty"`
	Error            string          `json:"error,omitempty"`
	ProcessingTimeMs int64           `json:"processingTimeMs"`
	SessionID        string          `json:"sessionID,omitempty"`
	ResponseReceived bool            `json:"responseReceived"`

	MCP                *MCPAuditLog                `json:"mcp,omitempty"`
	LocalAgentToolCall *LocalAgentToolCallAuditLog `json:"local,omitempty"`
}

// MCPAuditLog contains fields meaningful only for MCP gateway/shim rows.
type MCPAuditLog struct {
	APIKey                    string          `json:"apiKey,omitempty"`
	MCPID                     string          `json:"mcpID"`
	PowerUserWorkspaceID      string          `json:"powerUserWorkspaceID,omitempty"`
	MCPServerDisplayName      string          `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string          `json:"mcpServerCatalogEntryName"`
	ClientIP                  string          `json:"clientIP"`
	RequestMutated            bool            `json:"requestMutated"`
	MutatedRequestBody        json.RawMessage `json:"mutatedRequestBody,omitempty"`
	ResponseMutated           bool            `json:"responseMutated"`
	OriginalResponseBody      json.RawMessage `json:"originalResponseBody,omitempty"`
	ResponseStatus            int             `json:"responseStatus"`
	WebhookStatuses           []WebhookStatus `json:"webhookStatuses,omitempty"`
	RequestID                 string          `json:"requestID,omitempty"`
	UserAgent                 string          `json:"userAgent,omitempty"`
	RequestHeaders            json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders           json.RawMessage `json:"responseHeaders,omitempty"`
}

// LocalAgentToolCallAuditLog contains fields meaningful only for local-agent audit events.
type LocalAgentToolCallAuditLog struct {
	EventID  string `json:"eventID,omitempty"`
	DeviceID string `json:"deviceID,omitempty"`

	ErrorDetail string                             `json:"errorDetail,omitempty"`
	RawEvent    json.RawMessage                    `json:"rawEvent,omitempty"`
	Context     *LocalAgentToolCallAuditLogContext `json:"context,omitempty"`
	PayloadMeta map[string]PayloadFieldMeta        `json:"payloadMeta,omitempty"`
}

// AuditEvent is the canonical generic audit event shape used for ingestion of
// non-MCP audit logs (e.g. local agent tool calls submitted by the CLI).
type AuditEvent struct {
	EventID    string             `json:"eventID"`
	SourceType AuditLogSourceType `json:"sourceType"`
	EventType  AuditLogEventType  `json:"eventType"`
	CreatedAt  Time               `json:"createdAt"`
	// ReceivedAt and UserID are assigned by the server on ingestion;
	// client-provided values are ignored.
	ReceivedAt  *Time                              `json:"receivedAt,omitempty"`
	UserID      string                             `json:"userID,omitempty"`
	DeviceID    string                             `json:"deviceID,omitempty"`
	Client      ClientInfo                         `json:"client"`
	Tool        ToolInfo                           `json:"tool"`
	Outcome     AuditLogOutcome                    `json:"outcome"`
	DurationMs  int64                              `json:"durationMs"`
	SessionID   string                             `json:"sessionID,omitempty"`
	Request     json.RawMessage                    `json:"request,omitempty"`
	Response    json.RawMessage                    `json:"response,omitempty"`
	Error       string                             `json:"error,omitempty"`
	RawEvent    json.RawMessage                    `json:"rawEvent,omitempty"`
	Context     *LocalAgentToolCallAuditLogContext `json:"context,omitempty"`
	PayloadMeta map[string]PayloadFieldMeta        `json:"payloadMeta,omitempty"`
}

// ToolInfo identifies what was invoked and what kind of invocation it was.
type ToolInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// LocalAgentToolCallAuditLogContext holds source-specific, non-indexed audit event metadata
// for local agent tool call audit logs.
type LocalAgentToolCallAuditLogContext struct {
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

type AuditLogResponse struct {
	AuditLogList `json:",inline"`
	Total        int64 `json:"total"`
	Limit        int   `json:"limit"`
	Offset       int   `json:"offset"`
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

// AuditLogList represents a list of audit logs
type AuditLogList List[AuditLog]

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
