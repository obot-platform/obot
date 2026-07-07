package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

// EnrollDevice registers a device's identity key with an enrollment credential,
// authenticating with the credential as a bearer (not the client's own token).
// The same call re-enrolls an already-known device presenting the same key.
func (c *Client) EnrollDevice(ctx context.Context, enrollmentCredential string, in types.DeviceEnrollRequest) (*types.DeviceEnrollResponse, error) {
	_, resp, err := c.postJSON(ctx, "/mdm/enroll", in, "Authorization", "Bearer "+enrollmentCredential)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.DeviceEnrollResponse{})
}
