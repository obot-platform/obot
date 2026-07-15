package mcpgateway

import (
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

	logs := make([]gatewaytypes.MCPAuditLog, 0, len(input.Logs))
	createdAt := time.Now().UTC()
	for i, manifest := range input.Logs {
		log := gatewaytypes.NewLocalAgentToolCallAuditLogFromManifest(
			manifest,
			req.User.GetUID(),
			requestinfo.GetSourceIP(req.Request),
			types.LocalAgentIdentityStatusAuthenticatedUser,
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
