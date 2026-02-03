package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	mcpTokenSecretLength = 32 // 32 bytes = 256 bits of entropy
	mcpTokenPrefix       = "mt1"
)

// CreateObotMCPToken generates a new MCP token for the given user.
// Returns the full token only once in the response.
func (c *Client) CreateObotMCPToken(ctx context.Context, userID uint) (*types.ObotMCPTokenCreateResponse, error) {
	// Generate cryptographically secure random secret
	secretBytes := make([]byte, mcpTokenSecretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	// Hash the secret with bcrypt for storage
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash secret: %w", err)
	}

	// Create the MCP token record
	token := &types.ObotMCPToken{
		UserID:       userID,
		HashedSecret: string(hashedSecret),
		CreatedAt:    time.Now(),
	}

	if err := c.db.WithContext(ctx).Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create MCP token: %w", err)
	}

	// Construct the full token with the auto-generated ID
	fullToken := fmt.Sprintf("%s-%d-%d-%s", mcpTokenPrefix, userID, token.ID, secret)

	return &types.ObotMCPTokenCreateResponse{
		ObotMCPToken: *token,
		Token:        fullToken,
	}, nil
}

// ListObotMCPTokens returns all MCP tokens for a user (without the secrets).
func (c *Client) ListObotMCPTokens(ctx context.Context, userID uint) ([]types.ObotMCPToken, error) {
	var tokens []types.ObotMCPToken
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("failed to list MCP tokens: %w", err)
	}
	return tokens, nil
}

// GetObotMCPToken retrieves a single MCP token by ID.
func (c *Client) GetObotMCPToken(ctx context.Context, userID uint, tokenID uint) (*types.ObotMCPToken, error) {
	var token types.ObotMCPToken
	if err := c.db.WithContext(ctx).Where("id = ?", tokenID).Where("user_id = ?", userID).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// DeleteObotMCPToken removes an MCP token.
// This operation is idempotent - deleting a non-existent token is not an error.
func (c *Client) DeleteObotMCPToken(ctx context.Context, userID uint, tokenID uint) error {
	result := c.db.WithContext(ctx).Where("id = ?", tokenID).Where("user_id = ?", userID).Delete(&types.ObotMCPToken{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete MCP token: %w", result.Error)
	}
	return nil
}

// ValidateObotMCPToken validates an MCP token and returns the associated ObotMCPToken record.
// The token format is: mt1-<user_id>-<token_id>-<secret>
// Lookup is done by token ID, then bcrypt is used to verify the secret.
// Also updates the last_used_at timestamp on successful validation.
func (c *Client) ValidateObotMCPToken(ctx context.Context, token string) (*types.ObotMCPToken, error) {
	// Parse the token to extract components
	_, userID, tokenID, secret, err := ParseObotMCPToken(token)
	if err != nil {
		return nil, err
	}

	var mcpToken types.ObotMCPToken
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Look up by token ID
		if err := tx.Where("id = ?", tokenID).Where("user_id = ?", userID).First(&mcpToken).Error; err != nil {
			return err
		}

		// Verify the secret using bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(mcpToken.HashedSecret), []byte(secret)); err != nil {
			return fmt.Errorf("invalid MCP token")
		}

		// Update last used timestamp if more than a minute has elapsed
		now := time.Now()
		if mcpToken.LastUsedAt == nil || now.Sub(*mcpToken.LastUsedAt) > time.Minute {
			mcpToken.LastUsedAt = &now
			return tx.Model(&mcpToken).Update("last_used_at", now).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &mcpToken, nil
}

// ParseObotMCPToken parses an MCP token string and extracts its components.
// Returns prefix, userID, tokenID, secret, and an error if the format is invalid.
func ParseObotMCPToken(token string) (prefix string, userID uint, tokenID uint, secret string, err error) {
	n, err := fmt.Sscanf(token, "%3s-%d-%d-%s", &prefix, &userID, &tokenID, &secret)
	if err != nil || n != 4 {
		return "", 0, 0, "", fmt.Errorf("invalid MCP token format")
	}
	if prefix != mcpTokenPrefix {
		return "", 0, 0, "", fmt.Errorf("invalid MCP token prefix")
	}
	return prefix, userID, tokenID, secret, nil
}
