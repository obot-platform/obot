package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	deviceCodeAlphabet      = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	deviceCodeLength        = 8
	deviceCodeLifetime      = 5 * time.Minute
	deviceCodeCreateRetries = 10
	tokenCleanupInterval    = 30 * time.Second
)

var (
	ErrInvalidOrExpiredDeviceCode = errors.New("invalid or expired device code")

	// oauthRoundTripPurposes are the token request purposes that drive a browser OAuth round
	// trip through /api/oauth/start and /api/oauth/redirect.
	oauthRoundTripPurposes = []string{
		types.TokenRequestPurposeSetup,
		types.TokenRequestPurposeAuthProviderVerify,
	}
)

// CreateTokenRequest creates a new token request in the database.
func (c *Client) CreateTokenRequest(ctx context.Context, tr *types.TokenRequest) error {
	return c.db.WithContext(ctx).Create(tr).Error
}

// CreateDeviceTokenRequest initializes and persists a device login request.
// Only the formatted device code is returned; the database stores its digest.
func (c *Client) CreateDeviceTokenRequest(ctx context.Context, tr *types.TokenRequest) (string, error) {
	for range deviceCodeCreateRetries {
		deviceCode, err := generateDeviceCode()
		if err != nil {
			return "", err
		}

		digest := digestNormalizedDeviceCode(normalizeDeviceCode(deviceCode))
		tr.ID = uuid.New().String()
		tr.Purpose = types.TokenRequestPurposeDeviceLogin
		tr.DeviceCodeDigest = &digest
		tr.RequestExpiresAt = time.Now().Add(deviceCodeLifetime)
		tr.DeviceCodeVerifiedAt = nil
		tr.State = ""
		tr.Token = ""
		tr.ExpiresAt = time.Time{}
		tr.CompletionRedirectURL = ""
		tr.Error = ""
		tr.TokenRetrieved = false

		if err := c.db.WithContext(ctx).Create(tr).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				continue
			}
			return "", fmt.Errorf("failed to create device token request: %w", err)
		}

		return deviceCode, nil
	}

	return "", fmt.Errorf("failed to create unique device code after %d attempts", deviceCodeCreateRetries)
}

// AuthorizeTokenRequestByDeviceCode consumes an unexpired device code and
// atomically creates and publishes an API key for the authenticated user.
func (c *Client) AuthorizeTokenRequestByDeviceCode(ctx context.Context, userID uint, code string) error {
	normalizedCode := normalizeDeviceCode(code)
	if !validNormalizedDeviceCode(normalizedCode) {
		return ErrInvalidOrExpiredDeviceCode
	}

	digest := digestNormalizedDeviceCode(normalizedCode)
	now := time.Now()
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tr := new(types.TokenRequest)
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("purpose = ? AND device_code_digest = ?", types.TokenRequestPurposeDeviceLogin, digest).
			First(tr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidOrExpiredDeviceCode
			}
			return fmt.Errorf("failed to load device token request: %w", err)
		}

		if tr.RequestExpiresAt.IsZero() || !now.Before(tr.RequestExpiresAt) || tr.DeviceCodeVerifiedAt != nil || tr.Token != "" {
			return ErrInvalidOrExpiredDeviceCode
		}

		claim := tx.Model(&types.TokenRequest{}).
			Where("id = ? AND purpose = ? AND device_code_verified_at IS NULL AND token = ? AND request_expires_at > ?", tr.ID, types.TokenRequestPurposeDeviceLogin, "", now).
			Update("device_code_verified_at", now)
		if claim.Error != nil {
			return fmt.Errorf("failed to consume device code: %w", claim.Error)
		}
		if claim.RowsAffected != 1 {
			return ErrInvalidOrExpiredDeviceCode
		}
		tr.DeviceCodeVerifiedAt = &now

		_, err := c.createAPIKeyFromTokenRequest(tx, userID, tr)
		return err
	})
}

// ListAuthTokens returns the auth tokens owned by a user.
func (c *Client) ListAuthTokens(ctx context.Context, userID uint) ([]types.AuthToken, error) {
	var tokens []types.AuthToken
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// DeleteAuthToken deletes an auth token owned by a user.
func (c *Client) DeleteAuthToken(ctx context.Context, userID uint, id string) error {
	return c.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).Delete(new(types.AuthToken)).Error
}

// AuthProviderVerificationActive reports whether this staged-provider verification is still open.
// It is possession-based because it guards the return leg of the OAuth round trip, where the caller
// is the replacement provider's identity rather than the owner who started the switch. Use
// AuthProviderVerificationStartableBy for the outbound leg.
func (c *Client) AuthProviderVerificationActive(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}

	var count int64
	if err := c.db.WithContext(ctx).Model(new(types.TokenRequest)).
		Where("id = ? AND purpose = ? AND request_expires_at > ?", id, types.TokenRequestPurposeAuthProviderVerify, time.Now()).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check for auth provider verification: %w", err)
	}
	return count > 0, nil
}

// AuthProviderVerificationStartableBy reports whether userID may open the login for this
// verification. A verification belongs to the owner who started the switch, so another signed-in
// user holding the ID must not be able to use it.
func (c *Client) AuthProviderVerificationStartableBy(ctx context.Context, id string, userID uint) (bool, error) {
	if id == "" || userID == 0 {
		return false, nil
	}

	var count int64
	if err := c.db.WithContext(ctx).Model(new(types.TokenRequest)).
		Where("id = ? AND purpose = ? AND request_expires_at > ? AND owner_user_id = ?",
			id, types.TokenRequestPurposeAuthProviderVerify, time.Now(), userID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check for auth provider verification: %w", err)
	}
	return count > 0, nil
}

// GetSetupTokenRequest returns a setup token request by ID.
func (c *Client) GetSetupTokenRequest(ctx context.Context, id string) (*types.TokenRequest, error) {
	tr := new(types.TokenRequest)
	if err := c.db.WithContext(ctx).Where("id = ? AND purpose = ?", id, types.TokenRequestPurposeSetup).First(tr).Error; err != nil {
		return nil, err
	}
	return tr, nil
}

// PollTokenRequest returns a token request and marks an available token as retrieved.
func (c *Client) PollTokenRequest(ctx context.Context, id string) (*types.TokenRequest, error) {
	tr := new(types.TokenRequest)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(tr).Error; err != nil {
			return err
		}
		if tr.Token != "" && !tr.TokenRetrieved {
			if err := tx.Model(tr).Where("id = ?", tr.ID).Update("token_retrieved", true).Error; err != nil {
				return err
			}
			tr.TokenRetrieved = true
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return tr, nil
}

// CreateTokenRequestState creates and stores a fresh OAuth state for a setup token request.
func (c *Client) CreateTokenRequestState(ctx context.Context, id string) (string, error) {
	state := strings.ReplaceAll(uuid.New().String(), "-", "")

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tr := new(types.TokenRequest)
		if err := tx.Where("id = ? AND purpose IN ?", id, oauthRoundTripPurposes).First(tr).Error; err != nil {
			return err
		}
		if tr.RequestExpiresAt.IsZero() || !time.Now().Before(tr.RequestExpiresAt) {
			return fmt.Errorf("token request has expired")
		}

		return tx.Model(tr).Updates(map[string]any{"state": state, "error": ""}).Error
	}); err != nil {
		return "", fmt.Errorf("failed to create state: %w", err)
	}
	slog.Info("Created OAuth state for token request", "tokenRequestID", id)

	return state, nil
}

// VerifyTokenRequestState consumes OAuth state and returns its setup token request.
func (c *Client) VerifyTokenRequestState(ctx context.Context, state string) (*types.TokenRequest, error) {
	if state == "" {
		return nil, fmt.Errorf("state is required")
	}

	tr := new(types.TokenRequest)
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state = ? AND purpose IN ?", state, oauthRoundTripPurposes).First(tr).Error; err != nil {
			if tr.ID == "" {
				return err
			}
			tr.Error = err.Error()
		}
		if tr.RequestExpiresAt.IsZero() || !time.Now().Before(tr.RequestExpiresAt) {
			return fmt.Errorf("token request has expired")
		}

		return tx.Model(tr).Clauses(clause.Returning{}).Updates(map[string]any{"state": "", "error": tr.Error}).Error
	})
	slog.Info("Verified OAuth state for token request", "tokenRequestID", tr.ID, "success", err == nil)
	return tr, err
}

// runTokenCleanup removes expired token requests, retrieved requests, and expired auth tokens.
func (c *Client) runTokenCleanup(ctx context.Context) {
	timer := time.NewTimer(tokenCleanupInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		var (
			errs []error
			now  = time.Now()
		)
		if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			errs = append(errs, tx.Where("request_expires_at < ?", now).Where("token_retrieved = ?", false).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("token_retrieved = ?", true).Where("updated_at < ?", now.Add(-tokenCleanupInterval)).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("no_expiration = ?", false).Where("expires_at < ?", now).Delete(new(types.AuthToken)).Error)
			return errors.Join(errs...)
		}); err != nil {
			slog.Error("error cleaning up state", "error", err)
		}

		timer.Reset(tokenCleanupInterval)
	}
}

func generateDeviceCode() (string, error) {
	random := make([]byte, deviceCodeLength)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("failed to generate device code: %w", err)
	}

	code := make([]byte, deviceCodeLength)
	for i, value := range random {
		code[i] = deviceCodeAlphabet[int(value)&(len(deviceCodeAlphabet)-1)]
	}

	return string(code[:4]) + "-" + string(code[4:]), nil
}

func normalizeDeviceCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func validNormalizedDeviceCode(code string) bool {
	if len(code) != deviceCodeLength {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(deviceCodeAlphabet, char) {
			return false
		}
	}
	return true
}

func digestNormalizedDeviceCode(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}
