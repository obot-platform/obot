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
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	serviceAccountValidationCacheTTL = 30 * time.Second
)

type serviceAccountValidationCacheEntry struct {
	apiKey    types.ServiceAccountAPIKey
	expiresAt time.Time
}

func (c *Client) CreateServiceAccountAPIKey(ctx context.Context, serviceAccountName string, now time.Time) (*types.ServiceAccountAPIKey, error) {
	secretBytes := make([]byte, apiKeySecretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate service account key secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash service account key secret: %w", err)
	}

	key := &types.ServiceAccountAPIKey{
		ServiceAccountName: serviceAccountName,
		HashedSecret:       string(hashedSecret),
		CreatedAt:          now.UTC(),
		ValidAfter:         now.UTC(),
	}
	if err := c.db.WithContext(ctx).Create(key).Error; err != nil {
		return nil, fmt.Errorf("failed to create service account key: %w", err)
	}

	key.Token = fmt.Sprintf("%s-%d-%s", serviceaccounts.TokenPrefix, key.ID, secret)
	return key, nil
}

func (c *Client) ListServiceAccountAPIKeys(ctx context.Context, serviceAccountName string) ([]types.ServiceAccountAPIKey, error) {
	var keys []types.ServiceAccountAPIKey
	if err := c.db.WithContext(ctx).
		Where("service_account_name = ?", serviceAccountName).
		Order("created_at ASC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list service account keys: %w", err)
	}
	return keys, nil
}

func (c *Client) ValidateStorageServiceAccountToken(ctx context.Context, token string) (*types.ServiceAccountAPIKey, error) {
	now := time.Now().UTC()
	if cachedKey, ok := c.getValidatedServiceAccountAPIKeyFromCache(token, now); ok {
		return cachedKey, nil
	}

	prefix, id, secret, err := parseServiceAccountToken(token)
	if err != nil || prefix != serviceaccounts.TokenPrefix {
		return nil, gorm.ErrRecordNotFound
	}

	var key types.ServiceAccountAPIKey
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&key).Error; err != nil {
		return nil, err
	}
	if key.ValidAfter.After(now) || (key.RetireAfter != nil && !key.RetireAfter.After(now)) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(key.HashedSecret), []byte(secret)); err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	key.Token = token
	c.putValidatedServiceAccountAPIKeyInCache(token, &key, now)
	return &key, nil
}

func (c *Client) DeleteExpiredServiceAccountAPIKeys(ctx context.Context, serviceAccountName string, now time.Time) error {
	result := c.db.WithContext(ctx).
		Where("service_account_name = ?", serviceAccountName).
		Where("retire_after IS NOT NULL AND retire_after <= ?", now.UTC()).
		Delete(&types.ServiceAccountAPIKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete expired service account keys: %w", result.Error)
	}
	return nil
}

func (c *Client) RetireOtherServiceAccountAPIKeys(ctx context.Context, serviceAccountName string, activeID uint, retireAfter time.Time) error {
	result := c.db.WithContext(ctx).
		Model(&types.ServiceAccountAPIKey{}).
		Where("service_account_name = ?", serviceAccountName).
		Where("id <> ?", activeID).
		Where("retire_after IS NULL OR retire_after > ?", retireAfter.UTC()).
		Update("retire_after", retireAfter.UTC())
	if result.Error != nil {
		return fmt.Errorf("failed to retire service account keys: %w", result.Error)
	}
	return nil
}

func (c *Client) DeleteServiceAccountAPIKeyByID(ctx context.Context, id uint) error {
	result := c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.ServiceAccountAPIKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete service account key: %w", result.Error)
	}
	return nil
}

func (c *Client) DeleteAllServiceAccountAPIKeys(ctx context.Context, serviceAccountName string) error {
	result := c.db.WithContext(ctx).Where("service_account_name = ?", serviceAccountName).Delete(&types.ServiceAccountAPIKey{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete service account keys: %w", result.Error)
	}
	return nil
}

func serviceAccountCacheFingerprint(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

func cloneServiceAccountAPIKey(apiKey types.ServiceAccountAPIKey) types.ServiceAccountAPIKey {
	cloned := apiKey
	if apiKey.RetireAfter != nil {
		cloned.RetireAfter = new(time.Time)
		*cloned.RetireAfter = *apiKey.RetireAfter
	}
	return cloned
}

func (c *Client) getValidatedServiceAccountAPIKeyFromCache(token string, now time.Time) (*types.ServiceAccountAPIKey, bool) {
	if c.serviceAccountCacheTTL <= 0 || c.serviceAccountCache == nil {
		return nil, false
	}

	fingerprint := serviceAccountCacheFingerprint(token)

	c.serviceAccountCacheLock.RLock()
	entry, ok := c.serviceAccountCache[fingerprint]
	c.serviceAccountCacheLock.RUnlock()
	if !ok {
		return nil, false
	}

	entryExpired := now.After(entry.expiresAt) || entry.apiKey.ValidAfter.After(now) || (entry.apiKey.RetireAfter != nil && !entry.apiKey.RetireAfter.After(now))
	if !entryExpired {
		apiKey := cloneServiceAccountAPIKey(entry.apiKey)
		apiKey.Token = token
		return &apiKey, true
	}

	// The entry looked expired under the read lock. Re-check under the write
	// lock before deleting because another goroutine may have refreshed it
	// between releasing RLock and acquiring Lock.
	c.serviceAccountCacheLock.Lock()
	defer c.serviceAccountCacheLock.Unlock()

	entry, ok = c.serviceAccountCache[fingerprint]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) || entry.apiKey.ValidAfter.After(now) || (entry.apiKey.RetireAfter != nil && !entry.apiKey.RetireAfter.After(now)) {
		delete(c.serviceAccountCache, fingerprint)
		return nil, false
	}

	apiKey := cloneServiceAccountAPIKey(entry.apiKey)
	apiKey.Token = token
	return &apiKey, true
}

func (c *Client) putValidatedServiceAccountAPIKeyInCache(token string, apiKey *types.ServiceAccountAPIKey, now time.Time) {
	if c.serviceAccountCacheTTL <= 0 || apiKey == nil || c.serviceAccountCache == nil {
		return
	}

	fingerprint := serviceAccountCacheFingerprint(token)
	entry := serviceAccountValidationCacheEntry{
		apiKey:    cloneServiceAccountAPIKey(*apiKey),
		expiresAt: now.Add(c.serviceAccountCacheTTL),
	}

	c.serviceAccountCacheLock.Lock()
	defer c.serviceAccountCacheLock.Unlock()

	c.serviceAccountCache[fingerprint] = entry
}

func (c *Client) pruneExpiredValidatedServiceAccountAPIKeys(now time.Time) {
	c.serviceAccountCacheLock.Lock()
	defer c.serviceAccountCacheLock.Unlock()

	for fingerprint, entry := range c.serviceAccountCache {
		if now.After(entry.expiresAt) || entry.apiKey.ValidAfter.After(now) || (entry.apiKey.RetireAfter != nil && !entry.apiKey.RetireAfter.After(now)) {
			delete(c.serviceAccountCache, fingerprint)
		}
	}
}

func parseServiceAccountToken(token string) (string, uint, string, error) {
	parts := strings.SplitN(token, "-", 3)
	if len(parts) != 3 {
		return "", 0, "", fmt.Errorf("invalid service account token format")
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid service account token ID: %w", err)
	}
	return parts[0], uint(id), parts[2], nil
}
