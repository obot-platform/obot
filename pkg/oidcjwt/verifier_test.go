package oidcjwt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/oidcjwt/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifier_AcceptsValidToken(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles", AdminRoles: []string{"admin"}}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
		"email":    "alice@example.com",
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.True(t, claims.Eligible)
	assert.Equal(t, []string{"admin"}, claims.Roles)
	assert.Equal(t, "alice@example.com", claims.Email)
}

func TestVerifier_ReturnsNotMineForDifferentIssuer(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", "https://other-issuer.example.com", "obot-default", "user-1", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	assert.True(t, errors.Is(err, ErrNotMyToken))
}

func TestVerifier_RejectsWrongAudience(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{IssuerURL: issuer.URL, Audience: "obot-default", EligibilityClaimName: "eligible", RolesClaimName: "roles"}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "wrong-aud", "user-1", 60*time.Second, nil)
	_, err = v.Verify(context.Background(), tok)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotMyToken))
}

func TestVerifier_UsesCanonicalConfiguredIssuer(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL + "/",
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": true,
		"roles":    []string{"admin"},
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, issuer.URL, claims.Issuer)
}

func TestVerifier_ExtractsFullProviderProfile(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	emailVerified := true
	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible":           true,
		"roles":              []string{"admin"},
		"email":              "alice@example.com",
		"email_verified":     emailVerified,
		"preferred_username": "alice",
		"name":               "Alice Example",
		"picture":            "https://example.com/alice.png",
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", claims.Email)
	require.NotNil(t, claims.EmailVerified)
	assert.True(t, *claims.EmailVerified)
	assert.Equal(t, "alice", claims.PreferredUsername)
	assert.Equal(t, "Alice Example", claims.Name)
	assert.Equal(t, "https://example.com/alice.png", claims.Picture)
}

func TestVerifier_EligibilityArrayRequiresStringValue(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": []int{1},
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.False(t, claims.Eligible)
}

func TestVerifier_EligibilityArrayAcceptsNonEmptyString(t *testing.T) {
	priv := testutil.MustRSAKey(t)
	issuer, cleanup := testutil.NewTestIssuer(t, priv, "kid-X")
	defer cleanup()

	cfg := Config{
		IssuerURL:            issuer.URL,
		Audience:             "obot-default",
		EligibilityClaimName: "eligible",
		RolesClaimName:       "roles",
	}
	v, err := NewVerifier(context.Background(), cfg)
	require.NoError(t, err)

	tok := testutil.MintTestJWT(t, priv, "kid-X", issuer.URL, "obot-default", "user-1", 60*time.Second, map[string]any{
		"eligible": []string{"studio"},
	})
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	assert.True(t, claims.Eligible)
}
