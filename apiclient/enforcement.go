package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

// Decide asks the server for a synchronous enforcement verdict on a normalized
// tool call. The caller supplies device authentication via WithToken; the server
// resolves the fleet from that identity and never from the request body.
//
// Every failure — transport, non-2xx, undecodable body — is returned as a
// non-nil error alongside a zero-valued response, never as a response on its
// own. A zero response has Decision == "", which a caller could otherwise read
// as "not a deny"; the verdict is only meaningful when the error is nil.
func (c *Client) Decide(ctx context.Context, req types.EnforcementDecisionRequest) (types.EnforcementDecisionResponse, error) {
	_, resp, err := c.postJSON(ctx, "/enforcement/decisions", req)
	if err != nil {
		return types.EnforcementDecisionResponse{}, err
	}
	defer resp.Body.Close()

	decision, err := toObject(resp, &types.EnforcementDecisionResponse{})
	if err != nil {
		return types.EnforcementDecisionResponse{}, err
	}
	return *decision, nil
}
