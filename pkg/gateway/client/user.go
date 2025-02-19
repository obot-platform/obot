package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) Users(ctx context.Context, query types.UserQuery) ([]types.User, error) {
	var users []types.User
	return users, c.db.WithContext(ctx).Scopes(query.Scope).Find(&users).Error
}

func (c *Client) User(ctx context.Context, username string) (*types.User, error) {
	u := new(types.User)
	return u, c.db.WithContext(ctx).Where("username = ?", username).First(u).Error
}

func (c *Client) UserByID(ctx context.Context, id string) (*types.User, error) {
	u := new(types.User)
	return u, c.db.WithContext(ctx).Where("id = ?", id).First(u).Error
}

func (c *Client) DeleteUser(ctx context.Context, username string) (*types.User, error) {
	existingUser := new(types.User)
	return existingUser, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ?", username).First(existingUser).Error; err != nil {
			return err
		}

		if existingUser.Role.HasRole(types2.RoleAdmin) {
			var adminCount int64
			// We filter out empty email users here, because that is the bootstrap user.
			if err := tx.Model(new(types.User)).Where("role = ? and email != ''", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
				return err
			}

			if adminCount <= 1 {
				return new(LastAdminError)
			}
		}

		if err := tx.Where("user_id = ?", existingUser.ID).Delete(new(types.Identity)).Error; err != nil {
			return err
		}

		return tx.Delete(existingUser).Error
	})
}

func (c *Client) UpdateUser(ctx context.Context, actingUserIsAdmin bool, updatedUser *types.User, username string) (*types.User, error) {
	existingUser := new(types.User)
	return existingUser, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ?", username).First(existingUser).Error; err != nil {
			return err
		}

		// If the username is being changed, then ensure that a user with that name doesn't already exist.
		if updatedUser.Username != "" && updatedUser.Username != username {
			if err := tx.Model(updatedUser).Where("username = ?", updatedUser.Username).First(new(types.User)).Error; err == nil {
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
		if actingUserIsAdmin {
			if updatedUser.Role > 0 && existingUser.Role.HasRole(types2.RoleAdmin) && !updatedUser.Role.HasRole(types2.RoleAdmin) {
				// If this user has been explicitly marked as an admin, then don't allow changing the role.
				if c.IsExplicitAdmin(existingUser.Email) {
					return &ExplicitAdminError{email: existingUser.Email}
				}
				// If the role is being changed from admin to non-admin, then ensure that this isn't the last admin.
				// We filter out empty email users here, because that is the bootstrap user.
				var adminCount int64
				if err := tx.Model(new(types.User)).Where("role = ? and email != ''", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
					return err
				}

				if adminCount <= 1 {
					return new(LastAdminError)
				}
			}

			existingUser.Role = updatedUser.Role
		}

		return tx.Updates(existingUser).Error
	})
}

func (c *Client) UpdateProfileIconIfNeeded(ctx context.Context, user *types.User, authProviderName, authProviderNamespace, authProviderURL string) error {
	if authProviderName == "" || authProviderNamespace == "" || authProviderURL == "" {
		return nil
	}

	accessToken := accesstoken.GetAccessToken(ctx)
	if accessToken == "" {
		return nil
	}

	var (
		identity types.Identity
	)
	if err := c.db.WithContext(ctx).Where("user_id = ?", user.ID).
		Where("auth_provider_name = ?", authProviderName).
		Where("auth_provider_namespace = ?", authProviderNamespace).
		First(&identity).Error; err != nil {
		return err
	}

	if user.IconURL == identity.IconURL && time.Until(identity.IconLastChecked) > -7*24*time.Hour {
		// Icon was checked less than 7 days ago.
		return nil
	}

	profileIconURL, err := c.fetchProfileIconURL(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}

	user.IconURL = profileIconURL
	identity.IconURL = profileIconURL
	identity.IconLastChecked = time.Now()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Updates(user).Error; err != nil {
			return err
		}
		return tx.Updates(&identity).Error
	})
}

func (c *Client) fetchProfileIconURL(ctx context.Context, authProviderURL, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authProviderURL+"/obot-get-icon-url", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch profile icon URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch profile icon URL: %s", resp.Status)
	}

	var body struct {
		IconURL string `json:"iconURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return body.IconURL, nil
}
