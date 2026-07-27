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
func (c *Client) CreateLocalAuthUser(ctx context.Context, email, passwordHash string) (*types.LocalAuthUser, error) {
	email = NormalizeEmail(email)
	user := types.LocalAuthUser{
		Email:        email,
		HashedEmail:  hash.String(email),
		PasswordHash: passwordHash,
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

// SetLocalAuthUserPassword updates a user's password hash and invalidates all of their sessions,
// so that a password reset actually kicks the old sessions out.
func (c *Client) SetLocalAuthUserPassword(ctx context.Context, id uint, passwordHash string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(new(types.LocalAuthUser)).Where("id = ?", id).Update("password_hash", passwordHash)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return tx.Where("user_id = ?", id).Delete(new(types.LocalAuthSession)).Error
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
