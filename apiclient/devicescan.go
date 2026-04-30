package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

// SubmitDeviceScan posts a device scan payload to the server.
func (c *Client) SubmitDeviceScan(ctx context.Context, scan types.DeviceScan) (*types.DeviceScan, error) {
	_, resp, err := c.postJSON(ctx, "/devices/scans", scan)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.DeviceScan{})
}
