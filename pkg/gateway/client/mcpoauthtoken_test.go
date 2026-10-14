package client

import (
	"context"
	"crypto/aes"
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	encryptaes "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

func TestCompleteMCPOAuthPendingState(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	conf := &oauth2.Config{ClientID: "client"}
	token := &oauth2.Token{AccessToken: "access", TokenType: "Bearer"}
	require.NoError(t, c.CreateMCPOAuthPendingState(ctx, "user", "mcp", "https://upstream.example/mcp", "request", "state", "verifier", "", conf))
	ps, err := c.GetMCPOAuthPendingState(ctx, "state")
	require.NoError(t, err)

	triggerCount := 0
	c.mcpOAuthTokenTrigger = func(ctx context.Context, mcpID string) error {
		triggerCount++
		// Notifications must run after the transaction commits.
		_, err := c.GetMCPOAuthPendingState(ctx, "state")
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		stored, err := c.GetMCPOAuthToken(ctx, "user", mcpID, ps.URL)
		require.NoError(t, err)
		require.Equal(t, token.AccessToken, stored.AccessToken)
		return nil
	}
	require.NoError(t, c.CompleteMCPOAuthPendingState(ctx, ps, conf, token))
	require.Equal(t, 1, triggerCount)
	require.ErrorIs(t, c.CompleteMCPOAuthPendingState(ctx, ps, conf, &oauth2.Token{AccessToken: "replayed"}), gorm.ErrRecordNotFound)
	require.Equal(t, 1, triggerCount)
	stored, err := c.GetMCPOAuthToken(ctx, "user", "mcp", ps.URL)
	require.NoError(t, err)
	require.Equal(t, "access", stored.AccessToken)

	c.mcpOAuthTokenTrigger = nil
	require.NoError(t, c.DeleteMCPOAuthTokenForAllUsers(ctx, "mcp"))
	_, err = c.GetMCPOAuthToken(ctx, "user", "mcp", ps.URL)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestCompleteMCPOAuthPendingStateRollsBackOnSaveFailure(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	conf := &oauth2.Config{ClientID: "client"}
	require.NoError(t, c.CreateMCPOAuthPendingState(ctx, "user", "mcp", "https://upstream.example/mcp", "request", "state", "verifier", "", conf))
	ps, err := c.GetMCPOAuthPendingState(ctx, "state")
	require.NoError(t, err)
	saveErr := errors.New("token save failed")
	require.NoError(t, c.db.WithContext(ctx).Callback().Create().Before("gorm:create").Register("test:fail_token_save", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "MCPOAuthToken" {
			_ = tx.AddError(saveErr)
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, c.db.WithContext(ctx).Callback().Create().Remove("test:fail_token_save"))
	})
	c.mcpOAuthTokenTrigger = func(context.Context, string) error {
		t.Error("token change triggered for a failed save")
		return nil
	}
	require.ErrorIs(t, c.CompleteMCPOAuthPendingState(ctx, ps, conf, &oauth2.Token{AccessToken: "access"}), saveErr)
	_, err = c.GetMCPOAuthPendingState(ctx, "state")
	require.NoError(t, err)
	_, err = c.GetMCPOAuthToken(ctx, "user", "mcp", ps.URL)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestCopyMCPOAuthTokens(t *testing.T) {
	client := newTestClient(t)
	block, err := aes.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	transformer, err := encryptaes.NewGCMTransformer(block)
	require.NoError(t, err)
	client.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{mcpOAuthTokenGroupResource: transformer},
	}
	config := &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://obot.example/callback",
		Scopes:       []string{"read", "write"},
		Endpoint:     oauth2.Endpoint{TokenURL: "https://upstream.example/token"},
	}
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer"}
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user1", "old", "https://upstream.example/mcp", "request1", config, token))
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user2", "old", "https://upstream.example/mcp", "request2", config, token))
	require.NoError(t, client.CopyMCPOAuthTokens(t.Context(), "user1", "old", "new"))
	oldToken, err := client.GetMCPOAuthToken(t.Context(), "user1", "old", "https://upstream.example/mcp")
	require.NoError(t, err)
	newToken, err := client.GetMCPOAuthToken(t.Context(), "user1", "new", "https://upstream.example/mcp")
	require.NoError(t, err)
	oldToken.MCPID = "new"
	require.Equal(t, oldToken, newToken)

	var stored types.MCPOAuthToken
	require.NoError(t, client.db.WithContext(t.Context()).Where("mcp_id = ? AND user_id = ?", "new", "user1").First(&stored).Error)
	require.True(t, stored.Encrypted)
	require.NotEqual(t, token.AccessToken, stored.AccessToken)
	require.NotEqual(t, config.ClientSecret, stored.ClientSecret)
	// GCM binds ciphertext to the MCP ID, so copying encrypted bytes directly
	// would either fail above or still decrypt under the old ID here.
	stored.MCPID = "old"
	require.Error(t, client.decryptMCPOAuthToken(t.Context(), &stored))
	_, err = client.GetMCPOAuthToken(t.Context(), "user2", "new", "https://upstream.example/mcp")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	token.AccessToken = "refreshed-destination"
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user1", "new", "https://upstream.example/mcp", "request1", config, token))
	require.NoError(t, client.CopyMCPOAuthTokens(t.Context(), "user1", "old", "new"))
	newToken, err = client.GetMCPOAuthToken(t.Context(), "user1", "new", "https://upstream.example/mcp")
	require.NoError(t, err)
	require.Equal(t, "refreshed-destination", newToken.AccessToken)
	require.NoError(t, client.CopyMCPOAuthTokens(t.Context(), "user1", "missing", "unused"))
}

func TestCopyMCPOAuthTokensRetainsSourceOnDecryptionFailure(t *testing.T) {
	client := newTestClient(t)
	block, err := aes.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	transformer, err := encryptaes.NewGCMTransformer(block)
	require.NoError(t, err)
	client.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{mcpOAuthTokenGroupResource: transformer},
	}
	source := types.MCPOAuthToken{
		MCPID:       "old",
		UserID:      "user1",
		AccessToken: "not-valid-ciphertext",
		Encrypted:   true,
	}
	require.NoError(t, client.db.WithContext(t.Context()).Create(&source).Error)
	require.ErrorContains(t, client.CopyMCPOAuthTokens(t.Context(), "user1", "old", "new"), "decrypt source")
	var stored []types.MCPOAuthToken
	require.NoError(t, client.db.WithContext(t.Context()).Find(&stored).Error)
	require.Equal(t, []types.MCPOAuthToken{source}, stored)
}
