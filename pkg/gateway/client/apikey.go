package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	apiKeySecretLength       = 32 // 32 bytes = 256 bits of entropy
	apiKeyPrefix             = "ok1"
	apiKeyValidationCacheTTL = 15 * time.Second
	apiKeyCacheCleanupPeriod = 5 * time.Minute
	expirationDur            = 7 * 24 * time.Hour
)

type apiKeyValidationCacheEntry struct {
	apiKey    types.APIKey
	expiresAt time.Time
	keyID     uint
}

// cloneAPIKey creates a deep copy of the provided APIKey, so that it's safe to return without corrupting the cache
func cloneAPIKey(apiKey types.APIKey) types.APIKey {
	cloned := apiKey
	if apiKey.MCPServerIDs != nil {
		cloned.MCPServerIDs = append([]string(nil), apiKey.MCPServerIDs...)
	}
	if apiKey.LastUsedAt != nil {
		cloned.LastUsedAt = new(*apiKey.LastUsedAt)
	}
	if apiKey.ExpiresAt != nil {
		cloned.ExpiresAt = new(*apiKey.ExpiresAt)
	}
	if apiKey.RevokedAt != nil {
		cloned.RevokedAt = new(*apiKey.RevokedAt)
	}
	return cloned
}

func apiKeyCacheFingerprint(key string) [32]byte {
	return sha256.Sum256([]byte(key))
}

func (c *Client) getValidatedAPIKeyFromCache(key string, now time.Time) (*types.APIKey, bool) {
	if c.apiKeyCacheTTL <= 0 {
		return nil, false
	}

	fingerprint := apiKeyCacheFingerprint(key)

	c.apiKeyCacheLock.RLock()
	entry, ok := c.apiKeyCache[fingerprint]
	if !ok {
		c.apiKeyCacheLock.RUnlock()
		return nil, false
	}

	// Fast path: entry appears valid under the read lock.
	entryExpired := entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now))
	if !entryExpired {
		cachedAPIKey := entry.apiKey
		c.apiKeyCacheLock.RUnlock()
		apiKey := cloneAPIKey(cachedAPIKey)
		return &apiKey, true
	}

	// Slow path: entry appears expired; re-check under write lock before deleting
	c.apiKeyCacheLock.RUnlock()

	c.apiKeyCacheLock.Lock()
	entry, ok = c.apiKeyCache[fingerprint]
	if !ok {
		c.apiKeyCacheLock.Unlock()
		return nil, false
	}

	if entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
		delete(c.apiKeyCache, fingerprint)
		c.apiKeyCacheLock.Unlock()
		return nil, false
	}

	cachedAPIKey := entry.apiKey
	c.apiKeyCacheLock.Unlock()
	apiKey := cloneAPIKey(cachedAPIKey)
	return &apiKey, true
}

func (c *Client) putValidatedAPIKeyInCache(key string, apiKey *types.APIKey, now time.Time) {
	if c.apiKeyCacheTTL <= 0 || apiKey == nil {
		return
	}

	c.apiKeyCacheLock.Lock()
	c.apiKeyCache[apiKeyCacheFingerprint(key)] = apiKeyValidationCacheEntry{
		apiKey:    cloneAPIKey(*apiKey),
		expiresAt: now.Add(c.apiKeyCacheTTL),
		keyID:     apiKey.ID,
	}
	c.apiKeyCacheLock.Unlock()
}

func (c *Client) pruneExpiredValidatedAPIKeys(now time.Time) {
	c.apiKeyCacheLock.Lock()
	defer c.apiKeyCacheLock.Unlock()

	for fingerprint, entry := range c.apiKeyCache {
		if entry.apiKey.RevokedAt != nil || now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
			delete(c.apiKeyCache, fingerprint)
		}
	}
}

func (c *Client) runAPIKeyCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(apiKeyCacheCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.pruneExpiredValidatedAPIKeys(now)
			c.pruneExpiredValidatedServiceAccountAPIKeys(now)
		}
	}
}

func (c *Client) invalidateValidatedAPIKeysByID(keyID uint) {
	c.apiKeyCacheLock.Lock()
	defer c.apiKeyCacheLock.Unlock()

	for fingerprint, entry := range c.apiKeyCache {
		if entry.keyID == keyID {
			delete(c.apiKeyCache, fingerprint)
		}
	}
}

// CreateAPIKey generates a new API key for the given user.
// Returns the full key only once in the response.
func (c *Client) CreateAPIKey(ctx context.Context, userID uint, name, description string, expiresAt *time.Time, scopes types.APIKeyScopes) (*types.APIKeyCreateResponse, error) {
	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error

		resp, err = c.createAPIKey(tx, userID, name, description, expiresAt, scopes)
		return err
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

// CreateHostedAgentAPIKey issues the credential a hosted agent sandbox uses to
// reach the MCP gateway and the model proxy.
//
// The key authenticates as the agent, not as its owner, so ownerUserID is
// recorded for attribution and cleanup only and confers none of that user's
// access. Scopes are deliberately limited to the gateway and the proxy: an
// agent must never reach the Obot API, or an exfiltrated credential could
// enumerate or modify resources.
//
// MCPServerIDs is left empty on purpose. Authentication resolves the instance
// and fills the authorized servers from its live configuration, so narrowing an
// agent takes effect without reissuing this key.
//
// The key does not expire. Agents read their credential once at startup and
// cannot reload it, so expiry would strand a running sandbox.
func (c *Client) CreateHostedAgentAPIKey(ctx context.Context, instanceID string, ownerUserID uint, name string) (*types.APIKeyCreateResponse, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("hosted agent instance ID is required")
	}

	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		resp, err = c.createAPIKey(tx, ownerUserID, name, "Hosted agent sandbox credential", nil, types.APIKeyScopes{
			CanAccessLLMProxy: true,
			CanAccessSkills:   true,
		})
		if err != nil {
			return err
		}
		resp.HostedAgentInstanceID = &instanceID
		return tx.Model(&types.APIKey{}).Where("id = ?", resp.ID).
			Update("hosted_agent_instance_id", instanceID).Error
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

// RevokeHostedAgentAPIKeys immediately disables every key bound to an instance
// while retaining the key metadata for audit history.
func (c *Client) RevokeHostedAgentAPIKeys(ctx context.Context, instanceID string) error {
	if instanceID == "" {
		return nil
	}
	if err := c.db.WithContext(ctx).Model(&types.APIKey{}).
		Where("hosted_agent_instance_id = ?", instanceID).
		Where("revoked_at IS NULL").
		Update("revoked_at", time.Now().UTC()).Error; err != nil {
		return fmt.Errorf("failed to revoke hosted agent API keys: %w", err)
	}
	return nil
}

// HostedAgentAPIKeys returns the keys bound to an instance, without secrets.
func (c *Client) HostedAgentAPIKeys(ctx context.Context, instanceID string) ([]types.APIKey, error) {
	var keys []types.APIKey
	if err := c.db.WithContext(ctx).Where("hosted_agent_instance_id = ?", instanceID).
		Where("revoked_at IS NULL").
		Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list hosted agent API keys: %w", err)
	}
	return keys, nil
}

// CreateAPIKeyFromSetupTokenRequest creates an API key from a token request.
func (c *Client) CreateAPIKeyFromSetupTokenRequest(ctx context.Context, userID uint, tr *types.TokenRequest) (*types.APIKeyCreateResponse, error) {
	if tr.Purpose != types.TokenRequestPurposeSetup {
		return nil, fmt.Errorf("token request %q is not a setup request", tr.ID)
	}

	var resp *types.APIKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		resp, err = c.createAPIKeyFromTokenRequest(tx, userID, tr)
		return err
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) createAPIKeyFromTokenRequest(tx *gorm.DB, userID uint, tr *types.TokenRequest) (*types.APIKeyCreateResponse, error) {
	var expiresAt *time.Time
	if !tr.NoExpiration {
		expiresAt = new(time.Now().Add(expirationDur))
	}

	resp, err := c.createAPIKey(tx, userID, tr.Name, tr.Description, expiresAt, tr.Scopes)
	if err != nil {
		return nil, err
	}

	if resp.ExpiresAt != nil {
		tr.ExpiresAt = *resp.ExpiresAt
	} else {
		tr.ExpiresAt = time.Time{}
	}
	tr.Token = resp.Key

	if err := tx.Model(tr).Updates(map[string]any{
		"expires_at": tr.ExpiresAt,
		"token":      tr.Token,
	}).Error; err != nil {
		return nil, err
	}

	return resp, nil
}

// APIKeyListOptions controls whether retained revoked keys are included.
type APIKeyListOptions struct {
	ShowRevoked bool
}

// ListAPIKeys returns active API keys for a user (without the secrets).
func (c *Client) ListAPIKeys(ctx context.Context, userID uint) ([]types.APIKey, error) {
	return c.ListAPIKeysWithOptions(ctx, userID, APIKeyListOptions{})
}

// ListAPIKeysWithOptions returns API keys for a user according to opts.
func (c *Client) ListAPIKeysWithOptions(ctx context.Context, userID uint, opts APIKeyListOptions) ([]types.APIKey, error) {
	var keys []types.APIKey
	db := c.db.WithContext(ctx).Where("user_id = ?", userID)
	if !opts.ShowRevoked {
		db = db.Where("revoked_at IS NULL")
	}
	if err := db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

// GetAPIKey retrieves a single API key by ID.
func (c *Client) GetAPIKey(ctx context.Context, userID uint, keyID uint) (*types.APIKey, error) {
	var key types.APIKey
	if err := c.db.WithContext(ctx).Where("id = ?", keyID).Where("user_id = ?", userID).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// RevokeAPIKey immediately disables an API key while retaining its metadata.
// Repeated revocation is idempotent and preserves the original revocation time.
func (c *Client) RevokeAPIKey(ctx context.Context, userID uint, keyID uint) error {
	return c.revokeAPIKey(ctx, keyID, &userID)
}

// ValidateAPIKey validates an API key and returns the associated APIKey record.
// The key format is: ok1-<user_id>-<key_id>-<secret>
// Lookup is done by key ID, then bcrypt is used to verify the secret.
// Cache hits avoid repeating bcrypt but still check persisted lifecycle state so
// revocation on one server replica takes effect on every replica immediately.
// On cache misses, last_used_at is updated only if more than a minute has elapsed.
func (c *Client) ValidateAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	cacheNow := time.Now()
	if cachedAPIKey, ok := c.getValidatedAPIKeyFromCache(key, cacheNow); ok {
		var lifecycle struct {
			ExpiresAt *time.Time
			RevokedAt *time.Time
		}
		if err := c.db.WithContext(ctx).Model(&types.APIKey{}).
			Select("expires_at", "revoked_at").
			Where("id = ?", cachedAPIKey.ID).
			Where("user_id = ?", cachedAPIKey.UserID).
			First(&lifecycle).Error; err != nil {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, err
		}
		if lifecycle.RevokedAt != nil {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, fmt.Errorf("API key has been revoked")
		}
		if lifecycle.ExpiresAt != nil && lifecycle.ExpiresAt.Before(cacheNow) {
			c.invalidateValidatedAPIKeysByID(cachedAPIKey.ID)
			return nil, fmt.Errorf("API key has expired")
		}
		return cachedAPIKey, nil
	}

	// Parse the key to extract components
	_, userID, keyID, secret, err := ParseAPIKey(key)
	if err != nil {
		return nil, err
	}

	var apiKey types.APIKey
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Look up by key ID
		if err := tx.Where("id = ?", keyID).Where("user_id = ?", userID).First(&apiKey).Error; err != nil {
			return err
		}

		// Verify the secret using bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(apiKey.HashedSecret), []byte(secret)); err != nil {
			return fmt.Errorf("invalid API key")
		}

		// Check expiration
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			return fmt.Errorf("API key has expired")
		}
		if apiKey.RevokedAt != nil {
			return fmt.Errorf("API key has been revoked")
		}

		// Update last used timestamp if more than a minute has elapsed
		lastUsedAtNow := time.Now()
		if apiKey.LastUsedAt == nil || lastUsedAtNow.Sub(*apiKey.LastUsedAt) > time.Minute {
			apiKey.LastUsedAt = &lastUsedAtNow
			return tx.Model(&apiKey).Update("last_used_at", lastUsedAtNow).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	c.putValidatedAPIKeyInCache(key, &apiKey, cacheNow)
	return &apiKey, nil
}

// ParseAPIKey parses an API key string and extracts its components.
// Returns prefix, userID, keyID, secret, and an error if the format is invalid.
func ParseAPIKey(key string) (prefix string, userID uint, keyID uint, secret string, err error) {
	n, err := fmt.Sscanf(key, "%3s-%d-%d-%s", &prefix, &userID, &keyID, &secret)
	if err != nil || n != 4 {
		return "", 0, 0, "", fmt.Errorf("invalid API key format")
	}
	if prefix != apiKeyPrefix {
		return "", 0, 0, "", fmt.Errorf("invalid API key prefix")
	}
	return prefix, userID, keyID, secret, nil
}

// ParseRedactedAPIKey extracts the owner and key IDs from the non-secret API
// key prefix emitted by nanobot audit logs. Nanobot retains the first three
// hyphen-delimited components or 12 characters, whichever is longer, so short
// numeric components can be followed by a small fragment of the redacted
// secret. Full bearer tokens are deliberately rejected.
func ParseRedactedAPIKey(key string) (userID uint, keyID uint, err error) {
	parts := strings.SplitN(key, "-", 4)
	if len(parts) != 4 {
		return 0, 0, fmt.Errorf("invalid redacted API key format")
	}
	if parts[0] != apiKeyPrefix {
		return 0, 0, fmt.Errorf("invalid API key prefix")
	}
	prefixLength := len(parts[0]) + len(parts[1]) + len(parts[2]) + 3
	if len(key) != max(prefixLength, 12) {
		return 0, 0, fmt.Errorf("invalid redacted API key format")
	}

	parsedUserID, err := strconv.ParseUint(parts[1], 10, 0)
	if err != nil || parsedUserID == 0 {
		return 0, 0, fmt.Errorf("invalid redacted API key format")
	}
	parsedKeyID, err := strconv.ParseUint(parts[2], 10, 0)
	if err != nil || parsedKeyID == 0 {
		return 0, 0, fmt.Errorf("invalid redacted API key format")
	}

	return uint(parsedUserID), uint(parsedKeyID), nil
}

// Admin methods - no user filtering

// ListAllAPIKeys returns system API keys according to opts.
func (c *Client) ListAllAPIKeys(ctx context.Context, opts APIKeyListOptions) ([]types.APIKey, error) {
	var keys []types.APIKey
	db := c.db.WithContext(ctx)
	if !opts.ShowRevoked {
		db = db.Where("revoked_at IS NULL")
	}
	if err := db.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

// GetAPIKeyByID retrieves an API key by ID without user filtering (for admin use).
func (c *Client) GetAPIKeyByID(ctx context.Context, keyID uint) (*types.APIKey, error) {
	var key types.APIKey
	if err := c.db.WithContext(ctx).Where("id = ?", keyID).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// RevokeAPIKeyByID immediately disables an API key without user filtering
// while retaining its metadata for audit history.
func (c *Client) RevokeAPIKeyByID(ctx context.Context, keyID uint) error {
	return c.revokeAPIKey(ctx, keyID, nil)
}

func (c *Client) revokeAPIKey(ctx context.Context, keyID uint, userID *uint) error {
	db := c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", keyID)
	if userID != nil {
		db = db.Where("user_id = ?", *userID)
	}
	result := db.Where("revoked_at IS NULL").Update("revoked_at", time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("failed to revoke API key: %w", result.Error)
	}
	c.invalidateValidatedAPIKeysByID(keyID)
	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key
// if more than a minute has elapsed since the previous timestamp.
func (c *Client) UpdateAPIKeyLastUsed(ctx context.Context, key *types.APIKey) error {
	now := time.Now()
	if key.LastUsedAt != nil && now.Sub(*key.LastUsedAt) <= time.Minute {
		return nil
	}

	result := c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", key.ID).Update("last_used_at", now)
	if result.Error != nil {
		return fmt.Errorf("failed to update API key last used time: %w", result.Error)
	}
	return nil
}

func (c *Client) createAPIKey(tx *gorm.DB, userID uint, name, description string, expiresAt *time.Time, scopes types.APIKeyScopes) (*types.APIKeyCreateResponse, error) {
	// Generate cryptographically secure random secret
	secretBytes := make([]byte, apiKeySecretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	// Hash the secret with bcrypt for storage
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash secret: %w", err)
	}

	// Create the API key record
	apiKey := &types.APIKey{
		UserID:       userID,
		Name:         name,
		Description:  description,
		HashedSecret: string(hashedSecret),
		APIKeyScopes: scopes,
		ExpiresAt:    expiresAt,
		CreatedAt:    time.Now(),
	}

	if err := tx.Create(apiKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Construct the full key with the auto-generated ID
	fullKey := fmt.Sprintf("%s-%d-%d-%s", apiKeyPrefix, userID, apiKey.ID, secret)

	return &types.APIKeyCreateResponse{
		APIKey: *apiKey,
		Key:    fullKey,
	}, nil
}
