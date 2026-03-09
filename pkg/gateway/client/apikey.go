package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	apiKeySecretLength       = 32 // 32 bytes = 256 bits of entropy
	apiKeyPrefix             = "ok1"
	apiKeyValidationCacheTTL = 15 * time.Second
	apiKeyCacheCleanupPeriod = 5 * time.Minute
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
	return cloned
}

var clientTracer = otel.Tracer("obot/gateway/client")

func recordClientSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
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
	entryExpired := now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now))
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

	if now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
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
		if now.After(entry.expiresAt) || (entry.apiKey.ExpiresAt != nil && entry.apiKey.ExpiresAt.Before(now)) {
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
// At least one mcpServerID must be specified.
func (c *Client) CreateAPIKey(ctx context.Context, userID uint, name, description string, expiresAt *time.Time, mcpServerIDs []string) (*types.APIKeyCreateResponse, error) {
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
		ExpiresAt:    expiresAt,
		CreatedAt:    time.Now(),
		MCPServerIDs: mcpServerIDs,
	}

	if err := c.db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Construct the full key with the auto-generated ID
	fullKey := fmt.Sprintf("%s-%d-%d-%s", apiKeyPrefix, userID, apiKey.ID, secret)

	return &types.APIKeyCreateResponse{
		APIKey: *apiKey,
		Key:    fullKey,
	}, nil
}

// ListAPIKeys returns all API keys for a user (without the secrets).
func (c *Client) ListAPIKeys(ctx context.Context, userID uint) ([]types.APIKey, error) {
	var keys []types.APIKey
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error; err != nil {
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

// DeleteAPIKey removes an API key.
func (c *Client) DeleteAPIKey(ctx context.Context, userID uint, keyID uint) error {
	result := c.db.WithContext(ctx).Where("id = ?", keyID).Where("user_id = ?", userID).Delete(&types.APIKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete API key: %w", result.Error)
	}
	c.invalidateValidatedAPIKeysByID(keyID)
	return nil
}

// ValidateAPIKey validates an API key and returns the associated APIKey record.
// The key format is: ok1-<user_id>-<key_id>-<secret>
// Lookup is done by key ID, then bcrypt is used to verify the secret.
// Cache hits return a previously validated key without touching the database.
// On cache misses, last_used_at is updated only if more than a minute has elapsed.
func (c *Client) ValidateAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	ctx, span := clientTracer.Start(ctx, "gateway.client.validate_api_key")
	defer span.End()

	cacheNow := time.Now()
	_, cacheLookupSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.cache_lookup")
	cachedAPIKey, cacheHit := c.getValidatedAPIKeyFromCache(key, cacheNow)
	cacheLookupSpan.SetAttributes(attribute.Bool("gateway.client.cache_hit", cacheHit))
	cacheLookupSpan.End()
	span.SetAttributes(attribute.Bool("gateway.client.cache_hit", cacheHit))
	if cacheHit {
		return cachedAPIKey, nil
	}

	// Parse the key to extract components
	_, parseSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.parse_key")
	_, userID, keyID, secret, err := ParseAPIKey(key)
	if err != nil {
		recordClientSpanError(parseSpan, err)
		parseSpan.End()
		recordClientSpanError(span, err)
		return nil, err
	}
	parseSpan.End()

	var apiKey types.APIKey
	_, txSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.transaction")
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Look up by key ID
		_, lookupSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.lookup_record")
		err := tx.WithContext(ctx).Where("id = ?", keyID).Where("user_id = ?", userID).First(&apiKey).Error
		if err != nil {
			recordClientSpanError(lookupSpan, err)
			lookupSpan.End()
			return err
		}
		lookupSpan.End()

		// Verify the secret using bcrypt
		_, verifySecretSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.verify_secret")
		if err := bcrypt.CompareHashAndPassword([]byte(apiKey.HashedSecret), []byte(secret)); err != nil {
			recordClientSpanError(verifySecretSpan, err)
			verifySecretSpan.End()
			return fmt.Errorf("invalid API key")
		}
		verifySecretSpan.End()

		// Check expiration
		_, expirationSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.check_expiration")
		expired := apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now())
		expirationSpan.SetAttributes(attribute.Bool("gateway.client.api_key_expired", expired))
		expirationSpan.End()
		if expired {
			return fmt.Errorf("API key has expired")
		}

		// Update last used timestamp if more than a minute has elapsed
		lastUsedAtNow := time.Now()
		if apiKey.LastUsedAt == nil || lastUsedAtNow.Sub(*apiKey.LastUsedAt) > time.Minute {
			_, shouldUpdateSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.should_update_last_used")
			shouldUpdateLastUsed := true
			shouldUpdateSpan.SetAttributes(attribute.Bool("gateway.client.should_update_last_used", shouldUpdateLastUsed))
			shouldUpdateSpan.End()
			_, updateSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.update_last_used")
			apiKey.LastUsedAt = &lastUsedAtNow
			err := tx.WithContext(ctx).Model(&apiKey).Update("last_used_at", lastUsedAtNow).Error
			if err != nil {
				recordClientSpanError(updateSpan, err)
				updateSpan.End()
				return err
			}
			updateSpan.End()
			return nil
		}

		_, shouldUpdateSpan := clientTracer.Start(ctx, "gateway.client.validate_api_key.should_update_last_used")
		shouldUpdateLastUsed := false
		shouldUpdateSpan.SetAttributes(attribute.Bool("gateway.client.should_update_last_used", shouldUpdateLastUsed))
		shouldUpdateSpan.End()
		return nil
	})
	if err != nil {
		recordClientSpanError(txSpan, err)
	}
	txSpan.End()
	if err != nil {
		recordClientSpanError(span, err)
		return nil, err
	}

	c.putValidatedAPIKeyInCache(key, &apiKey, cacheNow)
	span.SetAttributes(attribute.Bool("gateway.client.last_used_tracked", apiKey.LastUsedAt != nil))
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

// Admin methods - no user filtering

// ListAllAPIKeys returns all API keys in the system (for admin use).
func (c *Client) ListAllAPIKeys(ctx context.Context) ([]types.APIKey, error) {
	var keys []types.APIKey
	if err := c.db.WithContext(ctx).Order("created_at DESC").Find(&keys).Error; err != nil {
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

// DeleteAPIKeyByID removes an API key by ID without user filtering (for admin use).
func (c *Client) DeleteAPIKeyByID(ctx context.Context, keyID uint) error {
	result := c.db.WithContext(ctx).Where("id = ?", keyID).Delete(&types.APIKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete API key: %w", result.Error)
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
