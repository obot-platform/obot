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
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	userGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "users",
	}
)

func (c *Client) UserFromToken(ctx context.Context, token string) (*types.User, string, string, error) {
	id, token, _ := strings.Cut(token, ":")
	u := new(types.User)
	var namespace, name string
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tkn := new(types.AuthToken)
		if err := tx.Where("id = ? AND hashed_token = ?", id, hash.String(token)).First(tkn).Error; err != nil {
			return err
		}

		namespace = tkn.AuthProviderNamespace
		name = tkn.AuthProviderName
		return tx.Where("id = ?", tkn.UserID).First(u).Error
	}); err != nil {
		return nil, "", "", err
	}

	return u, namespace, name, c.decryptUser(ctx, u)
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
	if err := c.db.WithContext(ctx).Where("hashed_username = ?", hash.String(username)).First(u).Error; err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

func (c *Client) UserByID(ctx context.Context, id string) (*types.User, error) {
	u := new(types.User)
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(u).Error; err != nil {
		return nil, err
	}

	return u, c.decryptUser(ctx, u)
}

func (c *Client) DeleteUser(ctx context.Context, storageClient kclient.Client, userID string) (*types.User, error) {
	existingUser := new(types.User)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userID).First(existingUser).Error; err != nil {
			return err
		}

		if existingUser.Role.HasRole(types2.RoleAdmin) {
			var adminCount int64
			// We filter out empty email users here, because that is the bootstrap user.
			if err := tx.Model(new(types.User)).Where("role = ? and hashed_email != ''", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
				return err
			}

			if adminCount <= 1 {
				return new(LastAdminError)
			}
		}

		var identities []types.Identity
		if err := tx.Where("user_id = ?", existingUser.ID).Find(&identities).Error; err != nil {
			return err
		}

		for i, id := range identities {
			if err := c.decryptIdentity(ctx, &id); err != nil {
				return err
			}
			identities[i] = id
		}

		if err := c.deleteSessionsForUser(ctx, tx, storageClient, identities, ""); err != nil && !errors.Is(err, LogoutAllErr{}) {
			return err
		}

		if err := tx.Where("user_id = ?", existingUser.ID).Delete(new(types.Identity)).Error; err != nil {
			return err
		}

		return tx.Delete(existingUser).Error
	}); err != nil {
		return nil, err
	}

	return existingUser, c.decryptUser(ctx, existingUser)
}

func (c *Client) UpdateUser(ctx context.Context, actingUserIsAdmin bool, updatedUser *types.User, userID string) (*types.User, error) {
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
				if err := tx.Model(new(types.User)).Where("role = ? and hashed_email != ''", types2.RoleAdmin).Count(&adminCount).Error; err != nil {
					return err
				}

				if adminCount <= 1 {
					return new(LastAdminError)
				}
			}

			existingUser.Role = updatedUser.Role
		}

		// Copy the user object that is returned to the caller so they don't get the encrypted values
		u := existingUser
		if err := c.encryptUser(ctx, u); err != nil {
			return fmt.Errorf("failed to encrypt user: %w", err)
		}

		return tx.Updates(u).Error
	})
}

func (c *Client) UpdateUserInternalStatus(ctx context.Context, userID string, internal bool) error {
	return c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", userID).Update("internal", internal).Error
}

func (c *Client) UpdateProfileIconIfNeeded(ctx context.Context, user *types.User, authProviderName, authProviderNamespace, authProviderURL string) error {
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

	if user.IconURL == identity.IconURL && time.Until(identity.IconLastChecked) > -7*24*time.Hour {
		// Icon was checked less than 7 days ago, and the user is still using the same auth provider.
		return nil
	}

	profileIconURL, err := c.fetchProfileIconURL(ctx, authProviderURL, accessToken)
	if err != nil {
		return err
	}

	user.IconURL = profileIconURL
	identity.IconURL = profileIconURL
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

	return errors.Join(errs...)
}

func userDataCtx(user *types.User) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", userGroupResource.String(), user.HashedUsername))
}
