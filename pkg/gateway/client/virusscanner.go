package client

import (
	"context"
	"errors"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) GetVirusScannerConfig(ctx context.Context) (*types.FileScannerConfig, error) {
	var config types.FileScannerConfig
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&config).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&config).Error
	}); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Client) UpdateVirusScannerConfig(ctx context.Context, config *types.FileScannerConfig) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingConfig types.FileScannerConfig
		err := tx.First(&existingConfig).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(config).Error
		} else if err == nil {
			config.ID = existingConfig.ID
			return tx.Save(config).Error
		}
		return err
	})
}
