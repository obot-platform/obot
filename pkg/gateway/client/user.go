package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/proxy"
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
			if err := tx.Model(new(types.User)).Where("role = ?", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
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
				var adminCount int64
				if err := tx.Model(new(types.User)).Where("role = ?", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
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

func (c *Client) UpdateProfileIconIfNeeded(ctx context.Context, user *types.User, authProviderID uint) error {
	if authProviderID == 0 {
		return nil
	}

	accessToken := proxy.GetAccessToken(ctx)
	if accessToken == "" {
		return nil
	}

	var (
		authProvider types.AuthProvider
		identity     types.Identity
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", authProviderID).First(&authProvider).Error; err != nil {
			return err
		}

		return tx.Where("user_id = ?", user.ID).Where("auth_provider_id = ?", authProviderID).First(&identity).Error
	}); err != nil {
		return err
	}

	if time.Until(identity.IconLastChecked) > -7*24*time.Hour {
		// Icon was checked less than 7 days ago.
		return nil
	}

	profileIconURL, err := c.fetchProfileIconURL(ctx, authProvider, user.Username, accessToken)
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

func (c *Client) fetchProfileIconURL(ctx context.Context, authProvider types.AuthProvider, username, accessToken string) (string, error) {
	switch authProvider.Type {
	case types.AuthTypeGoogle:
		return c.fetchGoogleProfileIconURL(ctx, accessToken)
	case types.AuthTypeGitHub:
		return c.fetchGitHubProfileIconURL(ctx, username)
	default:
		return "", fmt.Errorf("unsupported auth provider type for icon fetch: %s", authProvider.Type)
	}
}

type googleProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	HD            string `json:"hd"`
}

func (c *Client) fetchGoogleProfileIconURL(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v1/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var profile googleProfile
	if err = json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", err
	}

	return profile.Picture, nil
}

func (c *Client) fetchGitHubProfileIconURL(ctx context.Context, username string) (string, error) {
	// GitHub will automatically redirect this URL to the user's GitHub profile icon.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://github.com/%s.png", username), nil)
	if err != nil {
		return "", err
	}

	resp, err := (&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Don't follow redirects, tiny optimization to only make one request.
			return http.ErrUseLastResponse
		},
	}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Get the final URL that GitHub redirected to.
	u, err := resp.Location()
	if err != nil || u == nil {
		return "", err
	}

	return u.String(), nil
}
