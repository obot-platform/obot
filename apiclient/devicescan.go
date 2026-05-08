package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

// SubmitDeviceScan posts a device scan submission manifest to the
// server and returns the persisted scan envelope (with server-assigned
// ID, ReceivedAt, SubmittedBy).
func (c *Client) SubmitDeviceScan(ctx context.Context, manifest types.DeviceScanManifest) (*types.DeviceScan, error) {
	_, resp, err := c.postJSON(ctx, "/devices/scans", manifest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.DeviceScan{})
}
