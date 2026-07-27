//nolint:revive
package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/datatypes"
)

// MCPAuditLog represents an audit log entry for MCP API calls
type MCPAuditLog struct {
	ID         uint                      `json:"id" gorm:"primaryKey"`
	CreatedAt  time.Time                 `json:"createdAt" gorm:"index"`
	SourceType types2.AuditLogSourceType `json:"sourceType" gorm:"index;default:mcp"`
	UserID     string                    `json:"userID" gorm:"index"`
	ClientIP   string                    `json:"clientIP" gorm:"index"`

	MCPFields                *MCPAuditLogFields                `json:"mcpFields,omitempty" gorm:"embedded"`
	LocalAgentToolCallFields *LocalAgentToolCallAuditLogFields `json:"localAgentToolCallFields,omitempty" gorm:"embedded"`
	Encrypted                bool                              `json:"encrypted"`
}

type MCPAuditLogFields struct {
	APIKey                    string                                `json:"apiKey,omitempty"`
	MCPID                     string                                `json:"mcpID" gorm:"index"`
	PowerUserWorkspaceID      string                                `json:"powerUserWorkspaceID,omitempty" gorm:"index"`
	MCPServerDisplayName      string                                `json:"mcpServerDisplayName" gorm:"index"`
	MCPServerCatalogEntryName string                                `json:"mcpServerCatalogEntryName" gorm:"index"`
	ClientName                string                                `json:"clientName" gorm:"index"`
	ClientVersion             string                                `json:"clientVersion" gorm:"index"`
	CallType                  string                                `json:"callType" gorm:"index"`
	CallIdentifier            string                                `json:"callIdentifier,omitempty" gorm:"index"`
	RequestMutated            bool                                  `json:"requestMutated"`
	RequestBody               json.RawMessage                       `json:"requestBody,omitempty"`
	MutatedRequestBody        json.RawMessage                       `json:"mutatedRequestBody,omitempty"`
	ResponseMutated           bool                                  `json:"responseMutated"`
	ResponseBody              json.RawMessage                       `json:"responseBody,omitempty"`
	OriginalResponseBody      json.RawMessage                       `json:"originalResponseBody,omitempty"`
	ResponseStatus            int                                   `json:"responseStatus" gorm:"index"`
	Error                     string                                `json:"error,omitempty"`
	ProcessingTimeMs          int64                                 `json:"processingTimeMs" gorm:"index"`
	SessionID                 string                                `json:"sessionID,omitempty" gorm:"index"`
	WebhookStatuses           datatypes.JSONSlice[MCPWebhookStatus] `json:"webhookStatuses,omitempty"`
	ResponseReceived          bool                                  `json:"responseReceived"`

	// Additional metadata
	RequestID       string          `json:"requestID,omitempty" gorm:"index"`
	UserAgent       string          `json:"userAgent,omitempty"`
	RequestHeaders  json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders json.RawMessage `json:"responseHeaders,omitempty"`
}

type LocalAgentToolCallAuditLogFields struct {
	OccurredAt time.Time  `json:"occurredAt" gorm:"index"`
	StartedAt  *time.Time `json:"startedAt,omitempty" gorm:"index"`

	// ActorType and ActorID are stamped from authenticated request context. They are never copied
	// from the client-reported event.
	ActorType types2.AuditLogActorType `json:"actorType" gorm:"index"`
	ActorID   string                   `json:"actorID,omitempty" gorm:"index"`

	ActionName string `json:"actionName" gorm:"index"`
	ActionKind string `json:"actionKind,omitempty" gorm:"index"`

	TargetType       types2.AuditLogTargetType `json:"targetType" gorm:"index"`
	TargetName       string                    `json:"targetName" gorm:"index"`
	TargetParentType types2.AuditLogTargetType `json:"targetParentType,omitempty" gorm:"index"`
	TargetParentName string                    `json:"targetParentName,omitempty" gorm:"index"`

	OutcomeStatus types2.AuditLogOutcomeStatus `json:"outcomeStatus" gorm:"index"`
	OutcomeReason string                       `json:"outcomeReason,omitempty" gorm:"index"`
	OutcomeError  string                       `json:"outcomeError,omitempty" gorm:"column:local_agent_error"`
	DurationMs    int64                        `json:"durationMs,omitempty" gorm:"index"`

	// IdempotencyKey deduplicates repeated submissions of the same completed audit entry.
	IdempotencyKey string `json:"idempotencyKey" gorm:"uniqueIndex"`
	// ToolUseID is the tool-use identifier from the agent runtime, when available.
	ToolUseID string `json:"toolUseID,omitempty" gorm:"index"`
	SessionID string `json:"sessionID,omitempty" gorm:"index"`
	// TurnID identifies the conversation turn that produced this tool call.
	TurnID string `json:"turnID,omitempty" gorm:"index"`

	AgentProvider  types2.LocalAgentProvider `json:"agentProvider" gorm:"index"`
	AgentVersion   string                    `json:"agentVersion,omitempty" gorm:"index"`
	CLIName        string                    `json:"cliName,omitempty"`
	CLIVersion     string                    `json:"cliVersion" gorm:"index"`
	Model          string                    `json:"model,omitempty" gorm:"index"`
	ModelID        string                    `json:"modelID,omitempty" gorm:"index"`
	PermissionMode string                    `json:"permissionMode,omitempty" gorm:"index"`

	DeviceID           string `json:"deviceID,omitempty" gorm:"index"`
	DeviceDeploymentID uint   `json:"deviceDeploymentID,omitempty" gorm:"index"`
	Hostname           string `json:"hostname,omitempty"`
	OS                 string `json:"os,omitempty" gorm:"index"`
	Architecture       string `json:"architecture,omitempty" gorm:"index"`
	LocalUsername      string `json:"localUsername,omitempty"`

	CWD               string                      `json:"cwd,omitempty"`
	GitRoot           string                      `json:"gitRoot,omitempty"`
	GitRemotes        datatypes.JSONSlice[string] `json:"gitRemotes,omitempty"`
	GitBranch         string                      `json:"gitBranch,omitempty"`
	GitCommit         string                      `json:"gitCommit,omitempty" gorm:"index"`
	ReportedUserEmail string                      `json:"reportedUserEmail,omitempty"`

	// TranscriptPath is the local path to the agent transcript, if the client reported one.
	TranscriptPath string `json:"transcriptPath,omitempty"`

	RequestBody  json.RawMessage `json:"requestBody,omitempty" gorm:"column:local_agent_request_body"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty" gorm:"column:local_agent_response_body"`
	// RawEvent preserves the original hook payload for debugging and future parsers.
	RawEvent json.RawMessage `json:"rawEvent,omitempty" gorm:"column:local_agent_raw_event"`
}

func (a *MCPAuditLog) NormalizeMCPFields() {
	if a == nil {
		return
	}
	a.SourceType = types2.AuditLogSourceTypeMCP
	if a.MCPFields == nil {
		a.MCPFields = &MCPAuditLogFields{}
	}
}

func (a *MCPAuditLog) MCP() *MCPAuditLogFields {
	if a == nil {
		return nil
	}
	if a.SourceType != types2.AuditLogSourceTypeMCP {
		return nil
	}
	if a.MCPFields == nil {
		a.MCPFields = &MCPAuditLogFields{}
	}
	return a.MCPFields
}

func (a *MCPAuditLog) ValidateSourceFields() error {
	if a == nil {
		return nil
	}
	hasMCPFields := a.MCPFields != nil
	hasPopulatedMCPFields := !isZeroMCPAuditLogFields(a.MCPFields)
	hasLocalAgentFields := a.LocalAgentToolCallFields != nil
	hasPopulatedLocalAgentFields := !isZeroLocalAgentToolCallAuditLogFields(a.LocalAgentToolCallFields)

	switch a.SourceType {
	case types2.AuditLogSourceTypeMCP:
		if !hasMCPFields {
			return errors.New("MCP audit fields are required for MCP audit logs")
		}
		if hasPopulatedLocalAgentFields {
			return errors.New("local agent audit fields cannot be populated for MCP audit logs")
		}
	case types2.AuditLogSourceTypeLocalAgentToolCall:
		if hasPopulatedMCPFields {
			return errors.New("MCP audit fields cannot be populated for local agent tool call audit logs")
		}
		if !hasLocalAgentFields {
			return errors.New("local agent audit fields are required for local agent tool call audit logs")
		}
		if err := a.validateLocalAgentToolCallFields(); err != nil {
			return err
		}
	default:
		return errors.New("invalid audit log source type")
	}
	return nil
}

// maxOccurredAtFutureSkew is the largest amount a local-agent audit log's client-reported
// occurredAt may exceed the server's current time before the submission is rejected.
const maxOccurredAtFutureSkew = time.Hour

func (a *MCPAuditLog) validateLocalAgentToolCallFields() error {
	local := a.LocalAgentToolCallFields
	if local == nil {
		return errors.New("local agent audit fields are required for local agent tool call audit logs")
	}

	var missing []string
	if local.AgentProvider == "" {
		missing = append(missing, "agentProvider")
	}
	if local.OccurredAt.IsZero() {
		missing = append(missing, "occurredAt")
	}
	if local.ActionName == "" {
		missing = append(missing, "action.name")
	}
	if local.TargetName == "" {
		missing = append(missing, "target.name")
	}
	if isMissingRequiredJSONPayload(local.RequestBody) {
		missing = append(missing, "details.request.body")
	}
	if isMissingRequiredJSONPayload(local.ResponseBody) {
		missing = append(missing, "details.response.body")
	}
	if local.OutcomeStatus == "" {
		missing = append(missing, "outcome.status")
	}
	if local.IdempotencyKey == "" {
		missing = append(missing, "idempotencyKey")
	}
	if isMissingRequiredJSONPayload(local.RawEvent) {
		missing = append(missing, "details.rawEvent")
	}
	if local.CLIVersion == "" {
		missing = append(missing, "cliVersion")
	}
	if local.ActorType == "" {
		missing = append(missing, "actorType")
	}

	if len(missing) > 0 {
		return errors.New("local agent audit fields missing required field(s): " + strings.Join(missing, ", "))
	}
	// Client-reported occurredAt is used for ordering and filtering, so a far-future value
	// could push real events out of typical time windows and undermine audit-log integrity.
	// Allow a small amount of clock skew, but reject timestamps beyond it. Comparison is done
	// in UTC so client and server time zones don't affect the result.
	if local.OccurredAt.UTC().After(time.Now().UTC().Add(maxOccurredAtFutureSkew)) {
		return errors.New("local agent audit occurredAt cannot be more than an hour in the future")
	}
	switch local.AgentProvider {
	case types2.LocalAgentProviderClaudeCode,
		types2.LocalAgentProviderCodex,
		types2.LocalAgentProviderVSCode,
		types2.LocalAgentProviderCursor:
	default:
		return errors.New("local agent audit provider must be one of: claude_code, codex, vscode, cursor")
	}
	switch local.OutcomeStatus {
	case types2.AuditLogOutcomeStatusSuccess,
		types2.AuditLogOutcomeStatusFailure,
		types2.AuditLogOutcomeStatusDenied,
		types2.AuditLogOutcomeStatusTimeout:
	default:
		return errors.New("local agent audit outcome status must be one of: success, failure, denied, timeout")
	}
	switch local.ActorType {
	case types2.AuditLogActorTypeUser, types2.AuditLogActorTypeDevice:
		if local.ActorID == "" {
			return errors.New("local agent audit actor ID is required for a user or device actor")
		}
	case types2.AuditLogActorTypeUnknown:
		if local.ActorID != "" {
			return errors.New("local agent audit actor ID must be empty for an unknown actor")
		}
	default:
		return errors.New("local agent audit actor type must be one of: user, device, unknown")
	}
	switch local.TargetType {
	case types2.AuditLogTargetTypeLocalTool:
		if local.TargetParentType != "" || local.TargetParentName != "" {
			return errors.New("local tool targets cannot have a parent")
		}
	case types2.AuditLogTargetTypeMCPTool:
		if local.TargetParentType != "" && local.TargetParentType != types2.AuditLogTargetTypeMCPServer {
			return errors.New("local agent MCP tool target parent must be an MCP server")
		}
		if (local.TargetParentType == "") != (local.TargetParentName == "") {
			return errors.New("local agent MCP tool target parent type and name must be supplied together")
		}
	default:
		return errors.New("local agent audit target type must be one of: local_tool, mcp_tool")
	}
	return nil
}

func isZeroMCPAuditLogFields(mcp *MCPAuditLogFields) bool {
	return mcp == nil || reflect.ValueOf(*mcp).IsZero()
}

func isZeroLocalAgentToolCallAuditLogFields(local *LocalAgentToolCallAuditLogFields) bool {
	return local == nil || reflect.ValueOf(*local).IsZero()
}

// isMissingRequiredJSONPayload reports whether a required JSON payload field
// was omitted. An explicit JSON null counts as present: terminal failure,
// denial, timeout, and other no-output paths are expected to submit an explicit
// empty or null value rather than omitting the field, so null must be accepted.
func isMissingRequiredJSONPayload(payload json.RawMessage) bool {
	return len(bytes.TrimSpace(payload)) == 0
}

type MCPWebhookStatus struct {
	Type    string `json:"type,omitempty"`
	URL     string `json:"url,omitempty"`
	Method  string `json:"method,omitempty"`
	Name    string `json:"name,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
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

type MCPUsageStatsList struct {
	TotalCalls  int64              `json:"totalCalls"`
	UniqueUsers int64              `json:"uniqueUsers"`
	TimeStart   time.Time          `json:"timeStart"`
	TimeEnd     time.Time          `json:"timeEnd"`
	Items       []MCPUsageStatItem `json:"items"`
}

type MCPToolCallStatsItem struct {
	ToolName         string    `json:"toolName"`
	CreatedAt        time.Time `json:"createdAt"`
	UserID           string    `json:"userID"`
	ProcessingTimeMs int64     `json:"processingTimeMs"`
	ResponseStatus   int       `json:"responseStatus"`
	Error            string    `json:"error"`
}

// MCPToolCallStats represents statistics for individual tool calls
type MCPToolCallStats struct {
	ToolName  string                 `json:"-"`
	CallCount int64                  `json:"callCount"`
	Items     []MCPToolCallStatsItem `json:"items"`
}

// MCPResourceReadStats represents statistics for individual resource reads
type MCPResourceReadStats struct {
	ResourceURI string `json:"resourceUri"`
	ReadCount   int64  `json:"readCount"`
}

// MCPPromptReadStats represents statistics for individual prompt reads
type MCPPromptReadStats struct {
	PromptName string `json:"promptName"`
	ReadCount  int64  `json:"readCount"`
}

func NewLocalAgentToolCallAuditLogFromInput(input types2.LocalAgentToolCallAuditLogInput, actorType types2.AuditLogActorType, actorID, clientIP string, deviceDeploymentID uint, createdAt time.Time) MCPAuditLog {
	var startedAt *time.Time
	if input.Details.StartedAt != nil && !input.Details.StartedAt.IsZero() {
		t := input.Details.StartedAt.GetTime()
		startedAt = &t
	}
	var targetParentType types2.AuditLogTargetType
	var targetParentName string
	if input.Target.Parent != nil {
		targetParentType = input.Target.Parent.TargetType
		targetParentName = input.Target.Parent.Name
	}
	var userID, deviceID string
	if actorType == types2.AuditLogActorTypeUser {
		userID = actorID
	}
	if actorType == types2.AuditLogActorTypeDevice {
		deviceID = actorID
	}

	return MCPAuditLog{
		CreatedAt:  createdAt,
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     userID,
		ClientIP:   clientIP,
		LocalAgentToolCallFields: &LocalAgentToolCallAuditLogFields{
			OccurredAt:         input.OccurredAt.GetTime(),
			StartedAt:          startedAt,
			ActorType:          actorType,
			ActorID:            actorID,
			ActionName:         input.Action.Name,
			ActionKind:         input.Action.Kind,
			TargetType:         input.Target.TargetType,
			TargetName:         input.Target.Name,
			TargetParentType:   targetParentType,
			TargetParentName:   targetParentName,
			OutcomeStatus:      input.Outcome.Status,
			OutcomeReason:      input.Outcome.Reason,
			OutcomeError:       input.Outcome.Error,
			DurationMs:         input.Outcome.DurationMs,
			IdempotencyKey:     input.Details.Trace.IdempotencyKey,
			ToolUseID:          input.Details.Trace.ToolUseID,
			SessionID:          input.Details.Trace.SessionID,
			TurnID:             input.Details.Trace.TurnID,
			AgentProvider:      input.Details.Agent.Provider,
			AgentVersion:       input.Details.Agent.Version,
			CLIName:            input.Details.Agent.CLIName,
			CLIVersion:         input.Details.Agent.CLIVersion,
			Model:              input.Details.Agent.Model,
			ModelID:            input.Details.Agent.ModelID,
			PermissionMode:     input.Details.Agent.PermissionMode,
			DeviceID:           deviceID,
			DeviceDeploymentID: deviceDeploymentID,
			Hostname:           input.Details.Device.Hostname,
			OS:                 input.Details.Device.OS,
			Architecture:       input.Details.Device.Architecture,
			LocalUsername:      input.Details.Device.LocalUsername,
			CWD:                input.Details.Environment.CWD,
			GitRoot:            input.Details.Environment.GitRoot,
			GitRemotes:         datatypes.JSONSlice[string](input.Details.Environment.GitRemotes),
			GitBranch:          input.Details.Environment.GitBranch,
			GitCommit:          input.Details.Environment.GitCommit,
			ReportedUserEmail:  input.Details.Environment.ReportedUserEmail,
			TranscriptPath:     input.Details.Environment.TranscriptPath,
			RequestBody:        input.Details.Request.Body,
			ResponseBody:       input.Details.Response.Body,
			RawEvent:           input.Details.RawEvent,
		},
	}
}

// ConvertMCPUsageStats converts internal MCPUsageStatItem to API type
func ConvertMCPUsageStats(s MCPUsageStatItem) types2.MCPUsageStatItem {
	toolCalls := make([]types2.MCPToolCallStats, len(s.ToolCalls))
	for i, tc := range s.ToolCalls {
		items := make([]types2.MCPToolCallStatsItem, len(tc.Items))
		for j, item := range tc.Items {
			items[j] = types2.MCPToolCallStatsItem{
				CreatedAt:        *types2.NewTime(item.CreatedAt),
				UserID:           item.UserID,
				ProcessingTimeMs: item.ProcessingTimeMs,
				ResponseStatus:   item.ResponseStatus,
				Error:            item.Error,
			}
		}

		toolCalls[i] = types2.MCPToolCallStats{
			ToolName:  tc.ToolName,
			CallCount: tc.CallCount,
			Items:     items,
		}
	}

	resourceReads := make([]types2.MCPResourceReadStats, len(s.ResourceReads))
	for i, rr := range s.ResourceReads {
		resourceReads[i] = types2.MCPResourceReadStats{
			ResourceURI: rr.ResourceURI,
			ReadCount:   rr.ReadCount,
		}
	}

	promptReads := make([]types2.MCPPromptReadStats, len(s.PromptReads))
	for i, pr := range s.PromptReads {
		promptReads[i] = types2.MCPPromptReadStats{
			PromptName: pr.PromptName,
			ReadCount:  pr.ReadCount,
		}
	}

	return types2.MCPUsageStatItem{
		MCPID:                     s.MCPID,
		MCPServerDisplayName:      s.MCPServerDisplayName,
		MCPServerCatalogEntryName: s.MCPServerCatalogEntryName,
		ToolCalls:                 toolCalls,
		ResourceReads:             resourceReads,
		PromptReads:               promptReads,
	}
}
