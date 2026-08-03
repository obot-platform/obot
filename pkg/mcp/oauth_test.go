package mcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestRequiresStaticOAuth(t *testing.T) {
	server := v1.MCPServer{}
	server.Spec.Manifest.Runtime = types.RuntimeRemote
	server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{StaticOAuthRequired: true}

	require.True(t, RequiresStaticOAuth(server))

	server.Status.UserHasAuthenticated = true
	require.False(t, RequiresStaticOAuth(server))

	server.Status.UserHasAuthenticated = false
	server.Spec.Manifest.RemoteConfig.StaticOAuthRequired = false
	require.False(t, RequiresStaticOAuth(server))

	server.Spec.Manifest.Runtime = types.RuntimeComposite
	server.Spec.Manifest.RemoteConfig.StaticOAuthRequired = true
	require.False(t, RequiresStaticOAuth(server))
}

func TestGetOAuthMetadataAssumesOAuthAfterSuccessfulInitialize(t *testing.T) {
	var initializeRequests atomic.Int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/mcp":
			initializeRequests.Add(1)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"test","version":"1"}}}`))
		case strings.Contains(req.URL.Path, "oauth-protected-resource"):
			rw.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(rw, `{"resource":%q,"authorization_servers":[%q]}`, serverURL+"/mcp", serverURL)
		case req.URL.Path == "/.well-known/oauth-authorization-server":
			rw.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(rw, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"response_types_supported":["code"]}`, serverURL, serverURL+"/authorize", serverURL+"/token")
		default:
			http.NotFound(rw, req)
		}
	}))
	serverURL = server.URL
	t.Cleanup(server.Close)

	config := ServerConfig{Runtime: types.RuntimeRemote, URL: server.URL + "/mcp"}
	metadata, err := getOAuthMetadataWithClient(t.Context(), server.Client(), config, "test", "https://obot.example/oauth/mcp/callback", false)
	require.NoError(t, err)
	require.Empty(t, metadata.AuthorizationServerMetadata)
	require.Equal(t, int32(1), initializeRequests.Load())

	metadata, err = getOAuthMetadataWithClient(t.Context(), server.Client(), config, "test", "https://obot.example/oauth/mcp/callback", true)
	require.NoError(t, err)
	require.NotEmpty(t, metadata.AuthorizationServerMetadata)
	require.NotEmpty(t, metadata.ClientRegistration)
	// Forced discovery must not create a real MCP session as a side effect.
	require.Equal(t, int32(1), initializeRequests.Load())
}
