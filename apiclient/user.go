package apiclient

import (
	"context"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

// GetMe returns the user the client acts for, with their effective role and groups.
func (c *Client) GetMe(ctx context.Context) (types.User, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/me", nil)
	if err != nil {
		return types.User{}, err
	}
	var user types.User
	_, err = toObject(resp, &user)
	return user, err
}
