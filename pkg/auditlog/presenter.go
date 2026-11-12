package auditlog

import (
	"fmt"
	"slices"
	"strings"

	api "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
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
	// MCPFields.APIKey is a legacy field that may contain the bearer token. Only expose the
	// event-time, non-secret display snapshot through the normalized audit-log API.
	event.Actor = mcpActor(log, apiKeyDisplayName(actorOf(log), log.APIKeyID, log.APIKeyName))
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
	event.Client = mcp.ClientName

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
			VMCPID:                    mcp.VMCPID,
			VMCPComponentID:           mcp.VMCPComponentID,
			ProfileIDs:                slices.Clone(mcp.ProfileIDs),
			FallbackProfileIDs:        slices.Clone(mcp.FallbackProfileIDs),
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
		WebhookStatuses:     webhooks,
		PayloadRedacted:     opts.PayloadRedacted,
		UnavailablePayloads: log.UnavailablePayloads,
	}
}

// actorOf returns the principal a row is attributed to.
func actorOf(log gatewaytypes.MCPAuditLog) gatewaytypes.Actor {
	return gatewaytypes.Actor{PrincipalKind: log.PrincipalKind, PrincipalID: log.PrincipalID, UserID: log.UserID}
}

// apiKeyDisplayName labels an API key with its event-time name and its masked identifier. actor is the principal the
// key belongs to; the masked identifier of an agent's key is written with the agent key prefix.
func apiKeyDisplayName(actor gatewaytypes.Actor, apiKeyID *uint, name string) string {
	owner := actor.Owner()
	if name == "" || owner == "" || apiKeyID == nil {
		return name
	}
	// A user ID that has a colon is not one the masked identifier can be built from.
	if !actor.IsAgent() && strings.Contains(owner, ":") {
		return name
	}

	maskedKey := principal.MaskedAPIKeyName(owner, *apiKeyID)
	if name == maskedKey {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, maskedKey)
}

func mcpActor(log gatewaytypes.MCPAuditLog, credentialID string) api.AuditLogActor {
	userID, apiKeyRevoked := log.UserID, log.APIKeyRevoked
	if actorOf(log).IsAgent() {
		return api.AuditLogActor{
			ActorType:     api.AuditLogActorTypeAgent,
			ID:            log.PrincipalID,
			DisplayName:   log.AgentDisplayName,
			CredentialID:  credentialID,
			APIKeyRevoked: apiKeyRevoked,
		}
	}
	if userID != "" {
		return api.AuditLogActor{
			ActorType:     api.AuditLogActorTypeUser,
			ID:            userID,
			CredentialID:  credentialID,
			APIKeyRevoked: apiKeyRevoked,
		}
	}
	if credentialID != "" {
		return api.AuditLogActor{
			ActorType:     api.AuditLogActorTypeCredential,
			ID:            credentialID,
			APIKeyRevoked: apiKeyRevoked,
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
		ActorType:     local.ActorType,
		ID:            local.ActorID,
		CredentialID:  apiKeyDisplayName(actorOf(log), log.APIKeyID, log.APIKeyName),
		APIKeyRevoked: log.APIKeyRevoked,
	}
	event.Action = api.AuditLogAction{
		Operation: "tools/call",
		Name:      local.ActionName,
		Kind:      local.ActionKind,
	}
	event.Target = api.AuditLogTarget{
		TargetType: local.TargetType,
		Name:       local.TargetName,
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
	event.Client = string(local.AgentProvider)

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
		Request:             &api.AuditLogPayloadDetails{Body: local.RequestBody},
		Response:            &api.AuditLogPayloadDetails{Body: local.ResponseBody},
		RawEvent:            restoreRawEvent(local.RawEvent, local.RequestBody, local.ResponseBody),
		StartedAt:           startedAt,
		PayloadRedacted:     opts.PayloadRedacted,
		UnavailablePayloads: log.UnavailablePayloads,
	}
}
