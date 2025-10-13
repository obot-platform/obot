package client

import (
	"context"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// CreateTokenRequest creates a new token request in the database.
func (c *Client) CreateTokenRequest(ctx context.Context, tr *types.TokenRequest) error {
	return c.db.WithContext(ctx).Create(tr).Error
}

// GetTokenRequest retrieves a token request by ID.
func (c *Client) GetTokenRequest(ctx context.Context, id string) (*types.TokenRequest, error) {
	var tr types.TokenRequest
	if err := c.db.WithContext(ctx).First(&tr, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &tr, nil
}
