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
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	userGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "users",
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
	if err != nil {
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

// UsersIncludeDeleted returns all users including soft-deleted ones (for audit purposes)
func (c *Client) UsersIncludeDeleted(ctx context.Context, query types.UserQuery) ([]types.User, error) {
	query.IncludeDeleted = true
	return c.Users(ctx, query)
}

func (c *Client) DeleteUser(ctx context.Context, userID string) (*types.User, error) {
	var (
		existingUser = new(types.User)
		responseUser = types.User{}
	)

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).First(existingUser).Error; err != nil {
			return err
		}

		// Decrypt user to get original values before soft delete
		if err := c.decryptUser(ctx, existingUser); err != nil {
			return fmt.Errorf("failed to decrypt user: %w", err)
		}

		// Check if we're deleting a user with Owner or Admin role
		// We filter out empty email users here, because that is the bootstrap user.
		// Also exclude already soft-deleted users and the current user from the count.
		if existingUser.Role.HasRole(types2.RoleOwner) {
			// Can't delete the user if they're an owner and there are no other owners
			var ownerCount int64
			if err := tx.Model(new(types.User)).Where("id != ? AND role IN ? AND hashed_email != '' AND deleted_at IS NULL",
				userID,
				[]types2.Role{types2.RoleOwner, types2.RoleOwner | types2.RoleAuditor},
			).Count(&ownerCount).Error; err != nil {
				return err
			}

			if ownerCount == 0 {
				return new(LastOwnerError)
			}
		} else if existingUser.Role.HasRole(types2.RoleAdmin) {
			// Can't delete the user if they're an admin and there are no other owners or admins
			var adminCount int64
			if err := tx.Model(new(types.User)).Where("id != ? AND role IN ? AND hashed_email != '' AND deleted_at IS NULL",
				userID,
				[]types2.Role{types2.RoleOwner, types2.RoleAdmin, types2.RoleOwner | types2.RoleAuditor, types2.RoleAdmin | types2.RoleAuditor},
			).Count(&adminCount).Error; err != nil {
				return err
			}

			if adminCount == 0 {
				return new(LastAdminError)
			}
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

		// Copy the existing user before we encrypt it so that we can return the right values in the response.
		responseUser = *existingUser
		responseUser.Email = responseUser.OriginalEmail
		responseUser.Username = responseUser.OriginalUsername

		// Encrypt the modified user
		if err := c.encryptUser(ctx, existingUser); err != nil {
			return fmt.Errorf("failed to encrypt user: %w", err)
		}

		// Clean up group memberships for the deleted user
		if err := c.deleteGroupMembershipsForUser(ctx, tx, existingUser.ID); err != nil {
			return err
		}

		// Update the user with soft delete fields and modified email/username
		if err := tx.Save(existingUser).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &responseUser, nil
}

func (c *Client) UpdateUser(ctx context.Context, actingUserCanChangeRole bool, updatedUser *types.User, userID string) (*types.User, error) {
	existingUser := new(types.User)
	return existingUser, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).First(existingUser).Error; err != nil {
			return err
		}

		if err := c.decryptUser(ctx, existingUser); err != nil {
			return fmt.Errorf("failed to decrypt user: %w", err)
		}

		// If the username is being changed, then ensure that a user with that name doesn't already exist.
		if len(updatedUser.Username) != 0 && updatedUser.Username != existingUser.Username {
			if err := tx.Model(updatedUser).Where("username = ? AND deleted_at IS NULL", updatedUser.Username).First(new(types.User)).Error; err == nil {
				return &AlreadyExistsError{name: fmt.Sprintf("user with username %q", updatedUser.Username)}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			existingUser.Username = updatedUser.Username
		}

		// Anyone can update their timezone
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
					// If this user has been explicitly marked as an admin or owner, then don't allow changing the role.
					if c.HasExplicitRole(existingUser.Email) != types2.RoleUnknown {
						return &ExplicitRoleError{email: existingUser.Email}
					}

					// We filter out empty email users here, because that is the bootstrap user.
					// Also exclude soft-deleted users and the current user from the count.
					if removingOwner {
						// Can't change role from owner if there are no other owners
						var ownerCount int64
						if err := tx.Model(new(types.User)).Where("id != ? AND role IN ? AND hashed_email != '' AND deleted_at IS NULL",
							userID,
							[]types2.Role{types2.RoleOwner, types2.RoleOwner | types2.RoleAuditor},
						).Count(&ownerCount).Error; err != nil {
							return err
						}

						if ownerCount == 0 {
							return new(LastOwnerError)
						}
					} else if removingAdmin {
						// Can't change role from admin if there are no other owners or admins
						var adminCount int64
						if err := tx.Model(new(types.User)).Where("id != ? AND role IN ? AND hashed_email != '' AND deleted_at IS NULL",
							userID,
							[]types2.Role{types2.RoleOwner, types2.RoleAdmin, types2.RoleOwner | types2.RoleAuditor, types2.RoleAdmin | types2.RoleAuditor},
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

		return tx.Updates(&u).Error
	})
}

func (c *Client) UpdateUserInternalStatus(ctx context.Context, userID string, internal bool) error {
	return c.db.WithContext(ctx).Model(new(types.User)).Where("id = ? AND deleted_at IS NULL", userID).Update("internal", internal).Error
}

func (c *Client) UpdateProfileIfNeeded(ctx context.Context, user *types.User, authProviderName, authProviderNamespace, authProviderURL string) error {
	if authProviderName == "" || authProviderNamespace == "" || authProviderURL == "" {
		return nil
	}

	accessToken := accesstoken.GetAccessToken(ctx)
	if accessToken == "" {
		return nil
	}

	var identity types.Identity
	if err := c.db.WithContext(ctx).Where("user_id = ?", user.ID).
		Where("auth_provider_name = ?", authProviderName).
		Where("auth_provider_namespace = ?", authProviderNamespace).
		First(&identity).Error; err != nil {
		return err
	}

	if err := c.decryptIdentity(ctx, &identity); err != nil {
		return fmt.Errorf("failed to decrypt identity: %w", err)
	}

	if user.IconURL == identity.IconURL && time.Since(identity.IconLastChecked) < 7*24*time.Hour && user.DisplayName != "" {
		// Icon was checked less than 7 days ago, and the user is still using the same auth provider and DisplayName is already set
		return nil
	}

	profile, err := c.fetchUserProfile(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}

	switch authProviderName {
	case "github-auth-provider":
		if iconURL, ok := profile["avatar_url"].(string); ok {
			user.IconURL = iconURL
			identity.IconURL = iconURL
		}
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
			if user.DisplayName == "" {
				if login, ok := profile["login"].(string); ok {
					user.DisplayName = login
				}
			}
		}
	case "google-auth-provider":
		if iconURL, ok := profile["picture"].(string); ok {
			user.IconURL = iconURL
			identity.IconURL = iconURL
		}
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
		}
	case "entra-auth-provider":
		if iconURL, ok := profile["icon_url"].(string); ok {
			user.IconURL = iconURL
			identity.IconURL = iconURL
		}
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
		}
	case "okta-auth-provider":
		// Okta does not support profile pictures
		if displayName, ok := profile["name"].(string); ok {
			user.DisplayName = displayName
		}
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
		if err := tx.Updates(u).Error; err != nil {
			return err
		}
		return tx.Updates(&identity).Error
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

			if err := tx.Updates(users[i]).Error; err != nil {
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
		return nil, fmt.Errorf("failed to fetch profile icon URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch profile icon URL: %s", resp.Status)
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

// UserInfoByID returns a user.Info object for the given user ID,
// suitable for use with ACR helper methods. This fetches the user
// and their group memberships from the database.
func (c *Client) UserInfoByID(ctx context.Context, userID uint) (kuser.Info, error) {
	u, groupIDs, err := c.getUserAndGroupIDs(ctx, userID, "", "")
	if err != nil {
		return nil, err
	}

	return &kuser.DefaultInfo{
		Name:   u.Username,
		UID:    fmt.Sprintf("%d", u.ID),
		Groups: u.Role.Groups(),
		Extra: map[string][]string{
			"auth_provider_groups": groupIDs,
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

func userDataCtx(user *types.User) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", userGroupResource.String(), user.HashedUsername))
}
