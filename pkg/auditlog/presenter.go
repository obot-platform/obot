package auditlog

import (
	api "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// PresentOptions controls which authorized portions of an audit event are exposed by Present.
type PresentOptions struct {
	// IncludeDetails controls whether the event includes typed metadata and payload sections. List
	// callers should leave it false; detail and export callers normally set it true.
	IncludeDetails bool
	// PayloadRedacted records the caller's payload authorization in the public event. It does not
	// perform redaction itself: the gateway client must decrypt or blank sensitive fields before
	// Present is called.
	PayloadRedacted bool
}

// Present converts an internal persisted audit-log row into the normalized public read model.
// The caller is responsible for authorization and for loading the row with the appropriate
// payload visibility. Present does not mutate the input row and performs no decryption.
func Present(log gatewaytypes.MCPAuditLog, opts PresentOptions) api.AuditLogEvent {
	event := api.AuditLogEvent{
		ID: log.ID,
		Timestamp: api.AuditLogTimestamp{
			OccurredAt: *api.NewTime(log.CreatedAt),
			RecordedAt: *api.NewTime(log.CreatedAt),
			Source:     api.AuditLogTimestampSourceServer,
		},
	}

	switch log.SourceType {
	case api.AuditLogSourceTypeLocalAgentToolCall:
		presentLocalAgent(&event, log, opts)
	default:
		presentMCP(&event, log, opts)
	}

	return event
}

// ClassifyMCPOutcome derives the normalized outcome of an MCP request/response pair from the
// recorded HTTP status and error. Explicit denial and timeout statuses take precedence over
// generic errors. A row with no recorded status and no error is classified as unknown.
func ClassifyMCPOutcome(status int, responseError string) api.AuditLogOutcomeStatus {
	switch {
	case status == 401 || status == 403:
		return api.AuditLogOutcomeStatusDenied
	case status == 408 || status == 504:
		return api.AuditLogOutcomeStatusTimeout
	case responseError != "" || status >= 400:
		return api.AuditLogOutcomeStatusFailure
	case status > 0 && status < 400:
		return api.AuditLogOutcomeStatusSuccess
	default:
		return api.AuditLogOutcomeStatusUnknown
	}
}

func presentMCP(event *api.AuditLogEvent, log gatewaytypes.MCPAuditLog, opts PresentOptions) {
	mcp := log.MCP()
	if mcp == nil {
		mcp = &gatewaytypes.MCPAuditLogFields{}
	}

	event.EventType = api.AuditLogEventTypeMCPCall
	event.Actor = mcpActor(log.UserID, mcp.APIKey)
	event.Action = api.AuditLogAction{
		Operation: mcp.CallType,
		Name:      mcp.CallIdentifier,
	}
	event.Target = mcpTarget(mcp)
	event.Outcome = api.AuditLogOutcome{
		Status:     ClassifyMCPOutcome(mcp.ResponseStatus, mcp.Error),
		HTTPStatus: mcp.ResponseStatus,
		Error:      mcp.Error,
		DurationMs: mcp.ProcessingTimeMs,
	}

	if !opts.IncludeDetails {
		return
	}

	webhooks := make([]api.WebhookStatus, len(mcp.WebhookStatuses))
	for i, webhook := range mcp.WebhookStatuses {
		webhooks[i] = api.WebhookStatus{
			Type:    webhook.Type,
			Method:  webhook.Method,
			URL:     webhook.URL,
			Name:    webhook.Name,
			Tool:    webhook.Tool,
			Status:  webhook.Status,
			Message: webhook.Message,
		}
	}

	event.Details = &api.AuditLogDetails{
		Trace: &api.AuditLogTraceDetails{
			SessionID: mcp.SessionID,
			RequestID: mcp.RequestID,
		},
		Network: &api.AuditLogNetworkDetails{ClientIP: log.ClientIP},
		Client: &api.AuditLogClientDetails{
			Name:      mcp.ClientName,
			Version:   mcp.ClientVersion,
			UserAgent: mcp.UserAgent,
		},
		Scope: &api.AuditLogScopeDetails{
			PowerUserWorkspaceID:      mcp.PowerUserWorkspaceID,
			MCPServerCatalogEntryName: mcp.MCPServerCatalogEntryName,
		},
		Request: &api.AuditLogPayloadDetails{
			Headers:     mcp.RequestHeaders,
			Body:        mcp.RequestBody,
			MutatedBody: mcp.MutatedRequestBody,
			Mutated:     mcp.RequestMutated,
		},
		Response: &api.AuditLogPayloadDetails{
			Headers:      mcp.ResponseHeaders,
			Body:         mcp.ResponseBody,
			OriginalBody: mcp.OriginalResponseBody,
			Mutated:      mcp.ResponseMutated,
		},
		WebhookStatuses: webhooks,
		PayloadRedacted: opts.PayloadRedacted,
	}
}

func mcpActor(userID, credentialID string) api.AuditLogActor {
	if userID != "" {
		return api.AuditLogActor{
			ActorType:    api.AuditLogActorTypeUser,
			ID:           userID,
			CredentialID: credentialID,
		}
	}
	if credentialID != "" {
		return api.AuditLogActor{
			ActorType: api.AuditLogActorTypeCredential,
			ID:        credentialID,
		}
	}
	return api.AuditLogActor{ActorType: api.AuditLogActorTypeUnknown}
}

func mcpTarget(mcp *gatewaytypes.MCPAuditLogFields) api.AuditLogTarget {
	server := api.AuditLogTargetRef{
		TargetType: api.AuditLogTargetTypeMCPServer,
		ID:         mcp.MCPID,
		Name:       mcp.MCPServerDisplayName,
	}
	target := api.AuditLogTarget{
		AuditLogTargetRef: server,
		Resolved:          true,
	}
	if mcp.CallIdentifier == "" {
		return target
	}

	var targetType api.AuditLogTargetType
	switch mcp.CallType {
	case "tools/call":
		targetType = api.AuditLogTargetTypeMCPTool
	case "resources/read":
		targetType = api.AuditLogTargetTypeMCPResource
	case "prompts/get":
		targetType = api.AuditLogTargetTypeMCPPrompt
	default:
		return target
	}

	target.AuditLogTargetRef = api.AuditLogTargetRef{
		TargetType: targetType,
		Name:       mcp.CallIdentifier,
	}
	target.Parent = &server
	return target
}

func presentLocalAgent(event *api.AuditLogEvent, log gatewaytypes.MCPAuditLog, opts PresentOptions) {
	local := log.LocalAgentToolCallFields
	if local == nil {
		local = &gatewaytypes.LocalAgentToolCallAuditLogFields{}
	}

	event.EventType = api.AuditLogEventTypeLocalAgentToolCall
	event.Timestamp.Source = api.AuditLogTimestampSourceClientReported
	event.Timestamp.OccurredAt = *api.NewTime(local.OccurredAt)
	event.Actor = api.AuditLogActor{
		ActorType: local.ActorType,
		ID:        local.ActorID,
	}
	event.Action = api.AuditLogAction{
		Operation: "tools/call",
		Name:      local.ActionName,
		Kind:      local.ActionKind,
	}
	event.Target = api.AuditLogTarget{
		AuditLogTargetRef: api.AuditLogTargetRef{
			TargetType: local.TargetType,
			Name:       local.TargetName,
		},
		Resolved: false,
	}
	if local.TargetParentType != "" {
		event.Target.Parent = &api.AuditLogTargetRef{
			TargetType: local.TargetParentType,
			Name:       local.TargetParentName,
		}
	}
	event.Outcome = api.AuditLogOutcome{
		Status:     local.OutcomeStatus,
		Reason:     local.OutcomeReason,
		Error:      local.OutcomeError,
		DurationMs: local.DurationMs,
	}

	if !opts.IncludeDetails {
		return
	}

	var startedAt *api.Time
	if local.StartedAt != nil {
		startedAt = api.NewTime(*local.StartedAt)
	}
	event.Details = &api.AuditLogDetails{
		Trace: &api.AuditLogTraceDetails{
			SessionID:      local.SessionID,
			IdempotencyKey: local.IdempotencyKey,
			ToolUseID:      local.ToolUseID,
			TurnID:         local.TurnID,
		},
		Network: &api.AuditLogNetworkDetails{ClientIP: log.ClientIP},
		Agent: &api.AuditLogAgentDetails{
			Provider:       local.AgentProvider,
			Version:        local.AgentVersion,
			CLIName:        local.CLIName,
			CLIVersion:     local.CLIVersion,
			Model:          local.Model,
			ModelID:        local.ModelID,
			PermissionMode: local.PermissionMode,
		},
		Device: &api.AuditLogDeviceDetails{
			ID:            local.DeviceID,
			DeploymentID:  local.DeviceDeploymentID,
			Hostname:      local.Hostname,
			OS:            local.OS,
			Architecture:  local.Architecture,
			LocalUsername: local.LocalUsername,
		},
		Environment: &api.AuditLogEnvironmentDetails{
			CWD:               local.CWD,
			GitRoot:           local.GitRoot,
			GitRemotes:        []string(local.GitRemotes),
			GitBranch:         local.GitBranch,
			GitCommit:         local.GitCommit,
			ReportedUserEmail: local.ReportedUserEmail,
			TranscriptPath:    local.TranscriptPath,
		},
		Request:         &api.AuditLogPayloadDetails{Body: local.RequestBody},
		Response:        &api.AuditLogPayloadDetails{Body: local.ResponseBody},
		RawEvent:        local.RawEvent,
		StartedAt:       startedAt,
		PayloadRedacted: opts.PayloadRedacted,
	}
}
