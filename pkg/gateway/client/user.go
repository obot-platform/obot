package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/storage/value"
)

const (
	// profileIconCheckInterval is how long sign-in waits before asking the auth provider for a user's profile
	// picture again.
	profileIconCheckInterval = 7 * 24 * time.Hour
)

var (
	userGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "users",
	}

	// userEncryptedColumns are the columns that encryptUser writes, for an update that writes a user encrypted again
	// without the rest of its row. It must list every column that encryptUser writes.
	userEncryptedColumns = []string{"username", "email", "icon_url", "display_name", "original_email", "original_username", "encrypted"}

	// profileIconURLs read the profile picture URL in an auth provider's answer to /obot-get-user-info, by auth
	// provider, and report whether the answer sets one. Auth providers without profile pictures, such as Okta,
	// OneLogin, and the local auth provider, have none.
	profileIconURLs = map[string]func(profile map[string]any) (string, bool){
		"github-auth-provider":   profileString("avatar_url"),
		"google-auth-provider":   profileString("picture"),
		"entra-auth-provider":    profileString("icon_url"),
		"auth0-auth-provider":    profileString("icon_url"),
		"keycloak-auth-provider": profileString("icon_url"),
		"jumpcloud-auth-provider": func(profile map[string]any) (string, bool) {
			iconURL, _ := profile["icon_url"].(string)
			if iconURL == "" {
				iconURL, _ = profile["picture"].(string)
			}
			return iconURL, iconURL != ""
		},
	}
)

func (c *Client) UserFromToken(ctx context.Context, token string) (*types.User, string, string, string, []string, error) {
	// Extract the id and hashed token value from the bearer token.
	id, token, _ := strings.Cut(token, ":")

	var (
		namespace, name, providerUserID string
		userID                          uint
	)

	// Look up the auth token to get user ID and auth provider info
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tkn := new(types.AuthToken)
		if err := tx.Where("id = ? AND hashed_token = ?", id, hash.String(token)).First(tkn).Error; err != nil {
			return err
		}

		namespace = tkn.AuthProviderNamespace
		name = tkn.AuthProviderName
		providerUserID = tkn.AuthProviderUserID
		userID = tkn.UserID

		return nil
	}); err != nil {
		return nil, "", "", "", nil, err
	}

	// Get the user and their group IDs for this auth provider
	u, groupIDs, err := c.getUserAndGroupIDs(ctx, userID, namespace, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The token outlived its user. Deny it rather than treating it as an unknown token, which would let the
		// request continue as anonymous.
		return nil, "", "", "", nil, &UserAccessDeniedError{
			UserID: userID,
			Status: types2.UserStatusDeleted,
		}
	} else if err != nil {
		return nil, "", "", "", nil, err
	}

	return u, namespace, name, providerUserID, groupIDs, nil
}

func (c *Client) Users(ctx context.Context, query types.UserQuery) ([]types.User, error) {
	var users []types.User
	if err := c.db.WithContext(ctx).Scopes(query.Scope).Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		if err := c.decryptUser(ctx, &users[i]); err != nil {
			return nil, err
		}
	}

	return users, nil
}

func (c *Client) UserCount(ctx context.Context) (int64, error) {
	return countUsersTowardLimit(c.db.WithContext(ctx))
}

func (c *Client) User(ctx context.Context, username string) (*types.User, error) {
	u := new(types.User)
	if err := c.db.WithContext(ctx).Where("hashed_username = ? AND deleted_at IS NULL", hash.String(username)).First(u).Error; err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

func (c *Client) UserByID(ctx context.Context, id string) (*types.User, error) {
	u := new(types.User)
	if err := c.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(u).Error; err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

// UserByIDIncludeDeleted returns a user by ID including soft-deleted users (for audit purposes)
func (c *Client) UserByIDIncludeDeleted(ctx context.Context, id string) (*types.User, error) {
	u := new(types.User)
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(u).Error; err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

// UserByIDWithEffectiveRole returns the user with the corresponding ID with their effiective role
// and auth provider groups.
func (c *Client) UserByIDWithEffectiveRole(ctx context.Context, id uint) (*types.User, []string, error) {
	u, groupIDs, err := c.getUserAndGroupIDs(ctx, id, "", "")
	if err != nil {
		return nil, nil, err
	}

	role, err := c.ResolveUserEffectiveRole(ctx, u, groupIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve effective role: %w", err)
	}

	u.Role = role
	return u, groupIDs, nil
}

// UserInfoByID returns a user.Info object for the given user ID,
// suitable for use with ACR helper methods. This fetches the user
// and their group memberships from the database.
func (c *Client) UserInfoByID(ctx context.Context, userID uint) (kuser.Info, error) {
	u, groupIDs, err := c.UserByIDWithEffectiveRole(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &kuser.DefaultInfo{
		Name:   u.Username,
		UID:    fmt.Sprintf("%d", u.ID),
		Groups: u.Role.Groups(),
		Extra: map[string][]string{
			"obot_groups":             u.Role.Groups(),
			"auth_provider_groups":    groupIDs,
			"email":                   {u.Email},
			principal.UserStatusExtra: {string(u.Status())},
		},
	}, nil
}

// getUserAndGroupIDs fetches a user and their group memberships.
// If authProviderNamespace and authProviderName are provided, only groups from that provider are returned.
// Otherwise, all groups are returned.
func (c *Client) getUserAndGroupIDs(ctx context.Context, userID any, authProviderNamespace, authProviderName string) (*types.User, []string, error) {
	var (
		u        = new(types.User)
		groupIDs []string
	)

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get the user
		if err := tx.Where("id = ? AND deleted_at IS NULL", userID).First(u).Error; err != nil {
			return err
		}

		// Build the group query
		query := tx.Table("groups").
			Joins("JOIN group_memberships ON groups.id = group_memberships.group_id").
			Where("group_memberships.user_id = ?", userID)

		// Filter by auth provider if specified
		if authProviderNamespace != "" && authProviderName != "" {
			query = query.Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", authProviderNamespace, authProviderName)
		}

		// Get the group IDs
		if err := query.Pluck("groups.id", &groupIDs).Error; err != nil {
			return fmt.Errorf("failed to list auth provider groups for user: %w", err)
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	if err := c.decryptUser(ctx, u); err != nil {
		return nil, nil, err
	}

	return u, groupIDs, nil
}

// UserFromProviderUserID returns a user by their provider user ID
func (c *Client) UserFromProviderUserID(ctx context.Context, providerNamespace, providerName, providerUserID string) (*types.User, error) {
	u := new(types.User)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id := new(types.Identity)
		if err := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", providerNamespace, providerName, hash.String(providerUserID)).First(id).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND deleted_at IS NULL", id.UserID).First(u).Error
	}); err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

// UsersIncludeDeleted returns all users including soft-deleted ones (for audit purposes)
func (c *Client) UsersIncludeDeleted(ctx context.Context, query types.UserQuery) ([]types.User, error) {
	query.IncludeDeleted = true
	return c.Users(ctx, query)
}

// DeleteUser deletes a user as an administrator does: the user is soft-deleted and renamed, their group memberships
// are removed, and an outbox event removes what they own once the deletion commits. A user that SCIM has provisioned
// can be deleted only once the identity provider has deprovisioned them.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := refuseClaudeTagUserMutation(tx, userID); err != nil {
			return err
		}

		// A deleted user is not deleted again, which would rename them again and repeat their cleanup.
		existingUser := new(types.User)
		if err := tx.Where("id = ? AND deleted_at IS NULL", userID).First(existingUser).Error; err != nil {
			return err
		}

		// The user's SCIM ID is retired, so it answers 404 and userName lookups no longer find them.
		if err := retireSCIMUserBindingForDeletionTx(tx, existingUser.ID); err != nil {
			return err
		}

		if err := refuseLastOwnerOrAdminTx(tx, existingUser); err != nil {
			return err
		}
		return c.deleteUserTx(ctx, tx, existingUser)
	}); err != nil {
		return err
	}

	c.kickUserLifecycleDelivery()
	return nil
}

// refuseLastOwnerOrAdminTx refuses to delete user when they are the last Owner, or the last Admin when there is no
// other Owner, with *LastOwnerError or *LastAdminError. user need not be decrypted.
func refuseLastOwnerOrAdminTx(tx *gorm.DB, user *types.User) error {
	// We filter out empty email users here, because that is the bootstrap user.
	// Also exclude soft-deleted and disabled users, who cannot act as Owners or Admins, and the current user from the
	// count.
	if user.Role.HasRole(types2.RoleOwner) {
		// Can't delete the user if they're an owner and there are no other owners
		var ownerCount int64
		if err := tx.Model(new(types.User)).Where("id != ? AND (role & ?) != 0 AND hashed_email != '' AND deleted_at IS NULL AND disabled_at IS NULL",
			user.ID,
			types2.RoleOwner,
		).Count(&ownerCount).Error; err != nil {
			return err
		}

		if ownerCount == 0 {
			return new(LastOwnerError)
		}
	} else if user.Role.HasRole(types2.RoleAdmin) {
		// Can't delete the user if they're an admin and there are no other owners or admins
		var adminCount int64
		if err := tx.Model(new(types.User)).Where("id != ? AND ((role & ?) != 0 OR (role & ?) != 0) AND hashed_email != '' AND deleted_at IS NULL AND disabled_at IS NULL",
			user.ID,
			types2.RoleOwner,
			types2.RoleAdmin,
		).Count(&adminCount).Error; err != nil {
			return err
		}

		if adminCount == 0 {
			return new(LastAdminError)
		}
	}
	return nil
}

// deleteUserTx deletes existingUser, which was read in tx and is not decrypted, within tx. Callers that act for an
// administrator or an identity provider check refuseLastOwnerOrAdminTx first. It records the event whose delivery
// removes what the user owns, so the caller must kick lifecycle delivery after committing.
func (c *Client) deleteUserTx(ctx context.Context, tx *gorm.DB, existingUser *types.User) error {
	// Decrypt user to get original values before soft delete
	if err := c.decryptUser(ctx, existingUser); err != nil {
		return fmt.Errorf("failed to decrypt user: %w", err)
	}

	// Soft delete: set timestamp and preserve original values
	now := time.Now()
	existingUser.DeletedAt = &now
	existingUser.OriginalEmail = existingUser.Email
	existingUser.OriginalUsername = existingUser.Username

	// Clear sensitive data and make fields unique to allow re-signup
	// Append timestamp to prevent conflicts with new users using same email
	deletedSuffix := fmt.Sprintf("_deleted_%d", now.Unix())
	existingUser.Email = existingUser.Email + deletedSuffix
	existingUser.Username = existingUser.Username + deletedSuffix
	existingUser.HashedEmail = hash.String(existingUser.Email)
	existingUser.HashedUsername = hash.String(existingUser.Username)

	// Encrypt the modified user
	if err := c.encryptUser(ctx, existingUser); err != nil {
		return fmt.Errorf("failed to encrypt user: %w", err)
	}

	// Clean up group memberships for the deleted user
	if err := c.deleteGroupMembershipsForUser(ctx, tx, existingUser.ID); err != nil {
		return err
	}

	// Update the user with soft delete fields and modified email/username
	if err := tx.Omit(types.UserLifecycleColumns...).Save(existingUser).Error; err != nil {
		return err
	}

	// What the user owns lives in the controller store, which cannot share this transaction, so it is removed once
	// the deletion commits, and until that succeeds.
	if err := tx.Create(&types.UserLifecycleEvent{
		UserID: existingUser.ID,
		Type:   types.UserLifecycleEventDeleted,
	}).Error; err != nil {
		return fmt.Errorf("failed to record the deletion event of user %d: %w", existingUser.ID, err)
	}
	return nil
}

func (c *Client) UpdateUser(ctx context.Context, actingUserCanChangeRole bool, updatedUser *types.User, userID string) (*types.User, error) {
	existingUser := new(types.User)
	return existingUser, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := refuseClaudeTagUserMutation(tx, userID); err != nil {
			return err
		}

		// Every column of the user is written back, so it is locked, and a concurrent SCIM write of its profile is
		// never overwritten with a stale copy.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(existingUser).Error; err != nil {
			return err
		}

		if err := c.decryptUser(ctx, existingUser); err != nil {
			return fmt.Errorf("failed to decrypt user: %w", err)
		}

		// If the username is being changed, then ensure that a user with that name doesn't already exist.
		if len(updatedUser.Username) != 0 && updatedUser.Username != existingUser.Username {
			if err := refuseSCIMProvisionedUserTx(tx, existingUser.ID, "this user's profile is managed by the identity provider through SCIM"); err != nil {
				return err
			}

			if err := tx.Model(updatedUser).Where("username = ? AND deleted_at IS NULL", updatedUser.Username).First(new(types.User)).Error; err == nil {
				return &AlreadyExistsError{name: fmt.Sprintf("user with username %q", updatedUser.Username)}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			existingUser.Username = updatedUser.Username
		}

		if updatedUser.Timezone != "" {
			existingUser.Timezone = updatedUser.Timezone
		}

		// Only admins can change user roles.
		if actingUserCanChangeRole {
			if updatedUser.Role > 0 {
				// Check if we're removing the Owner role
				removingOwner := existingUser.Role.HasRole(types2.RoleOwner) && !updatedUser.Role.HasRole(types2.RoleOwner)
				// Check if we're removing the Admin role
				removingAdmin := existingUser.Role.HasRole(types2.RoleAdmin) && !updatedUser.Role.HasRole(types2.RoleAdmin)

				if removingOwner || removingAdmin {
					// We filter out empty email users here, because that is the bootstrap user.
					// Also exclude soft-deleted and disabled users, who cannot act as Owners or Admins, and the current
					// user from the count.
					if removingOwner {
						// Can't change role from owner if there are no other owners
						var ownerCount int64
						if err := tx.Model(new(types.User)).Where("id != ? AND (role & ?) != 0 AND hashed_email != '' AND deleted_at IS NULL AND disabled_at IS NULL",
							userID,
							types2.RoleOwner,
						).Count(&ownerCount).Error; err != nil {
							return err
						}

						if ownerCount == 0 {
							return new(LastOwnerError)
						}
					} else if removingAdmin {
						// Can't change role from admin if there are no other owners or admins
						var adminCount int64
						if err := tx.Model(new(types.User)).Where("id != ? AND ((role & ?) != 0 OR (role & ?) != 0) AND hashed_email != '' AND deleted_at IS NULL AND disabled_at IS NULL",
							userID,
							types2.RoleOwner,
							types2.RoleAdmin,
						).Count(&adminCount).Error; err != nil {
							return err
						}

						if adminCount == 0 {
							return new(LastAdminError)
						}
					}
				}

				existingUser.Role = updatedUser.Role
			}
		}

		// Copy the user object that is returned to the caller so they don't get the encrypted values
		u := *existingUser
		if err := c.encryptUser(ctx, &u); err != nil {
			return fmt.Errorf("failed to encrypt user: %w", err)
		}

		return tx.Omit(types.UserLifecycleColumns...).Updates(&u).Error
	})
}

func (c *Client) UpdateUserInternalStatus(ctx context.Context, userID string, internal bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := refuseClaudeTagUserMutation(tx, userID); err != nil {
			return err
		}

		return tx.Model(new(types.User)).Where("id = ? AND deleted_at IS NULL", userID).Update("internal", internal).Error
	})
}

func (c *Client) UpdateProfileIfNeeded(ctx context.Context, user *types.User, authProviderName, authProviderNamespace, authProviderURL string) error {
	if authProviderName == "" || authProviderNamespace == "" || authProviderURL == "" {
		return nil
	}

	accessToken := accesstoken.GetAccessToken(ctx)
	if accessToken == "" {
		return nil
	}

	// While a SCIM connection manages the auth provider, only SCIM writes the names and emails of its users, and of
	// users SCIM has provisioned. Sign-in still refreshes what SCIM does not send, the profile picture, and fills in
	// the display name of a user whom SCIM has not provisioned yet and who has none, because until SCIM provisions the
	// user it is the only name Obot has for them. Both are read with the identity, before the provider is asked, and
	// the binding is checked again before its answer is written. A failed lookup fails the refresh.
	var rows []struct {
		types.Identity
		SCIMManaged bool
		SCIMBound   bool
	}
	if err := c.db.WithContext(ctx).Model(new(types.Identity)).
		Select("identities.*, "+
			"EXISTS (SELECT 1 FROM scim_connections WHERE scim_connections.auth_provider_namespace = identities.auth_provider_namespace AND scim_connections.auth_provider_name = identities.auth_provider_name) AS scim_managed, "+
			"EXISTS (SELECT 1 FROM scim_user_bindings WHERE scim_user_bindings.user_id = identities.user_id AND scim_user_bindings.retired_at IS NULL) AS scim_bound").
		Where("identities.user_id = ?", user.ID).
		Where("identities.auth_provider_name = ?", authProviderName).
		Where("identities.auth_provider_namespace = ?", authProviderNamespace).
		Limit(1).
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return gorm.ErrRecordNotFound
	}
	if rows[0].SCIMBound || rows[0].SCIMManaged {
		return c.refreshSCIMManagedProfile(ctx, user, rows[0].Identity, rows[0].SCIMBound, authProviderName, authProviderURL, accessToken)
	}
	identity := rows[0].Identity

	if err := c.decryptIdentity(ctx, &identity); err != nil {
		return fmt.Errorf("failed to decrypt identity: %w", err)
	}

	if user.IconURL == identity.IconURL && time.Since(identity.IconLastChecked) < profileIconCheckInterval && user.DisplayName != "" {
		// Icon was checked less than 7 days ago, and the user is still using the same auth provider and DisplayName is already set
		return nil
	}

	profile, err := c.fetchUserProfile(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}

	if iconURL, ok := profileIconURL(authProviderName, profile); ok {
		user.IconURL = iconURL
		identity.IconURL = iconURL
	}

	switch authProviderName {
	case "github-auth-provider":
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
			if user.DisplayName == "" {
				if login, ok := profile["login"].(string); ok {
					user.DisplayName = login
				}
			}
		}
	case "entra-auth-provider", "auth0-auth-provider", "keycloak-auth-provider":
		if displayName, ok := profile["name"].(string); ok && displayName != "" {
			user.DisplayName = displayName
		}
	case "google-auth-provider", "okta-auth-provider", "onelogin-auth-provider", system.LocalAuthProvider,
		"jumpcloud-auth-provider":
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
		}
	}

	// Update the provider-native group lookup ID from the profile response.
	if lookupID := extractProfileID(profile); lookupID != "" {
		identity.ProviderGroupLookupID = lookupID
	}

	identity.IconLastChecked = time.Now()

	if err = c.encryptIdentity(ctx, &identity); err != nil {
		return fmt.Errorf("failed to encrypt identity: %w", err)
	}

	// Copy user so that the caller's copy doesn't get encrypted.
	u := *user
	if err = c.encryptUser(ctx, &u); err != nil {
		return fmt.Errorf("failed to encrypt user: %w", err)
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A SCIM connection may have been created, or SCIM may have provisioned the user, while the profile was being
		// fetched. SCIM's profile wins. The mode lock keeps a connection from being created, and the user lock keeps
		// SCIM from provisioning the user, between these checks and the write.
		conn, err := scimConnectionForAuthProviderLockedTx(tx, authProviderNamespace, authProviderName)
		if err != nil {
			return err
		}
		if conn != nil {
			return nil
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", user.ID).Take(new(types.User)).Error; err != nil {
			return err
		}
		binding, err := activeSCIMUserBindingForUserTx(tx, user.ID, false)
		if err != nil {
			return err
		}
		if binding != nil {
			return nil
		}

		// The user was read before the profile was fetched, so its lifecycle state may be stale. Only lifecycle
		// operations write that state.
		if err := tx.Omit(types.UserLifecycleColumns...).Updates(u).Error; err != nil {
			return err
		}

		// Omit the group check column to prevent resetting the group refresh window
		return tx.Omit(groupsLastCheckedColumn).Updates(&identity).Error
	})
}

// profileIconURL returns the profile picture URL in an auth provider's answer to /obot-get-user-info, and whether the
// answer sets one.
func profileIconURL(authProviderName string, profile map[string]any) (string, bool) {
	if read, ok := profileIconURLs[authProviderName]; ok {
		return read(profile)
	}
	return "", false
}

// hasProfilePictures reports whether an auth provider's users have profile pictures.
func hasProfilePictures(authProviderName string) bool {
	_, ok := profileIconURLs[authProviderName]
	return ok
}

// profileString reads the string at key in an auth provider's answer to /obot-get-user-info.
func profileString(key string) func(profile map[string]any) (string, bool) {
	return func(profile map[string]any) (string, bool) {
		value, ok := profile[key].(string)
		return value, ok
	}
}

// refreshSCIMManagedProfile refreshes, at sign-in, what SCIM does not write in the profile of a user of a SCIM-managed
// auth provider: the profile picture, as often as for any other user, and the display name of a user whom SCIM has
// not provisioned (bound is false) and who has none. identity is the user's identity as stored. Nothing is written,
// and no lock is taken, when there is nothing to refresh.
func (c *Client) refreshSCIMManagedProfile(ctx context.Context, user *types.User, identity types.Identity, bound bool, authProviderName, authProviderURL, accessToken string) error {
	if err := c.decryptStoredIdentity(ctx, &identity); err != nil {
		return err
	}
	fillName := !bound && user.DisplayName == ""
	iconDue := hasProfilePictures(authProviderName) &&
		(user.IconURL != identity.IconURL || time.Since(identity.IconLastChecked) >= profileIconCheckInterval)
	if !fillName && !iconDue {
		return nil
	}

	profile, err := c.fetchUserProfile(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}
	iconURL, hasIcon := profileIconURL(authProviderName, profile)
	displayName, _ := profile["name"].(string)
	displayName = strings.TrimSpace(displayName)
	if !iconDue && displayName == "" {
		return nil
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// SCIM may have provisioned the user, or another sign-in may have filled in the display name, while the
		// profile was being fetched. The SCIM write lock keeps SCIM from provisioning the user between these checks
		// and the write; the user lock alone does not, because SCIM binds a user without locking it when the profile
		// carries neither an email nor a display name. The locks are taken in the order SCIM takes them, also when
		// only the picture is written, so that this write cannot deadlock with SCIM's writes of the user and identity.
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}
		current := new(types.User)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).Take(current).Error; err != nil {
			return err
		}
		// The identity is read again too, so that only its picture and the time it was checked change.
		var identities []types.Identity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID).
			Limit(1).
			Find(&identities).Error; err != nil {
			return fmt.Errorf("failed to get identity: %w", err)
		}
		// The user was deleted, or the identity moved to another user, while the profile was being fetched, so the
		// answer is no longer this user's.
		if current.DeletedAt != nil || len(identities) == 0 || identities[0].UserID != user.ID {
			return nil
		}
		if err := c.decryptStoredUser(ctx, current); err != nil {
			return err
		}

		if fillName && displayName != "" && current.DisplayName == "" {
			binding, err := activeSCIMUserBindingForUserTx(tx, user.ID, false)
			if err != nil {
				return err
			}
			if binding == nil {
				current.DisplayName = displayName
			}
		}
		if iconDue && hasIcon {
			current.IconURL = iconURL
		}
		name, icon := current.DisplayName, current.IconURL
		if err := c.encryptUser(ctx, current); err != nil {
			return fmt.Errorf("failed to encrypt user: %w", err)
		}
		// Every encrypted column is written, so that the row stays consistent with its encrypted flag, and so is a
		// picture the provider no longer has.
		if err := tx.Model(current).Select(userEncryptedColumns).Updates(current).Error; err != nil {
			return err
		}

		if iconDue {
			stored := &identities[0]
			if err := c.decryptStoredIdentity(ctx, stored); err != nil {
				return err
			}
			stored.IconURL = icon
			if err := c.encryptIdentity(ctx, stored); err != nil {
				return fmt.Errorf("failed to encrypt identity: %w", err)
			}
			columns := identityEncryptedColumns(stored)
			columns["icon_last_checked"] = time.Now()
			if err := tx.Model(new(types.Identity)).
				Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", stored.AuthProviderNamespace, stored.AuthProviderName, stored.HashedProviderUserID).
				UpdateColumns(columns).Error; err != nil {
				return fmt.Errorf("failed to store the profile picture of identity: %w", err)
			}
		}

		user.DisplayName, user.IconURL = name, icon
		return nil
	})
}

// EncryptUsers will pull all users out of the database and ensure they are encrypted.
func (c *Client) EncryptUsers(ctx context.Context, force bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var users []types.User
		if err := tx.Find(&users).Error; err != nil {
			return err
		}

		for i := range users {
			if !force && users[i].Encrypted {
				continue
			}

			if err := c.decryptUser(ctx, &users[i]); err != nil {
				return fmt.Errorf("failed to decrypt user: %w", err)
			}

			if err := c.encryptUser(ctx, &users[i]); err != nil {
				return fmt.Errorf("failed to encrypt user: %w", err)
			}

			if err := tx.Omit(types.UserLifecycleColumns...).Updates(users[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (c *Client) fetchUserProfile(ctx context.Context, authProviderURL, accessToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authProviderURL+"/obot-get-user-info", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch user profile: %s", resp.Status)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return body, nil
}

func (c *Client) encryptUser(ctx context.Context, user *types.User) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = userDataCtx(user)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(user.Username), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.Username = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(user.Email), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.Email = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(user.IconURL), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.IconURL = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(user.DisplayName), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.DisplayName = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(user.OriginalEmail), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.OriginalEmail = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(user.OriginalUsername), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		user.OriginalUsername = base64.StdEncoding.EncodeToString(b)
	}

	user.Encrypted = true

	return errors.Join(errs...)
}

// decryptStoredUser decrypts a user read from the database, refusing one that is encrypted while encryption is not
// configured, which decryptUser would leave encrypted.
func (c *Client) decryptStoredUser(ctx context.Context, user *types.User) error {
	if user.Encrypted && (c.encryptionConfig == nil || c.encryptionConfig.Transformers[userGroupResource] == nil) {
		return errors.New("user is encrypted but encryption is not configured")
	}
	if err := c.decryptUser(ctx, user); err != nil {
		return fmt.Errorf("failed to decrypt user: %w", err)
	}
	return nil
}

func (c *Client) decryptUser(ctx context.Context, user *types.User) error {
	if !user.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = userDataCtx(user)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.Username)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.Username))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.Username = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.Email)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.Email))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.Email = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.IconURL)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.IconURL))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.IconURL = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.DisplayName)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.DisplayName))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.DisplayName = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.OriginalEmail)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.OriginalEmail))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.OriginalEmail = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(user.OriginalUsername)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(user.OriginalUsername))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			user.OriginalUsername = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func userDataCtx(user *types.User) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", userGroupResource.String(), user.HashedUsername))
}
