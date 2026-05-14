package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestValidatedAPIKeyCacheHit(t *testing.T) {
	t.Parallel()

	c := &Client{
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}

	now := time.Now()
	want := &types.APIKey{UserID: 7, Name: "cache-key"}

	c.putValidatedAPIKeyInCache("ok1-7-1-secret", want, now)

	got, ok := c.getValidatedAPIKeyFromCache("ok1-7-1-secret", now.Add(time.Second))
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.UserID != want.UserID || got.Name != want.Name {
		t.Fatalf("unexpected cached value: %+v", got)
	}
}

func TestValidatedAPIKeyCacheExpires(t *testing.T) {
	t.Parallel()

	c := &Client{
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Second,
	}

	now := time.Now()
	c.putValidatedAPIKeyInCache("ok1-7-1-secret", &types.APIKey{ID: 1, UserID: 7}, now)

	if _, ok := c.getValidatedAPIKeyFromCache("ok1-7-1-secret", now.Add(2*time.Second)); ok {
		t.Fatal("expected expired cache entry to miss")
	}
}

func TestInvalidateValidatedAPIKeysByID(t *testing.T) {
	t.Parallel()

	c := &Client{
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}

	now := time.Now()
	c.putValidatedAPIKeyInCache("ok1-7-1-secret", &types.APIKey{ID: 1, UserID: 7}, now)
	c.putValidatedAPIKeyInCache("ok1-7-2-secret", &types.APIKey{ID: 2, UserID: 7}, now)

	c.invalidateValidatedAPIKeysByID(1)

	if _, ok := c.getValidatedAPIKeyFromCache("ok1-7-1-secret", now); ok {
		t.Fatal("expected keyID 1 cache entry to be invalidated")
	}
	if _, ok := c.getValidatedAPIKeyFromCache("ok1-7-2-secret", now); !ok {
		t.Fatal("expected other cache entry to remain")
	}
}

func TestPruneExpiredValidatedAPIKeys(t *testing.T) {
	t.Parallel()

	c := &Client{
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}

	now := time.Now()
	expiredKey := "ok1-7-1-secret"
	activeKey := "ok1-7-2-secret"

	c.apiKeyCache[apiKeyCacheFingerprint(expiredKey)] = apiKeyValidationCacheEntry{
		apiKey:    types.APIKey{ID: 1, UserID: 7},
		expiresAt: now.Add(-time.Second),
		keyID:     1,
	}

	c.putValidatedAPIKeyInCache(activeKey, &types.APIKey{ID: 2, UserID: 7}, now)
	c.pruneExpiredValidatedAPIKeys(now)

	if _, ok := c.apiKeyCache[apiKeyCacheFingerprint(expiredKey)]; ok {
		t.Fatal("expected expired cache entry to be pruned during cleanup")
	}
	if _, ok := c.apiKeyCache[apiKeyCacheFingerprint(activeKey)]; !ok {
		t.Fatal("expected active cache entry to be inserted")
	}
}

func TestValidatedAPIKeyCacheReturnsDeepCopy(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(time.Hour)
	lastUsedAt := time.Now()
	c := &Client{
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}

	original := &types.APIKey{
		ID:           1,
		UserID:       7,
		Name:         "cache-key",
		MCPServerIDs: []string{"server-a"},
		LastUsedAt:   &lastUsedAt,
		ExpiresAt:    &expiresAt,
	}

	now := time.Now()
	c.putValidatedAPIKeyInCache("ok1-7-1-secret", original, now)

	got, ok := c.getValidatedAPIKeyFromCache("ok1-7-1-secret", now)
	if !ok {
		t.Fatal("expected cache hit")
	}

	got.MCPServerIDs[0] = "mutated"
	*got.LastUsedAt = got.LastUsedAt.Add(time.Minute)
	*got.ExpiresAt = got.ExpiresAt.Add(time.Minute)

	cached := c.apiKeyCache[apiKeyCacheFingerprint("ok1-7-1-secret")].apiKey
	if cached.MCPServerIDs[0] != "server-a" {
		t.Fatal("expected cached MCPServerIDs to be isolated from returned value")
	}
	if !cached.LastUsedAt.Equal(lastUsedAt) {
		t.Fatal("expected cached LastUsedAt to be isolated from returned value")
	}
	if !cached.ExpiresAt.Equal(expiresAt) {
		t.Fatal("expected cached ExpiresAt to be isolated from returned value")
	}
}

func TestValidatedServiceAccountAPIKeyCacheHit(t *testing.T) {
	t.Parallel()

	c := &Client{
		serviceAccountCache:    make(map[[32]byte]serviceAccountValidationCacheEntry),
		serviceAccountCacheTTL: time.Minute,
	}

	now := time.Now()
	want := &types.ServiceAccountAPIKey{ID: 7, ServiceAccountName: "NetworkPolicyProvider", ValidAfter: now.Add(-time.Minute)}

	c.putValidatedServiceAccountAPIKeyInCache("sa-7-secret", want, now)

	got, ok := c.getValidatedServiceAccountAPIKeyFromCache("sa-7-secret", now.Add(time.Second))
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != want.ID || got.ServiceAccountName != want.ServiceAccountName {
		t.Fatalf("unexpected cached value: %+v", got)
	}
	if got.PlaintextToken() != "sa-7-secret" {
		t.Fatalf("expected returned cached key to include token, got %q", got.PlaintextToken())
	}
}

func TestValidatedServiceAccountAPIKeyCacheExpires(t *testing.T) {
	t.Parallel()

	c := &Client{
		serviceAccountCache:    make(map[[32]byte]serviceAccountValidationCacheEntry),
		serviceAccountCacheTTL: time.Second,
	}

	now := time.Now()
	c.putValidatedServiceAccountAPIKeyInCache("sa-7-secret", &types.ServiceAccountAPIKey{ID: 7, ValidAfter: now.Add(-time.Minute)}, now)

	if _, ok := c.getValidatedServiceAccountAPIKeyFromCache("sa-7-secret", now.Add(2*time.Second)); ok {
		t.Fatal("expected expired cache entry to miss")
	}
}
