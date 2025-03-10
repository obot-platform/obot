package client

import (
	"context"
	"errors"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// CreateGeneratedImage stores a new generated image in the database
func (c *Client) CreateGeneratedImage(ctx context.Context, data []byte, mimeType string) (*types.GeneratedImage, error) {
	img := &types.GeneratedImage{
		Data:     data,
		MIMEType: mimeType,
	}

	err := c.db.WithContext(ctx).Create(img).Error
	if err != nil {
		return nil, err
	}

	return img, nil
}

// GetGeneratedImage retrieves an image by its ID
func (c *Client) GetGeneratedImage(ctx context.Context, id string) (*types.GeneratedImage, error) {
	img := new(types.GeneratedImage)
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(img).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return img, nil
}

// DeleteGeneratedImage removes an image from the database
func (c *Client) DeleteGeneratedImage(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.GeneratedImage{}).Error
}
