package mcpgateway

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/provenance/device"
)

type LocalAgentAuditLogHandler struct {
	// engine is the provenance engine the tool results are ingested into, so
	// what the agent read on the laptop taints its session. It may be nil.
	engine *provenance.Engine
}

func NewLocalAgentAuditLogHandler(engine *provenance.Engine) *LocalAgentAuditLogHandler {
	return &LocalAgentAuditLogHandler{engine: engine}
}

// Submit handles POST /api/local-agent-audit-logs for completed local-agent tool calls.
func (h *LocalAgentAuditLogHandler) Submit(req api.Context) error {
	var input types.LocalAgentToolCallAuditLogSubmitRequest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}
	if len(input.Events) == 0 {
		return types.NewErrBadRequest("at least one local agent audit event is required")
	}

	logs := make([]gatewaytypes.MCPAuditLog, 0, len(input.Events))
	createdAt := time.Now().UTC()
	actorType, actorID, deviceDeploymentID := localAgentSubmitterAttribution(req)
	apiKeyAttribution, hasAPIKeyAttribution := principal.APIKeyAttributionFromUser(req.User)
	for i, event := range input.Events {
		log := gatewaytypes.NewLocalAgentToolCallAuditLogFromInput(
			event,
			actorType,
			actorID,
			requestinfo.GetSourceIP(req.Request),
			deviceDeploymentID,
			createdAt,
		)
		if hasAPIKeyAttribution {
			log.APIKeyID = new(apiKeyAttribution.ID)
			log.APIKeyName = apiKeyAttribution.Name
		}
		if err := log.ValidateSourceFields(); err != nil {
			return types.NewErrBadRequest("invalid local agent audit log at index %d: %v", i, err)
		}
		logs = append(logs, log)
	}

	if err := req.GatewayClient.InsertLocalAgentAuditLogs(req.Context(), logs); err != nil {
		return err
	}

	h.ingest(req, input.Events, actorType, actorID)
	return nil
}

// ingest feeds the result of every completed tool call into the provenance
// session graph. The audit rows are already stored; a failure here is logged
// and does not fail the submission.
func (h *LocalAgentAuditLogHandler) ingest(req api.Context, events []types.LocalAgentToolCallAuditLogInput, actorType types.AuditLogActorType, actorID string) {
	if h.engine == nil {
		return
	}
	var deviceID, userID string
	switch actorType {
	case types.AuditLogActorTypeDevice:
		deviceID = actorID
	case types.AuditLogActorTypeUser:
		userID = actorID
	default:
		return
	}
	resolver := device.Resolver{
		ToolCapabilities: func(server, tool string) provenance.Labels {
			return h.engine.ToolCapabilities(req.Context(), server, tool, "")
		},
		IsExternal: func(host string) bool {
			return h.engine.IsExternal(req.Context(), host)
		},
	}
	for _, event := range events {
		switch event.Outcome.Status {
		case types.AuditLogOutcomeStatusSuccess, types.AuditLogOutcomeStatusFailure:
		default:
			// A denied or timed-out call produced no content the agent saw.
			continue
		}
		in := device.BuildIngest(event, deviceID, userID, resolver)
		if _, err := h.engine.Ingest(req.Context(), in); err != nil {
			slog.WarnContext(req.Context(), "provenance: ingesting local agent tool result failed",
				"device", deviceID, "agent", in.Session.Agent, "tool", in.Tool, "error", err)
		}
	}
}

func localAgentSubmitterAttribution(req api.Context) (actorType types.AuditLogActorType, actorID string, deviceDeploymentID uint) {
	extra := req.User.GetExtra()
	if deviceID := firstExtra(extra, "device_id"); deviceID != "" {
		if deployment := firstExtra(extra, "mdm_configuration_id"); deployment != "" {
			if parsed, err := strconv.ParseUint(deployment, 10, 64); err == nil {
				deviceDeploymentID = uint(parsed)
			}
		}
		return types.AuditLogActorTypeDevice, deviceID, deviceDeploymentID
	}
	if userID := req.User.GetUID(); userID != "" {
		return types.AuditLogActorTypeUser, userID, 0
	}
	return types.AuditLogActorTypeUnknown, "", 0
}

func firstExtra(extra map[string][]string, key string) string {
	values := extra[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
