package client

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestCreateDeviceTokenRequest(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	request := &types.TokenRequest{
		ID:          "caller-supplied-id",
		Name:        "CLI token",
		Description: "device login",
		Scopes: types.APIKeyScopes{
			CanAccessAPI:         true,
			CanAccessDeviceScans: true,
		},
	}
	before := time.Now()
	code, err := c.CreateDeviceTokenRequest(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	if !regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}$`).MatchString(code) {
		t.Fatalf("device code %q does not use the expected format", code)
	}
	if request.ID == "" || request.ID == "caller-supplied-id" {
		t.Fatalf("request ID = %q, want a server-generated ID", request.ID)
	}

	var stored types.TokenRequest
	if err := c.db.WithContext(ctx).First(&stored, "id = ?", request.ID).Error; err != nil {
		t.Fatalf("load device request: %v", err)
	}
	if stored.Purpose != types.TokenRequestPurposeDeviceLogin {
		t.Fatalf("Purpose = %q, want %q", stored.Purpose, types.TokenRequestPurposeDeviceLogin)
	}
	if stored.DeviceCodeDigest == nil || len(*stored.DeviceCodeDigest) != 64 {
		t.Fatalf("DeviceCodeDigest = %v, want a SHA-256 hex digest", stored.DeviceCodeDigest)
	}
	if strings.Contains(*stored.DeviceCodeDigest, normalizeDeviceCode(code)) || *stored.DeviceCodeDigest == code {
		t.Fatal("stored device code digest contains the plaintext code")
	}
	if stored.RequestExpiresAt.Before(before.Add(deviceCodeLifetime)) || stored.RequestExpiresAt.After(after.Add(deviceCodeLifetime)) {
		t.Fatalf("RequestExpiresAt = %s, want about five minutes after creation", stored.RequestExpiresAt)
	}
	if stored.Name != request.Name || stored.Description != request.Description {
		t.Fatalf("stored metadata = %q/%q, want %q/%q", stored.Name, stored.Description, request.Name, request.Description)
	}
	assertAPIKeyScopes(t, stored.Scopes, request.Scopes)
}

func TestAuthorizeTokenRequestByDeviceCode(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	request := &types.TokenRequest{
		Name:        "CLI token",
		Description: "device login",
		Scopes: types.APIKeyScopes{
			CanAccessAPI:                true,
			CanAccessSkills:             true,
			CanAccessLLMProxy:           true,
			CanAccessDeviceScans:        true,
			CanAccessPublishedArtifacts: true,
			MCPServerIDs:                []string{"*"},
		},
	}
	code, err := c.CreateDeviceTokenRequest(ctx, request)
	if err != nil {
		t.Fatal(err)
	}

	input := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	before := time.Now()
	if err := c.AuthorizeTokenRequestByDeviceCode(ctx, 42, input); err != nil {
		t.Fatal(err)
	}

	keys, err := c.ListAPIKeys(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("API key count = %d, want 1", len(keys))
	}
	key := keys[0]
	if key.UserID != 42 || key.Name != request.Name || key.Description != request.Description {
		t.Fatalf("stored API key = %+v, want user and metadata from authorization/request", key)
	}
	if key.ExpiresAt == nil || key.ExpiresAt.Before(before.Add(expirationDur-time.Minute)) || key.ExpiresAt.After(before.Add(expirationDur+time.Minute)) {
		t.Fatalf("ExpiresAt = %v, want about seven days after authorization", key.ExpiresAt)
	}
	assertAPIKeyScopes(t, key.APIKeyScopes, request.Scopes)

	var stored types.TokenRequest
	if err := c.db.WithContext(ctx).First(&stored, "id = ?", request.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DeviceCodeVerifiedAt == nil {
		t.Fatal("expected device code verification timestamp")
	}
	if !strings.HasPrefix(stored.Token, "ok1-42-") {
		t.Fatalf("published token = %q, want authenticated user prefix", stored.Token)
	}
}

func TestAuthorizeTokenRequestByDeviceCodeNoExpiration(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	request := &types.TokenRequest{NoExpiration: true}
	code, err := c.CreateDeviceTokenRequest(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AuthorizeTokenRequestByDeviceCode(ctx, 8, code); err != nil {
		t.Fatal(err)
	}

	keys, err := c.ListAPIKeys(ctx, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ExpiresAt != nil {
		t.Fatalf("keys = %+v, want one key without expiration", keys)
	}
}

func TestAuthorizeTokenRequestByDeviceCodeRejectsInvalidExpiredAndReplayedCodes(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		c := newTestClient(t)
		err := c.AuthorizeTokenRequestByDeviceCode(t.Context(), 1, "2345-6789")
		if !errors.Is(err, ErrInvalidOrExpiredDeviceCode) {
			t.Fatalf("error = %v, want generic invalid or expired error", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		c := newTestClient(t)
		request := new(types.TokenRequest)
		code, err := c.CreateDeviceTokenRequest(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.db.WithContext(t.Context()).Model(request).Update("request_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}

		err = c.AuthorizeTokenRequestByDeviceCode(t.Context(), 1, code)
		if !errors.Is(err, ErrInvalidOrExpiredDeviceCode) {
			t.Fatalf("error = %v, want generic invalid or expired error", err)
		}
		assertAPIKeyCount(t, c, 0)
	})

	t.Run("replayed", func(t *testing.T) {
		c := newTestClient(t)
		request := new(types.TokenRequest)
		code, err := c.CreateDeviceTokenRequest(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AuthorizeTokenRequestByDeviceCode(t.Context(), 1, code); err != nil {
			t.Fatal(err)
		}
		err = c.AuthorizeTokenRequestByDeviceCode(t.Context(), 2, code)
		if !errors.Is(err, ErrInvalidOrExpiredDeviceCode) {
			t.Fatalf("error = %v, want generic invalid or expired error", err)
		}
		assertAPIKeyCount(t, c, 1)
	})
}

func TestAuthorizeTokenRequestByDeviceCodeConcurrentSubmissionsCreateOneKey(t *testing.T) {
	c := newTestClient(t)
	sqlDB, err := c.db.WithContext(t.Context()).DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	request := new(types.TokenRequest)
	code, err := c.CreateDeviceTokenRequest(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(userID uint) {
			defer wg.Done()
			<-start
			results <- c.AuthorizeTokenRequestByDeviceCode(t.Context(), userID, code)
		}(uint(i + 1))
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrInvalidOrExpiredDeviceCode) {
			t.Fatalf("losing authorization error = %v, want generic invalid or expired error", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful submissions = %d, want 1", successes)
	}
	assertAPIKeyCount(t, c, 1)
}

func assertAPIKeyCount(t *testing.T, c *Client, want int64) {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.APIKey{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("API key count = %d, want %d", count, want)
	}
}
