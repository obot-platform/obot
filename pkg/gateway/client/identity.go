package client

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

var verifiedAuthProviders = []string{
	"default/google-auth-provider",
	"default/github-auth-provider",
}

func (c *Client) FindIdentitiesForUser(ctx context.Context, userID uint) ([]types.Identity, error) {
	var identities []types.Identity
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Find(&identities).Error; err != nil {
		return nil, err
	}
	return identities, nil
}

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
	verified := slices.Contains(verifiedAuthProviders, fmt.Sprintf("%s/%s", id.AuthProviderNamespace, id.AuthProviderName))

	email := id.Email

	// See if the identity already exists.
	if err := tx.First(id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		// The identity does not exist.
		// Before we try creating a new identity, we need to check if there is one that has not been fully migrated yet.
		migratedIdentity := &types.Identity{
			ProviderUsername:      id.ProviderUsername,
			ProviderUserID:        fmt.Sprintf("OBOT_PLACEHOLDER_%s", id.ProviderUsername),
			AuthProviderName:      id.AuthProviderName,
			AuthProviderNamespace: id.AuthProviderNamespace,
		}
		if err := tx.First(migratedIdentity).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			// The identity does not exist, so create it.
			if err = tx.Create(id).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			// The migrated identity exists. We need to update it with the right provider_user_id.
			if err := tx.Model(&migratedIdentity).Where("provider_user_id = ?", fmt.Sprintf("OBOT_PLACEHOLDER_%s", id.ProviderUsername)).Update("provider_user_id", id.ProviderUserID).Error; err != nil {
				return nil, err
			}

			// Now we should be able to load the identity.
			if err := tx.First(id).Error; err != nil {
				return nil, err
			}
		}
	} else if err != nil {
		return nil, err
	}

	// Check to see if the email got updated.
	if id.Email != email {
		id.Email = email
		if err := tx.Updates(id).Error; err != nil {
			return nil, err
		}
	}

	user := &types.User{
		ID:            id.UserID,
		Username:      id.ProviderUsername,
		Email:         id.Email,
		VerifiedEmail: &verified,
		Role:          role,
	}
	if user.Role == types2.RoleUnknown {
		user.Role = types2.RoleBasic
	}

	var checkForExistingUser bool
	userQuery := tx
	if user.ID != 0 {
		// Check for an existing user with this exact ID.
		userQuery = userQuery.Where("id = ?", user.ID)
		checkForExistingUser = true
	} else if verified {
		// Check for an existing user with this exact verified email address.
		// We check for both true and null values, because the email might have been verified before we started tracking verified emails.
		userQuery = userQuery.Where("email = ? and (verified_email = true or verified_email is null)", user.Email)
		checkForExistingUser = true
	}

	if checkForExistingUser {
		if err := userQuery.First(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err = tx.Create(&user).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			// We're using an existing user. See if there are any fields that need to be updated.
			var userChanged bool
			if role != types2.RoleUnknown && user.Role != role {
				user.Role = role
				userChanged = true
			}

			if user.Timezone == "" && timezone != "" {
				user.Timezone = timezone
				userChanged = true
			}

			if time.Since(user.LastActiveDay) > 24*time.Hour {
				user.LastActiveDay = time.Now().UTC().Truncate(24 * time.Hour)
				userChanged = true
			}

			// Update the verified email status if needed.
			// This can happen in two cases:
			// 1. The user was created before we started tracking verified emails (user.VerifiedEmail is nil)
			// 2. The user was created before we started tracking verified emails, and associated with both a verified
			//    and unverified auth provider. They logged in with the unverified provider and we marked the email as unverified,
			//    but now they've logged in with the verified provider and we can mark the email as verified. (verified is true, but user.VerifiedEmail is false)
			if user.VerifiedEmail == nil || (verified && !*user.VerifiedEmail) {
				user.VerifiedEmail = &verified
				userChanged = true
			}

			if userChanged {
				if err := tx.Updates(user).Error; err != nil {
					return nil, err
				}
			}
		}
	} else {
		if err := tx.Create(&user).Error; err != nil {
			return nil, err
		}
	}

	// Update the user ID saved on the identity if needed.
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
