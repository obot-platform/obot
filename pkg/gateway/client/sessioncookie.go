package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var sessionCookieGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "sessioncookies",
}

// GetSessionCookie finds a session cookie by hashed session ID.
func (c *Client) GetSessionCookie(ctx context.Context, hashedSessionID string, authProviderNamespace string, authProviderName string) (*types.SessionCookie, error) {
	var sessionCookie types.SessionCookie
	if err := c.db.WithContext(ctx).Where(&types.SessionCookie{
		HashedSessionID:       hashedSessionID,
		AuthProviderNamespace: authProviderNamespace,
		AuthProviderName:      authProviderName,
	}).First(&sessionCookie).Error; err != nil {
		return nil, fmt.Errorf("failed to get session cookie: %w", err)
	}

	if err := c.decryptSessionCookie(ctx, &sessionCookie); err != nil {
		return nil, fmt.Errorf("failed to decrypt session cookie: %w", err)
	}

	return &sessionCookie, nil
}

// EnsureSessionCookie creates a new session cookie when one doesn't exist.
func (c *Client) EnsureSessionCookie(ctx context.Context, sessionCookie types.SessionCookie) error {
	if err := c.encryptSessionCookie(ctx, &sessionCookie); err != nil {
		return fmt.Errorf("failed to encrypt session cookie: %w", err)
	}

	if err := c.db.WithContext(ctx).Save(&sessionCookie).Error; err != nil {
		return fmt.Errorf("failed to create session cookie: %w", err)
	}

	return nil
}

// DeleteSessionCookie deletes a session cookie by hashed session ID along with any dangling refresh tokens.
func (c *Client) DeleteSessionCookie(ctx context.Context, hashedSessionID string, authProviderNamespace string, authProviderName string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where(&types.SessionCookie{
				HashedSessionID:       hashedSessionID,
				AuthProviderNamespace: authProviderNamespace,
				AuthProviderName:      authProviderName,
			}).
			Delete(new(types.SessionCookie)).Error; err != nil {
			return err
		}

		// Delete dangling refresh tokens
		if err := tx.
			Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_session_id = ?", authProviderNamespace, authProviderName, hashedSessionID).
			Delete(new(types.AuthToken)).Error; err != nil {
			return err
		}

		return nil
	})
}

func (c *Client) encryptSessionCookie(ctx context.Context, sessionCookie *types.SessionCookie) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[sessionCookieGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = sessionCookieDataCtx(sessionCookie)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(sessionCookie.Cookie), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		sessionCookie.Cookie = base64.StdEncoding.EncodeToString(b)
	}

	sessionCookie.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptSessionCookie(ctx context.Context, sessionCookie *types.SessionCookie) error {
	if !sessionCookie.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[sessionCookieGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = sessionCookieDataCtx(sessionCookie)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(sessionCookie.Cookie)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(sessionCookie.Cookie))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			sessionCookie.Cookie = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	if len(errs) < 1 {
		sessionCookie.Encrypted = false
	}

	return errors.Join(errs...)
}

func sessionCookieDataCtx(sessionCookie *types.SessionCookie) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s/%s", sessionCookieGroupResource.String(), sessionCookie.AuthProviderNamespace, sessionCookie.AuthProviderName, sessionCookie.HashedSessionID))
}
