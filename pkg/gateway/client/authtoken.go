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

func (c *Client) NewAuthToken(
	ctx context.Context,
	authProviderNamespace,
	authProviderName string,
	authProviderUserID string,
	userID uint,
	tr *types.TokenRequest,
) (*types.AuthToken, error) {
	randBytes := make([]byte, tokenIDLength+randomTokenLength)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("could not generate token id: %w", err)
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]

	tkn := &types.AuthToken{
		ID: fmt.Sprintf("%x", id),
		// Hash the token again for long-term storage
		HashedToken:           hash.String(fmt.Sprintf("%x", token)),
		NoExpiration:          tr.NoExpiration,
		ExpiresAt:             time.Now().Add(expirationDur),
		AuthProviderNamespace: authProviderNamespace,
		AuthProviderName:      authProviderName,
		AuthProviderUserID:    authProviderUserID,
	}
	if tkn.NoExpiration {
		tkn.ExpiresAt = time.Time{}
	}

	return tkn, c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tr != nil {
			tr.Token = publicToken(id, token)
			tr.ExpiresAt = tkn.ExpiresAt

			if err := tx.Updates(tr).Error; err != nil {
				return err
			}
		}

		tkn.UserID = userID

		return tx.Create(tkn).Error
	})
}

func publicToken(id, token []byte) string {
	return fmt.Sprintf("%x:%x", id, token)
}
