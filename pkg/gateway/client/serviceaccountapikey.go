package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
)

const (
	serviceAccountTokenSecretLength = 32
)

func (c *Client) CreateServiceAccountAPIKey(ctx context.Context, serviceAccountName string, validAfter time.Time) (*types.ServiceAccountAPIKeyCreateResponse, error) {
	if _, ok := serviceaccounts.Get(serviceAccountName); !ok {
		return nil, fmt.Errorf("unknown service account %q", serviceAccountName)
	}

	secretBytes := make([]byte, serviceAccountTokenSecretLength)
	_, _ = rand.Read(secretBytes) // rand.Read never returns an error and always fills the slice entirely

	token := fmt.Sprintf("%s.%s.%s", serviceaccounts.TokenPrefix, serviceAccountName, base64.RawURLEncoding.EncodeToString(secretBytes))
	apiKey := &types.ServiceAccountAPIKey{
		ServiceAccountName: serviceAccountName,
		HashedToken:        hash.String(token),
		CreatedAt:          validAfter.UTC(),
		ValidAfter:         validAfter.UTC(),
	}

	if err := c.db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create service account API key: %w", err)
	}

	return &types.ServiceAccountAPIKeyCreateResponse{
		ServiceAccountAPIKey: *apiKey,
		Token:                token,
	}, nil
}

func (c *Client) ListServiceAccountAPIKeys(ctx context.Context, serviceAccountName string) ([]types.ServiceAccountAPIKey, error) {
	var keys []types.ServiceAccountAPIKey
	if err := c.db.WithContext(ctx).
		Where("service_account_name = ?", serviceAccountName).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list service account API keys: %w", err)
	}
	return keys, nil
}

func (c *Client) ValidateStorageServiceAccountToken(ctx context.Context, token string) (*types.ServiceAccountAPIKey, error) {
	var apiKey types.ServiceAccountAPIKey
	now := time.Now().UTC()
	if err := c.db.WithContext(ctx).
		Where("hashed_token = ?", hash.String(token)).
		Where("valid_after <= ?", now).
		Where("retire_after IS NULL OR retire_after > ?", now).
		First(&apiKey).Error; err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (c *Client) RetireOtherServiceAccountAPIKeys(ctx context.Context, serviceAccountName string, keepID uint, retireAfter time.Time) error {
	return c.db.WithContext(ctx).
		Model(&types.ServiceAccountAPIKey{}).
		Where("service_account_name = ?", serviceAccountName).
		Where("id <> ?", keepID).
		Where("retire_after IS NULL OR retire_after > ?", retireAfter.UTC()).
		Update("retire_after", retireAfter.UTC()).Error
}

func (c *Client) DeleteExpiredServiceAccountAPIKeys(ctx context.Context, serviceAccountName string, now time.Time) error {
	return c.db.WithContext(ctx).
		Where("service_account_name = ?", serviceAccountName).
		Where("retire_after IS NOT NULL AND retire_after <= ?", now.UTC()).
		Delete(&types.ServiceAccountAPIKey{}).Error
}

func (c *Client) DeleteServiceAccountAPIKeyByID(ctx context.Context, keyID uint) error {
	return c.db.WithContext(ctx).
		Where("id = ?", keyID).
		Delete(&types.ServiceAccountAPIKey{}).Error
}

func (c *Client) DeleteAllServiceAccountAPIKeys(ctx context.Context, serviceAccountName string) error {
	return c.db.WithContext(ctx).
		Where("service_account_name = ?", serviceAccountName).
		Delete(&types.ServiceAccountAPIKey{}).Error
}
