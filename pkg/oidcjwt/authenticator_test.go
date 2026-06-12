package oidcjwt

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticator_AdminRoleGrantsAdminOwner(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(&gwtypes.User{ID: 1, Username: "alice", Email: "alice@example.com"}))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
	})
	req, _ := http.NewRequest("GET", "/api/system-mcp-catalogs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.Contains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAuthenticated)
	assert.Equal(t, "1", resp.User.GetUID())
	assert.Equal(t, "alice", resp.User.GetName())

	extra := resp.User.GetExtra()
	assert.Equal(t, []string{"alice@example.com"}, extra["email"])
	assert.Equal(t, []string{"generic-oauth-auth-provider"}, extra["auth_provider_name"])
	require.NotEmpty(t, extra["auth_provider_user_id"])
	assert.Contains(t, extra["auth_provider_user_id"][0], "iss:")
	assert.Contains(t, extra["auth_provider_user_id"][0], "\x00sub:user-1")
}

func TestBuildIdentity_UsernameFallback(t *testing.T) {
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
			id := buildIdentity(tc.claims)
			assert.Equal(t, tc.wantUser, id.ProviderUsername)
		})
	}
}

func TestBuildIdentity_ProviderUserIDFormat(t *testing.T) {
	claims := Claims{Issuer: "https://issuer.example.com", Subject: "user-1"}
	id := buildIdentity(claims)
	assert.Equal(t, "iss:https://issuer.example.com\x00sub:user-1", id.ProviderUserID)
	assert.Equal(t, "https://issuer.example.com", id.ProviderIssuer)
}

func TestAuthenticator_NonAdminRoleGetsAuthenticatedOnly(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(&gwtypes.User{ID: 2, Username: "bob"}))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-2", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"user"},
	})
	req, _ := http.NewRequest("GET", "/api/mcp-servers", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, ok, err := auth.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, resp.User.GetGroups(), types.GroupAdmin)
	assert.NotContains(t, resp.User.GetGroups(), types.GroupOwner)
	assert.Contains(t, resp.User.GetGroups(), types.GroupAuthenticated)
}

func TestAuthenticator_FailsWhenIneligible(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)
	auth := NewAuthenticator(cfg, v, stubResolver(nil))

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
	auth := NewAuthenticator(cfg, v, stubResolver(nil))

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-3", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err = auth.AuthenticateRequest(req)
	assert.Error(t, err)
}

func TestAuthenticator_NoBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, nil, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestAuthenticator_NonJWTBearerFallsThrough(t *testing.T) {
	cfg := Config{IssuerURL: "https://example.com", Audience: "obot-default"}
	auth := NewAuthenticator(cfg, &Verifier{}, nil)
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
	auth := NewAuthenticator(cfg, v, stubResolver(nil))

	tok := testutil.MintTestJWT(t, priv, "kid-X", "https://other-issuer.example.com", "obot-default", "user-4", 60*time.Second, nil)
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, ok, err := auth.AuthenticateRequest(req)
	assert.NoError(t, err)
	assert.False(t, ok)
}

type stubResolverImpl struct {
	out *gwtypes.User
	err error
}

func (s *stubResolverImpl) ResolveOrCreate(_ context.Context, _ *gwtypes.Identity, _ string) (*gwtypes.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.out == nil {
		return nil, errors.New("stub: no user")
	}
	return s.out, nil
}

func stubResolver(out *gwtypes.User) IdentityResolver {
	return &stubResolverImpl{out: out}
}
