package mcpgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestGatewayTokenContextScopesAuthenticatedUserToMCPServer(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	authenticatedUser := &user.DefaultInfo{
		Name: "Studio User",
		UID:  "42",
		Extra: map[string][]string{
			"email":                   {"studio@example.test"},
			"auth_provider_name":      {"generic-oauth-auth-provider"},
			"auth_provider_namespace": {system.DefaultNamespace},
			"auth_provider_user_id":   {"studio-user-1"},
		},
	}
	server := mcp.ServerConfig{
		MCPServerName: "ms1server",
	}

	got := gatewayTokenContext(
		authenticatedUser,
		server,
		"https://obot.example.test/mcp-connect/ms1server",
		now,
	)

	require.Equal(t, "42", got.UserID)
	require.Equal(t, "Studio User", got.UserName)
	require.Equal(t, "studio@example.test", got.UserEmail)
	require.Equal(t, "ms1server", got.MCPID)
	require.Equal(t, "https://obot.example.test/mcp-connect/ms1server", got.Audience)
	require.Equal(t, []string{"mcp", "authenticated"}, []string(got.UserGroups))
	require.Equal(t, "generic-oauth-auth-provider", got.AuthProviderName)
	require.Equal(t, system.DefaultNamespace, got.AuthProviderNamespace)
	require.Equal(t, "studio-user-1", got.AuthProviderUserID)
	require.Equal(t, now, got.IssuedAt.Time)
	require.Equal(t, now.Add(gatewayTokenExpiration), got.ExpiresAt.Time)
}

func TestGatewayAuthorizationOmitsCredentialsWhenAuthenticationIsDisabled(t *testing.T) {
	mintCalled := false
	handler := Handler{
		mintToken: func(context.Context, persistent.TokenContext) (string, error) {
			mintCalled = true
			return "unexpected", nil
		},
	}

	got, err := handler.gatewayToken(t.Context(), &user.DefaultInfo{}, mcp.ServerConfig{})

	require.NoError(t, err)
	require.Empty(t, got)
	require.False(t, mintCalled)
}

func TestGatewayTokenUsesExactServerAudience(t *testing.T) {
	server := mcp.ServerConfig{
		MCPServerName: "ms1server",
		Audiences: []string{
			"https://obot.example.test/mcp-connect/catalog-entry",
			"https://obot.example.test/mcp-connect/ms1server",
		},
	}
	var minted persistent.TokenContext
	handler := Handler{
		mintToken: func(_ context.Context, tokenContext persistent.TokenContext) (string, error) {
			minted = tokenContext
			return "scoped-gateway-token", nil
		},
	}

	got, err := handler.gatewayToken(t.Context(), &user.DefaultInfo{UID: "42"}, server)

	require.NoError(t, err)
	require.Equal(t, "scoped-gateway-token", got)
	require.Equal(t, "ms1server", minted.MCPID)
	require.Equal(t, "https://obot.example.test/mcp-connect/ms1server", minted.Audience)
	require.WithinDuration(t, minted.IssuedAt.Add(gatewayTokenExpiration), minted.ExpiresAt.Time, time.Second)
}

func TestGatewayTokenUsesExactServerAudienceBelowBasePath(t *testing.T) {
	server := mcp.ServerConfig{
		MCPServerName: "ms1server",
		Audiences: []string{
			"https://studio.example.test/obot/mcp-connect/catalog-entry",
			"https://studio.example.test/obot/mcp-connect/ms1server",
		},
	}
	var minted persistent.TokenContext
	handler := Handler{
		mintToken: func(_ context.Context, tokenContext persistent.TokenContext) (string, error) {
			minted = tokenContext
			return "scoped-gateway-token", nil
		},
	}

	got, err := handler.gatewayToken(t.Context(), &user.DefaultInfo{UID: "42"}, server)

	require.NoError(t, err)
	require.Equal(t, "scoped-gateway-token", got)
	require.Equal(t, "ms1server", minted.MCPID)
	require.Equal(t, "https://studio.example.test/obot/mcp-connect/ms1server", minted.Audience)
}

func TestProxyReplacesInboundStudioBearerForNanobotAgent(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	nanobot := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
		receivedAuthorization <- request.Header.Get("Authorization")
		rw.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(nanobot.Close)
	handler := Handler{
		transport: http.DefaultTransport,
		resolveServer: func(api.Context) (mcp.ServerConfig, error) {
			return mcp.ServerConfig{
				URL:              nanobot.URL,
				MCPServerName:    "ms1server",
				NanobotAgentName: "studio-agent",
				Audiences:        []string{"https://obot.example.test/mcp-connect/ms1server"},
			}, nil
		},
		mintToken: func(context.Context, persistent.TokenContext) (string, error) {
			return "scoped-gateway-token", nil
		},
	}
	req, response := authenticatedProxyContext()

	err := handler.Proxy(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "Bearer scoped-gateway-token", <-receivedAuthorization)
}

func TestProxyReplacesInboundStudioBearerForEmbeddedNanobot(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	receivedToken := make(chan string, 1)
	handler := Handler{
		resolveServer: func(api.Context) (mcp.ServerConfig, error) {
			return mcp.ServerConfig{
				MCPServerName: "ms1server",
				Audiences:     []string{"https://obot.example.test/mcp-connect/ms1server"},
			}, nil
		},
		mintToken: func(context.Context, persistent.TokenContext) (string, error) {
			return "scoped-gateway-token", nil
		},
		nanobot: http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
			receivedAuthorization <- request.Header.Get("Authorization")
			receivedToken <- nmcp.TokenFromContext(request.Context())
			rw.WriteHeader(http.StatusNoContent)
		}),
	}
	req, response := authenticatedProxyContext()

	err := handler.Proxy(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "Bearer scoped-gateway-token", <-receivedAuthorization)
	require.Equal(t, "scoped-gateway-token", <-receivedToken)
}

func TestProxyRemovesInboundStudioBearerWhenAuthenticationIsDisabled(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	handler := Handler{
		resolveServer: func(api.Context) (mcp.ServerConfig, error) {
			return mcp.ServerConfig{MCPServerName: "ms1server"}, nil
		},
		mintToken: func(context.Context, persistent.TokenContext) (string, error) {
			return "unexpected", nil
		},
		nanobot: http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
			receivedAuthorization <- request.Header.Get("Authorization")
			require.Empty(t, nmcp.TokenFromContext(request.Context()))
			rw.WriteHeader(http.StatusNoContent)
		}),
	}
	req, response := authenticatedProxyContext()

	err := handler.Proxy(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Empty(t, <-receivedAuthorization)
}

func TestProxyRemovesInboundStudioBearerForNanobotAgentWhenAuthenticationIsDisabled(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	nanobot := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, request *http.Request) {
		receivedAuthorization <- request.Header.Get("Authorization")
		rw.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(nanobot.Close)
	handler := Handler{
		transport: http.DefaultTransport,
		resolveServer: func(api.Context) (mcp.ServerConfig, error) {
			return mcp.ServerConfig{
				URL:              nanobot.URL,
				MCPServerName:    "ms1server",
				NanobotAgentName: "studio-agent",
			}, nil
		},
		mintToken: func(context.Context, persistent.TokenContext) (string, error) {
			return "unexpected", nil
		},
	}
	req, response := authenticatedProxyContext()

	err := handler.Proxy(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Empty(t, <-receivedAuthorization)
}

func authenticatedProxyContext() (api.Context, *httptest.ResponseRecorder) {
	request := httptest.NewRequest(http.MethodPost, "http://obot.test/mcp-connect/ms1server", nil)
	request.Header.Set("Authorization", "Bearer studio-iat")
	request.SetPathValue("mcp_id", "ms1server")
	response := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: response,
		Request:        request,
		User: &user.DefaultInfo{
			UID:    "42",
			Groups: []string{types.GroupAuthenticated},
		},
	}, response
}
