package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	llmGatewaySettingsKey = "llm-gateway-settings"
)

// GetLLMGatewaySettings returns the LLM gateway settings Obot enforces, or the defaults when
// none were saved.
func (c *Client) GetLLMGatewaySettings(ctx context.Context) (types.LLMGatewaySettings, error) {
	var settings types.LLMGatewaySettings
	p, err := c.GetProperty(ctx, llmGatewaySettingsKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return settings, nil
	} else if err != nil {
		return settings, err
	}
	if err := json.Unmarshal([]byte(p.Value), &settings); err != nil {
		return settings, fmt.Errorf("invalid LLM gateway settings: %w", err)
	}
	return settings, nil
}

// SetLLMGatewaySettings saves the LLM gateway settings.
func (c *Client) SetLLMGatewaySettings(ctx context.Context, settings types.LLMGatewaySettings) error {
	_, _, err := c.UpdateLLMGatewaySettings(ctx, types.LLMGatewaySettingsUpdate{
		ClaudeSubscriptionPassthrough: &settings.ClaudeSubscriptionPassthrough,
		CodexSubscriptionPassthrough:  &settings.CodexSubscriptionPassthrough,
	})
	return err
}

// UpdateLLMGatewaySettings atomically merges supplied fields and reports whether they changed.
func (c *Client) UpdateLLMGatewaySettings(ctx context.Context, update types.LLMGatewaySettingsUpdate) (types.LLMGatewaySettings, bool, error) {
	var settings types.LLMGatewaySettings
	var changed bool
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		initial := types.Property{Key: llmGatewaySettingsKey, Value: "{}"}
		if err := c.encryptProperty(ctx, &initial); err != nil {
			return err
		}
		// Write before reading: the upsert locks even an existing row across replicas
		// on both SQLite and Postgres, without replacing its current settings.
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"key"}),
		}).Create(&initial).Error; err != nil {
			return err
		}

		var stored types.Property
		if err := tx.Where("key = ?", llmGatewaySettingsKey).First(&stored).Error; err != nil {
			return err
		}
		if err := c.decryptProperty(ctx, &stored); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(stored.Value), &settings); err != nil {
			return fmt.Errorf("invalid LLM gateway settings: %w", err)
		}
		previous := settings
		if update.ClaudeSubscriptionPassthrough != nil {
			settings.ClaudeSubscriptionPassthrough = *update.ClaudeSubscriptionPassthrough
		}
		if update.CodexSubscriptionPassthrough != nil {
			settings.CodexSubscriptionPassthrough = *update.CodexSubscriptionPassthrough
		}
		changed = previous != settings
		if !changed {
			return nil
		}

		data, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		stored.Value = string(data)
		if err := c.encryptProperty(ctx, &stored); err != nil {
			return err
		}
		return tx.Model(&types.Property{}).Where("key = ?", llmGatewaySettingsKey).
			Updates(map[string]any{"value": stored.Value, "encrypted": stored.Encrypted}).Error
	})
	return settings, changed, err
}
