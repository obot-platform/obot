package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

// SubmitLocalAgentAuditLogs posts completed local-agent tool-call audit logs.
func (c *Client) SubmitLocalAgentAuditLogs(ctx context.Context, logs []types.LocalAgentToolCallAuditLogManifest) error {
	_, resp, err := c.postJSON(ctx, "/local-agent-audit-logs", types.LocalAgentToolCallAuditLogSubmitRequest{
		Logs: logs,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
