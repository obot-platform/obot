package oidcjwt

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticator_ReturnsGenericOAuthProviderIdentity(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v)

	emailVerified := true
	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible":           true,
		"roles":              []string{"admin"},
		"email":              "alice@example.com",
		"email_verified":     emailVerified,
		"preferred_username": "alice@example.com",
		"name":               "Alice Example",
	})
	req, _ := http.NewRequest("GET", "/api/system-mcp-catalogs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, resp.User.GetGroups(), "UserDecorator owns canonical Obot group derivation")
	assert.Equal(t, "user-1", resp.User.GetUID())
	assert.Equal(t, "alice@example.com", resp.User.GetName())

	extra := resp.User.GetExtra()
	assert.Equal(t, []string{"alice@example.com"}, extra["email"])
	assert.Equal(t, []string{"generic-oauth-auth-provider"}, extra["auth_provider_name"])
	assert.Equal(t, []string{system.DefaultNamespace}, extra["auth_provider_namespace"])
	assert.Equal(t, []string{"user-1"}, extra["auth_provider_user_id"])
	assert.Equal(t, []string{issuer.URL}, extra["auth_provider_issuer"])
	assert.Equal(t, []string{"true"}, extra["auth_provider_email_verified"])
	assert.Equal(t, []string{"admin"}, extra[jwtRolesExtraKey])
}

func TestProviderUsername_UsernameFallback(t *testing.T) {
	cases := []struct {
		name     string
		claims   Claims
		wantUser string
	}{
		{"preferred_username wins", Claims{Subject: "s", Email: "e@x", Name: "N", PreferredUsername: "p"}, "p"},
		{"name when no preferred", Claims{Subject: "s", Email: "e@x", Name: "N"}, "N"},
		{"email when no name", Claims{Subject: "s", Email: "e@x"}, "e@x"},
		{"sub when nothing else", Claims{Subject: "s"}, "s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.claims.Issuer = "https://issuer.example.com"
			assert.Equal(t, tc.wantUser, providerUsername(tc.claims))
		})
	}
}

func TestAuthenticator_NonAdminRoleReturnsProviderIdentityOnly(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-2", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"user"},
	})
	req, _ := http.NewRequest("GET", "/api/mcp-servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, resp.User.GetGroups())
	assert.Equal(t, []string{"user"}, resp.User.GetExtra()[jwtRolesExtraKey])
}

func TestAuthenticator_FailsWhenIneligible(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-3", 60*time.Second, map[string]any{
		"eligible": false,
	})
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err = auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

func TestAuthenticator_FailsWhenEligibilityMissing(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-3", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err = auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

func TestAuthenticator_NoBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_NonJWTBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, &Verifier{})
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer bootstrap-token")

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_DifferentIssuerFallsThrough(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v)

	tok := testutil.MintTestJWT(t, priv, "kid-X", "https://other-issuer.example.com", "obot-default", "user-4", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}
