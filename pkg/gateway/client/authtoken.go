package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

const (
	randomTokenLength = 32
	tokenIDLength     = 8
	expirationDur     = 7 * 24 * time.Hour
)

func (c *Client) newAuthToken(ctx context.Context, authProviderNamespace, authProviderName string, userID uint, expiresIn time.Duration, tr *types.TokenRequest) (*types.AuthToken, string, error) {
	randBytes := make([]byte, tokenIDLength+randomTokenLength)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, "", fmt.Errorf("could not generate token id: %w", err)
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]

	tkn := &types.AuthToken{
		ID: fmt.Sprintf("%x", id),
		// Hash the token again for long-term storage
		HashedToken:           hash.String(fmt.Sprintf("%x", token)),
		ExpiresAt:             time.Now().Add(expiresIn),
		AuthProviderNamespace: authProviderNamespace,
		AuthProviderName:      authProviderName,
	}

	t := publicToken(id, token)
	return tkn, t, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tr != nil {
			tr.Token = t
			tr.ExpiresAt = tkn.ExpiresAt

			if err := tx.Updates(tr).Error; err != nil {
				return err
			}
		}

		tkn.UserID = userID

		return tx.Create(tkn).Error
	})
}

func (c *Client) NewAuthToken(ctx context.Context, authProviderNamespace, authProviderName string, userID uint, tr *types.TokenRequest) (*types.AuthToken, error) {
	tkn, _, err := c.newAuthToken(ctx, authProviderNamespace, authProviderName, userID, expirationDur, tr)
	return tkn, err
}

func (c *Client) NewAuthTokenWithExpiration(ctx context.Context, authProviderNamespace, authProviderName string, userID uint, expiresIn time.Duration) (*types.AuthToken, string, error) {
	return c.newAuthToken(ctx, authProviderNamespace, authProviderName, userID, expiresIn, nil)
}

func publicToken(id, token []byte) string {
	return fmt.Sprintf("%x:%x", id, token)
}
