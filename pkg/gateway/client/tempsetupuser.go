package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// SetTempUserCache stores a temporary user in the database for the bootstrap setup flow.
// Returns an error if a user is already cached.
func (c *Client) SetTempUserCache(ctx context.Context, user *types.User, authProviderName, authProviderNamespace string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if a temp user already exists
		var count int64
		if err := tx.Model(&types.TempSetupUser{}).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to check for existing temp setup user: %w", err)
		}

		if count > 0 {
			// Get the existing user's email for the error message
			var existingUser types.TempSetupUser
			if err := tx.First(&existingUser).Error; err != nil {
				return fmt.Errorf("temporary user already cached")
			}
			return fmt.Errorf("temporary user already cached: %s", existingUser.Email)
		}

		// Create new temp user entry
		tempUser := &types.TempSetupUser{
			UserID:                user.ID,
			Username:              user.Username,
			Email:                 user.Email,
			Role:                  user.Role,
			IconURL:               user.IconURL,
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			CreatedAt:             time.Now(),
		}

		if err := tx.Create(tempUser).Error; err != nil {
			return fmt.Errorf("failed to create temp user: %w", err)
		}

		return nil
	})
}

// GetTempUserCache retrieves the cached temporary user, if one exists.
// Returns nil if no user is cached.
func (c *Client) GetTempUserCache(ctx context.Context) *types.TempSetupUser {
	var tempUser types.TempSetupUser

	if err := c.db.WithContext(ctx).First(&tempUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		// Log error but return nil to maintain existing behavior
		log.Errorf("failed to get temp user cache: %v", err)
		return nil
	}

	return &tempUser
}

// ClearTempUserCache removes all cached temporary users from the database.
func (c *Client) ClearTempUserCache(ctx context.Context) error {
	// Delete all temp users (should only be one, but delete all to be safe)
	if err := c.db.WithContext(ctx).Where("1 = 1").Delete(&types.TempSetupUser{}).Error; err != nil {
		return fmt.Errorf("failed to clear temp user cache: %w", err)
	}
	return nil
}
