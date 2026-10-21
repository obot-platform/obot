package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeLLMCredential(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	user := types.User{Username: "claude", HashedUsername: "claude", Email: "claude@example.com"}
	require.NoError(t, c.db.WithContext(ctx).Create(&user).Error)

	mcp, err := c.EnsureClaudeCredential(ctx, user.ID, "acr-test", []string{"vmcp_test"}, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	llm, err := c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, ClaudeLLMCredentialContext, llm.Context)
	require.NotEqual(t, mcp.Secrets["keyID"], llm.Secrets["keyID"], "the session's keys are separate")
	key, err := c.ValidateAPIKey(ctx, llm.Secrets["key"])
	require.NoError(t, err)
	require.Equal(t, user.ID, key.UserID)
	require.Equal(t, types.APIKeyScopes{CanAccessLLMProxy: true}, key.APIKeyScopes)
	require.True(t, key.ClaudeEnvironment)

	// A revoked LLM key stays as the session's revocation marker through retention cleanup.
	require.NoError(t, c.RevokeClaudeCredentials(ctx, "acr-test"))
	_, err = c.ValidateAPIKey(ctx, llm.Secrets["key"])
	require.ErrorIs(t, err, ErrInvalidAPIKey)
	require.NoError(t, c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", key.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	require.NoError(t, c.deleteOldRevokedAPIKeys(ctx, time.Now().AddDate(0, 0, 100), 90))
	_, err = c.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	_, err = c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(24*time.Hour))
	require.ErrorIs(t, err, ErrClaudeCredentialRevoked)

	_, err = c.DeleteCredential(ctx, ClaudeLLMCredentialContext, "acr-test")
	require.NoError(t, err)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	_, err = c.GetAPIKeyByID(ctx, key.ID)
	require.Error(t, err)
}
