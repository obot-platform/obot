package client

import (
	"context"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (c *Client) ClaudeEnvironments(ctx context.Context) ([]types.ClaudeEnvironment, error) {
	environments := []types.ClaudeEnvironment{}
	err := c.db.WithContext(ctx).Order("display_name, id").Find(&environments).Error
	return environments, err
}

func (c *Client) ClaudeEnvironment(ctx context.Context, id string) (*types.ClaudeEnvironment, error) {
	var environment types.ClaudeEnvironment
	err := c.db.WithContext(ctx).Where("id = ?", id).First(&environment).Error
	return &environment, err
}

// SaveClaudeEnvironment atomically stores metadata and write-only credentials.
// Environment keys follow the same encryption configuration as other credentials.
func (c *Client) SaveClaudeEnvironment(ctx context.Context, environment *types.ClaudeEnvironment, secrets map[string]string) error {
	credential := types.Credential{Context: "claude-environment", Name: environment.ID, Secrets: secrets}
	if secrets != nil {
		if err := c.encryptCredential(ctx, &credential); err != nil {
			return err
		}
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if secrets != nil {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
			}).Create(&credential).Error; err != nil {
				return err
			}
		}
		return tx.Save(environment).Error
	})
}

func (c *Client) DeleteClaudeEnvironment(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("context = ? AND name = ?", "claude-environment", id).Delete(&types.Credential{}).Error; err != nil {
			return err
		}
		var environment types.ClaudeEnvironment
		if err := tx.Where("id = ?", id).First(&environment).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&types.ClaudeEnvironment{}).Error
	})
}
