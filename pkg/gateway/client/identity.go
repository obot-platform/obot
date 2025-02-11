package client

import (
	"context"
	"errors"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func (c *Client) EnsureIdentity(ctx context.Context, id *types.Identity, timezone string) (*types.User, error) {
	var role types2.Role
	if _, ok := c.adminEmails[id.Email]; ok {
		role = types2.RoleAdmin
	}

	return c.EnsureIdentityWithRole(ctx, id, timezone, role)
}

// EnsureIdentityWithRole ensures the given identity exists in the database with the given role, and returns the user associated with it.
func (c *Client) EnsureIdentityWithRole(ctx context.Context, id *types.Identity, timezone string, role types2.Role) (*types.User, error) {
	var user *types.User
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = ensureIdentity(tx, id, timezone, role)
		return err
	}); err != nil {
		return nil, err
	}

	return user, nil
}

// ensureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func ensureIdentity(tx *gorm.DB, id *types.Identity, timezone string, role types2.Role) (*types.User, error) {
	email := id.Email
	if err := tx.First(id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Create(id).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if id.Email != email {
		id.Email = email
		if err = tx.Updates(id).Error; err != nil {
			return nil, err
		}
	}

	user := &types.User{
		ID:       id.UserID,
		Username: id.ProviderUsername,
		Email:    id.Email,
		Role:     role,
	}
	if user.Role == types2.RoleUnknown {
		user.Role = types2.RoleBasic
	}

	userQuery := tx
	if user.ID != 0 {
		userQuery = userQuery.Where("id = ?", user.ID)
	} else {
		userQuery = userQuery.Where("email = ?", user.Email)
	}

	if err := userQuery.First(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Create(&user).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	var userChanged bool
	if role != types2.RoleUnknown && user.Role != role {
		user.Role = role
		userChanged = true
	}

	if user.Timezone == "" && timezone != "" {
		user.Timezone = timezone
		userChanged = true
	}

	if userChanged {
		if err := tx.Updates(user).Error; err != nil {
			return nil, err
		}
	}

	if id.UserID != user.ID {
		id.UserID = user.ID
		if err := tx.Updates(id).Error; err != nil {
			return nil, err
		}
	}

	return user, nil
}

// RemoveIdentity deletes an identity and the associated user from the database.
// The identity and user are deleted using UserID if set, otherwise ProviderUsername.
// The method is idempotent and ignores not-found errors, returning only unexpected errors.
func (c *Client) RemoveIdentity(ctx context.Context, id *types.Identity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityQuery, userQuery *gorm.DB

		// Build queries based on UserID or ProviderUsername
		if id.UserID != 0 {
			// Use UserID if set
			identityQuery = tx.Where("user_id = ?", id.UserID)
			userQuery = tx.Where("id = ?", id.UserID)
		} else {
			// Fall back to ProviderUsername
			identityQuery = tx.Where("provider_username = ?", id.ProviderUsername)
			userQuery = tx.Where("username = ?", id.ProviderUsername)
		}

		// Attempt to delete the identity
		if err := identityQuery.Delete(&types.Identity{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Attempt to delete the user
		if err := userQuery.Delete(&types.User{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return nil
	})
}
