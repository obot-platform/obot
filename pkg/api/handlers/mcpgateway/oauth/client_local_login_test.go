package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestHTTPClientLocalLoginResumesOriginalRequest(t *testing.T) {
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
		return gomcp.NewServer(&gomcp.Implementation{Name: "provider", Version: "test"}, nil)
	}, nil)
	var providerURL string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": providerURL + "/mcp", "authorization_servers": []string{providerURL}})
		case strings.HasPrefix(r.URL.Path, "/.well-known/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": providerURL, "authorization_endpoint": providerURL + "/authorize", "token_endpoint": providerURL + "/token", "registration_endpoint": providerURL + "/register", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}})
		case r.URL.Path == "/register":
			_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "provider-client", "token_endpoint_auth_method": "none"})
		case r.URL.Path == "/token":
			require.NoError(t, r.ParseForm())
			require.Regexp(t, `^http://localhost:[0-9]+/custom/callback$`, r.Form.Get("redirect_uri"))
			require.NotEmpty(t, r.Form.Get("code_verifier"))
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "provider-token", "token_type": "Bearer"})
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
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL:                 providerURL + "/mcp",
			LocalhostCallbackEnabled: true,
			LocalhostCallbackPath:    "/custom/callback",
		},
	}
	instance := syncedVMCPInstance(vmcpComponentInstance("vmcpi1local", vmcp.Name), vmcp)
	server := vmcpComponentServer("ms1local", "", vmcp.Name)
	server.Spec.Manifest = types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{
			URL:                      providerURL + "/mcp",
			LocalhostCallbackEnabled: true,
			LocalhostCallbackPath:    "/custom/callback",
		},
	}
	connection := syncedVMCPConnection(&v1.MCPServerInstance{
		Name:      "msi1local",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			MCPServerName:   server.Name,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: "component",
			UserID:          "42",
		},
	}, vmcp, instance)
	request := &v1.OAuthAuthRequest{
		Name:      "oar1local",
		Namespace: system.DefaultNamespace,
		Spec: v1.OAuthAuthRequestSpec{
			UserID:          42,
			MCPID:           instance.Name,
			ClientID:        "client",
			RedirectURI:     "https://client.example/callback",
			State:           "original-state",
			ConsentPrepared: true,
		},
	}
	storage := &vmcpOAuthInitialEventsClient{WithWatch: vmcpConsentStorage(vmcp, instance, server, connection, request)}
	gateway := vmcpConsentGateway(t)
	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	factory := NewMCPOAuthHandlerFactory("https://obot.example", manager, storage, gateway, tokens, "", false)
	factory.resolveOAuthClient = func(context.Context, kclient.Client, string) (v1.OAuthClient, error) {
		return v1.OAuthClient{Spec: v1.OAuthClientSpec{Manifest: types.OAuthClientManifest{ClientName: "HTTP client"}}}, nil
	}
	h := &handler{oauthChecker: factory}
	req := vmcpComponentRequest(storage, instance.Name, server.Name)
	req.GatewayClient = gateway
	req.SetPathValue("oauth_auth_request", request.Name)
	response := httptest.NewRecorder()
	req.ResponseWriter = response
	require.NoError(t, h.approveConsent(req))
	require.Equal(t, compositeConsentURL("https://obot.example", instance.Name, request.Name), response.Header().Get("Location"))
	req.URL.RawQuery = "oauth_auth_request=" + request.Name

	response = httptest.NewRecorder()
	req.ResponseWriter = response
	require.NoError(t, h.checkVMCPComponentAuth(req))
	var status componentAuthStatus
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Contains(t, status.AuthURL, "/oauth/mcp/login/ui-local-")
	loginURL, err := url.Parse(status.AuthURL)
	require.NoError(t, err)
	state := loginURL.Path[strings.LastIndex(loginURL.Path, "/")+1:]
	pending, err := gateway.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, request.Name, pending.OAuthAuthRequestID)
	require.Equal(t, "42", pending.UserID)
	require.NotEqual(t, request.Spec.RedirectURI, pending.RedirectURL)
	// The original client cannot complete before provider authentication succeeds.
	require.Error(t, h.oauthComplete(req))

	req.Request = httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?state="+state+"&code=provider-code", nil)
	response = httptest.NewRecorder()
	req.ResponseWriter = response
	require.NoError(t, h.oauthCallback(req))
	completion, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "localhost", completion.Hostname())
	require.Equal(t, "complete", completion.Query().Get("status"))
	require.Equal(t, state, completion.Query().Get("state"))

	req.SetPathValue("oauth_auth_request", request.Name)
	req.SetPathValue("mcp_id", instance.Name)
	req.SetPathValue("component_mcp_id", server.Name)
	req.URL.RawQuery = "oauth_auth_request=" + request.Name
	response = httptest.NewRecorder()
	req.ResponseWriter = response
	require.NoError(t, h.checkVMCPComponentAuth(req))
	status = componentAuthStatus{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Empty(t, status.AuthURL)
	response = httptest.NewRecorder()
	req.ResponseWriter = response
	require.NoError(t, h.oauthComplete(req))
	destination, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "client.example", destination.Host)
	require.Equal(t, "/callback", destination.Path)
	require.Equal(t, "original-state", destination.Query().Get("state"))
	require.NotEmpty(t, destination.Query().Get("code"))
}
