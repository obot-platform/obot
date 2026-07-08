package types

import "encoding/json"

// AuditLogEventType identifies the normalized kind of activity represented by an audit event.
// It is intentionally distinct from AuditLogSourceType, whose values describe the internal
// ingestion and storage format.
type AuditLogEventType string

const (
	// AuditLogEventTypeMCPCall identifies a call received by an Obot-managed MCP server.
	AuditLogEventTypeMCPCall AuditLogEventType = "mcp_call"
	// AuditLogEventTypeLocalAgentToolCall identifies a tool call reported by a local agent runtime.
	AuditLogEventTypeLocalAgentToolCall AuditLogEventType = "local_agent_tool_call"
)

// AuditLogTimestampSource describes who supplied an event's occurred-at timestamp. Consumers can
// use it to distinguish server-controlled timestamps from client-reported timestamps.
type AuditLogTimestampSource string

const (
	// AuditLogTimestampSourceServer means OccurredAt was assigned by the Obot server.
	AuditLogTimestampSourceServer AuditLogTimestampSource = "server"
	// AuditLogTimestampSourceClientReported means OccurredAt came from the submitting client and
	// should not be treated as an independently verified server timestamp.
	AuditLogTimestampSourceClientReported AuditLogTimestampSource = "client_reported"
)

// AuditLogActorType identifies the server-established principal responsible for an event. Reported
// context such as hostname, local username, agent provider, or email never determines this value.
type AuditLogActorType string

const (
	// AuditLogActorTypeUser identifies an authenticated Obot user.
	AuditLogActorTypeUser AuditLogActorType = "user"
	// AuditLogActorTypeDevice identifies an enrolled device.
	AuditLogActorTypeDevice AuditLogActorType = "device"
	// AuditLogActorTypeCredential is used when a redacted MCP API-key identifier is the only
	// server-established principal and no user subject is available.
	AuditLogActorTypeCredential AuditLogActorType = "credential"
	// AuditLogActorTypeUnknown means the stored identity evidence was missing, unresolved, or
	// internally inconsistent, so no principal could be safely attributed.
	AuditLogActorTypeUnknown AuditLogActorType = "unknown"
)

// AuditLogTargetType identifies the kind of object on which an action operated.
type AuditLogTargetType string

const (
	// AuditLogTargetTypeMCPServer identifies an MCP server as the direct target.
	AuditLogTargetTypeMCPServer AuditLogTargetType = "mcp_server"
	// AuditLogTargetTypeMCPTool identifies a tool exposed by an MCP server.
	AuditLogTargetTypeMCPTool AuditLogTargetType = "mcp_tool"
	// AuditLogTargetTypeMCPResource identifies a resource exposed by an MCP server.
	AuditLogTargetTypeMCPResource AuditLogTargetType = "mcp_resource"
	// AuditLogTargetTypeMCPPrompt identifies a prompt exposed by an MCP server.
	AuditLogTargetTypeMCPPrompt AuditLogTargetType = "mcp_prompt"
	// AuditLogTargetTypeLocalTool identifies a non-MCP tool reported by a local agent runtime.
	AuditLogTargetTypeLocalTool AuditLogTargetType = "local_tool"
)

// AuditLogOutcomeStatus is the source-independent classification of an event's result.
type AuditLogOutcomeStatus string

const (
	// AuditLogOutcomeStatusSuccess means the operation completed successfully.
	AuditLogOutcomeStatusSuccess AuditLogOutcomeStatus = "success"
	// AuditLogOutcomeStatusFailure means the operation completed with an error or failing status.
	AuditLogOutcomeStatusFailure AuditLogOutcomeStatus = "failure"
	// AuditLogOutcomeStatusDenied means authentication or authorization rejected the operation.
	AuditLogOutcomeStatusDenied AuditLogOutcomeStatus = "denied"
	// AuditLogOutcomeStatusTimeout means the source explicitly reported a timeout.
	AuditLogOutcomeStatusTimeout AuditLogOutcomeStatus = "timeout"
	// AuditLogOutcomeStatusUnknown means the stored evidence is insufficient to infer a result. An
	// unmatched MCP request is unknown rather than being assumed to have timed out.
	AuditLogOutcomeStatusUnknown AuditLogOutcomeStatus = "unknown"
)

// AuditLogEvent is the normalized public read model shared by MCP and local-agent audit logs.
// List responses normally omit Details; detail responses and JSONL exports may include them
// according to the caller's payload access.
type AuditLogEvent struct {
	ID        uint              `json:"id"`
	Timestamp AuditLogTimestamp `json:"timestamp"`
	EventType AuditLogEventType `json:"eventType"`
	Actor     AuditLogActor     `json:"actor"`
	Action    AuditLogAction    `json:"action"`
	Target    AuditLogTarget    `json:"target"`
	Outcome   AuditLogOutcome   `json:"outcome"`
	Details   *AuditLogDetails  `json:"details,omitempty"`
}

// AuditLogTimestamp contains both the effective event time and the server-controlled recording
// time. OccurredAt drives filtering and ordering, while RecordedAt exposes clock skew or delayed
// submission for client-reported events.
type AuditLogTimestamp struct {
	OccurredAt Time                    `json:"occurredAt"`
	RecordedAt Time                    `json:"recordedAt"`
	Source     AuditLogTimestampSource `json:"source"`
}

// AuditLogActor describes the server-established principal attributed to an event. ID refers to
// the principal selected by ActorType; CredentialID is supplementary authentication context and
// does not represent a second actor.
type AuditLogActor struct {
	ActorType AuditLogActorType `json:"actorType"`
	ID        string            `json:"id,omitempty"`
	// CredentialID records authentication context when the actor itself is a known user.
	CredentialID string `json:"credentialID,omitempty"`
}

// AuditLogAction describes what the actor attempted. Operation contains the protocol operation,
// Name identifies the operation-specific item when available, and Kind carries source-reported
// tool context when it is meaningful.
type AuditLogAction struct {
	Operation string `json:"operation"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// AuditLogTargetRef is a compact reference to an audit target or its parent. ID is used for
// server-resolved objects; Name may also contain a client-reported hint.
type AuditLogTargetRef struct {
	TargetType AuditLogTargetType `json:"targetType"`
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name,omitempty"`
}

// AuditLogTarget describes the direct object of an action. Parent expresses containment, such as
// an MCP tool belonging to an MCP server. Resolved is true only when Obot correlated the target to
// a server-side object; client-reported names and hints remain unresolved.
type AuditLogTarget struct {
	AuditLogTargetRef `json:",inline"`
	Parent            *AuditLogTargetRef `json:"parent,omitempty"`
	Resolved          bool               `json:"resolved"`
}

// AuditLogOutcome contains the normalized result plus source-specific evidence. Reason is a stable
// failure category when supplied, Error is human-readable diagnostic text, and DurationMs is the
// source-reported or server-measured processing duration.
type AuditLogOutcome struct {
	Status     AuditLogOutcomeStatus `json:"status"`
	HTTPStatus int                   `json:"httpStatus,omitempty"`
	Reason     string                `json:"reason,omitempty"`
	Error      string                `json:"error,omitempty"`
	DurationMs int64                 `json:"durationMs,omitempty"`
}

// AuditLogDetails contains optional typed metadata used by detail views and JSONL exports. The
// common event summary remains usable without this object. PayloadRedacted explicitly reports
// whether payload and sensitive environment values were withheld for the caller.
type AuditLogDetails struct {
	Trace           *AuditLogTraceDetails       `json:"trace,omitempty"`
	Network         *AuditLogNetworkDetails     `json:"network,omitempty"`
	Client          *AuditLogClientDetails      `json:"client,omitempty"`
	Agent           *AuditLogAgentDetails       `json:"agent,omitempty"`
	Device          *AuditLogDeviceDetails      `json:"device,omitempty"`
	Scope           *AuditLogScopeDetails       `json:"scope,omitempty"`
	Environment     *AuditLogEnvironmentDetails `json:"environment,omitempty"`
	Request         *AuditLogPayloadDetails     `json:"request,omitempty"`
	Response        *AuditLogPayloadDetails     `json:"response,omitempty"`
	WebhookStatuses []WebhookStatus             `json:"webhookStatuses,omitempty"`
	RawEvent        json.RawMessage             `json:"rawEvent,omitempty"`
	StartedAt       *Time                       `json:"startedAt,omitempty"`
	PayloadRedacted bool                        `json:"payloadRedacted"`
}

// AuditLogTraceDetails groups identifiers that correlate an event with requests, sessions, tool
// uses, and agent conversation turns.
type AuditLogTraceDetails struct {
	SessionID      string `json:"sessionID,omitempty"`
	RequestID      string `json:"requestID,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	ToolUseID      string `json:"toolUseID,omitempty"`
	TurnID         string `json:"turnID,omitempty"`
}

// AuditLogNetworkDetails contains server-observed network context for an event.
type AuditLogNetworkDetails struct {
	ClientIP string `json:"clientIP,omitempty"`
}

// AuditLogClientDetails describes the MCP client that issued a request.
type AuditLogClientDetails struct {
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
}

// AuditLogAgentDetails describes a local agent runtime. It is reported execution context, not the
// authenticated actor identity.
type AuditLogAgentDetails struct {
	Provider       LocalAgentProvider `json:"provider,omitempty"`
	Version        string             `json:"version,omitempty"`
	CLIName        string             `json:"cliName,omitempty"`
	CLIVersion     string             `json:"cliVersion,omitempty"`
	Model          string             `json:"model,omitempty"`
	ModelID        string             `json:"modelID,omitempty"`
	PermissionMode string             `json:"permissionMode,omitempty"`
}

// AuditLogDeviceDetails describes the device context reported for a local-agent event. ID and
// DeploymentID may be server-established; the remaining fields are client-reported metadata.
type AuditLogDeviceDetails struct {
	ID            string `json:"id,omitempty"`
	DeploymentID  uint   `json:"deploymentID,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	OS            string `json:"os,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	LocalUsername string `json:"localUsername,omitempty"`
}

// AuditLogScopeDetails contains Obot workspace and MCP catalog context without overloading the
// event target.
type AuditLogScopeDetails struct {
	PowerUserWorkspaceID      string `json:"powerUserWorkspaceID,omitempty"`
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
}

// AuditLogEnvironmentDetails contains client-reported local execution and source-control context.
// These values may be sensitive and are omitted when PayloadRedacted is true.
type AuditLogEnvironmentDetails struct {
	CWD               string   `json:"cwd,omitempty"`
	GitRoot           string   `json:"gitRoot,omitempty"`
	GitRemotes        []string `json:"gitRemotes,omitempty"`
	GitBranch         string   `json:"gitBranch,omitempty"`
	GitCommit         string   `json:"gitCommit,omitempty"`
	ReportedUserEmail string   `json:"reportedUserEmail,omitempty"`
	TranscriptPath    string   `json:"transcriptPath,omitempty"`
}

// AuditLogPayloadDetails represents either the request side or response side of an event. For a
// local tool call, Body contains the tool input or output. MutatedBody and OriginalBody preserve
// webhook mutation context for MCP calls.
type AuditLogPayloadDetails struct {
	Headers      json.RawMessage `json:"headers,omitempty"`
	Body         json.RawMessage `json:"body,omitempty"`
	MutatedBody  json.RawMessage `json:"mutatedBody,omitempty"`
	OriginalBody json.RawMessage `json:"originalBody,omitempty"`
	Mutated      bool            `json:"mutated"`
}

// AuditLogEventResponse is a paginated response containing one globally ordered event window and
// the total number of matching events across the selected sources.
type AuditLogEventResponse struct {
	AuditLogEventList `json:",inline"`
	Total             int64 `json:"total"`
	Limit             int   `json:"limit"`
	Offset            int   `json:"offset"`
}

// AuditLogEventList is the standard list envelope for normalized audit events.
type AuditLogEventList List[AuditLogEvent]
