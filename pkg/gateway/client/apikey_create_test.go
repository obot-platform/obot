package client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestCreateAPIKeyFromTokenRequestCopiesScopesAndUpdatesRequest(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	tr := &types.TokenRequest{
		ID:          "token-request-1",
		Name:        "CLI login",
		Description: "created by tests",
		Scopes: types.APIKeyScopes{
			CanAccessAPI:                true,
			CanAccessLLMProxy:           true,
			CanAccessSkills:             true,
			CanAccessPublishedArtifacts: true,
			MCPServerIDs:                []string{"*"},
		},
	}
	if err := c.db.WithContext(ctx).Create(tr).Error; err != nil {
		t.Fatalf("create token request: %v", err)
	}

	before := time.Now()
	resp, err := c.CreateAPIKeyFromTokenRequest(ctx, 7, tr)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Key == "" {
		t.Fatal("expected API key response to include plaintext key")
	}
	if !strings.HasPrefix(resp.Key, "ok1-7-") {
		t.Fatalf("API key = %q, want ok1-7-* prefix", resp.Key)
	}
	if resp.UserID != 7 {
		t.Fatalf("UserID = %d, want 7", resp.UserID)
	}
	if resp.Name != "CLI login" {
		t.Fatalf("Name = %q, want CLI login", resp.Name)
	}
	if resp.ExpiresAt == nil {
		t.Fatal("expected token-request API key to expire by default")
	}
	if resp.ExpiresAt.Before(before.Add(7*24*time.Hour-time.Minute)) || resp.ExpiresAt.After(before.Add(7*24*time.Hour+time.Minute)) {
		t.Fatalf("ExpiresAt = %s, want about 7 days after creation", resp.ExpiresAt)
	}
	assertAPIKeyScopes(t, resp.APIKeyScopes, tr.Scopes)

	var storedKey types.APIKey
	if err := c.db.WithContext(ctx).First(&storedKey, resp.ID).Error; err != nil {
		t.Fatalf("load stored API key: %v", err)
	}
	assertAPIKeyScopes(t, storedKey.APIKeyScopes, tr.Scopes)

	var storedRequest types.TokenRequest
	if err := c.db.WithContext(ctx).First(&storedRequest, "id = ?", tr.ID).Error; err != nil {
		t.Fatalf("load stored token request: %v", err)
	}
	if storedRequest.Token != resp.Key {
		t.Fatalf("stored token request Token = %q, want response key", storedRequest.Token)
	}
	if storedRequest.ExpiresAt.IsZero() {
		t.Fatal("expected stored token request expiration")
	}
}

func TestCreateAPIKeyFromTokenRequestNoExpiration(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	tr := &types.TokenRequest{
		ID:           "token-request-1",
		Name:         "CLI login",
		NoExpiration: true,
		Scopes:       types.APIKeyScopes{CanAccessAPI: true},
	}
	if err := c.db.WithContext(ctx).Create(tr).Error; err != nil {
		t.Fatalf("create token request: %v", err)
	}

	resp, err := c.CreateAPIKeyFromTokenRequest(ctx, 7, tr)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %s, want nil", resp.ExpiresAt)
	}

	var storedRequest types.TokenRequest
	if err := c.db.WithContext(ctx).First(&storedRequest, "id = ?", tr.ID).Error; err != nil {
		t.Fatalf("load stored token request: %v", err)
	}
	if !storedRequest.ExpiresAt.IsZero() {
		t.Fatalf("stored token request ExpiresAt = %s, want zero", storedRequest.ExpiresAt)
	}
}

func assertAPIKeyScopes(t *testing.T, got, want types.APIKeyScopes) {
	t.Helper()

	if got.CanAccessAPI != want.CanAccessAPI ||
		got.CanAccessLLMProxy != want.CanAccessLLMProxy ||
		got.CanAccessSkills != want.CanAccessSkills ||
		got.CanAccessPublishedArtifacts != want.CanAccessPublishedArtifacts ||
		strings.Join(got.MCPServerIDs, ",") != strings.Join(want.MCPServerIDs, ",") {
		t.Fatalf("APIKeyScopes = %+v, want %+v", got, want)
	}
}
