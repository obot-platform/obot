package client

import (
	"context"

	"github.com/otto8-ai/otto8/pkg/gateway/types"
)

func (c *Client) User(ctx context.Context, username string) (*types.User, error) {
	u := new(types.User)
	return u, c.db.WithContext(ctx).Where("username = ?", username).First(u).Error
}
