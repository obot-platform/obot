package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oauthDebuggerTestUserID = "debugger-user"
)

type oauthDebuggerRouteScope struct {
	catalogID   string
	workspaceID string
}

// oauthDebuggerAuthServer is a minimal OAuth authorization server that supports dynamic
// client registration and the authorization code token exchange.
type oauthDebuggerAuthServer struct {
	*httptest.Server

	lock       sync.Mutex
	tokenForms []url.Values
}

type oauthDebuggerRequestHarness struct {
	handler       *MCPHandler
	storage       kclient.WithWatch
	gatewayClient *gatewayclient.Client
}

func (s oauthDebuggerRouteScope) setPathValues(req *http.Request, serverID string) {
	req.SetPathValue("mcp_server_id", serverID)
	if s.catalogID != "" {
		req.SetPathValue("catalog_id", s.catalogID)
	}
	if s.workspaceID != "" {
		req.SetPathValue("workspace_id", s.workspaceID)
	}
}

func newOAuthDebuggerAuthServer(t *testing.T) *oauthDebuggerAuthServer {
	t.Helper()

	s := &oauthDebuggerAuthServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":                  "debugger-client",
			"client_secret":              "debugger-secret",
			"token_endpoint_auth_method": "client_secret_post",
		})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.lock.Lock()
		s.tokenForms = append(s.tokenForms, r.PostForm)
		s.lock.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token-12345678",
			"refresh_token": "refresh-token-12345678",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func (s *oauthDebuggerAuthServer) lastTokenForm(t *testing.T) url.Values {
	t.Helper()
	s.lock.Lock()
	defer s.lock.Unlock()
	require.NotEmpty(t, s.tokenForms, "token endpoint was not called")
	return s.tokenForms[len(s.tokenForms)-1]
}

func newOAuthDebuggerTestServer(t *testing.T, name string, scope oauthDebuggerRouteScope, authServer *oauthDebuggerAuthServer) *v1.MCPServer {
	t.Helper()

	authServerMetadata, err := json.Marshal(mcp.AuthorizationServerMetadata{
		Issuer:                            authServer.URL,
		AuthorizationEndpoint:             authServer.URL + "/authorize",
		TokenEndpoint:                     authServer.URL + "/token",
		RegistrationEndpoint:              authServer.URL + "/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	})
	require.NoError(t, err)

	return &v1.MCPServer{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID:               oauthDebuggerTestUserID,
			MCPCatalogID:         scope.catalogID,
			PowerUserWorkspaceID: scope.workspaceID,
			Manifest: types.MCPServerManifest{
				Name:    name,
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: authServer.URL + "/mcp",
				},
			},
		},
		Status: v1.MCPServerStatus{
			OAuthMetadata: &v1.OAuthMetadata{
				AuthorizationServerURL:      authServer.URL,
				AuthorizationServerMetadata: runtime.RawExtension{Raw: authServerMetadata},
			},
		},
	}
}

func newOAuthDebuggerRequestHarness(t *testing.T, objects ...kclient.Object) oauthDebuggerRequestHarness {
	t.Helper()

	manager, storage, gatewayClient := newVMCPActionSessionManager(t, objects...)
	return oauthDebuggerRequestHarness{
		handler: &MCPHandler{
			mcpSessionManager: manager,
			serverURL:         "http://obot.example.com",
		},
		storage:       storage,
		gatewayClient: gatewayClient,
	}
}

func (h oauthDebuggerRequestHarness) do(t *testing.T, handler func(*MCPHandler, api.Context) error, userID, serverID string, scope oauthDebuggerRouteScope, body any) (*httptest.ResponseRecorder, error) {
	t.Helper()

	var reqBody bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&reqBody).Encode(body))
	}
	req := httptest.NewRequest(http.MethodPost, "/", &reqBody)
	req.Header.Set("Content-Type", "application/json")
	scope.setPathValues(req, serverID)

	recorder := httptest.NewRecorder()
	err := handler(h.handler, api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        h.storage,
		GatewayClient:  h.gatewayClient,
		User:           testUser(userID),
	})
	return recorder, err
}

func TestOAuthDebuggerRequestsAcrossRouteScopes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope oauthDebuggerRouteScope
	}{
		{
			name:  "standalone",
			scope: oauthDebuggerRouteScope{},
		},
		{
			name: "catalog",
			scope: oauthDebuggerRouteScope{
				catalogID: system.DefaultCatalog,
			},
		},
		{
			name: "workspace",
			scope: oauthDebuggerRouteScope{
				workspaceID: "workspace1",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authServer := newOAuthDebuggerAuthServer(t)
			server := newOAuthDebuggerTestServer(t, "ms1debugger", tt.scope, authServer)
			h := newOAuthDebuggerRequestHarness(t, server)

			// Register a client through dynamic client registration.
			recorder, err := h.do(t, (*MCPHandler).RegisterOAuthDebuggerClient, oauthDebuggerTestUserID, server.Name, tt.scope, nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)

			var registered struct {
				State  string            `json:"state"`
				Client types.OAuthClient `json:"client"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &registered))
			require.NotEmpty(t, registered.State)
			assert.Equal(t, "debugger-client", registered.Client.ClientID)

			pendingState, err := h.gatewayClient.GetMCPOAuthPendingState(t.Context(), registered.State)
			require.NoError(t, err)
			assert.Equal(t, oauthDebuggerTestUserID, pendingState.UserID)
			assert.Equal(t, server.Name, pendingState.MCPID)
			assert.Equal(t, OAuthDebuggerPendingStateMarker, pendingState.OAuthAuthRequestID)

			// Build the authorization URL from the stored state.
			recorder, err = h.do(t, (*MCPHandler).GetOAuthDebuggerAuthorizationURL, oauthDebuggerTestUserID, server.Name, tt.scope, types.OAuthDebuggerAuthorizationURLRequest{
				State: registered.State,
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)

			var authorization types.OAuthDebuggerAuthorizationURL
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &authorization))
			assert.Equal(t, registered.State, authorization.State)

			authorizationURL, err := url.Parse(authorization.OAuthURL)
			require.NoError(t, err)
			assert.Equal(t, authServer.URL+"/authorize", authorizationURL.Scheme+"://"+authorizationURL.Host+authorizationURL.Path)
			assert.Equal(t, "debugger-client", authorizationURL.Query().Get("client_id"))
			assert.Equal(t, registered.State, authorizationURL.Query().Get("state"))
			assert.Equal(t, system.MCPOAuthCallbackURL(h.handler.serverURL), authorizationURL.Query().Get("redirect_uri"))
			assert.NotEmpty(t, authorizationURL.Query().Get("code_challenge"))

			// Exchange the authorization code for a token.
			recorder, err = h.do(t, (*MCPHandler).ExchangeOAuthDebuggerToken, oauthDebuggerTestUserID, server.Name, tt.scope, types.OAuthDebuggerTokenRequest{
				Code:  "authorization-code",
				State: registered.State,
			})
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)

			var token types.OAuthToken
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &token))
			assert.Equal(t, "acces...", token.AccessToken)
			assert.Equal(t, "refre...", token.RefreshToken)
			assert.Equal(t, "Bearer", token.TokenType)
			assert.Positive(t, token.ExpiresIn)

			tokenForm := authServer.lastTokenForm(t)
			assert.Equal(t, "authorization_code", tokenForm.Get("grant_type"))
			assert.Equal(t, "authorization-code", tokenForm.Get("code"))
			assert.Equal(t, "debugger-client", tokenForm.Get("client_id"))
			assert.Equal(t, "debugger-secret", tokenForm.Get("client_secret"))
			assert.Equal(t, pendingState.Verifier, tokenForm.Get("code_verifier"))

			storedToken, err := h.gatewayClient.GetMCPOAuthToken(t.Context(), oauthDebuggerTestUserID, server.Name, authServer.URL+"/mcp")
			require.NoError(t, err)
			assert.Equal(t, "access-token-12345678", storedToken.AccessToken)

			_, err = h.gatewayClient.GetMCPOAuthPendingState(t.Context(), registered.State)
			assert.Error(t, err, "pending state should be deleted after the token exchange")
		})
	}
}

func TestOAuthDebuggerRequestsRejectMismatchedRouteScope(t *testing.T) {
	scopes := map[string]oauthDebuggerRouteScope{
		"standalone": {},
		"catalog": {
			catalogID: system.DefaultCatalog,
		},
		"workspace": {
			workspaceID: "workspace1",
		},
	}
	handlers := map[string]func(*MCPHandler, api.Context) error{
		"register":          (*MCPHandler).RegisterOAuthDebuggerClient,
		"authorization-url": (*MCPHandler).GetOAuthDebuggerAuthorizationURL,
		"token":             (*MCPHandler).ExchangeOAuthDebuggerToken,
	}

	authServer := newOAuthDebuggerAuthServer(t)
	for serverScopeName, serverScope := range scopes {
		for routeScopeName, routeScope := range scopes {
			if serverScopeName == routeScopeName {
				continue
			}
			for handlerName, handler := range handlers {
				t.Run(serverScopeName+" server via "+routeScopeName+" route/"+handlerName, func(t *testing.T) {
					server := newOAuthDebuggerTestServer(t, "ms1debugger", serverScope, authServer)
					h := newOAuthDebuggerRequestHarness(t, server)

					_, err := h.do(t, handler, oauthDebuggerTestUserID, server.Name, routeScope, nil)
					assertOAuthDebuggerNotFound(t, err)
				})
			}
		}
	}
}

func TestOAuthDebuggerRequestsRejectStateFromAnotherUserOrServer(t *testing.T) {
	authServer := newOAuthDebuggerAuthServer(t)
	scope := oauthDebuggerRouteScope{
		catalogID: system.DefaultCatalog,
	}
	server := newOAuthDebuggerTestServer(t, "ms1debugger", scope, authServer)
	otherServer := newOAuthDebuggerTestServer(t, "ms1debuggerother", scope, authServer)
	otherUserServer := newOAuthDebuggerTestServer(t, "ms1debuggeruser", scope, authServer)
	otherUserServer.Spec.UserID = "other-user"
	h := newOAuthDebuggerRequestHarness(t, server, otherServer, otherUserServer)

	recorder, err := h.do(t, (*MCPHandler).RegisterOAuthDebuggerClient, oauthDebuggerTestUserID, server.Name, scope, nil)
	require.NoError(t, err)
	var registered struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &registered))
	require.NotEmpty(t, registered.State)

	for _, tt := range []struct {
		name     string
		userID   string
		serverID string
	}{
		{
			name:     "other server",
			userID:   oauthDebuggerTestUserID,
			serverID: otherServer.Name,
		},
		{
			name:     "other user",
			userID:   "other-user",
			serverID: server.Name,
		},
	} {
		t.Run(tt.name+"/authorization-url", func(t *testing.T) {
			_, err := h.do(t, (*MCPHandler).GetOAuthDebuggerAuthorizationURL, tt.userID, tt.serverID, scope, types.OAuthDebuggerAuthorizationURLRequest{
				State: registered.State,
			})
			assertOAuthDebuggerNotFound(t, err)
		})
		t.Run(tt.name+"/token", func(t *testing.T) {
			_, err := h.do(t, (*MCPHandler).ExchangeOAuthDebuggerToken, tt.userID, tt.serverID, scope, types.OAuthDebuggerTokenRequest{
				Code:  "authorization-code",
				State: registered.State,
			})
			assertOAuthDebuggerNotFound(t, err)
		})
	}

	// The rejected attempts must not consume the state for the rightful owner.
	_, err = h.gatewayClient.GetMCPOAuthPendingState(t.Context(), registered.State)
	require.NoError(t, err)
}

func assertOAuthDebuggerNotFound(t *testing.T, err error) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
}
