//nolint:revive
package types

import (
	"encoding/json"
	"errors"
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
	ObotAuditCorrelationID    string                                `json:"obotAuditCorrelationID,omitempty" gorm:"column:obot_audit_correlation_id;index"`
	ResponseReceived          bool                                  `json:"responseReceived"`

	// Additional metadata
	RequestID       string          `json:"requestID,omitempty" gorm:"index"`
	UserAgent       string          `json:"userAgent,omitempty"`
	RequestHeaders  json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders json.RawMessage `json:"responseHeaders,omitempty"`
}

type LocalAgentToolCallAuditLogFields struct {
	AgentProvider string `json:"agentProvider" gorm:"index"`
	AgentVersion  string `json:"agentVersion,omitempty" gorm:"index"`
	CLIName       string `json:"cliName,omitempty"`
	CLIVersion    string `json:"cliVersion" gorm:"index"`

	Status      string     `json:"status" gorm:"index"`
	FailureType string     `json:"failureType,omitempty" gorm:"index"`
	ObservedAt  time.Time  `json:"observedAt" gorm:"index"`
	StartedAt   *time.Time `json:"startedAt,omitempty" gorm:"index"`
	DurationMs  int64      `json:"durationMs,omitempty" gorm:"index"`
	Error       string     `json:"error,omitempty"`

	// IdempotencyKey deduplicates repeated submissions of the same completed audit entry.
	IdempotencyKey string `json:"idempotencyKey" gorm:"uniqueIndex"`
	// ToolUseID is the tool-use identifier from the agent runtime, when available.
	ToolUseID string `json:"toolUseID,omitempty" gorm:"index"`
	SessionID string `json:"sessionID,omitempty" gorm:"index"`
	// TurnID identifies the conversation turn that produced this tool call.
	TurnID string `json:"turnID,omitempty" gorm:"index"`

	ToolName string `json:"toolName" gorm:"index"`
	ToolKind string `json:"toolKind,omitempty" gorm:"index"`
	// MCPServerHint is the agent-reported server name before Obot resolves it.
	MCPServerHint string `json:"mcpServerHint,omitempty" gorm:"index"`
	// MCPToolName is the MCP tool name after parsing or resolving the local tool call.
	MCPToolName string `json:"mcpToolName,omitempty" gorm:"index"`

	// ObotAuditCorrelationID is the client-provided key used to correlate with MCP audit logs.
	ObotAuditCorrelationID string `json:"obotAuditCorrelationID,omitempty" gorm:"column:obot_audit_correlation_id;index"`

	Model          string `json:"model,omitempty" gorm:"index"`
	ModelID        string `json:"modelID,omitempty" gorm:"index"`
	PermissionMode string `json:"permissionMode,omitempty" gorm:"index"`

	DeviceID          string `json:"deviceID,omitempty" gorm:"index"`
	Hostname          string `json:"hostname,omitempty" gorm:"index"`
	OS                string `json:"os,omitempty" gorm:"index"`
	Arch              string `json:"arch,omitempty" gorm:"index"`
	LocalUsername     string `json:"localUsername,omitempty"`
	ReportedUserEmail string `json:"reportedUserEmail,omitempty" gorm:"index"`
	// IdentityStatus records whether the reported user was authenticated, anonymous, or unresolved.
	IdentityStatus string `json:"identityStatus" gorm:"index"`

	CWD           string                      `json:"cwd,omitempty" gorm:"index"`
	GitRepoRoot   string                      `json:"gitRepoRoot,omitempty" gorm:"index"`
	GitRemoteURLs datatypes.JSONSlice[string] `json:"gitRemoteURLs,omitempty"`
	GitBranch     string                      `json:"gitBranch,omitempty" gorm:"index"`
	GitCommitSHA  string                      `json:"gitCommitSHA,omitempty" gorm:"index"`

	// TranscriptPath is the local path to the agent transcript, if the client reported one.
	TranscriptPath string `json:"transcriptPath,omitempty"`

	ToolInput  json.RawMessage `json:"toolInput,omitempty"`
	ToolOutput json.RawMessage `json:"toolOutput,omitempty"`
	// RawHookPayload preserves the original hook payload for debugging and future parsers.
	RawHookPayload json.RawMessage `json:"rawHookPayload,omitempty"`
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
	return a.MCPFields
}

func (a *MCPAuditLog) ValidateSourceFields() error {
	if a == nil {
		return nil
	}
	hasMCPFields := a.MCPFields != nil
	hasLocalAgentFields := a.LocalAgentToolCallFields != nil

	switch a.SourceType {
	case types2.AuditLogSourceTypeMCP:
		if hasLocalAgentFields {
			return errors.New("local agent audit fields cannot be populated for MCP audit logs")
		}
	case types2.AuditLogSourceTypeLocalAgentToolCall:
		if hasMCPFields {
			return errors.New("MCP audit fields cannot be populated for local agent tool call audit logs")
		}
	default:
		return errors.New("invalid audit log source type")
	}
	return nil
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

// ConvertMCPAuditLog converts internal MCPAuditLog to API type
func ConvertMCPAuditLog(a MCPAuditLog) types2.MCPAuditLog {
	mcp := a.MCP()
	return types2.MCPAuditLog{
		ID:                       a.ID,
		CreatedAt:                *types2.NewTime(a.CreatedAt),
		SourceType:               a.SourceType,
		UserID:                   a.UserID,
		ClientIP:                 a.ClientIP,
		MCPFields:                convertMCPAuditLogFields(mcp),
		LocalAgentToolCallFields: convertLocalAgentToolCallAuditLogFields(a.LocalAgentToolCallFields),
	}
}

func convertMCPAuditLogFields(mcp *MCPAuditLogFields) *types2.MCPAuditLogFields {
	if mcp == nil {
		return nil
	}
	webhookStatus := make([]types2.WebhookStatus, len(mcp.WebhookStatuses))
	for i, ws := range mcp.WebhookStatuses {
		webhookStatus[i] = types2.WebhookStatus{
			Type:    ws.Type,
			Method:  ws.Method,
			URL:     ws.URL,
			Name:    ws.Name,
			Tool:    ws.Tool,
			Status:  ws.Status,
			Message: ws.Message,
		}
	}
	return &types2.MCPAuditLogFields{
		APIKey:                    mcp.APIKey,
		MCPID:                     mcp.MCPID,
		PowerUserWorkspaceID:      mcp.PowerUserWorkspaceID,
		MCPServerDisplayName:      mcp.MCPServerDisplayName,
		MCPServerCatalogEntryName: mcp.MCPServerCatalogEntryName,
		ClientInfo: types2.ClientInfo{
			Name:    mcp.ClientName,
			Version: mcp.ClientVersion,
		},
		CallType:               mcp.CallType,
		CallIdentifier:         mcp.CallIdentifier,
		RequestMutated:         mcp.RequestMutated,
		RequestBody:            mcp.RequestBody,
		MutatedRequestBody:     mcp.MutatedRequestBody,
		ResponseMutated:        mcp.ResponseMutated,
		ResponseBody:           mcp.ResponseBody,
		OriginalResponseBody:   mcp.OriginalResponseBody,
		ResponseStatus:         mcp.ResponseStatus,
		WebhookStatuses:        webhookStatus,
		Error:                  mcp.Error,
		ProcessingTimeMs:       mcp.ProcessingTimeMs,
		SessionID:              mcp.SessionID,
		ObotAuditCorrelationID: mcp.ObotAuditCorrelationID,
		RequestID:              mcp.RequestID,
		UserAgent:              mcp.UserAgent,
		RequestHeaders:         mcp.RequestHeaders,
		ResponseHeaders:        mcp.ResponseHeaders,
	}
}

func convertLocalAgentToolCallAuditLogFields(local *LocalAgentToolCallAuditLogFields) *types2.LocalAgentToolCallAuditLogFields {
	if local == nil {
		return nil
	}

	var startedAt *types2.Time
	if local.StartedAt != nil {
		startedAt = types2.NewTime(*local.StartedAt)
	}

	return &types2.LocalAgentToolCallAuditLogFields{
		AgentProvider:          local.AgentProvider,
		AgentVersion:           local.AgentVersion,
		CLIName:                local.CLIName,
		CLIVersion:             local.CLIVersion,
		Status:                 local.Status,
		FailureType:            local.FailureType,
		ObservedAt:             *types2.NewTime(local.ObservedAt),
		StartedAt:              startedAt,
		DurationMs:             local.DurationMs,
		Error:                  local.Error,
		IdempotencyKey:         local.IdempotencyKey,
		ToolUseID:              local.ToolUseID,
		SessionID:              local.SessionID,
		TurnID:                 local.TurnID,
		ToolName:               local.ToolName,
		ToolKind:               local.ToolKind,
		MCPServerHint:          local.MCPServerHint,
		MCPToolName:            local.MCPToolName,
		ObotAuditCorrelationID: local.ObotAuditCorrelationID,
		Model:                  local.Model,
		ModelID:                local.ModelID,
		PermissionMode:         local.PermissionMode,
		DeviceID:               local.DeviceID,
		Hostname:               local.Hostname,
		OS:                     local.OS,
		Arch:                   local.Arch,
		LocalUsername:          local.LocalUsername,
		ReportedUserEmail:      local.ReportedUserEmail,
		IdentityStatus:         local.IdentityStatus,
		CWD:                    local.CWD,
		GitRepoRoot:            local.GitRepoRoot,
		GitRemoteURLs:          []string(local.GitRemoteURLs),
		GitBranch:              local.GitBranch,
		GitCommitSHA:           local.GitCommitSHA,
		TranscriptPath:         local.TranscriptPath,
		ToolInput:              local.ToolInput,
		ToolOutput:             local.ToolOutput,
		RawHookPayload:         local.RawHookPayload,
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
