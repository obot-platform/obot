package persistent

import (
	"crypto/ed25519"
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

	token, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL,
		IssuedAt:   time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Hour),
		UserID:     "123",
		UserName:   "test-user",
		UserEmail:  "test@example.com",
		UserGroups: []string{types.GroupAdmin, "", types.GroupAuthenticated},
	})
	require.NoError(t, err)

	tokenContext, err := tokenService.DecodeToken(t.Context(), token)
	require.NoError(t, err)

	assert.Equal(t, []string{types.GroupAdmin, types.GroupAuthenticated}, tokenContext.UserGroups)
	assert.Equal(t, testServerURL, tokenContext.Audience)
	assert.Equal(t, "123", tokenContext.UserID)
	assert.Equal(t, "test-user", tokenContext.UserName)
	assert.Equal(t, "test@example.com", tokenContext.UserEmail)
}

func TestDecodeTokenUsesMCPGroupsForMCPConnectAudience(t *testing.T) {
	tokenService := newTestTokenService(t)
	audience := testServerURL + "/mcp-connect/server-id"

	token, err := tokenService.NewToken(t.Context(), TokenContext{
		Audience:   audience,
		IssuedAt:   time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Hour),
		UserID:     "123",
		UserGroups: []string{types.GroupAdmin, types.GroupOwner, types.GroupAuthenticated},
	})
	require.NoError(t, err)

	tokenContext, err := tokenService.DecodeToken(t.Context(), token)
	require.NoError(t, err)

	assert.Equal(t, []string{types.GroupMCP, types.GroupAuthenticated}, tokenContext.UserGroups)
	assert.Equal(t, audience, tokenContext.Audience)
}

func TestDecodeTokenRejectsMissingAudience(t *testing.T) {
	tokenService := newTestTokenService(t)

	_, token, err := tokenService.NewTokenWithClaims(t.Context(), jwt.MapClaims{
		"aud": nil,
		"exp": float64(time.Now().Add(time.Hour).Unix()),
		"iat": float64(time.Now().Add(-time.Minute).Unix()),
		"sub": "123",
	})
	require.NoError(t, err)

	_, err = tokenService.DecodeToken(t.Context(), token)
	require.ErrorContains(t, err, "no audience")
}
