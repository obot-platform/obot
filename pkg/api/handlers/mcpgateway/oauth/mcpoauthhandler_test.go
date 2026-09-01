package oauth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type staticOAuthTokenStorage struct {
	config *oauth2.Config
	token  *oauth2.Token
}

type staticOAuthGlobalTokenStore struct {
	storage mcp.TokenStorage
}

func TestNewMCPOAuthHandlerFactoryConfiguresCIMD(t *testing.T) {
	tests := []struct {
		name               string
		baseURL            string
		forceDynamicClient bool
		want               string
	}{
		{
			name:    "enabled by default for HTTPS",
			baseURL: "https://obot.example.com",
			want:    system.OAuthClientIDMetadataURL("https://obot.example.com"),
		},
		{
			name:               "disabled when dynamic registration is forced",
			baseURL:            "https://obot.example.com",
			forceDynamicClient: true,
		},
		{
			name:    "disabled for HTTP",
			baseURL: "http://obot.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := NewMCPOAuthHandlerFactory(tt.baseURL, nil, nil, nil, nil, "", tt.forceDynamicClient)
			require.Equal(t, tt.want, factory.cimdDocumentURL)
		})
	}
}

func (s *staticOAuthTokenStorage) TokenSource(context.Context) (oauth2.TokenSource, error) {
	if s.config == nil || s.token == nil {
		return nil, nil
	}
	return oauth2.StaticTokenSource(s.token), nil
}

func (s *staticOAuthTokenStorage) GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error) {
	return s.config, s.token, nil
}

func (*staticOAuthTokenStorage) SetTokenConfig(context.Context, *oauth2.Config, *oauth2.Token) error {
	return nil
}

func (*staticOAuthTokenStorage) DeleteTokenConfig(context.Context) error {
	return nil
}

func (s staticOAuthGlobalTokenStore) ForUserAndMCP(string, string, string) mcp.TokenStorage {
	return s.storage
}

func TestStaticOAuthPendingUsesStoredAuthentication(t *testing.T) {
	storage := &staticOAuthTokenStorage{}
	factory := MCPOAuthHandlerFactory{tokenStore: staticOAuthGlobalTokenStore{storage: storage}}
	handler := &mcpOAuthHandler{userID: "user", mcpID: "server", mcpURL: "https://example.com/mcp"}
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					StaticOAuthRequired: true,
				},
			},
		},
	}

	pending, err := factory.staticOAuthPending(t.Context(), server, handler)
	require.NoError(t, err)
	require.True(t, pending)

	storage.config = &oauth2.Config{ClientID: "client"}
	storage.token = &oauth2.Token{}
	pending, err = factory.staticOAuthPending(t.Context(), server, handler)
	require.NoError(t, err)
	require.True(t, pending)

	storage.token = &oauth2.Token{AccessToken: "token"}
	pending, err = factory.staticOAuthPending(t.Context(), server, handler)
	require.NoError(t, err)
	require.False(t, pending)
}

func TestNewMCPOAuthHandlerCarriesCatalogEntry(t *testing.T) {
	factory := &MCPOAuthHandlerFactory{}

	// Tool preview servers are never written to storage, so the catalog entry name is the
	// only thing tying them back to the credentials the admin configured.
	handler := factory.newMCPOAuthHandler(nil, "user", "tool-preview-63cb5f4336ed0dcf", "https://example.com/mcp", "", "slack-entry")
	require.Equal(t, "slack-entry", handler.catalogEntryName)
}

func TestStaticOAuthMetadataDefaultsMissingClientRegistration(t *testing.T) {
	authorizationServer := mcp.AuthorizationServerMetadata{
		Issuer:                            "https://auth.example.com",
		AuthorizationEndpoint:             "https://auth.example.com/authorize",
		TokenEndpoint:                     "https://auth.example.com/token",
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		ResponseTypesSupported:            []string{"code"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	}
	authorizationServerJSON, err := json.Marshal(authorizationServer)
	require.NoError(t, err)

	parsedAuthorizationServer, registration, err := staticOAuthMetadata(mcp.OAuthMetadata{
		AuthorizationServerMetadata: authorizationServerJSON,
	}, "https://obot.example.com/oauth/mcp/callback")
	require.NoError(t, err)
	require.Equal(t, authorizationServer, parsedAuthorizationServer)
	require.Equal(t, mcp.ClientRegistrationMetadata{
		RedirectURIs:            []string{"https://obot.example.com/oauth/mcp/callback"},
		TokenEndpointAuthMethod: "client_secret_post",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		ClientName:              "Obot MCP Gateway",
	}, registration)
}

func TestStaticOAuthMetadataReportsMissingAuthorizationServer(t *testing.T) {
	_, _, err := staticOAuthMetadata(mcp.OAuthMetadata{}, "https://obot.example.com/oauth/mcp/callback")
	require.EqualError(t, err, "static OAuth is required but authorization server metadata was not found")
}
