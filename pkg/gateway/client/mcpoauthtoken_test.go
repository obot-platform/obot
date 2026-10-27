package client

import (
	"crypto/aes"
	"strings"
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

func TestReplaceMCPOAuthTokenPreservesScopes(t *testing.T) {
	for _, test := range []struct {
		name     string
		scopes   []string
		authURL  string
		tokenURL string
		url      string
		stale    bool
	}{
		{
			name:   "overlapping authorization",
			scopes: []string{"other"},
			stale:  true,
		},
		{
			name:   "stale refresh",
			scopes: []string{"read"},
			stale:  true,
		},
		{
			name:   "reordered duplicate scopes",
			scopes: []string{"write", "read", "write"},
		},
		{
			name:   "expanded grant",
			scopes: []string{"read", "write", "other"},
		},
		{
			name:    "changed authorization server",
			scopes:  []string{"other"},
			authURL: "https://other.test/authorize",
		},
		{
			name:     "changed token endpoint",
			scopes:   []string{"other"},
			tokenURL: "https://other.test/token",
		},
		{
			name:   "changed MCP target",
			scopes: []string{"other"},
			url:    "https://other.test/mcp",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t)
			conf := &oauth2.Config{
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://provider.test/authorize",
					TokenURL: "https://provider.test/token",
				},
				Scopes: []string{"read", "write"},
			}
			mcpURL := "https://provider.test/mcp"
			require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user", "mcp", mcpURL, "first", conf, &oauth2.Token{AccessToken: "first"}))
			original, err := client.GetMCPOAuthToken(t.Context(), "user", "mcp", mcpURL)
			require.NoError(t, err)

			conf.Scopes = test.scopes
			if test.authURL != "" {
				conf.Endpoint.AuthURL = test.authURL
			}

			if test.tokenURL != "" {
				conf.Endpoint.TokenURL = test.tokenURL
			}

			if test.url != "" {
				mcpURL = test.url
			}

			err = client.ReplaceMCPOAuthToken(t.Context(), "user", "mcp", mcpURL, "second", conf, &oauth2.Token{AccessToken: "second"})
			if test.stale {
				require.ErrorContains(t, err, "start a new authorization")
			} else {
				require.NoError(t, err)
			}

			stored, err := client.GetMCPOAuthToken(t.Context(), "user", "mcp", mcpURL)
			require.NoError(t, err)
			if test.stale {
				require.Equal(t, original, stored)
			} else {
				require.Equal(t, "second", stored.AccessToken)
				require.Equal(t, strings.Join(test.scopes, " "), stored.Scopes)
			}
		})
	}
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

func TestMCPOAuthDiscoveryRevision(t *testing.T) {
	client := newTestClient(t)
	revision := func(userID string, ids ...string) string {
		t.Helper()
		revision, err := client.MCPOAuthDiscoveryRevision(t.Context(), userID, ids)
		require.NoError(t, err)
		return revision
	}
	absent := revision("user1", "one", "two")
	require.NotEqual(t, absent, revision("user2", "one", "two"))

	conf := &oauth2.Config{Scopes: []string{"read"}}
	token := &oauth2.Token{AccessToken: "first"}
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user1", "one", "https://one.test", "grant1", conf, token))
	granted := revision("user1", "one", "two")
	require.NotEqual(t, absent, granted)
	require.Equal(t, granted, revision("user1", "two", "one"))

	token.AccessToken = "refreshed"
	stored, err := client.GetMCPOAuthToken(t.Context(), "user1", "one", "https://one.test")
	require.NoError(t, err)
	require.NoError(t, client.RefreshMCPOAuthToken(t.Context(), "user1", "one", "https://one.test", stored.GrantID, conf, token))
	require.Equal(t, granted, revision("user1", "one", "two"))
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user2", "one", "https://one.test", "other-user", conf, token))
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user1", "unrelated", "https://other.test", "other-server", conf, token))
	require.Equal(t, granted, revision("user1", "one", "two"))

	conf.Scopes = []string{"read", "write"}
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user1", "one", "https://one.test", "grant2", conf, token))
	require.NotEqual(t, granted, revision("user1", "one", "two"))
	require.NoError(t, client.DeleteMCPOAuthTokens(t.Context(), "user1", "one"))
	require.Equal(t, absent, revision("user1", "one", "two"))
}

func TestMCPOAuthGrantIdentitySurvivesRefreshButChangesOnLogin(t *testing.T) {
	client := newTestClient(t)
	conf := &oauth2.Config{Scopes: []string{"read"}}
	token := &oauth2.Token{AccessToken: "account-one"}
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test", "", conf, token, "challenge"))
	first, err := client.GetMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test")
	require.NoError(t, err)
	before, err := client.MCPOAuthDiscoveryRevision(t.Context(), "user", []string{"mcp"})
	require.NoError(t, err)

	token.AccessToken = "refreshed"
	require.NoError(t, client.RefreshMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test", first.GrantID, conf, token))
	after, err := client.MCPOAuthDiscoveryRevision(t.Context(), "user", []string{"mcp"})
	require.NoError(t, err)
	require.Equal(t, before, after)
	refreshed, err := client.GetMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test")
	require.NoError(t, err)
	require.Equal(t, "challenge", refreshed.UpstreamChallenge)

	token.AccessToken = "account-two"
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test", "", conf, token))
	after, err = client.MCPOAuthDiscoveryRevision(t.Context(), "user", []string{"mcp"})
	require.NoError(t, err)
	require.NotEqual(t, before, after)
	require.ErrorContains(t, client.RefreshMCPOAuthToken(t.Context(), "user", "mcp", "https://mcp.test", first.GrantID, conf, token), "grant changed")
}
