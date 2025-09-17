package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	verifiedAuthProviders = []string{
		"default/google-auth-provider",
		"default/github-auth-provider",
	}

	identityGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "identities",
	}
)

// FindIdentitiesForUser finds all identities for the given user.
func (c *Client) FindIdentitiesForUser(ctx context.Context, userID uint) ([]types.Identity, error) {
	var identities []types.Identity
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Find(&identities).Error; err != nil {
		return nil, err
	}

	for i := range identities {
		if err := c.decryptIdentity(ctx, &identities[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt identity: %w", err)
		}
	}

	return identities, nil
}

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func (c *Client) EnsureIdentity(ctx context.Context, id *types.Identity, timezone string) (*types.User, error) {
	return c.EnsureIdentityWithRole(ctx, id, timezone, c.emailsWithExplictRoles[id.Email])
}

// EnsureIdentityWithRole ensures the given identity exists in the database with the at least the given role, and returns the user associated with it.
// If the user already exists with a superset of the given role, it will not be updated.
func (c *Client) EnsureIdentityWithRole(ctx context.Context, id *types.Identity, timezone string, role types2.Role) (*types.User, error) {
	var (
		user    *types.User
		created bool
	)
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, created, err = c.ensureIdentity(ctx, tx, id, timezone, role)
		return err
	})
	if err != nil {
		return nil, err
	}

	if created {
		if user.Role == types2.RoleUnknown {
			user.Role, err = c.getDefaultRole(ctx)
			if err != nil {
				return nil, err
			}

			if user, err = c.UpdateUser(ctx, true, user, fmt.Sprintf("%d", user.ID)); err != nil {
				return nil, err
			}
		}

		if user.Role.HasRole(types2.RolePowerUser) {
			if err = c.ensureNewPrivilegedUser(ctx, user); err != nil {
				return nil, err
			}
		}
	}

	return user, nil
}

// EncryptIdentities will pull all identities out of the database and ensure they are encrypted.
func (c *Client) EncryptIdentities(ctx context.Context, force bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identities []types.Identity
		if err := tx.Find(&identities).Error; err != nil {
			return err
		}

		for i := range identities {
			if !force && identities[i].Encrypted {
				continue
			}

			if err := c.decryptIdentity(ctx, &identities[i]); err != nil {
				return fmt.Errorf("failed to decrypt identity: %w", err)
			}

			if err := c.encryptIdentity(ctx, &identities[i]); err != nil {
				return fmt.Errorf("failed to encrypt identity: %w", err)
			}

			if err := tx.Updates(identities[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ensureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func (c *Client) ensureIdentity(ctx context.Context, tx *gorm.DB, id *types.Identity, timezone string, role types2.Role) (*types.User, bool, error) {
	verified := slices.Contains(verifiedAuthProviders, fmt.Sprintf("%s/%s", id.AuthProviderNamespace, id.AuthProviderName))

	email := id.Email
	providerUserID := id.ProviderUserID

	if id.ProviderUserID != "" {
		id.HashedProviderUserID = hash.String(id.ProviderUserID)
	}
	if id.Email != "" {
		id.HashedEmail = hash.String(id.Email)
	}
	// See if the identity already exists.
	if err := tx.First(id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		// The identity does not exist.
		// Before we try creating a new identity, we need to check if there is one that has not been fully migrated yet.
		migratedIdentity := &types.Identity{
			ProviderUsername:      id.ProviderUsername,
			HashedProviderUserID:  hash.String(fmt.Sprintf("OBOT_PLACEHOLDER_%s", id.ProviderUsername)),
			AuthProviderName:      id.AuthProviderName,
			AuthProviderNamespace: id.AuthProviderNamespace,
		}
		if err = tx.First(migratedIdentity).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			// The identity does not exist, so create it.
			if err = c.encryptIdentity(ctx, id); err != nil {
				return nil, false, fmt.Errorf("failed to encrypt identity: %w", err)
			}
			if err = tx.Create(id).Error; err != nil {
				return nil, false, err
			}
		} else if err != nil {
			return nil, false, err
		} else {
			if err = c.encryptIdentity(ctx, id); err != nil {
				return nil, false, fmt.Errorf("failed to encrypt identity: %w", err)
			}

			// The migrated identity exists. We need to update it with the right provider_user_id.
			if err = tx.Model(&migratedIdentity).Where("hashed_provider_user_id = ?", migratedIdentity.HashedProviderUserID).Updates(map[string]any{"provider_user_id": id.ProviderUserID, "hashed_provider_user_id": id.HashedProviderUserID}).Error; err != nil {
				return nil, false, err
			}

			// Now we should be able to load the identity.
			if err = tx.First(id).Error; err != nil {
				return nil, false, err
			}
		}
	} else if err != nil {
		return nil, false, err
	}
	if err := c.decryptIdentity(ctx, id); err != nil {
		return nil, false, fmt.Errorf("failed to decrypt identity: %w", err)
	}

	user := &types.User{
		ID:             id.UserID,
		Username:       id.ProviderUsername,
		HashedUsername: hash.String(id.ProviderUsername),
		Email:          id.Email,
		HashedEmail:    id.HashedEmail,
		VerifiedEmail:  &verified,
		Role:           role,
	}

	var created, checkForExistingUser bool
	userQuery := tx
	if user.ID != 0 {
		// Check for an existing user with this exact ID.
		userQuery = userQuery.Where("id = ?", user.ID)
		checkForExistingUser = true
	} else if verified {
		// Check for an existing user with this exact verified email address.
		// We check for both true and null values, because the email might have been verified before we started tracking verified emails.
		userQuery = userQuery.Where("hashed_email = ? and (verified_email = true or verified_email is null)", user.HashedEmail)
		checkForExistingUser = true
	}

	if checkForExistingUser {
		// Copy the user so that we don't have to decrypt unless the user already exists.
		u := *user
		if err := userQuery.First(&u).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			created = true
			if err = c.encryptUser(ctx, &u); err != nil {
				return nil, false, fmt.Errorf("failed to encrypt user: %w", err)
			}
			if err = tx.Create(&u).Error; err != nil {
				return nil, false, err
			}

			// Copy the auto-generated values back to the user object.
			user.ID = u.ID
			user.CreatedAt = u.CreatedAt
			user.Role = u.Role
		} else if err != nil {
			return nil, false, err
		} else {
			if err = c.decryptUser(ctx, &u); err != nil {
				return nil, false, fmt.Errorf("failed to decrypt user: %w", err)
			}

			// Copy the decrypted existing user back.
			*user = u

			// We're using an existing user. See if there are any fields that need to be updated.
			var userChanged bool
			if !user.Role.HasRole(role) {
				user.Role = user.Role.SwitchBaseRole(role)
				userChanged = true
			}

			if r := c.HasExplicitRole(user.Email); !user.Role.HasRole(r) {
				user.Role = user.Role.SwitchBaseRole(r)
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

			if user.Username != id.ProviderUsername {
				user.Username = id.ProviderUsername
				user.HashedUsername = hash.String(user.Username)
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
				// Copy user so we don't have to decrypt
				u = *user
				if err := c.encryptUser(ctx, &u); err != nil {
					return nil, false, fmt.Errorf("failed to encrypt user: %w", err)
				}
				if err = tx.Updates(u).Error; err != nil {
					return nil, false, err
				}
			}
		}
	} else {
		// Copy the user so we don't have to decrypt
		u := *user
		if err := c.encryptUser(ctx, &u); err != nil {
			return nil, false, fmt.Errorf("failed to encrypt user: %w", err)
		}
		if err := tx.Create(&u).Error; err != nil {
			return nil, false, err
		}

		// Copy the values that were created instead of decrypting the whole object.
		user.ID = u.ID
		user.CreatedAt = u.CreatedAt
	}

	// Update the user ID saved on the identity if needed.
	// This also corrects the provider user ID to correct a bug introduced when re-encrypting all users and identities
	if id.Email != email || id.UserID != user.ID || id.ProviderUserID != providerUserID {
		id.Email = email
		id.UserID = user.ID
		id.ProviderUserID = providerUserID
		id.HashedProviderUserID = hash.String(id.ProviderUserID)

		if err := c.encryptAndUpdateIdentity(ctx, tx, *id); err != nil {
			return nil, false, err
		}
	}

	// Ensure groups and group memberships are up to date
	groupsLastChecked := id.AuthProviderGroupsLastChecked
	if err := c.ensureGroups(ctx, tx, id); err != nil {
		return nil, false, fmt.Errorf("failed to update groups for identity: %w", err)
	}
	if !groupsLastChecked.Equal(id.AuthProviderGroupsLastChecked) {
		// Groups were updated, so we should update the last checked time on the identity.
		if err := c.encryptAndUpdateIdentity(ctx, tx, *id); err != nil {
			return nil, false, err
		}
	}

	return user, created, nil
}

// encryptAndUpdateIdentity encrypts the identity and updates it in the database.
// It does not take a pointer so that the caller can use the identity object after the call without decrypting it.
func (c *Client) encryptAndUpdateIdentity(ctx context.Context, tx *gorm.DB, id types.Identity) error {
	if err := c.encryptIdentity(ctx, &id); err != nil {
		return fmt.Errorf("failed to encrypt identity: %w", err)
	}

	if err := tx.Updates(&id).Error; err != nil {
		return fmt.Errorf("failed to update identity: %w", err)
	}

	return nil
}

// RemoveIdentity deletes an identity from the database.
// The identity is deleted using UserID if set, otherwise ProviderUsername.
// The method is idempotent and ignores not-found errors, returning only unexpected errors.
func (c *Client) RemoveIdentity(ctx context.Context, id *types.Identity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityQuery *gorm.DB

		// Build queries based on UserID or ProviderUsername
		if id.UserID != 0 {
			// Use UserID if set
			identityQuery = tx.Where("user_id = ?", id.UserID)
		} else {
			// Fall back to ProviderUsername
			identityQuery = tx.Where("hashed_provider_user_id = ?", id.HashedProviderUserID)
		}

		// Attempt to delete the identity
		if err := identityQuery.Delete(&types.Identity{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return nil
	})
}

// RemoveIdentityAndUser deletes an identity and the associated user from the database.
// The identity and user are deleted using UserID if set, otherwise ProviderUsername.
// The method is idempotent and ignores not-found errors, returning only unexpected errors.
func (c *Client) RemoveIdentityAndUser(ctx context.Context, id *types.Identity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityQuery, userQuery *gorm.DB

		// Build queries based on UserID or ProviderUsername
		if id.UserID != 0 {
			// Use UserID if set
			identityQuery = tx.Where("user_id = ?", id.UserID)
			userQuery = tx.Where("id = ?", id.UserID)
		} else {
			// Fall back to ProviderUsername
			identityQuery = tx.Where("hashed_provider_user_id = ?", id.HashedProviderUserID)
			userQuery = tx.Where("hashed_username = ?", hash.String(id.ProviderUsername))
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

func (c *Client) ensureNewPrivilegedUser(ctx context.Context, user *types.User) error {
	var defaultRole v1.UserDefaultRoleSetting
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &defaultRole); err != nil {
		return fmt.Errorf("failed to get default role setting: %w", err)
	}

	if err := c.storageClient.Create(ctx, &v1.UserRoleChange{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.UserRoleChangePrefix,
			Namespace:    system.DefaultNamespace,
		},
		Spec: v1.UserRoleChangeSpec{
			UserID:  user.ID,
			OldRole: types2.RoleBasic, // New users start as basic
			NewRole: defaultRole.Spec.Role,
		},
	}); err != nil {
		return fmt.Errorf("failed to create user role change event for new privileged user %d: %w", user.ID, err)
	}

	return nil
}

func (c *Client) getDefaultRole(ctx context.Context) (types2.Role, error) {
	var defaultRole v1.UserDefaultRoleSetting
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &defaultRole); err != nil {
		return types2.RoleBasic, fmt.Errorf("failed to get default role setting: %w", err)
	}

	return defaultRole.Spec.Role, nil
}

func (c *Client) encryptIdentity(ctx context.Context, identity *types.Identity) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[identityGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = identityDataCtx(identity)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.ProviderUsername), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.ProviderUsername = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.Email), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.Email = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.ProviderUserID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.ProviderUserID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.IconURL), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.IconURL = base64.StdEncoding.EncodeToString(b)
	}

	identity.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptIdentity(ctx context.Context, identity *types.Identity) error {
	if !identity.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[identityGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = identityDataCtx(identity)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.ProviderUsername)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.ProviderUsername))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.ProviderUsername = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.Email)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.Email))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.Email = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.ProviderUserID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.ProviderUserID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.ProviderUserID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.IconURL)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.IconURL))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.IconURL = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func identityDataCtx(identity *types.Identity) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s/%s", identityGroupResource.String(), identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID))
}
