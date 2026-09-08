package client

import (
	"crypto/aes"
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
