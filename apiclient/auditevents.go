package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) SubmitAuditEvents(ctx context.Context, events []types.AuditEvent) (*types.AuditEventSubmitResponse, error) {
	_, resp, err := c.postJSON(ctx, "/audit-events", events)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.AuditEventSubmitResponse{})
}
