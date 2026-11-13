package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
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
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = c.SetProperty(ctx, llmGatewaySettingsKey, string(data))
	return err
}
