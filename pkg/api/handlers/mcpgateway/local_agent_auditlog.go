package mcpgateway

import (
	"strconv"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type LocalAgentAuditLogHandler struct{}

func NewLocalAgentAuditLogHandler() *LocalAgentAuditLogHandler {
	return nil
}

// Submit handles POST /api/local-agent-audit-logs for completed local-agent tool calls.
func (*LocalAgentAuditLogHandler) Submit(req api.Context) error {
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
	for i, event := range input.Events {
		log := gatewaytypes.NewLocalAgentToolCallAuditLogFromInput(
			event,
			actorType,
			actorID,
			requestinfo.GetSourceIP(req.Request),
			deviceDeploymentID,
			createdAt,
		)
		if err := log.ValidateSourceFields(); err != nil {
			return types.NewErrBadRequest("invalid local agent audit log at index %d: %v", i, err)
		}
		logs = append(logs, log)
	}

	if err := req.GatewayClient.InsertLocalAgentAuditLogs(req.Context(), logs); err != nil {
		return err
	}

	return nil
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
