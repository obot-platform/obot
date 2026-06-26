package types

import (
	"encoding/json"
)

// MCPAuditLog represents an audit log entry for MCP API calls
type MCPAuditLog struct {
	ID                       uint                              `json:"id"`
	CreatedAt                Time                              `json:"createdAt"`
	SourceType               AuditLogSourceType                `json:"sourceType"`
	UserID                   string                            `json:"userID"`
	ClientIP                 string                            `json:"clientIP"`
	MCPFields                *MCPAuditLogFields                `json:"mcpFields,omitempty"`
	LocalAgentToolCallFields *LocalAgentToolCallAuditLogFields `json:"localAgentToolCallFields,omitempty"`
}

type AuditLogSourceType string

const (
	AuditLogSourceTypeMCP                AuditLogSourceType = "mcp"
	AuditLogSourceTypeLocalAgentToolCall AuditLogSourceType = "local_agent_tool_call"
)

type LocalAgentProvider string

const (
	LocalAgentProviderClaudeCode LocalAgentProvider = "claude_code"
	LocalAgentProviderCodex      LocalAgentProvider = "codex"
	LocalAgentProviderVSCode     LocalAgentProvider = "vscode"
	LocalAgentProviderCursor     LocalAgentProvider = "cursor"
)

type LocalAgentAuditLogPhase string

const (
	LocalAgentAuditLogPhasePreTool  LocalAgentAuditLogPhase = "pre_tool"
	LocalAgentAuditLogPhasePostTool LocalAgentAuditLogPhase = "post_tool"
	LocalAgentAuditLogPhaseFailure  LocalAgentAuditLogPhase = "failure"
)

type LocalAgentAuditLogStatus string

const (
	LocalAgentAuditLogStatusDenied    LocalAgentAuditLogStatus = "denied"
	LocalAgentAuditLogStatusSucceeded LocalAgentAuditLogStatus = "succeeded"
	LocalAgentAuditLogStatusFailed    LocalAgentAuditLogStatus = "failed"
	LocalAgentAuditLogStatusTimeout   LocalAgentAuditLogStatus = "timeout"
)

type LocalAgentIdentityStatus string

const (
	LocalAgentIdentityStatusAuthenticatedUser LocalAgentIdentityStatus = "authenticated_user"
	LocalAgentIdentityStatusAnonymousDevice   LocalAgentIdentityStatus = "anonymous_device"
	LocalAgentIdentityStatusUnresolved        LocalAgentIdentityStatus = "unresolved"
)

type MCPAuditLogFields struct {
	MCPID                     string          `json:"mcpID"`
	APIKey                    string          `json:"apiKey,omitempty"`
	PowerUserWorkspaceID      string          `json:"powerUserWorkspaceID,omitempty"`
	MCPServerDisplayName      string          `json:"mcpServerDisplayName"`
	MCPServerCatalogEntryName string          `json:"mcpServerCatalogEntryName"`
	ClientInfo                ClientInfo      `json:"client"`
	CallType                  string          `json:"callType"`
	CallIdentifier            string          `json:"callIdentifier,omitempty"`
	RequestMutated            bool            `json:"requestMutated"`
	RequestBody               json.RawMessage `json:"requestBody,omitempty"`
	MutatedRequestBody        json.RawMessage `json:"mutatedRequestBody,omitempty"`
	ResponseMutated           bool            `json:"responseMutated"`
	ResponseBody              json.RawMessage `json:"responseBody,omitempty"`
	OriginalResponseBody      json.RawMessage `json:"originalResponseBody,omitempty"`
	ResponseStatus            int             `json:"responseStatus"`
	WebhookStatuses           []WebhookStatus `json:"webhookStatuses,omitempty"`
	Error                     string          `json:"error,omitempty"`
	ProcessingTimeMs          int64           `json:"processingTimeMs"`
	SessionID                 string          `json:"sessionID,omitempty"`
	ObotAuditCorrelationID    string          `json:"obotAuditCorrelationID,omitempty"`
	RequestID                 string          `json:"requestID,omitempty"`
	UserAgent                 string          `json:"userAgent,omitempty"`
	RequestHeaders            json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders           json.RawMessage `json:"responseHeaders,omitempty"`
}

type LocalAgentToolCallAuditLogFields struct {
	AgentProvider string `json:"agentProvider"`
	AgentVersion  string `json:"agentVersion,omitempty"`
	CLIName       string `json:"cliName,omitempty"`
	CLIVersion    string `json:"cliVersion"`

	Status      string `json:"status"`
	FailureType string `json:"failureType,omitempty"`
	ObservedAt  Time   `json:"observedAt"`
	StartedAt   *Time  `json:"startedAt,omitempty"`
	DurationMs  int64  `json:"durationMs,omitempty"`
	Error       string `json:"error,omitempty"`

	IdempotencyKey string `json:"idempotencyKey"`
	ToolUseID      string `json:"toolUseID,omitempty"`
	SessionID      string `json:"sessionID,omitempty"`
	TurnID         string `json:"turnID,omitempty"`

	ToolName      string `json:"toolName"`
	ToolKind      string `json:"toolKind,omitempty"`
	MCPServerHint string `json:"mcpServerHint,omitempty"`
	MCPToolName   string `json:"mcpToolName,omitempty"`

	ObotAuditCorrelationID string `json:"obotAuditCorrelationID,omitempty"`

	Model          string `json:"model,omitempty"`
	ModelID        string `json:"modelID,omitempty"`
	PermissionMode string `json:"permissionMode,omitempty"`

	DeviceID          string `json:"deviceID,omitempty"`
	Hostname          string `json:"hostname,omitempty"`
	OS                string `json:"os,omitempty"`
	Arch              string `json:"arch,omitempty"`
	LocalUsername     string `json:"localUsername,omitempty"`
	ReportedUserEmail string `json:"reportedUserEmail,omitempty"`
	IdentityStatus    string `json:"identityStatus"`

	CWD           string   `json:"cwd,omitempty"`
	GitRepoRoot   string   `json:"gitRepoRoot,omitempty"`
	GitRemoteURLs []string `json:"gitRemoteURLs,omitempty"`
	GitBranch     string   `json:"gitBranch,omitempty"`
	GitCommitSHA  string   `json:"gitCommitSHA,omitempty"`

	TranscriptPath string `json:"transcriptPath,omitempty"`

	ToolInput      json.RawMessage `json:"toolInput,omitempty"`
	ToolOutput     json.RawMessage `json:"toolOutput,omitempty"`
	RawHookPayload json.RawMessage `json:"rawHookPayload,omitempty"`
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
