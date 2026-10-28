package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
)

func TestClaudeAuthorizationLinksReuseOAuthAcrossReplicas(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "Gmail", Version: "test"}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	var providerURL string
	var providerRequests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": providerURL + "/mcp", "authorization_servers": []string{providerURL}})
		case strings.HasPrefix(r.URL.Path, "/.well-known/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": providerURL, "authorization_endpoint": providerURL + "/authorize", "token_endpoint": providerURL + "/token", "registration_endpoint": providerURL + "/register", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}})
		case r.URL.Path == "/register":
			_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "provider-client", "token_endpoint_auth_method": "none"})
		default:
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+providerURL+`/.well-known/oauth-protected-resource"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			transport.ServeHTTP(w, r)
		}
	}))
	defer provider.Close()
	providerURL = provider.URL

	vmcp := vmcpComponentVMCP(false)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest = types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: providerURL + "/mcp"},
	}
	instance := syncedVMCPInstance(vmcpComponentInstance("vmcpi1elicit", vmcp.Name), vmcp)
	component := vmcpComponentServer("ms1elicit", "", vmcp.Name)
	component.Spec.Manifest = types.MCPServerManifest{
		Name:         "Gmail",
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{URL: providerURL + "/mcp"},
	}
	connection := syncedVMCPConnection(&v1.MCPServerInstance{
		Name:      "msi1elicit",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			MCPServerName:   component.Name,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "component",
			UserID:          "42",
		},
	}, vmcp, instance)
	storage := &vmcpOAuthInitialEventsClient{WithWatch: vmcpConsentStorage(vmcp, instance, component, connection)}
	gateway := vmcpConsentGateway(t)
	newReplica := func() (*MCPOAuthHandlerFactory, mcp.GlobalTokenStore) {
		tokens := mcp.NewGlobalTokenStore(gateway)
		manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
			mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
			&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
		require.NoError(t, err)
		return NewMCPOAuthHandlerFactory("https://obot.example", manager, storage, gateway, tokens, "", false), tokens
	}
	first, _ := newReplica()
	second, callbackTokens := newReplica()
	req := vmcpComponentRequest(storage, instance.Name, component.Name)
	req.GatewayClient = gateway

	// Matching user or client metadata cannot enable the Claude-only API.
	_, err := first.ClaudeAuthorizationURLs(req)
	require.Error(t, err)
	req.User = &kuser.DefaultInfo{UID: "42", Groups: []string{types.GroupClaudeEnvironment}}
	links, err := first.ClaudeAuthorizationURLs(req)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, "https://obot.example/auth/mcp/composite/"+instance.Name, links[0].URL)
	require.Zero(t, providerRequests.Load())

	// Opening the setup page starts the existing component OAuth flow on either replica.
	pending, err := second.pendingVMCPAuth(req, instance.Name, "")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Contains(t, pending[0].AuthURL, "/authorize?")
	require.Contains(t, pending[0].AuthURL, "state=")
	require.Contains(t, pending[0].AuthURL, "code_challenge=")

	// The existing OAuth callback writes shared tokens, even when another replica handles it.
	require.NoError(t, callbackTokens.ForUserAndMCP("42", connection.Name, providerURL+"/mcp").SetTokenConfig(context.Background(),
		&oauth2.Config{ClientID: "provider-client"}, &oauth2.Token{AccessToken: "provider-token", TokenType: "Bearer"}))
	for _, replica := range []*MCPOAuthHandlerFactory{first, second} {
		remaining, err := replica.pendingVMCPAuth(req, instance.Name, "")
		require.NoError(t, err)
		require.Empty(t, remaining)
	}
}
