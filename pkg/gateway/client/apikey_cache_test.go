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

func TestPutValidatedAPIKeyInCachePrunesExpiredEntries(t *testing.T) {
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

	if _, ok := c.apiKeyCache[apiKeyCacheFingerprint(expiredKey)]; ok {
		t.Fatal("expected expired cache entry to be pruned on put")
	}
	if _, ok := c.apiKeyCache[apiKeyCacheFingerprint(activeKey)]; !ok {
		t.Fatal("expected active cache entry to be inserted")
	}
}
