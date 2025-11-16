package client

import (
	"context"
	"errors"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) GetProperty(ctx context.Context, key string) (types.Property, error) {
	var p types.Property
	return p, c.db.WithContext(ctx).Where("key = ?", key).First(&p).Error
}

func (c *Client) SetProperty(ctx context.Context, key, value string) (types.Property, error) {
	now := time.Now()
	var p types.Property
	return p, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key = ?", key).First(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				p = types.Property{
					Key:       key,
					Value:     value,
					CreatedAt: now,
					UpdatedAt: now,
				}
				return tx.Create(&p).Error
			}
			return c.db.WithContext(ctx).Create(&p).Error
		}
		p.Value = value
		p.UpdatedAt = time.Now()
		return tx.Save(&p).Error
	})
}
