package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

type oauthTestClientCredLookup struct {
	clientID     string
	clientSecret string
	calls        int
}

func (l *oauthTestClientCredLookup) Lookup(context.Context, string) (string, string, error) {
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

	clientInfo, err := o.resolveClientInfo(t.Context(), "test-server", oauthMetadataDiscovery{
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
	require.Zero(t, lookup.calls)
}

func TestResolveClientInfoUsesStaticClientLookup(t *testing.T) {
	lookup := &oauthTestClientCredLookup{
		clientID:     "static-client-id",
		clientSecret: "static-client-secret",
	}
	o := &oauth{clientLookup: lookup}

	clientInfo, err := o.resolveClientInfo(t.Context(), "test-server", oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{"https://issuer.example"},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{},
	})
	require.NoError(t, err)
	require.Equal(t, lookup.clientID, clientInfo.ClientID)
	require.Equal(t, lookup.clientSecret, clientInfo.ClientSecret)
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
		clientInfo clientRegistrationResponse
		err        error
		panicValue any
	)
	func() {
		defer func() { panicValue = recover() }()
		clientInfo, err = o.resolveClientInfo(t.Context(), "test-server", discovery)
	}()
	if panicValue != nil {
		t.Fatalf("resolveClientInfo panicked with nil client lookup: %v", panicValue)
	}
	require.NoError(t, err)
	require.Equal(t, "dynamic-client-id", clientInfo.ClientID)
	require.Equal(t, "dynamic-client-secret", clientInfo.ClientSecret)
}

func TestTokenEndpointAuthStyle(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		hasClientSecret bool
		want            oauth2.AuthStyle
	}{
		{name: "no client secret", method: "client_secret_basic", want: oauth2.AuthStyleInParams},
		{name: "basic", method: "client_secret_basic", hasClientSecret: true, want: oauth2.AuthStyleInHeader},
		{name: "post", method: "client_secret_post", hasClientSecret: true, want: oauth2.AuthStyleInParams},
		{name: "unknown", method: "", hasClientSecret: true, want: oauth2.AuthStyleAutoDetect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tokenEndpointAuthStyle(tt.method, tt.hasClientSecret))
		})
	}
}

func TestParseProtectedResourceMetadataResourceForms(t *testing.T) {
	for _, tt := range []struct {
		name string
		json string
	}{
		{name: "string", json: `{"resource":"https://example.com/mcp"}`},
		{name: "singleton array", json: `{"resource":["https://example.com/mcp"]}`},
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
		{name: "empty string", resource: `""`},
		{name: "empty array", resource: `[]`},
		{name: "multiple resources", resource: `["https://example.com/one","https://example.com/two"]`},
		{name: "empty array resource", resource: `[""]`},
		{name: "number", resource: `42`},
		{name: "boolean", resource: `true`},
		{name: "null", resource: `null`},
		{name: "object", resource: `{}`},
		{name: "non-string array element", resource: `[42]`},
		{name: "null array element", resource: `[null]`},
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
		{name: "keeps only authorization code and refresh token", supported: []string{"client_credentials", "refresh_token", "authorization_code", "implicit"}, want: []string{"authorization_code", "refresh_token"}},
		{name: "omits unsupported grant types", supported: []string{"client_credentials", "implicit"}},
		{name: "keeps refresh token when advertised", supported: []string{"refresh_token"}, want: []string{"refresh_token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registration := AuthServerMetadataToClientRegistration(AuthorizationServerMetadata{GrantTypesSupported: tt.supported}, "", "", "")
			require.Equal(t, tt.want, registration.GrantTypes)
		})
	}
}

type recordingTokenStorage struct {
	setCalls  int
	setErr    error
	lastConf  *oauth2.Config
	lastToken *oauth2.Token
}

func (*recordingTokenStorage) NewTokenSource(_ context.Context, _ *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(token), nil
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

func TestStorageBackedTokenSourceDoesNotPersistUnchangedToken(t *testing.T) {
	tok := &oauth2.Token{AccessToken: "access-token", RefreshToken: "refresh-token", Expiry: time.Now().Add(time.Hour)}
	storage := &recordingTokenStorage{}
	ts := &storageBackedTokenSource{
		tokenStorage: storage,
		conf:         &oauth2.Config{},
		tok:          tok,
		tokenSource:  oauth2.StaticTokenSource(tok),
	}

	got, err := ts.Token()
	require.NoError(t, err)
	require.Same(t, tok, got)
	require.Zero(t, storage.setCalls)
}

func TestStorageBackedTokenSourcePropagatesPersistenceError(t *testing.T) {
	persistenceErr := errors.New("persistence failed")
	old := &oauth2.Token{AccessToken: "old-access-token", Expiry: time.Now().Add(-time.Hour)}
	newToken := &oauth2.Token{AccessToken: "new-access-token", Expiry: time.Now().Add(time.Hour)}
	storage := &recordingTokenStorage{setErr: persistenceErr}
	ts := &storageBackedTokenSource{
		tokenStorage: storage,
		conf:         &oauth2.Config{},
		tok:          old,
		tokenSource:  oauth2.StaticTokenSource(newToken),
	}

	got, err := ts.Token()
	require.ErrorIs(t, err, persistenceErr)
	require.Nil(t, got)
	require.Equal(t, 1, storage.setCalls)
}

type oauthAuthorizeCallbackHandler struct {
	authURL  string
	verifier string
	callback chan CallbackPayload
}

func (h *oauthAuthorizeCallbackHandler) HandleAuthURL(_ context.Context, _ string, authURL string) (bool, error) {
	h.authURL = authURL
	h.callback <- CallbackPayload{Code: "authorization-code"}
	return true, nil
}

func (h *oauthAuthorizeCallbackHandler) NewState(_ context.Context, _ *oauth2.Config, verifier string) (string, <-chan CallbackPayload, error) {
	h.verifier = verifier
	h.callback = make(chan CallbackPayload, 1)
	return "state", h.callback, nil
}

func TestOAuthAuthorizeDiscoversRegistersExchangesAndPersists(t *testing.T) {
	var serverURL string
	var registrationCalled, tokenCalled atomic.Bool
	callback := &oauthAuthorizeCallbackHandler{}
	storage := &recordingTokenStorage{}
	lookup := &oauthTestClientCredLookup{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(rw).Encode(map[string]any{
				"resource":              serverURL + "/mcp",
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
				"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
			})
		case "/register":
			registrationCalled.Store(true)
			require.Equal(t, http.MethodPost, req.Method)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"client_id":"dynamic-client","client_secret":"dynamic-secret"}`))
		case "/token":
			tokenCalled.Store(true)
			require.Equal(t, http.MethodPost, req.Method)
			require.NoError(t, req.ParseForm())
			require.Equal(t, "authorization-code", req.Form.Get("code"))
			require.Equal(t, hVerifier(callback), req.Form.Get("code_verifier"))
			require.Equal(t, "dynamic-client", req.Form.Get("client_id"))
			require.Equal(t, "dynamic-secret", req.Form.Get("client_secret"))
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
	o := newOAuth(server.Client(), callback, lookup, storage, "test-server", "test-client", "https://obot.example.com/callback", "")
	require.NoError(t, o.Authorize(t.Context(), request, response))
	require.True(t, registrationCalled.Load())
	require.True(t, tokenCalled.Load())
	require.NotEmpty(t, callback.authURL)
	require.Contains(t, callback.authURL, "code_challenge=")
	require.Equal(t, "access-token", o.currentToken.AccessToken)
	require.Equal(t, 1, storage.setCalls)
	require.Equal(t, "access-token", storage.lastToken.AccessToken)
}

func hVerifier(callback *oauthAuthorizeCallbackHandler) string {
	return callback.verifier
}
