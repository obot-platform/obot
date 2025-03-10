package client

import (
	"context"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// CreateImage stores a new image in the database
func (c *Client) CreateImage(ctx context.Context, data []byte, mimeType string) (*types.Image, error) {
	img := &types.Image{
		Data:     data,
		MIMEType: mimeType,
	}

	return img, c.db.WithContext(ctx).Create(img).Error
}

// GetImage retrieves an image by its ID
func (c *Client) GetImage(ctx context.Context, id string) (*types.Image, error) {
	img := new(types.Image)
	return img, c.db.WithContext(ctx).Where("id = ?", id).First(img).Error
}

// DeleteImage removes an image from the database
func (c *Client) DeleteImage(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Image{}).Error
}
