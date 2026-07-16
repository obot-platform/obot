package types

import "encoding/json"

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

// LocalAgentToolCallAuditLogInput is the client-reported portion of a completed local-agent tool
// call. Server-owned values such as the actor, recording time, source IP, target resolution, and
// payload visibility are deliberately absent.
type LocalAgentToolCallAuditLogInput struct {
	OccurredAt Time                                      `json:"occurredAt"`
	Action     LocalAgentToolCallAuditLogAction          `json:"action"`
	Target     LocalAgentToolCallAuditLogTarget          `json:"target"`
	Outcome    LocalAgentToolCallAuditLogOutcome         `json:"outcome"`
	Details    LocalAgentToolCallAuditLogReportedDetails `json:"details"`
}

// LocalAgentToolCallAuditLogAction identifies the tool name reported by the agent runtime. The
// protocol operation is always tools/call and is stamped by the server.
type LocalAgentToolCallAuditLogAction struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// LocalAgentToolCallAuditLogTarget is an unresolved, client-reported target. TargetType must be
// local_tool or mcp_tool. An MCP tool may include an mcp_server parent by name.
type LocalAgentToolCallAuditLogTarget struct {
	TargetType AuditLogTargetType                   `json:"targetType"`
	Name       string                               `json:"name"`
	Parent     *LocalAgentToolCallAuditLogTargetRef `json:"parent,omitempty"`
}

type LocalAgentToolCallAuditLogTargetRef struct {
	TargetType AuditLogTargetType `json:"targetType"`
	Name       string             `json:"name"`
}

// LocalAgentToolCallAuditLogOutcome uses the normalized read-side outcome vocabulary. Unknown is
// not accepted for completed local-agent submissions.
type LocalAgentToolCallAuditLogOutcome struct {
	Status     AuditLogOutcomeStatus `json:"status"`
	Reason     string                `json:"reason,omitempty"`
	Error      string                `json:"error,omitempty"`
	DurationMs int64                 `json:"durationMs,omitempty"`
}

type LocalAgentToolCallAuditLogReportedDetails struct {
	StartedAt   *Time                                 `json:"startedAt,omitempty"`
	Trace       LocalAgentToolCallAuditLogTrace       `json:"trace"`
	Agent       LocalAgentToolCallAuditLogAgent       `json:"agent"`
	Device      LocalAgentToolCallAuditLogDevice      `json:"device,omitzero"`
	Environment LocalAgentToolCallAuditLogEnvironment `json:"environment,omitzero"`
	Request     LocalAgentToolCallAuditLogPayload     `json:"request"`
	Response    LocalAgentToolCallAuditLogPayload     `json:"response"`
	RawEvent    json.RawMessage                       `json:"rawEvent"`
}

type LocalAgentToolCallAuditLogTrace struct {
	IdempotencyKey string `json:"idempotencyKey"`
	ToolUseID      string `json:"toolUseID,omitempty"`
	SessionID      string `json:"sessionID,omitempty"`
	TurnID         string `json:"turnID,omitempty"`
}

type LocalAgentToolCallAuditLogAgent struct {
	Provider       LocalAgentProvider `json:"provider"`
	Version        string             `json:"version,omitempty"`
	CLIName        string             `json:"cliName,omitempty"`
	CLIVersion     string             `json:"cliVersion"`
	Model          string             `json:"model,omitempty"`
	ModelID        string             `json:"modelID,omitempty"`
	PermissionMode string             `json:"permissionMode,omitempty"`
}

// LocalAgentToolCallAuditLogDevice contains reported device context only. The authenticated device
// ID and deployment ID are stamped by the server and cannot be supplied here.
type LocalAgentToolCallAuditLogDevice struct {
	Hostname      string `json:"hostname,omitempty"`
	OS            string `json:"os,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	LocalUsername string `json:"localUsername,omitempty"`
}

type LocalAgentToolCallAuditLogEnvironment struct {
	CWD               string   `json:"cwd,omitempty"`
	GitRoot           string   `json:"gitRoot,omitempty"`
	GitRemotes        []string `json:"gitRemotes,omitempty"`
	GitBranch         string   `json:"gitBranch,omitempty"`
	GitCommit         string   `json:"gitCommit,omitempty"`
	ReportedUserEmail string   `json:"reportedUserEmail,omitempty"`
	TranscriptPath    string   `json:"transcriptPath,omitempty"`
}

type LocalAgentToolCallAuditLogPayload struct {
	Body json.RawMessage `json:"body"`
}

type LocalAgentToolCallAuditLogSubmitRequest struct {
	Events []LocalAgentToolCallAuditLogInput `json:"events"`
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
