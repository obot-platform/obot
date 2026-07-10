package persistent

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServerURL = "https://obot.example.com"

func newTestTokenService(t *testing.T) *TokenService {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tokenService, err := NewTokenService(testServerURL, nil)
	require.NoError(t, err)
	require.NoError(t, tokenService.replaceKey(t.Context(), privateKey))

	return tokenService
}

func TestDecodeTokenUsesUserGroupsForAPIAudience(t *testing.T) {
	tokenService := newTestTokenService(t)

	_, token, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		IssuedAt:   NewTime(time.Now().Add(-time.Minute)),
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     "123",
		UserName:   "test-user",
		UserEmail:  "test@example.com",
		UserGroups: []string{types.GroupAdmin, "", types.GroupAuthenticated},
	})
	require.NoError(t, err)

	tokenContext, err := tokenService.DecodeToken(t.Context(), token)
	require.NoError(t, err)

	assert.Equal(t, StringSlice([]string{types.GroupAdmin, types.GroupAuthenticated}), tokenContext.UserGroups)
	assert.Equal(t, testServerURL, tokenContext.Audience)
	assert.Equal(t, "123", tokenContext.UserID)
	assert.Equal(t, "test-user", tokenContext.UserName)
	assert.Equal(t, "test@example.com", tokenContext.UserEmail)
}

func TestDecodeTokenUsesMCPGroupsForMCPConnectAudience(t *testing.T) {
	tokenService := newTestTokenService(t)
	audience := testServerURL + "/mcp-connect/server-id"

	_, token, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   audience,
		IssuedAt:   NewTime(time.Now().Add(-time.Minute)),
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     "123",
		UserGroups: []string{types.GroupMCP, types.GroupAuthenticated},
	})
	require.NoError(t, err)

	tokenContext, err := tokenService.DecodeToken(t.Context(), token)
	require.NoError(t, err)

	assert.Equal(t, StringSlice([]string{types.GroupMCP, types.GroupAuthenticated}), tokenContext.UserGroups)
	assert.Equal(t, audience, tokenContext.Audience)
}

func TestNewTokenRejectsMissingAudience(t *testing.T) {
	tokenService := newTestTokenService(t)

	_, _, err := tokenService.NewToken(t.Context(), TokenContext{
		ExpiresAt: NewTime(time.Now().Add(time.Hour)),
		IssuedAt:  NewTime(time.Now().Add(-time.Minute)),
		UserID:    "123",
	})
	require.ErrorContains(t, err, "audience is required")
}

func TestDecodeTokenRejectsMissingAudience(t *testing.T) {
	tokenService := newTestTokenService(t)

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, TokenContext{
		Issuer:    testServerURL,
		ExpiresAt: NewTime(time.Now().Add(time.Hour)),
		IssuedAt:  NewTime(time.Now().Add(-time.Minute)),
		UserID:    "123",
	})
	signedToken, err := token.SignedString(tokenService.privateKey)
	require.NoError(t, err)

	_, err = tokenService.DecodeToken(t.Context(), signedToken)
	require.ErrorContains(t, err, "no audience")
}

func TestNewTokenUsesStandardOAuthClaims(t *testing.T) {
	tokenService := newTestTokenService(t)

	token, signedToken, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		IssuedAt:   NewTime(time.Now().Add(-time.Minute)),
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     "123",
		UserName:   "test-user",
		UserEmail:  "test@example.com",
		OAuthScope: "profile email",
		Picture:    "https://example.com/avatar.png",
	})
	require.NoError(t, err)

	claimsJSON, err := json.Marshal(token.Claims)
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(claimsJSON, &claims))

	assert.Equal(t, "test-user", claims["name"])
	assert.Equal(t, "test@example.com", claims["email"])
	assert.Equal(t, "profile email", claims["scope"])
	assert.Equal(t, "https://example.com/avatar.png", claims["picture"])
	assert.NotContains(t, claims, "UserName")
	assert.NotContains(t, claims, "UserEmail")
	assert.NotContains(t, claims, "OAuthScope")

	decoded, err := tokenService.DecodeToken(t.Context(), signedToken)
	require.NoError(t, err)
	assert.Equal(t, "test-user", decoded.UserName)
	assert.Equal(t, "test@example.com", decoded.UserEmail)
	assert.Equal(t, "profile email", decoded.OAuthScope)
	assert.Equal(t, "https://example.com/avatar.png", decoded.Picture)
}

func TestDecodeTokenSupportsLegacyUserClaims(t *testing.T) {
	tokenService := newTestTokenService(t)
	now := time.Now()
	legacyClaims := jwt.MapClaims{
		"iss":       testServerURL,
		"aud":       testServerURL,
		"iat":       now.Add(-time.Minute).Unix(),
		"exp":       now.Add(time.Hour).Unix(),
		"sub":       "123",
		"UserName":  "legacy-user",
		"UserEmail": "legacy@example.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, legacyClaims)
	signedToken, err := token.SignedString(tokenService.privateKey)
	require.NoError(t, err)

	decoded, err := tokenService.DecodeToken(t.Context(), signedToken)
	require.NoError(t, err)
	assert.Equal(t, "legacy-user", decoded.UserName)
	assert.Equal(t, "legacy@example.com", decoded.UserEmail)
}
