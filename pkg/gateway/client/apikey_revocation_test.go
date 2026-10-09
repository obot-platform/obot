package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestRevokeAPIKeyRetainsMetadataAndImmediatelyRejectsCachedCredential(t *testing.T) {
	c := newTestClient(t)
	c.apiKeyCache = make(map[[32]byte]apiKeyValidationCacheEntry)
	c.apiKeyCacheTTL = time.Minute

	created, err := c.CreateAPIKey(t.Context(), 7, "CLI token", "developer laptop", nil, types.APIKeyScopes{
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"mcp-one"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.ValidateAPIKey(t.Context(), created.Key); err != nil {
		t.Fatalf("validate created API key: %v", err)
	}

	if err := c.RevokeAPIKey(t.Context(), 7, created.ID); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	if _, err := c.ValidateAPIKey(t.Context(), created.Key); err == nil {
		t.Fatal("revoked API key was still accepted")
	}

	retained, err := c.GetAPIKey(t.Context(), 7, created.ID)
	if err != nil {
		t.Fatalf("get revoked API key: %v", err)
	}
	if retained.RevokedAt == nil {
		t.Fatal("revoked API key is missing revokedAt")
	}
	if retained.Name != "CLI token" || retained.Description != "developer laptop" {
		t.Fatalf("revoked API key metadata was not retained: %+v", retained)
	}
	if !retained.CanAccessLLMProxy || len(retained.MCPServerIDs) != 1 || retained.MCPServerIDs[0] != "mcp-one" {
		t.Fatalf("revoked API key scopes were not retained: %+v", retained.APIKeyScopes)
	}

	firstRevokedAt := *retained.RevokedAt
	if err := c.RevokeAPIKey(t.Context(), 7, created.ID); err != nil {
		t.Fatalf("revoke API key again: %v", err)
	}
	retained, err = c.GetAPIKey(t.Context(), 7, created.ID)
	if err != nil {
		t.Fatalf("get API key after second revocation: %v", err)
	}
	if retained.RevokedAt == nil || !retained.RevokedAt.Equal(firstRevokedAt) {
		t.Fatalf("second revocation changed revokedAt: first=%s second=%v", firstRevokedAt, retained.RevokedAt)
	}
}

func TestRevokeAPIKeyImmediatelyRejectsCredentialCachedByAnotherClient(t *testing.T) {
	revoker := newTestClient(t)
	revoker.apiKeyCache = make(map[[32]byte]apiKeyValidationCacheEntry)
	revoker.apiKeyCacheTTL = time.Minute
	validator := &Client{
		db:             revoker.db,
		apiKeyCache:    make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL: time.Minute,
	}

	created, err := revoker.CreateAPIKey(t.Context(), 7, "CLI token", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.ValidateAPIKey(t.Context(), created.Key); err != nil {
		t.Fatalf("populate second client cache: %v", err)
	}

	if err := revoker.RevokeAPIKey(t.Context(), 7, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := validator.ValidateAPIKey(t.Context(), created.Key); err == nil {
		t.Fatal("another client's cached API key remained valid after revocation")
	}
}

func TestRevokedAPIKeysAreHiddenFromOperationalListsAndRetainedForAdmins(t *testing.T) {
	c := newTestClient(t)

	active, err := c.CreateAPIKey(t.Context(), 7, "active", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := c.CreateAPIKey(t.Context(), 7, "revoked", "", nil, types.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeAPIKey(t.Context(), 7, revoked.ID); err != nil {
		t.Fatal(err)
	}

	userKeys, err := c.ListAPIKeys(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(userKeys) != 1 || userKeys[0].ID != active.ID {
		t.Fatalf("user API key list = %+v, want only active key %d", userKeys, active.ID)
	}

	adminKeys, err := c.ListAllAPIKeys(t.Context(), APIKeyListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminKeys) != 1 || adminKeys[0].ID != active.ID {
		t.Fatalf("default admin API key list = %+v, want only active key %d", adminKeys, active.ID)
	}

	adminKeys, err = c.ListAllAPIKeys(t.Context(), APIKeyListOptions{ShowRevoked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminKeys) != 2 {
		t.Fatalf("admin API key history list has %d keys, want 2", len(adminKeys))
	}
	var foundRevoked bool
	for _, key := range adminKeys {
		if key.ID == revoked.ID {
			foundRevoked = key.RevokedAt != nil
		}
	}
	if !foundRevoked {
		t.Fatalf("admin API key list did not retain revoked key %d: %+v", revoked.ID, adminKeys)
	}
}
