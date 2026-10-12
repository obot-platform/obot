package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type oauthTestClientCredLookup struct {
	clientID     string
	clientSecret string
	calls        int
}

type recordingTokenStorage struct {
	setCalls  int
	setErr    error
	lastConf  *oauth2.Config
	lastToken *oauth2.Token
}

type oauthAuthorizeCallbackHandler struct {
	authURL     string
	verifier    string
	resourceURL string
	callback    chan CallbackPayload
}

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
}

func TestServerConfigHeadersCanonicalizesNames(t *testing.T) {
	headers := serverConfigHeaders(ServerConfig{
		PassthroughHeaderNames:  []string{"x-request-id"},
		PassthroughHeaderValues: []string{"request-1"},
		Headers:                 []string{"AUTHORIZATION=Bearer token"},
	})

	require.Equal(t, []string{"Bearer token"}, headers["Authorization"])
	require.Equal(t, []string{"request-1"}, headers["X-Request-Id"])
	require.NotContains(t, headers, "AUTHORIZATION")
}

func TestGetOAuthMetadataSkipsDiscoveryAfterInitializeWithRequiredHeaders(t *testing.T) {
	var initializeRequests, metadataRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/mcp":
			initializeRequests.Add(1)
			if req.Header.Get("Content-Type") != "application/json" {
				http.Error(rw, "Invalid Content-Type. Expected application/json.", http.StatusUnsupportedMediaType)
				return
			}
			accept := strings.Split(req.Header.Get("Accept"), ",")
			for i := range accept {
				accept[i] = strings.TrimSpace(accept[i])
			}
			if !slices.Contains(accept, "application/json") || !slices.Contains(accept, "text/event-stream") {
				http.Error(rw, "Expected JSON and SSE response support.", http.StatusNotAcceptable)
				return
			}
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"test","version":"1"}}}`))
		case strings.Contains(req.URL.Path, "oauth-protected-resource"):
			metadataRequests.Add(1)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"meta":{"title":"AWS Knowledge MCP Server"},"payload":{}}`))
		default:
			http.NotFound(rw, req)
		}
	}))
	t.Cleanup(server.Close)

	metadata, err := getOAuthMetadataWithClient(t.Context(), server.Client(), ServerConfig{URL: server.URL + "/mcp"}, "test", "https://obot.example/oauth/mcp/callback", false)
	require.NoError(t, err)
	require.Empty(t, metadata)
	require.Equal(t, int32(1), initializeRequests.Load())
	require.Zero(t, metadataRequests.Load(), "metadata should not be fetched after successful initialize")
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

func TestGetOAuthMetadataPathAndRootFallbackUsesMetadataScope(t *testing.T) {
	var serverURL string
	var pathMetadataRequested, rootMetadataRequested atomic.Bool
	const (
		clientName  = "Test Client"
		redirectURL = "http://localhost/callback"
	)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/mcp":
			if req.Method != http.MethodPost {
				http.NotFound(rw, req)
				return
			}
			rw.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			pathMetadataRequested.Store(true)
			http.NotFound(rw, req)
		case "/.well-known/oauth-protected-resource":
			rootMetadataRequested.Store(true)
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"resource":              serverURL,
				"authorization_servers": []string{serverURL + "/issuer"},
				"scopes_supported":      []string{"read"},
			})
		case "/.well-known/oauth-authorization-server/issuer":
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write(fmt.Appendf(nil, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"registration_endpoint":%q,"response_types_supported":["code"],"client_id_metadata_document_supported":true}`, serverURL+"/issuer", serverURL+"/authorize", serverURL+"/token", serverURL+"/register"))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	metadata, err := GetOAuthMetadataWithClient(t.Context(), server.Client(), ServerConfig{URL: server.URL + "/mcp"}, clientName, redirectURL)
	require.NoError(t, err)
	require.Equal(t, server.URL, metadata.ResourceURL)
	require.Equal(t, server.URL+"/.well-known/oauth-protected-resource", metadata.ProtectedResourceMetadataURL)
	require.True(t, pathMetadataRequested.Load(), "expected path-specific metadata URL to be attempted")
	require.True(t, rootMetadataRequested.Load(), "expected root metadata URL to be attempted")
	require.Equal(t, server.URL+"/.well-known/oauth-authorization-server/issuer", metadata.AuthorizationServerMetadataURL)
	require.True(t, metadata.DynamicClientRegistration)
	require.True(t, metadata.ClientIDMetadataDocumentSupported)

	var registration ClientRegistrationMetadata
	require.NoError(t, json.Unmarshal(metadata.ClientRegistration, &registration))
	require.Equal(t, clientName, registration.ClientName)
	require.Equal(t, []string{redirectURL}, registration.RedirectURIs)
	require.Equal(t, "read", registration.Scope)
	require.Equal(t, []string{"authorization_code"}, registration.GrantTypes)
}

func TestGetOAuthMetadataInitializeSuccessDeletesSessionWithSessionHeader(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/mcp":
			rw.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  map[string]any{},
			})
		case req.Method == http.MethodDelete && req.URL.Path == "/mcp":
			if req.Header.Get("Mcp-Session-Id") != "session-1" {
				http.Error(rw, "missing session id", http.StatusBadRequest)
				return
			}
			deleted.Store(true)
			rw.WriteHeader(http.StatusNoContent)
		case req.URL.Path == "/.well-known/oauth-protected-resource":
			http.Error(rw, "metadata should not be fetched after successful initialize", http.StatusInternalServerError)
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()

	metadata, err := GetOAuthMetadataWithClient(t.Context(), server.Client(), ServerConfig{URL: server.URL + "/mcp"}, "", "")
	require.NoError(t, err)
	require.Empty(t, metadata.ProtectedResourceMetadataURL)
	require.True(t, deleted.Load(), "expected successful initialize session to be deleted with its session header")
}

func TestGetOAuthMetadataAuthorizationServerOIDCFallbackWithoutDynamicRegistration(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(rw).Encode(map[string]any{"resource": serverURL})
		case "/.well-known/oauth-authorization-server":
			http.NotFound(rw, req)
		case "/.well-known/openid-configuration":
			_, _ = rw.Write([]byte(`{"issuer":"issuer","authorization_endpoint":"authorize","token_endpoint":"token","response_types_supported":["code"]}`))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	metadata, err := GetOAuthMetadataWithClient(t.Context(), server.Client(), ServerConfig{URL: server.URL}, "", "")
	require.NoError(t, err)
	require.Equal(t, server.URL+"/.well-known/openid-configuration", metadata.AuthorizationServerMetadataURL)
	require.False(t, metadata.DynamicClientRegistration)
}

func (l *oauthTestClientCredLookup) Lookup(context.Context) (string, string, error) {
	l.calls++
	return l.clientID, l.clientSecret, nil
}

func TestResolveClientInfoUsesClientIDMetadataDocument(t *testing.T) {
	lookup := &oauthTestClientCredLookup{
		clientID:     "static-client-id",
		clientSecret: "static-client-secret",
	}
	o := &oauth{
		clientIDMetadataDocument: "https://client.example/oauth-client-metadata.json",
		clientLookup:             lookup,
	}

	clientInfo, staticClient, err := o.resolveClientInfo(t.Context(), "test-server", oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{"https://issuer.example"},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{
			ClientIDMetadataDocumentSupported: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, o.clientIDMetadataDocument, clientInfo.ClientID)
	require.Empty(t, clientInfo.ClientSecret)
	require.False(t, staticClient)
	require.Zero(t, lookup.calls)
}

func TestResolveClientInfoUsesStaticClientLookup(t *testing.T) {
	lookup := &oauthTestClientCredLookup{
		clientID:     "static-client-id",
		clientSecret: "static-client-secret",
	}
	o := &oauth{clientLookup: lookup}

	clientInfo, staticClient, err := o.resolveClientInfo(t.Context(), "test-server", oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{"https://issuer.example"},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{},
	})
	require.NoError(t, err)
	require.Equal(t, lookup.clientID, clientInfo.ClientID)
	require.Equal(t, lookup.clientSecret, clientInfo.ClientSecret)
	require.True(t, staticClient)
	require.Equal(t, 1, lookup.calls)
}

func TestResolveClientInfoUsesPublicStaticClientLookup(t *testing.T) {
	lookup := &oauthTestClientCredLookup{clientID: "public-client-id"}
	o := &oauth{clientLookup: lookup}

	clientInfo, staticClient, err := o.resolveClientInfo(t.Context(), "test-server", oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{"https://issuer.example"},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{},
	})
	require.NoError(t, err)
	require.Equal(t, lookup.clientID, clientInfo.ClientID)
	require.Empty(t, clientInfo.ClientSecret)
	require.True(t, staticClient)
	require.Equal(t, 1, lookup.calls)
}

func TestResolveClientInfoNilLookupFallsThroughToDynamicRegistration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodPost, req.Method)
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"client_id":"dynamic-client-id","client_secret":"dynamic-client-secret"}`))
	}))
	defer server.Close()

	o := &oauth{metadataClient: server.Client()}
	discovery := oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{server.URL},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{
			RegistrationEndpoint: server.URL,
		},
		ClientRegistration: ClientRegistrationMetadata{
			ClientName: "test-client",
		},
	}

	var (
		clientInfo   clientRegistrationResponse
		staticClient bool
		err          error
		panicValue   any
	)
	func() {
		defer func() { panicValue = recover() }()
		clientInfo, staticClient, err = o.resolveClientInfo(t.Context(), "test-server", discovery)
	}()
	if panicValue != nil {
		t.Fatalf("resolveClientInfo panicked with nil client lookup: %v", panicValue)
	}
	require.NoError(t, err)
	require.Equal(t, "dynamic-client-id", clientInfo.ClientID)
	require.Equal(t, "dynamic-client-secret", clientInfo.ClientSecret)
	require.False(t, staticClient)
}

func TestTokenEndpointAuthStyle(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		hasClientSecret bool
		staticClient    bool
		want            oauth2.AuthStyle
	}{
		{
			name:            "static confidential client",
			method:          "client_secret_basic",
			hasClientSecret: true,
			staticClient:    true,
			want:            oauth2.AuthStyleAutoDetect,
		},
		{
			name:         "static public client",
			method:       "client_secret_basic",
			staticClient: true,
			want:         oauth2.AuthStyleAutoDetect,
		},
		{
			name:   "dynamic client without secret",
			method: "client_secret_basic",
			want:   oauth2.AuthStyleInParams,
		},
		{
			name:            "dynamic basic client",
			method:          "client_secret_basic",
			hasClientSecret: true,
			want:            oauth2.AuthStyleInHeader,
		},
		{
			name:            "dynamic post client",
			method:          "client_secret_post",
			hasClientSecret: true,
			want:            oauth2.AuthStyleInParams,
		},
		{
			name:            "dynamic client with unknown method",
			method:          "",
			hasClientSecret: true,
			want:            oauth2.AuthStyleAutoDetect,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tokenEndpointAuthStyle(tt.method, tt.hasClientSecret, tt.staticClient))
		})
	}
}

func TestParseProtectedResourceMetadataResourceForms(t *testing.T) {
	for _, tt := range []struct {
		name string
		json string
	}{
		{
			name: "string",
			json: `{"resource":"https://example.com/mcp"}`,
		},
		{
			name: "singleton array",
			json: `{"resource":["https://example.com/mcp"]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := parseProtectedResourceMetadata(strings.NewReader(tt.json))
			require.NoError(t, err)
			require.Equal(t, "https://example.com/mcp", string(metadata.Resource))

			encoded, err := json.Marshal(metadata)
			require.NoError(t, err)
			var output struct {
				Resource string `json:"resource"`
			}
			require.NoError(t, json.Unmarshal(encoded, &output))
			require.Equal(t, "https://example.com/mcp", output.Resource)
		})
	}
}

func TestParseProtectedResourceMetadataRejectsInvalidResourceShapes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		resource string
	}{
		{
			name:     "empty string",
			resource: `""`,
		},
		{
			name:     "empty array",
			resource: `[]`,
		},
		{
			name:     "multiple resources",
			resource: `["https://example.com/one","https://example.com/two"]`,
		},
		{
			name:     "empty array resource",
			resource: `[""]`,
		},
		{
			name:     "number",
			resource: `42`,
		},
		{
			name:     "boolean",
			resource: `true`,
		},
		{
			name:     "null",
			resource: `null`,
		},
		{
			name:     "object",
			resource: `{}`,
		},
		{
			name:     "non-string array element",
			resource: `[42]`,
		},
		{
			name:     "null array element",
			resource: `[null]`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseProtectedResourceMetadata(strings.NewReader(`{"resource":` + tt.resource + `}`))
			require.Error(t, err)
		})
	}
}

func TestOAuthResourceMetadataURLs(t *testing.T) {
	tests := []struct {
		name               string
		baseURL            string
		authenticateHeader string
		wantURLs           []string
		wantScope          string
	}{
		{
			name:    "defaults to path-specific then root metadata without an auth header",
			baseURL: "https://mcp.example.com/mcp",
			wantURLs: []string{
				"https://mcp.example.com/.well-known/oauth-protected-resource/mcp",
				"https://mcp.example.com/.well-known/oauth-protected-resource",
			},
		},
		{
			name:               "retains challenge scope for default metadata URLs",
			baseURL:            "https://mcp.example.com/v1/mcp",
			authenticateHeader: `Bearer scope="read write"`,
			wantURLs: []string{
				"https://mcp.example.com/.well-known/oauth-protected-resource/v1/mcp",
				"https://mcp.example.com/.well-known/oauth-protected-resource",
			},
			wantScope: "read write",
		},
		{
			name:               "uses advertised resource metadata URL exclusively",
			baseURL:            "https://mcp.example.com/mcp",
			authenticateHeader: `Bearer resource_metadata="https://auth.example.com/resources/mcp" scope="read"`,
			wantURLs:           []string{"https://auth.example.com/resources/mcp"},
			wantScope:          "read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls, scope, err := oauthResourceMetadataURLs(tt.baseURL, tt.authenticateHeader)
			require.NoError(t, err)
			gotURLs := make([]string, len(urls))
			for i, u := range urls {
				gotURLs[i] = u.String()
			}
			require.True(t, slices.Equal(gotURLs, tt.wantURLs), "got URLs %v, want %v", gotURLs, tt.wantURLs)
			require.Equal(t, tt.wantScope, scope)
		})
	}
}

func TestAuthServerMetadataToClientRegistrationFiltersGrantTypes(t *testing.T) {
	tests := []struct {
		name      string
		supported []string
		want      []string
	}{
		{
			name:      "keeps only authorization code and refresh token",
			supported: []string{"client_credentials", "refresh_token", "authorization_code", "implicit"},
			want:      []string{"authorization_code", "refresh_token"},
		},
		{
			name:      "omits unsupported grant types",
			supported: []string{"client_credentials", "implicit"},
		},
		{
			name:      "keeps refresh token when advertised",
			supported: []string{"refresh_token"},
			want:      []string{"refresh_token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registration := AuthServerMetadataToClientRegistration(AuthorizationServerMetadata{GrantTypesSupported: tt.supported}, "", "", "")
			require.Equal(t, tt.want, registration.GrantTypes)
		})
	}
}

func (*recordingTokenStorage) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return nil, nil
}

func (*recordingTokenStorage) GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error) {
	return nil, nil, nil
}

func (s *recordingTokenStorage) SetTokenConfig(_ context.Context, conf *oauth2.Config, token *oauth2.Token) error {
	s.setCalls++
	s.lastConf = conf
	s.lastToken = token
	return s.setErr
}

func (*recordingTokenStorage) DeleteTokenConfig(context.Context) error {
	return nil
}

func TestStorageBackedTokenSourcePersistsRefreshedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodPost, req.Method)
		require.NoError(t, req.ParseForm())
		require.Equal(t, "refresh-token", req.Form.Get("refresh_token"))
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"access_token":"new-access-token","refresh_token":"new-refresh-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	storage := &recordingTokenStorage{}
	conf := &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	initial := &oauth2.Token{
		AccessToken:  "old-access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
	}

	tok, err := newStorageBackedTokenSource(storage, conf, initial).Token()
	require.NoError(t, err)
	require.Equal(t, "new-access-token", tok.AccessToken)
	require.Equal(t, "new-refresh-token", tok.RefreshToken)
	require.Equal(t, 1, storage.setCalls)
	require.Same(t, conf, storage.lastConf)
	require.Same(t, tok, storage.lastToken)
}

func TestOAuthResourceHandlingByAuthorizationServer(t *testing.T) {
	const resourceURL = "https://resource.example.com/mcp"
	tests := []struct {
		name             string
		authorizationURL string
		expectedResource string
	}{
		{
			name:             "Entra global",
			authorizationURL: "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize",
		},
		{
			name:             "Entra US Government",
			authorizationURL: "https://login.microsoftonline.us/tenant/oauth2/v2.0/authorize",
		},
		{
			name:             "Entra China",
			authorizationURL: "https://login.partner.microsoftonline.cn/tenant/oauth2/v2.0/authorize",
		},
		{
			name:             "non-Entra China authority",
			authorizationURL: "https://login.chinacloudapi.cn/tenant/oauth2/v2.0/authorize",
			expectedResource: resourceURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callback := &oauthAuthorizeCallbackHandler{}
			tokenServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				require.NoError(t, req.ParseForm())
				require.Equal(t, tt.expectedResource, req.Form.Get("resource"))
				rw.Header().Set("Content-Type", "application/json")
				_, _ = rw.Write([]byte(`{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenServer.Close()

			conf := &oauth2.Config{
				ClientID:    "client-id",
				RedirectURL: "https://obot.example.com/callback",
				Endpoint: oauth2.Endpoint{
					AuthURL:  tt.authorizationURL,
					TokenURL: tokenServer.URL,
				},
			}

			authURL, _, verifier, err := GetOAuthAuthorizationURL(t.Context(), callback, conf, conf.Endpoint.AuthURL, resourceURL)
			require.NoError(t, err)
			parsedAuthURL, err := url.Parse(authURL)
			require.NoError(t, err)
			require.Equal(t, tt.expectedResource, parsedAuthURL.Query().Get("resource"))
			require.Equal(t, tt.expectedResource, callback.resourceURL)

			_, err = ExchangeOAuthToken(t.Context(), conf, "authorization-code", verifier, resourceURL)
			require.NoError(t, err)
		})
	}
}

func TestOAuthAuthorizationURLConsent(t *testing.T) {
	const resourceURL = "https://resource.example.com/mcp"
	tests := []struct {
		name               string
		authorizationURL   string
		expectedPrompt     string
		expectedAccessType string
		expectedResource   string
	}{
		{
			name:               "Google",
			authorizationURL:   "https://accounts.google.com/o/oauth2/v2/auth",
			expectedPrompt:     "consent",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "Google legacy endpoint",
			authorizationURL:   "https://accounts.google.com/o/oauth2/auth",
			expectedPrompt:     "consent",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "Google uppercase hostname",
			authorizationURL:   "https://ACCOUNTS.GOOGLE.COM/o/oauth2/v2/auth",
			expectedPrompt:     "consent",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "Google explicit port",
			authorizationURL:   "https://accounts.google.com:443/o/oauth2/v2/auth",
			expectedPrompt:     "consent",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "other provider",
			authorizationURL:   "https://auth.example.com/authorize",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "Google lookalike",
			authorizationURL:   "https://accounts.google.com.example.com/authorize",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:               "Google subdomain",
			authorizationURL:   "https://other.accounts.google.com/authorize",
			expectedAccessType: "offline",
			expectedResource:   resourceURL,
		},
		{
			name:             "Zoho",
			authorizationURL: "https://mcp.zoho.com/authorize",
			expectedResource: resourceURL,
		},
		{
			name:               "Entra",
			authorizationURL:   "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize",
			expectedAccessType: "offline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callback := &oauthAuthorizeCallbackHandler{}
			conf := &oauth2.Config{
				ClientID:    "client-id",
				RedirectURL: "https://obot.example.com/callback",
				Scopes:      []string{"read", "write"},
				Endpoint: oauth2.Endpoint{
					AuthURL: tt.authorizationURL,
				},
			}

			authURL, _, verifier, err := GetOAuthAuthorizationURL(t.Context(), callback, conf, conf.Endpoint.AuthURL, resourceURL)
			require.NoError(t, err)
			parsedAuthURL, err := url.Parse(authURL)
			require.NoError(t, err)
			query := parsedAuthURL.Query()
			require.Equal(t, tt.expectedPrompt, query.Get("prompt"))
			require.Equal(t, tt.expectedAccessType, query.Get("access_type"))
			require.Equal(t, tt.expectedResource, query.Get("resource"))
			require.Equal(t, "state", query.Get("state"))
			require.Equal(t, "client-id", query.Get("client_id"))
			require.Equal(t, conf.RedirectURL, query.Get("redirect_uri"))
			require.Equal(t, "read write", query.Get("scope"))
			require.Equal(t, "code", query.Get("response_type"))
			require.Equal(t, "S256", query.Get("code_challenge_method"))
			require.Equal(t, oauth2.S256ChallengeFromVerifier(verifier), query.Get("code_challenge"))
		})
	}
}

func TestResolveOAuthResourceURL(t *testing.T) {
	require.Equal(t,
		"https://connection.example.com/mcp",
		ResolveOAuthResourceURL("https://auth.example.com/authorize", "", "https://connection.example.com/mcp"),
	)
	require.Equal(t,
		"https://resource.example.com/mcp",
		ResolveOAuthResourceURL("https://auth.example.com/authorize", "https://resource.example.com/mcp", "https://connection.example.com/mcp"),
	)
	require.Empty(t,
		ResolveOAuthResourceURL("https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize", "", "https://connection.example.com/mcp"),
	)
}

func TestStorageBackedTokenSourceDoesNotPersistUnchangedToken(t *testing.T) {
	tok := &oauth2.Token{AccessToken: "access-token", RefreshToken: "refresh-token", Expiry: time.Now().Add(time.Hour)}
	storage := &recordingTokenStorage{}
	ts := newStorageBackedTokenSource(storage, &oauth2.Config{}, tok)

	got, err := ts.Token()
	require.NoError(t, err)
	require.Same(t, tok, got)
	require.Zero(t, storage.setCalls)
}

func TestStorageBackedTokenSourcePropagatesPersistenceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"access_token":"new-access-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	persistenceErr := errors.New("persistence failed")
	old := &oauth2.Token{AccessToken: "old-access-token", RefreshToken: "refresh-token", Expiry: time.Now().Add(-time.Hour)}
	storage := &recordingTokenStorage{setErr: persistenceErr}
	conf := &oauth2.Config{
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	ts := newStorageBackedTokenSource(storage, conf, old)

	got, err := ts.Token()
	require.ErrorIs(t, err, persistenceErr)
	require.Nil(t, got)
	require.Equal(t, 1, storage.setCalls)
}

func (h *oauthAuthorizeCallbackHandler) HandleAuthURL(_ context.Context, _ string, authURL string) (bool, error) {
	h.authURL = authURL
	h.callback <- CallbackPayload{Code: "authorization-code"}
	return true, nil
}

func (h *oauthAuthorizeCallbackHandler) NewState(_ context.Context, _ *oauth2.Config, resourceURL, verifier string) (string, <-chan CallbackPayload, error) {
	h.verifier = verifier
	h.resourceURL = resourceURL
	h.callback = make(chan CallbackPayload, 1)
	return "state", h.callback, nil
}

func TestOAuthAuthorizeDiscoversRegistersExchangesAndPersists(t *testing.T) {
	var serverURL string
	var registrationCalled, tokenCalled atomic.Bool
	const redirectURL = "https://obot.example.com/callback"
	callback := &oauthAuthorizeCallbackHandler{}
	storage := &recordingTokenStorage{}
	lookup := &oauthTestClientCredLookup{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"resource":              serverURL,
				"authorization_servers": []string{serverURL},
				"scopes_supported":      []string{"read"},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"issuer":                                serverURL,
				"authorization_endpoint":                serverURL + "/authorize",
				"token_endpoint":                        serverURL + "/token",
				"registration_endpoint":                 serverURL + "/register",
				"response_types_supported":              []string{"code"},
				"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/register":
			registrationCalled.Store(true)
			require.Equal(t, http.MethodPost, req.Method)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"client_id":"dynamic-client"}`))
		case "/token":
			tokenCalled.Store(true)
			require.Equal(t, http.MethodPost, req.Method)
			require.NoError(t, req.ParseForm())
			require.Equal(t, "authorization-code", req.Form.Get("code"))
			require.Equal(t, hVerifier(callback), req.Form.Get("code_verifier"))
			require.Equal(t, "dynamic-client", req.Form.Get("client_id"))
			require.Equal(t, redirectURL, req.Form.Get("redirect_uri"))
			require.Equal(t, serverURL, req.Form.Get("resource"))
			require.Empty(t, req.Form.Get("client_secret"))
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	request := httptest.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
	response := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"WWW-Authenticate": []string{fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, server.URL)},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}
	o := newOAuth(server.Client(), callback, lookup, storage, "test-server", "", "test-client", redirectURL, "")
	require.NoError(t, o.Authorize(t.Context(), request, response))
	require.True(t, registrationCalled.Load())
	require.True(t, tokenCalled.Load())
	require.NotEmpty(t, callback.authURL)
	require.Contains(t, callback.authURL, "code_challenge=")
	authorizationRequest, err := http.NewRequest(http.MethodGet, callback.authURL, nil)
	require.NoError(t, err)
	require.Equal(t, serverURL, authorizationRequest.URL.Query().Get("resource"))
	require.Equal(t, "access-token", o.currentToken.AccessToken)
	require.Equal(t, 1, storage.setCalls)
	require.Equal(t, "access-token", storage.lastToken.AccessToken)
}

func TestOAuthAuthorizeFallsBackToConnectURLWithoutProtectedResourceMetadata(t *testing.T) {
	var serverURL string
	const redirectURL = "https://obot.example.com/callback"
	callback := &oauthAuthorizeCallbackHandler{}
	tokenRequest := make(chan url.Values, 1)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource":
			http.NotFound(rw, req)
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"issuer":                                serverURL,
				"authorization_endpoint":                serverURL + "/authorize",
				"token_endpoint":                        serverURL + "/token",
				"response_types_supported":              []string{"code"},
				"grant_types_supported":                 []string{"authorization_code"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/token":
			require.NoError(t, req.ParseForm())
			tokenRequest <- req.Form
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	connectURL := server.URL + "/mcp"

	request := httptest.NewRequest(http.MethodGet, connectURL, nil)
	response := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
	o := newOAuth(server.Client(), callback, &oauthTestClientCredLookup{clientID: "static-client", clientSecret: "static-secret"}, nil, "test-server", "", "test-client", redirectURL, "")
	require.NoError(t, o.Authorize(t.Context(), request, response))

	authorizationRequest, err := http.NewRequest(http.MethodGet, callback.authURL, nil)
	require.NoError(t, err)
	require.Equal(t, connectURL, callback.resourceURL)
	require.Equal(t, connectURL, authorizationRequest.URL.Query().Get("resource"))
	require.Equal(t, connectURL, (<-tokenRequest).Get("resource"))
}

func hVerifier(callback *oauthAuthorizeCallbackHandler) string {
	return callback.verifier
}

func TestValidateProtectedResource(t *testing.T) {
	tests := []struct {
		name       string
		resource   string
		connectURL string
		wantErr    bool
	}{
		{
			name:       "exact match",
			resource:   "https://mcp.example.com/mcp",
			connectURL: "https://mcp.example.com/mcp",
		},
		{
			name:       "origin as resource",
			resource:   "https://mcp.example.com",
			connectURL: "https://mcp.example.com/mcp",
		},
		{
			name:       "parent path as resource",
			resource:   "https://mcp.example.com/tenant/",
			connectURL: "https://mcp.example.com/tenant/mcp",
		},
		{
			name:       "resource without the endpoint's trailing slash",
			resource:   "https://mcp.example.com/mcp",
			connectURL: "https://mcp.example.com/mcp/",
		},
		{
			name:       "resource with a trailing slash the endpoint lacks",
			resource:   "https://mcp.example.com/mcp/",
			connectURL: "https://mcp.example.com/mcp",
		},
		{
			name:       "dots inside a path segment",
			resource:   "https://mcp.example.com/v1.2",
			connectURL: "https://mcp.example.com/v1.2/mcp..json",
		},
		{
			name:       "root resource for an endpoint with no path",
			resource:   "https://mcp.example.com/",
			connectURL: "https://mcp.example.com",
		},
		{
			name:       "zero-padded default port",
			resource:   "https://mcp.example.com/",
			connectURL: "https://mcp.example.com:0443/mcp",
		},
		{
			name:       "another service on the same origin",
			resource:   "https://example.com/victim/api",
			connectURL: "https://example.com/attacker/mcp",
			wantErr:    true,
		},
		{
			name:       "sibling path on the same origin",
			resource:   "https://mcp.example.com/protected-resource",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "path prefix without a segment boundary",
			resource:   "https://mcp.example.com/api",
			connectURL: "https://mcp.example.com/api123/mcp",
			wantErr:    true,
		},
		{
			name:       "dot segments in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: "https://example.com/victim/../attacker/mcp",
			wantErr:    true,
		},
		{
			name:       "encoded dot segments in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: "https://example.com/victim/%2e%2E/attacker/mcp",
			wantErr:    true,
		},
		{
			name:       "mixed encoded dot segment in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: "https://example.com/victim/.%2e/attacker/mcp",
			wantErr:    true,
		},
		{
			name:       "single dot segment in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: "https://example.com/victim/./mcp",
			wantErr:    true,
		},
		{
			name:       "backslash dot segments in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: "https://example.com/victim/%5C..%5Cattacker/mcp",
			wantErr:    true,
		},
		{
			name:       "encoded dot segment and slash in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/%2e%2e%2fattacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "uppercase encoded dot segment and slash in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/%2E%2E%2Fattacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "literal backslash dot segments in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim\..\attacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "dot segment and encoded slash in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/..%2fattacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "dot segment with a path parameter in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/..;/attacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "encoded dot segment with a path parameter in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/%2e%2e;x/attacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "double-encoded dot segments in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/%252e%252e%252fattacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "percent-encoded character in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/my%20server/mcp`,
			wantErr:    true,
		},
		{
			name:       "semicolon in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim;x/mcp`,
			wantErr:    true,
		},
		{
			name:       "percent-encoded character in the resource",
			resource:   "https://example.com/my%20server",
			connectURL: `https://example.com/my%20server/mcp`,
			wantErr:    true,
		},
		{
			name:       "encoded semicolon in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/..%3bx/attacker/mcp`,
			wantErr:    true,
		},
		{
			name:       "unnecessarily encoded letter in the MCP server URL",
			resource:   "https://example.com/victim",
			connectURL: `https://example.com/victim/%41/mcp`,
			wantErr:    true,
		},
		{
			name:       "query for another tenant",
			resource:   "https://example.com/mcp?tenant=victim",
			connectURL: "https://example.com/mcp?tenant=attacker",
			wantErr:    true,
		},
		{
			name:       "matching query",
			resource:   "https://example.com/mcp?tenant=a",
			connectURL: "https://example.com/mcp?tenant=a",
		},
		{
			name:       "resource without a query for an endpoint with one",
			resource:   "https://example.com/mcp",
			connectURL: "https://example.com/mcp?tenant=a",
		},
		{
			name:       "query on a resource for an endpoint without one",
			resource:   "https://example.com/mcp?tenant=a",
			connectURL: "https://example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "fragment in the resource",
			resource:   "https://example.com/mcp#x",
			connectURL: "https://example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "empty fragment in the resource",
			resource:   "https://example.com/mcp#",
			connectURL: "https://example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "dot segments in the resource",
			resource:   "https://example.com/attacker/../victim",
			connectURL: "https://example.com/attacker/../victim/mcp",
			wantErr:    true,
		},
		{
			name:       "child path of the connect URL",
			resource:   "https://mcp.example.com/mcp/other",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "case-insensitive scheme and host with default port",
			resource:   "HTTPS://MCP.Example.com:443/mcp",
			connectURL: "https://mcp.example.com/mcp",
		},
		{
			name:       "different host",
			resource:   "https://api.other-service.example/",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "subdomain of the server host",
			resource:   "https://api.mcp.example.com/",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "different scheme",
			resource:   "http://mcp.example.com/mcp",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
		{
			name:       "different port",
			resource:   "https://mcp.example.com:8443/mcp",
			connectURL: "https://mcp.example.com/mcp",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProtectedResource(tt.resource, tt.connectURL)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetOAuthMetadataRejectsResourceForAnotherService(t *testing.T) {
	var serverURL string
	var authServerMetadataRequested atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/mcp":
			rw.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"resource":              "https://api.other-service.example/",
				"authorization_servers": []string{serverURL + "/issuer"},
			})
		case "/.well-known/oauth-authorization-server/issuer":
			authServerMetadataRequested.Store(true)
			http.NotFound(rw, req)
		default:
			http.NotFound(rw, req)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	_, err := GetOAuthMetadataWithClient(t.Context(), server.Client(), ServerConfig{URL: server.URL + "/mcp"}, "Test Client", "http://localhost/callback")
	require.ErrorContains(t, err, "does not match MCP server URL")
	require.False(t, authServerMetadataRequested.Load(), "expected discovery to stop before contacting the authorization server")
}

func TestOAuthAuthorizeValidatesResourceAgainstServerURLForTunnels(t *testing.T) {
	const (
		redirectURL = "https://obot.example.com/callback"
		// The tunneled server's own URL. Obot reaches it through a bridge URL on its own host,
		// but the server's metadata names this URL as its resource.
		upstreamURL = "https://mcp.internal.example/mcp"
	)
	var bridgeURL string
	callback := &oauthAuthorizeCallbackHandler{}
	tokenRequest := make(chan url.Values, 1)

	bridge := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"resource":              upstreamURL,
				"authorization_servers": []string{bridgeURL},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"issuer":                                bridgeURL,
				"authorization_endpoint":                bridgeURL + "/authorize",
				"token_endpoint":                        bridgeURL + "/token",
				"response_types_supported":              []string{"code"},
				"grant_types_supported":                 []string{"authorization_code"},
				"token_endpoint_auth_methods_supported": []string{"none"},
			})
		case "/token":
			require.NoError(t, req.ParseForm())
			tokenRequest <- req.Form
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`))
		default:
			http.NotFound(rw, req)
		}
	}))
	defer bridge.Close()
	bridgeURL = bridge.URL

	request := httptest.NewRequest(http.MethodGet, bridge.URL+"/tunnel/bridge/encoded-target", nil)
	response := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"WWW-Authenticate": []string{fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, bridge.URL)},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}
	o := newOAuth(bridge.Client(), callback, &oauthTestClientCredLookup{clientID: "static-client"}, nil, "test-server", upstreamURL, "test-client", redirectURL, "")
	require.NoError(t, o.Authorize(t.Context(), request, response))
	require.Equal(t, upstreamURL, (<-tokenRequest).Get("resource"))
}
