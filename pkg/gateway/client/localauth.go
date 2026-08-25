package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/storage/value"
)

// ErrLocalAuthUserExists is returned when creating a local auth user whose email is already taken.
var ErrLocalAuthUserExists = errors.New("local auth user already exists")

// NormalizeEmail lowercases and trims an email address so that logins are case-insensitive.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (c *Client) LocalAuthUsers(ctx context.Context) ([]types.LocalAuthUser, error) {
	var users []types.LocalAuthUser
	if err := c.db.WithContext(ctx).Order("created_at").Find(&users).Error; err != nil {
		return nil, err
	}

	for i := range users {
		if err := c.decryptLocalAuthUser(ctx, &users[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt local auth user: %w", err)
		}
	}

	return users, nil
}

func (c *Client) LocalAuthUserByEmail(ctx context.Context, email string) (*types.LocalAuthUser, error) {
	var user types.LocalAuthUser
	if err := c.db.WithContext(ctx).Where("hashed_email = ?", hash.String(NormalizeEmail(email))).First(&user).Error; err != nil {
		return nil, err
	}

	if err := c.decryptLocalAuthUser(ctx, &user); err != nil {
		return nil, fmt.Errorf("failed to decrypt local auth user: %w", err)
	}

	return &user, nil
}

func (c *Client) LocalAuthUserByID(ctx context.Context, id uint) (*types.LocalAuthUser, error) {
	var user types.LocalAuthUser
	if err := c.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}

	if err := c.decryptLocalAuthUser(ctx, &user); err != nil {
		return nil, fmt.Errorf("failed to decrypt local auth user: %w", err)
	}

	return &user, nil
}

// CreateLocalAuthUser creates a new local auth user. The password must already be hashed.
func (c *Client) CreateLocalAuthUser(ctx context.Context, email, passwordHash string, requirePasswordChange bool) (*types.LocalAuthUser, error) {
	return c.createLocalAuthUser(ctx, email, passwordHash, requirePasswordChange, "", nil)
}

// CreateInitialLocalAuthUser creates a user that can only be claimed with the setup capability.
// Only the token's hash is persisted, and password completion revokes it.
func (c *Client) CreateInitialLocalAuthUser(ctx context.Context, email, passwordHash, setupTokenHash string, setupTokenExpiresAt time.Time) (*types.LocalAuthUser, error) {
	return c.createLocalAuthUser(ctx, email, passwordHash, true, setupTokenHash, &setupTokenExpiresAt)
}

func (c *Client) createLocalAuthUser(ctx context.Context, email, passwordHash string, requirePasswordChange bool, setupTokenHash string, setupTokenExpiresAt *time.Time) (*types.LocalAuthUser, error) {
	email = NormalizeEmail(email)
	user := types.LocalAuthUser{
		Email:                 email,
		HashedEmail:           hash.String(email),
		PasswordHash:          passwordHash,
		RequirePasswordChange: requirePasswordChange,
		SetupTokenHash:        setupTokenHash,
		SetupTokenExpiresAt:   setupTokenExpiresAt,
	}

	if err := c.encryptLocalAuthUser(ctx, &user); err != nil {
		return nil, fmt.Errorf("failed to encrypt local auth user: %w", err)
	}

	if err := c.db.WithContext(ctx).Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrLocalAuthUserExists
		}
		return nil, err
	}

	user.Email = email
	user.Encrypted = false
	return &user, nil
}

// ActivateLocalAuthUser validates a setup token and creates a restricted session. The setup token
// remains usable until the user successfully sets their password, so an interrupted setup can be
// resumed from the original link.
func (c *Client) ActivateLocalAuthUser(ctx context.Context, setupTokenHash, sessionID string, expiresAt time.Time) (*types.LocalAuthUser, error) {
	var user types.LocalAuthUser
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("setup_token_hash = ? AND setup_token_expires_at > ? AND require_password_change = ?", setupTokenHash, time.Now(), true).First(&user).Error; err != nil {
			return err
		}
		return tx.Create(&types.LocalAuthSession{
			ID:        sessionID,
			UserID:    user.ID,
			ExpiresAt: expiresAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}

	if err := c.decryptLocalAuthUser(ctx, &user); err != nil {
		return nil, fmt.Errorf("failed to decrypt local auth user: %w", err)
	}
	return &user, nil
}

// RefreshLocalAuthUserSetupToken rotates an initial account's still-pending setup capability and
// invalidates any sessions created with the old capability. Completed accounts cannot be rearmed.
func (c *Client) RefreshLocalAuthUserSetupToken(ctx context.Context, id uint, setupTokenHash string, setupTokenExpiresAt time.Time) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(new(types.LocalAuthUser)).Where("id = ? AND setup_token_hash != '' AND require_password_change = ?", id, true).Updates(map[string]any{
			"setup_token_hash":       setupTokenHash,
			"setup_token_expires_at": setupTokenExpiresAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("user_id = ?", id).Delete(new(types.LocalAuthSession)).Error
	})
}

// SetLocalAuthUserPassword updates a user's password hash and invalidates all of their sessions,
// so that a password reset actually kicks the old sessions out.
func (c *Client) SetLocalAuthUserPassword(ctx context.Context, id uint, passwordHash string, requirePasswordChange bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"password_hash":           passwordHash,
			"require_password_change": requirePasswordChange,
			// Any password write supersedes an outstanding setup capability. In particular, an
			// administrator reset must disarm an emailed activation link even when the new password
			// itself is marked for change at next login.
			"setup_token_hash":       "",
			"setup_token_expires_at": nil,
		}
		result := tx.Model(new(types.LocalAuthUser)).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return tx.Where("user_id = ?", id).Delete(new(types.LocalAuthSession)).Error
	})
}

// CompleteLocalAuthUserPasswordChange atomically completes a required password change. The
// current session must still exist and the user must still be pending, so only the first of
// multiple activated sessions can choose the authoritative password.
func (c *Client) CompleteLocalAuthUserPasswordChange(ctx context.Context, id uint, passwordHash, currentSessionID string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sessionCount int64
		if err := tx.Model(new(types.LocalAuthSession)).Where("id = ? AND user_id = ? AND expires_at > ?", currentSessionID, id, time.Now()).Count(&sessionCount).Error; err != nil {
			return err
		}
		if sessionCount != 1 {
			return gorm.ErrRecordNotFound
		}

		result := tx.Model(new(types.LocalAuthUser)).Where("id = ? AND require_password_change = ?", id, true).Updates(map[string]any{
			"password_hash":           passwordHash,
			"require_password_change": false,
			"setup_token_hash":        "",
			"setup_token_expires_at":  nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}

		return tx.Where("user_id = ? AND id != ?", id, currentSessionID).Delete(new(types.LocalAuthSession)).Error
	})
}

func (c *Client) DeleteLocalAuthUser(ctx context.Context, id uint) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(new(types.LocalAuthUser))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return tx.Where("user_id = ?", id).Delete(new(types.LocalAuthSession)).Error
	})
}

// CreateLocalAuthSession records a session for the given user. The ID must be a hash of the
// token that is handed to the browser, never the token itself.
func (c *Client) CreateLocalAuthSession(ctx context.Context, id string, userID uint, expiresAt time.Time) error {
	return c.db.WithContext(ctx).Create(&types.LocalAuthSession{
		ID:        id,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}).Error
}

// LocalAuthSession returns the unexpired session with the given ID, along with its user.
// Expired sessions are treated as missing and deleted.
func (c *Client) LocalAuthSession(ctx context.Context, id string) (*types.LocalAuthSession, *types.LocalAuthUser, error) {
	var session types.LocalAuthSession
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&session).Error; err != nil {
		return nil, nil, err
	}

	if !session.ExpiresAt.After(time.Now()) {
		_ = c.DeleteLocalAuthSession(ctx, id)
		return nil, nil, gorm.ErrRecordNotFound
	}

	var user types.LocalAuthUser
	if err := c.db.WithContext(ctx).Where("id = ?", session.UserID).First(&user).Error; err != nil {
		return nil, nil, err
	}

	if err := c.decryptLocalAuthUser(ctx, &user); err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt local auth user: %w", err)
	}

	return &session, &user, nil
}

func (c *Client) DeleteLocalAuthSession(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Where("id = ?", id).Delete(new(types.LocalAuthSession)).Error
}

// DeleteLocalAuthSessionsForEmail signs the local user with the given email out of all sessions.
// If exceptSessionID is non-empty, that one session is kept (used to preserve the caller's own
// session when they log out everywhere else).
func (c *Client) DeleteLocalAuthSessionsForEmail(ctx context.Context, email, exceptSessionID string) error {
	var user types.LocalAuthUser
	if err := c.db.WithContext(ctx).Where("hashed_email = ?", hash.String(NormalizeEmail(email))).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	q := c.db.WithContext(ctx).Where("user_id = ?", user.ID)
	if exceptSessionID != "" {
		q = q.Where("id != ?", exceptSessionID)
	}

	return q.Delete(new(types.LocalAuthSession)).Error
}

// DeleteAllLocalAuthSessions signs every local user out. It is used when the provider is
// deconfigured, so that reconfiguring it later doesn't bring old sessions back to life.
func (c *Client) DeleteAllLocalAuthSessions(ctx context.Context) error {
	return c.db.WithContext(ctx).Where("1 = 1").Delete(new(types.LocalAuthSession)).Error
}

// DeleteExpiredLocalAuthSessions removes sessions that are past their expiration.
func (c *Client) DeleteExpiredLocalAuthSessions(ctx context.Context) error {
	return c.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(new(types.LocalAuthSession)).Error
}

func localAuthUserDataCtx(user *types.LocalAuthUser) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", userGroupResource.String(), user.HashedEmail))
}

// encryptLocalAuthUser encrypts the email address at rest, reusing the same transformer as the
// users table. The password hash is not encrypted: it is already a one-way hash.
func (c *Client) encryptLocalAuthUser(ctx context.Context, user *types.LocalAuthUser) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	b, err := transformer.TransformToStorage(ctx, []byte(user.Email), localAuthUserDataCtx(user))
	if err != nil {
		return err
	}

	user.Email = base64.StdEncoding.EncodeToString(b)
	user.Encrypted = true

	return nil
}

func (c *Client) decryptLocalAuthUser(ctx context.Context, user *types.LocalAuthUser) error {
	if !user.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(user.Email)
	if err != nil {
		return err
	}

	out, _, err := transformer.TransformFromStorage(ctx, decoded, localAuthUserDataCtx(user))
	if err != nil {
		return err
	}

	user.Email = string(out)
	user.Encrypted = false

	return nil
}
