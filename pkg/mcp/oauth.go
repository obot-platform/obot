package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/oauth2"
)

// RequiresStaticOAuth reports whether a remote MCP server is configured to
// require static OAuth and has not yet been authenticated by the user.
func RequiresStaticOAuth(server v1.MCPServer) bool {
	return server.Spec.Manifest.Runtime == types.RuntimeRemote &&
		server.Spec.Manifest.RemoteConfig != nil &&
		server.Spec.Manifest.RemoteConfig.StaticOAuthRequired &&
		!server.Status.UserHasAuthenticated
}

// GetOAuthMetadata discovers OAuth metadata for a remote MCP server. When
// assumeOAuthRequired is true, discovery proceeds even if initialize succeeds.
// This supports servers that defer their OAuth challenge until tools/call.
func (m *SessionManager) GetOAuthMetadata(ctx context.Context, serverConfig ServerConfig, clientName, redirectURL string, assumeOAuthRequired bool) (OAuthMetadata, error) {
	var httpClient *http.Client
	if serverConfig.TunnelName != "" {
		var err error
		httpClient, err = m.HTTPClientForServer(serverConfig, nil, nil, 5*time.Second)
		if err != nil {
			return OAuthMetadata{}, err
		}
	} else {
		blockingConfig := m.RemoteMCPURLValidationConfig()
		httpClient = safehttp.NewClient(safehttp.ClientOptions{
			BlockLoopback:  !blockingConfig.AllowLocalhostMCP,
			BlockPrivateIP: !blockingConfig.AllowPrivateIPMCP,
			BlockLinkLocal: !blockingConfig.AllowLinkLocalMCP,
			Timeout:        5 * time.Second,
		})
	}

	return getOAuthMetadataWithClient(ctx, httpClient, serverConfig, clientName, redirectURL, assumeOAuthRequired)
}

func getOAuthMetadataWithClient(ctx context.Context, httpClient *http.Client, serverConfig ServerConfig, clientName, redirectURL string, assumeOAuthRequired bool) (OAuthMetadata, error) {
	if assumeOAuthRequired {
		cloned := *httpClient
		transport := cloned.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		cloned.Transport = &assumeOAuthRequiredTransport{base: transport}
		httpClient = &cloned
	}

	return GetOAuthMetadataWithClient(ctx, httpClient, serverConfig, clientName, redirectURL)
}

func serverConfigHeaders(serverConfig ServerConfig) map[string][]string {
	result := make(map[string][]string, len(serverConfig.PassthroughHeaderNames)+len(serverConfig.Headers))
	for i, key := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			result[key] = []string{serverConfig.PassthroughHeaderValues[i]}
		}
	}
	for _, header := range serverConfig.Headers {
		key, value, ok := strings.Cut(header, "=")
		if ok {
			result[key] = []string{value}
		}
	}
	return result
}

// assumeOAuthRequiredTransport synthesizes the initialize challenge used to
// begin standard OAuth metadata discovery. All subsequent metadata requests use
// the original, policy-enforcing transport.
type assumeOAuthRequiredTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	done bool
}

func (t *assumeOAuthRequiredTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if !t.done && req.Method == http.MethodPost {
		t.done = true
		t.mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

// storageBackedTokenSource implements the oauth2.TokenSource interface to store new tokens in the TokenStorage.
type storageBackedTokenSource struct {
	lock         sync.Mutex
	tokenStorage TokenStorage
	conf         *oauth2.Config
	tok          *oauth2.Token
	tokenSource  oauth2.TokenSource
}

func newStorageBackedTokenSource(tokenStorage TokenStorage, conf *oauth2.Config, tok *oauth2.Token) oauth2.TokenSource {
	return oauth2.ReuseTokenSource(tok, &storageBackedTokenSource{
		tokenStorage: tokenStorage,
		conf:         conf,
		tok:          tok,
		tokenSource:  conf.TokenSource(context.Background(), tok),
	})
}

func (ts *storageBackedTokenSource) Token() (*oauth2.Token, error) {
	tok, err := ts.tokenSource.Token()
	if err != nil {
		return nil, err
	}

	ts.lock.Lock()
	defer ts.lock.Unlock()

	if tok.AccessToken != ts.tok.AccessToken || tok.RefreshToken != ts.tok.RefreshToken || tok.Expiry.Unix() != ts.tok.Expiry.Unix() {
		ts.tok = tok

		if ts.tokenStorage != nil {
			if err = ts.tokenStorage.SetTokenConfig(context.Background(), ts.conf, ts.tok); err != nil {
				return nil, fmt.Errorf("failed to store token: %w", err)
			}
		}
	}

	return ts.tok, nil
}

var (
	resourceMetadataRegex = regexp.MustCompile(`resource_metadata="([^"]*)"`)
	scopeRegex            = regexp.MustCompile(`scope="([^"]*)"`)
)

type CallbackPayload struct {
	Code             string `json:"code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type AuthURLHandler interface {
	HandleAuthURL(context.Context, string, string) (bool, error)
}

type CallbackHandler interface {
	AuthURLHandler
	NewState(context.Context, *oauth2.Config, string) (string, <-chan CallbackPayload, error)
}

type ClientCredLookup interface {
	Lookup(context.Context, string) (string, string, error)
}

type oauth struct {
	redirectURL              string
	clientName               string
	serverName               string
	clientIDMetadataDocument string
	currentToken             oauth2.Token
	metadataClient           *http.Client
	callbackHandler          CallbackHandler
	clientLookup             ClientCredLookup
	tokenStorage             TokenStorage
}

type oauthMetadataDiscovery struct {
	ProtectedResourceURL              string
	ProtectedResourceMetadata         protectedResourceMetadata
	ProtectedResourceMetadataJSON     json.RawMessage
	AuthorizationServerURL            string
	AuthorizationServerMetadataURL    string
	AuthorizationServerMetadata       AuthorizationServerMetadata
	AuthorizationServerMetadataJSON   json.RawMessage
	DynamicClientRegistration         bool
	ClientRegistration                ClientRegistrationMetadata
	ClientIDMetadataDocumentSupported bool
}

// OAuthMetadata contains discovered OAuth metadata for an MCP server.
type OAuthMetadata struct {
	ProtectedResourceMetadataURL      string          `json:"protectedResourceMetadataUrl,omitempty"`
	AuthorizationServerMetadataURL    string          `json:"authorizationServerMetadataUrl,omitempty"`
	ProtectedResourceMetadata         json.RawMessage `json:"protectedResourceMetadata,omitempty"`
	AuthorizationServerMetadata       json.RawMessage `json:"authorizationServerMetadata,omitempty"`
	ClientRegistration                json.RawMessage `json:"clientRegistration,omitempty"`
	DynamicClientRegistration         bool            `json:"dynamicClientRegistration,omitempty"`
	ClientIDMetadataDocumentSupported bool            `json:"clientIdMetadataDocumentSupported,omitempty"`
}

func newOAuth(metadataClient *http.Client, callbackHandler CallbackHandler, clientLookup ClientCredLookup, tokenStorage TokenStorage, serverName, clientName, redirectURL, clientIDMetadataDocument string) *oauth {
	return &oauth{
		serverName:               serverName,
		clientName:               clientName,
		redirectURL:              redirectURL,
		clientIDMetadataDocument: clientIDMetadataDocument,
		callbackHandler:          callbackHandler,
		metadataClient:           metadataClient,
		clientLookup:             clientLookup,
		tokenStorage:             tokenStorage,
	}
}

func (o *oauth) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	oauthConfig, token, err := o.tokenStorage.GetTokenConfig(ctx)
	if err != nil || token == nil {
		return nil, err
	}

	return newStorageBackedTokenSource(o.tokenStorage, oauthConfig, token), nil
}

func (o *oauth) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	// This function is responsible for closing the response body.
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if o.callbackHandler == nil || o.redirectURL == "" {
		return fmt.Errorf("oauth callback server is not configured")
	}

	connectURL := req.URL.String()
	slog.Info("starting oauth flow", "server", o.serverName, "connect_url", connectURL)

	discovery, ok, err := discoverOAuthMetadata(ctx, o.metadataClient, connectURL, resp.Header.Get("WWW-Authenticate"), o.clientName, o.redirectURL)
	if err != nil {
		slog.Warn("oauth metadata discovery failed", "server", o.serverName, "connect_url", connectURL, "error", err)
		return err
	}
	if !ok {
		slog.Warn("oauth metadata discovery did not find authorization server metadata", "server", o.serverName, "connect_url", connectURL)
		return fmt.Errorf("failed to get authorization server metadata")
	}
	authorizationServerMetadata := discovery.AuthorizationServerMetadata
	slog.Info("resolved oauth scope for server", "server", o.serverName, "scope", discovery.ClientRegistration.Scope)
	slog.Info("resolved authorization server", "server", o.serverName, "authorization_server", discovery.AuthorizationServerURL)

	clientInfo, err := o.resolveClientInfo(ctx, o.serverName, discovery)
	if err != nil {
		return err
	}

	conf := &oauth2.Config{
		ClientID:     clientInfo.ClientID,
		ClientSecret: clientInfo.ClientSecret,
		RedirectURL:  discovery.ClientRegistration.RedirectURIs[0],
		Endpoint: oauth2.Endpoint{
			AuthURL:  authorizationServerMetadata.AuthorizationEndpoint,
			TokenURL: authorizationServerMetadata.TokenEndpoint,
		},
	}
	if discovery.ClientRegistration.Scope != "" {
		conf.Scopes = strings.Split(discovery.ClientRegistration.Scope, " ")
	}
	conf.Endpoint.AuthStyle = tokenEndpointAuthStyle(discovery.ClientRegistration.TokenEndpointAuthMethod, clientInfo.ClientSecret != "")
	authURL, ch, verifier, err := GetOAuthAuthorizationURL(ctx, o.callbackHandler, conf, authorizationServerMetadata.AuthorizationEndpoint, connectURL)
	if err != nil {
		return err
	}

	slog.Info("handing oauth authorization url to callback handler", "server", o.serverName, "auth_url", authorizationServerMetadata.AuthorizationEndpoint)
	handled, err := o.callbackHandler.HandleAuthURL(ctx, o.serverName, authURL)
	if err != nil {
		return fmt.Errorf("failed to handle auth url %s: %w", authURL, err)
	} else if !handled {
		slog.Info("oauth authorization url was not handled", "server", o.serverName)
		return nil
	}
	slog.Info("waiting for oauth callback", "server", o.serverName)

	var cb CallbackPayload
	select {
	case <-ctx.Done():
		return ctx.Err()
	case cb = <-ch:
		if cb.Error != "" {
			slog.Warn("oauth callback returned error", "server", o.serverName, "error", cb.Error, "description", cb.ErrorDescription)
			return fmt.Errorf("authorization failed: %s, %s", cb.Error, cb.ErrorDescription)
		}
		if cb.Code == "" {
			slog.Warn("oauth callback missing authorization code", "server", o.serverName)
			return fmt.Errorf("authorization failed: no code returned")
		}
	}

	tok, err := ExchangeOAuthToken(ctx, conf, cb.Code, verifier)
	if err != nil {
		slog.Warn("oauth code exchange failed",
			"server", o.serverName,
			"connect_url", connectURL,
			"token_endpoint", conf.Endpoint.TokenURL,
			"token_endpoint_auth_method", discovery.ClientRegistration.TokenEndpointAuthMethod,
			"has_client_secret", clientInfo.ClientSecret != "",
			"error", err)
		return err
	}
	slog.Info("oauth code exchange succeeded", "server", o.serverName)

	o.currentToken = *tok

	if o.tokenStorage != nil {
		if err = o.tokenStorage.SetTokenConfig(ctx, conf, tok); err != nil {
			slog.Info("failed to save token config", "server", o.serverName, "connect_url", connectURL, "error", err)
			return err
		}
		slog.Info("saved oauth token config", "server", o.serverName, "connect_url", connectURL)
	}

	return nil
}

func discoverOAuthMetadata(ctx context.Context, client *http.Client, baseURL, authenticateHeader, clientName, redirectURL string) (oauthMetadataDiscovery, bool, error) {
	resourceMetadataURLs, scope, err := oauthResourceMetadataURLs(baseURL, authenticateHeader)
	if err != nil {
		return oauthMetadataDiscovery{}, false, err
	}

	var (
		finalResourceMetadataURL      *url.URL
		protectedResourceMetadata     protectedResourceMetadata
		protectedResourceMetadataJSON json.RawMessage
		ok                            bool
	)
	for _, resourceMetadataURL := range resourceMetadataURLs {
		finalResourceMetadataURL = resourceMetadataURL
		slog.Info("fetching protected resource metadata", "url", resourceMetadataURL)

		protectedResourceMetadataJSON, ok, err = getOAuthMetadataJSON(ctx, client, resourceMetadataURL.String())
		if err != nil {
			return oauthMetadataDiscovery{}, false, fmt.Errorf("failed to get protected resource metadata: %w", err)
		}

		if ok {
			protectedResourceMetadata, err = parseProtectedResourceMetadata(bytes.NewReader(protectedResourceMetadataJSON))
			if err != nil {
				return oauthMetadataDiscovery{}, false, fmt.Errorf("failed to parse protected resource metadata: %w", err)
			}

			break
		}
	}

	// If no scopes were found in the WWW-Authenticate header, use the ones from the protected resource metadata as a fallback.
	// This follows the scope selection strategy outlined here: https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization#scope-selection-strategy
	if scope == "" {
		scope = strings.Join(protectedResourceMetadata.ScopesSupported, " ")
	}

	if len(protectedResourceMetadata.AuthorizationServers) == 0 {
		protectedResourceMetadata.AuthorizationServers = []string{fmt.Sprintf("%s://%s", finalResourceMetadataURL.Scheme, finalResourceMetadataURL.Host)}
	}
	authorizationServerURL := protectedResourceMetadata.AuthorizationServers[0]

	authorizationServerMetadata, authorizationServerMetadataURL, authorizationServerMetadataJSON, ok, err := getAuthServerMetadata(ctx, client, authorizationServerURL)
	if err != nil {
		return oauthMetadataDiscovery{}, false, fmt.Errorf("failed to get authorization server metadata: %w", err)
	}
	if !ok {
		return oauthMetadataDiscovery{}, false, nil
	}

	var rawAuthorizationServerMetadata struct {
		RegistrationEndpoint string `json:"registration_endpoint"`
	}
	if len(authorizationServerMetadataJSON) > 0 {
		if err := json.Unmarshal(authorizationServerMetadataJSON, &rawAuthorizationServerMetadata); err != nil {
			return oauthMetadataDiscovery{}, false, fmt.Errorf("failed to parse authorization server metadata: %w", err)
		}
	}

	return oauthMetadataDiscovery{
		ProtectedResourceURL:              finalResourceMetadataURL.String(),
		ProtectedResourceMetadata:         protectedResourceMetadata,
		ProtectedResourceMetadataJSON:     protectedResourceMetadataJSON,
		AuthorizationServerURL:            authorizationServerURL,
		AuthorizationServerMetadataURL:    authorizationServerMetadataURL,
		AuthorizationServerMetadata:       authorizationServerMetadata,
		AuthorizationServerMetadataJSON:   authorizationServerMetadataJSON,
		ClientRegistration:                AuthServerMetadataToClientRegistration(authorizationServerMetadata, clientName, redirectURL, scope),
		DynamicClientRegistration:         rawAuthorizationServerMetadata.RegistrationEndpoint != "",
		ClientIDMetadataDocumentSupported: authorizationServerMetadata.ClientIDMetadataDocumentSupported,
	}, true, nil
}

func oauthResourceMetadataURLs(baseURL, authenticateHeader string) ([]*url.URL, string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse MCP URL: %w", err)
	}

	var (
		resourceMetadataURLFromHeader string
		resourceMetadataURLs          []*url.URL
		scope                         string
	)
	if authenticateHeader != "" {
		resourceMetadataURLFromHeader = parseResourceMetadata(authenticateHeader)
		scope = parseScopeFromAuthenticateHeader(authenticateHeader)
	}

	if resourceMetadataURLFromHeader == "" {
		// If the authenticate header was not sent back or it did not have a resource metadata URL, then the spec says we should default to...
		slog.Info("no resource metadata URL in authenticate header, defaulting to .well-known/oauth-protected-resource")
		originalPath := strings.TrimPrefix(u.Path, "/")

		if originalPath != "" {
			withoutPathSuffix := *u
			withoutPathSuffix.Path = ".well-known/oauth-protected-resource/" + originalPath
			resourceMetadataURLs = append(resourceMetadataURLs, &withoutPathSuffix)
		}

		u.Path = "/.well-known/oauth-protected-resource"
		resourceMetadataURLs = append(resourceMetadataURLs, u)
	} else {
		parsedURL, err := url.Parse(resourceMetadataURLFromHeader)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse resource metadata URL: %w", err)
		}
		resourceMetadataURLs = []*url.URL{parsedURL}
	}

	return resourceMetadataURLs, scope, nil
}

func tokenEndpointAuthStyle(tokenEndpointAuthMethod string, hasClientSecret bool) oauth2.AuthStyle {
	if !hasClientSecret {
		return oauth2.AuthStyleInParams
	}

	switch tokenEndpointAuthMethod {
	case "client_secret_basic":
		return oauth2.AuthStyleInHeader
	case "client_secret_post":
		return oauth2.AuthStyleInParams
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

func (o *oauth) resolveClientInfo(ctx context.Context, serverName string, discovery oauthMetadataDiscovery) (clientRegistrationResponse, error) {
	authorizationServerMetadata := discovery.AuthorizationServerMetadata
	protectedResourceMetadata := discovery.ProtectedResourceMetadata

	if authorizationServerMetadata.ClientIDMetadataDocumentSupported && o.clientIDMetadataDocument != "" {
		slog.Info("using oauth client ID metadata document", "server", serverName, "client_id", o.clientIDMetadataDocument)
		return clientRegistrationResponse{
			ClientID: o.clientIDMetadataDocument,
		}, nil
	}

	// Before trying to register a client, check if there is a static client configuration.
	var (
		clientInfo clientRegistrationResponse
		lookupErr  error
	)
	if o.clientLookup != nil {
		clientInfo.ClientID, clientInfo.ClientSecret, lookupErr = o.clientLookup.Lookup(ctx, protectedResourceMetadata.AuthorizationServers[0])
	}
	if lookupErr == nil && clientInfo.ClientID != "" && clientInfo.ClientSecret != "" {
		slog.Info("using static oauth client credentials", "server", serverName, "authorization_server", protectedResourceMetadata.AuthorizationServers[0])
		return clientInfo, nil
	}
	if lookupErr != nil {
		slog.Debug("static oauth client credential lookup failed", "server", serverName, "authorization_server", protectedResourceMetadata.AuthorizationServers[0], "error", lookupErr)
	} else {
		slog.Debug("static oauth client credentials not configured",
			"server", serverName,
			"authorization_server", protectedResourceMetadata.AuthorizationServers[0],
			"has_client_id", clientInfo.ClientID != "",
			"has_client_secret", clientInfo.ClientSecret != "")
	}

	// If we didn't get a result from the lookup, register a client dynamically.
	clientInfo, err := registerOAuthClient(ctx, o.metadataClient, serverName, authorizationServerMetadata, discovery.ClientRegistration)
	if err != nil {
		slog.Warn("oauth dynamic client registration failed",
			"server", serverName,
			"authorization_server", protectedResourceMetadata.AuthorizationServers[0],
			"registration_endpoint", authorizationServerMetadata.RegistrationEndpoint,
			"error", err)
		if lookupErr != nil {
			return clientRegistrationResponse{}, fmt.Errorf("%w - static OAuth client lookup also failed: %v", err, lookupErr)
		}
		return clientRegistrationResponse{}, err
	}

	return clientInfo, nil
}

// GetOAuthMetadataWithClient discovers OAuth protected resource and authorization server
// metadata for an HTTP MCP server using a custom HTTP client.
// Missing metadata endpoints are not errors.
func GetOAuthMetadataWithClient(ctx context.Context, httpClient *http.Client, server ServerConfig, clientName, redirectURL string) (OAuthMetadata, error) {
	if server.URL == "" {
		return OAuthMetadata{}, nil
	}

	authenticateHeader, initialized, err := wwwAuthenticateFromInitialize(ctx, httpClient, server)
	if err != nil {
		return OAuthMetadata{}, err
	}
	if initialized {
		return OAuthMetadata{}, nil
	}

	discovery, ok, err := discoverOAuthMetadata(ctx, httpClient, server.URL, authenticateHeader, clientName, redirectURL)
	if err != nil {
		return OAuthMetadata{}, err
	}
	if !ok {
		return OAuthMetadata{}, nil
	}

	clientRegistrationJSON, err := json.Marshal(discovery.ClientRegistration)
	if err != nil {
		return OAuthMetadata{}, fmt.Errorf("failed to marshal client registration: %w", err)
	}

	return OAuthMetadata{
		ProtectedResourceMetadataURL:      discovery.ProtectedResourceURL,
		AuthorizationServerMetadataURL:    discovery.AuthorizationServerMetadataURL,
		ProtectedResourceMetadata:         discovery.ProtectedResourceMetadataJSON,
		AuthorizationServerMetadata:       discovery.AuthorizationServerMetadataJSON,
		ClientRegistration:                clientRegistrationJSON,
		DynamicClientRegistration:         discovery.DynamicClientRegistration,
		ClientIDMetadataDocumentSupported: discovery.ClientIDMetadataDocumentSupported,
	}, nil
}

// registerOAuthClient dynamically registers an OAuth client with an
// authorization server.
func registerOAuthClient(ctx context.Context, client *http.Client, serverName string, authServer AuthorizationServerMetadata, clientRegistration ClientRegistrationMetadata) (clientRegistrationResponse, error) {
	if authServer.RegistrationEndpoint == "" {
		return clientRegistrationResponse{}, fmt.Errorf("registration endpoint is not set")
	}

	b, err := json.Marshal(clientRegistration)
	if err != nil {
		return clientRegistrationResponse{}, fmt.Errorf("failed to marshal client metadata: %w", err)
	}

	slog.Info("registering oauth client dynamically", "server", serverName, "registration_endpoint", authServer.RegistrationEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authServer.RegistrationEndpoint, bytes.NewReader(b))
	if err != nil {
		return clientRegistrationResponse{}, fmt.Errorf("failed to create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return clientRegistrationResponse{}, fmt.Errorf("failed to register client: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return clientRegistrationResponse{}, fmt.Errorf("unexpected status registering client (%d): %s", resp.StatusCode, string(body))
	}

	clientInfo, err := parseClientRegistrationResponse(resp.Body)
	if err != nil {
		return clientRegistrationResponse{}, fmt.Errorf("failed to parse client registration response: %w", err)
	}
	slog.Info("oauth client registration succeeded", "server", serverName, "registration_endpoint", authServer.RegistrationEndpoint)

	return clientInfo, nil
}

// GetOAuthAuthorizationURL constructs the OAuth authorization URL and callback
// state for the authorization code flow.
func GetOAuthAuthorizationURL(ctx context.Context, callbackHandler CallbackHandler, conf *oauth2.Config, authorizationEndpoint, connectURL string) (string, <-chan CallbackPayload, string, error) {
	// use PKCE to protect against CSRF attacks
	// https://www.ietf.org/archive/id/draft-ietf-oauth-security-topics-22.html#name-countermeasures-6
	verifier := oauth2.GenerateVerifier()

	state, ch, err := callbackHandler.NewState(ctx, conf, verifier)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to create state: %w", err)
	}

	authURL, err := AuthCodeURL(conf, authorizationEndpoint, connectURL, state, verifier)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to generate auth code URL: %w", err)
	}

	return authURL, ch, verifier, nil
}

// ExchangeOAuthToken exchanges an OAuth authorization code for a token.
func ExchangeOAuthToken(ctx context.Context, conf *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return tok, nil
}

func wwwAuthenticateFromInitialize(ctx context.Context, httpClient *http.Client, server ServerConfig) (string, bool, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().Unix(),
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "Obot MCP OAuth Metadata Client",
				"version": version.Get().String(),
			},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusOK {
		if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
			deleteReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, server.URL, http.NoBody)
			if err != nil {
				return "", true, err
			}
			deleteReq.Header.Set("Mcp-Session-Id", sessionID)
			deleteResp, err := httpClient.Do(deleteReq)
			if err != nil {
				return "", true, err
			}
			_, _ = io.Copy(io.Discard, deleteResp.Body)
			deleteResp.Body.Close()
		}
		return "", true, nil
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return "", false, nil
	}

	return resp.Header.Get("WWW-Authenticate"), false, nil
}

func getAuthServerMetadata(ctx context.Context, client *http.Client, authURL string) (AuthorizationServerMetadata, string, json.RawMessage, bool, error) {
	authServerURL := strings.TrimSuffix(authURL, "/")

	authServerMetadata := authServerURL
	// If the authServer URL has a path, then the well-known path is prepended to the path
	if u, err := url.Parse(authServerMetadata); err != nil {
		return AuthorizationServerMetadata{}, "", nil, false, fmt.Errorf("failed to parse auth server URL: %w", err)
	} else if u.Path != "" {
		u.Path = "/.well-known/oauth-authorization-server" + u.Path
		authServerMetadata = u.String()
	} else {
		authServerMetadata = fmt.Sprintf("%s/.well-known/oauth-authorization-server", authServerMetadata)
	}

	metadataURLs := []string{
		authServerMetadata,
		strings.Replace(authServerMetadata, "/.well-known/oauth-authorization-server", "/.well-known/openid-configuration", 1),
		strings.Replace(authServerMetadata, "/.well-known/oauth-authorization-server", "", 1) + "/.well-known/openid-configuration",
	}

	var (
		authorizationServerMetadataContent AuthorizationServerMetadata
		authorizationServerMetadataJSON    json.RawMessage
		metadataURL                        string
		found                              bool
	)
	for _, metadataURL = range metadataURLs {
		var err error
		authorizationServerMetadataJSON, found, err = getOAuthMetadataJSON(ctx, client, metadataURL)
		if err != nil {
			return AuthorizationServerMetadata{}, "", nil, false, err
		}
		if !found {
			continue
		}

		authorizationServerMetadataContent, err = parseAuthorizationServerMetadata(bytes.NewReader(authorizationServerMetadataJSON))
		if err != nil {
			return AuthorizationServerMetadata{}, "", nil, false, fmt.Errorf("failed to parse authorization server metadata: %w", err)
		}
		break
	}
	if !found {
		slog.Debug("authorization server metadata not found", "authorization_server", authServerURL, "metadata_urls", metadataURLs)
		return AuthorizationServerMetadata{}, "", nil, false, nil
	}
	slog.Debug("authorization server metadata found", "authorization_server", authServerURL, "metadata_url", metadataURL)

	if authorizationServerMetadataContent.AuthorizationEndpoint == "" {
		authorizationServerMetadataContent.AuthorizationEndpoint = fmt.Sprintf("%s/authorize", authServerURL)
	}
	if authorizationServerMetadataContent.TokenEndpoint == "" {
		authorizationServerMetadataContent.TokenEndpoint = fmt.Sprintf("%s/token", authServerURL)
	}
	if authorizationServerMetadataContent.RegistrationEndpoint == "" {
		authorizationServerMetadataContent.RegistrationEndpoint = fmt.Sprintf("%s/register", authServerURL)
	}

	return authorizationServerMetadataContent, metadataURL, authorizationServerMetadataJSON, true, nil
}

func getOAuthMetadataJSON(ctx context.Context, client *http.Client, metadataURL string) (json.RawMessage, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
		// 400-level error means that the endpoint is not present or not accessible, which is not an error for our purposes, but log it for debugging.
		body, _ := io.ReadAll(resp.Body)
		slog.Debug("metadata endpoint did not return 200 OK", "url", metadataURL, "status_code", resp.StatusCode, "response_body", string(body))
		return nil, false, nil
	} else if resp.StatusCode >= http.StatusInternalServerError {
		// 500-level error means that the endpoint is present but there is a problem with it, which is an error for our purposes.
		// Limit the amount of body we read here to avoid potential issues with very large error responses.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return nil, false, fmt.Errorf("metadata endpoint returned server error: %d - %s", resp.StatusCode, string(body))
	}

	metadata, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if !json.Valid(metadata) {
		return nil, false, fmt.Errorf("invalid JSON metadata")
	}

	return metadata, true, nil
}

// parseAuthorizationServerMetadata parses OAuth 2.0 Authorization Server Metadata
// from a reader containing JSON data as defined in RFC 8414
func parseAuthorizationServerMetadata(reader io.Reader) (AuthorizationServerMetadata, error) {
	var metadata AuthorizationServerMetadata
	if err := json.NewDecoder(reader).Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("failed to decode authorization server metadata: %w", err)
	}

	// Validate required fields
	if metadata.Issuer == "" {
		return metadata, fmt.Errorf("issuer is required but not provided")
	}

	if len(metadata.ResponseTypesSupported) == 0 {
		return metadata, fmt.Errorf("response_types_supported is required but not provided")
	}

	// Set default values for optional fields if not provided
	if len(metadata.ResponseModesSupported) == 0 {
		metadata.ResponseModesSupported = []string{"query", "fragment"}
	}

	if len(metadata.GrantTypesSupported) == 0 {
		metadata.GrantTypesSupported = []string{"authorization_code", "implicit"}
	}

	if len(metadata.TokenEndpointAuthMethodsSupported) == 0 {
		metadata.TokenEndpointAuthMethodsSupported = []string{"client_secret_basic"}
	}

	if len(metadata.RevocationEndpointAuthMethodsSupported) == 0 {
		metadata.RevocationEndpointAuthMethodsSupported = []string{"client_secret_basic"}
	}

	return metadata, nil
}

// parseProtectedResourceMetadata parses OAuth 2.0 Protected Resource Metadata
// from a reader containing JSON data as defined in RFC 8707
func parseProtectedResourceMetadata(reader io.Reader) (protectedResourceMetadata, error) {
	var metadata protectedResourceMetadata
	if err := json.NewDecoder(reader).Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("failed to decode protected resource metadata: %w", err)
	}

	// Validate required fields
	if metadata.Resource == "" {
		return metadata, fmt.Errorf("resource is required but not provided")
	}

	// Set default values for optional fields if not provided
	// According to RFC 8707, if bearer_methods_supported is omitted, no default bearer methods are implied
	// The empty array [] can be used to indicate that no bearer methods are supported
	// We don't set defaults here as the absence has specific meaning

	// Validate that resource_signing_alg_values_supported does not contain "none"
	if slices.Contains(metadata.ResourceSigningAlgValuesSupported, "none") {
		return metadata, fmt.Errorf("resource_signing_alg_values_supported must not contain 'none'")
	}

	return metadata, nil
}

// parseResourceMetadata extracts the resource_metadata URL from a Bearer authenticate header
func parseResourceMetadata(authenticateHeader string) string {
	// Use regex to find resource_metadata parameter
	// Pattern matches: resource_metadata="<URL>"
	matches := resourceMetadataRegex.FindStringSubmatch(authenticateHeader)

	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

// parseScopeFromAuthenticateHeader extracts the scope parameter from a Bearer authenticate header
func parseScopeFromAuthenticateHeader(authenticateHeader string) string {
	matches := scopeRegex.FindStringSubmatch(authenticateHeader)

	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

// AuthCodeURL returns the authorization code URL for the given configuration and resource URL.
func AuthCodeURL(conf *oauth2.Config, urlFromMetadata, resourceURL, state, verifier string) (string, error) {
	authEndpoint, err := url.Parse(urlFromMetadata)
	if err != nil {
		return "", fmt.Errorf("failed to parse authorization endpoint: %w", err)
	}

	// Redirect user to consent page to ask for permission for the scopes specified above.
	authCodeURLOpts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if authEndpoint.Host != "login.microsoftonline.com" {
		// Entra does not like the resource parameter, and including it will often cause things to fail.
		// VSCode does something similar to this.
		authCodeURLOpts = append(authCodeURLOpts, oauth2.SetAuthURLParam("resource", resourceURL))
	}
	if authEndpoint.Host != "mcp.zoho.com" {
		// Zoho doesn't support the access_type parameter
		authCodeURLOpts = append(authCodeURLOpts, oauth2.AccessTypeOffline)
	}

	return conf.AuthCodeURL(state, authCodeURLOpts...), nil
}

type resourceIdentifier string

func (r *resourceIdentifier) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		var resources []json.RawMessage
		if err := json.Unmarshal(data, &resources); err != nil {
			return fmt.Errorf("resource must be a string or singleton string array: %w", err)
		}
		if len(resources) != 1 {
			return fmt.Errorf("resource array must contain exactly one identifier")
		}
		data = resources[0]
	}

	var resource string
	if err := json.Unmarshal(data, &resource); err != nil {
		return fmt.Errorf("resource must be a string or singleton string array: %w", err)
	}
	if resource == "" {
		return fmt.Errorf("resource identifier must not be empty")
	}

	*r = resourceIdentifier(resource)
	return nil
}

// protectedResourceMetadata represents OAuth 2.0 Protected Resource Metadata
// as defined in RFC 8707
type protectedResourceMetadata struct {
	// REQUIRED. The protected resource's resource identifier
	Resource resourceIdentifier `json:"resource"`

	// OPTIONAL. JSON array containing a list of OAuth authorization server issuer identifiers
	AuthorizationServers []string `json:"authorization_servers,omitempty"`

	// OPTIONAL. URL of the protected resource's JSON Web Key (JWK) Set document
	JwksURI string `json:"jwks_uri,omitempty"`

	// RECOMMENDED. JSON array containing a list of scope values
	ScopesSupported []string `json:"scopes_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of the supported methods of sending an OAuth 2.0 bearer token
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of the JWS signing algorithms supported by the protected resource
	ResourceSigningAlgValuesSupported []string `json:"resource_signing_alg_values_supported,omitempty"`

	// OPTIONAL. Human-readable name of the protected resource intended for display to the end user
	ResourceName string `json:"resource_name,omitempty"`

	// OPTIONAL. URL of a page containing human-readable information that developers might want or need to know
	ResourceDocumentation string `json:"resource_documentation,omitempty"`

	// OPTIONAL. URL of a page containing human-readable information about the protected resource's requirements
	ResourcePolicyURI string `json:"resource_policy_uri,omitempty"`

	// OPTIONAL. URL of a page containing human-readable information about the protected resource's terms of service
	ResourceTosURI string `json:"resource_tos_uri,omitempty"`

	// OPTIONAL. Boolean value indicating protected resource support for mutual-TLS client certificate-bound access tokens
	TLSClientCertificateBoundAccessTokens bool `json:"tls_client_certificate_bound_access_tokens,omitempty"`

	// OPTIONAL. JSON array containing a list of the authorization details type values supported by the resource server
	AuthorizationDetailsTypesSupported []string `json:"authorization_details_types_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of the JWS alg values supported by the resource server for validating DPoP proof JWTs
	DPoPSigningAlgValuesSupported []string `json:"dpop_signing_alg_values_supported,omitempty"`

	// OPTIONAL. Boolean value specifying whether the protected resource always requires the use of DPoP-bound access tokens
	DPoPBoundAccessTokensRequired bool `json:"dpop_bound_access_tokens_required,omitempty"`
}

// AuthorizationServerMetadata represents OAuth 2.0 Authorization Server Metadata
// as defined in RFC 8414
type AuthorizationServerMetadata struct {
	// REQUIRED. The authorization server's issuer identifier
	Issuer string `json:"issuer"`

	// URL of the authorization server's authorization endpoint
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`

	// URL of the authorization server's token endpoint
	TokenEndpoint string `json:"token_endpoint,omitempty"`

	// OPTIONAL. URL of the authorization server's JWK Set document
	JwksURI string `json:"jwks_uri,omitempty"`

	// OPTIONAL. URL of the authorization server's OAuth 2.0 Dynamic Client Registration endpoint
	RegistrationEndpoint string `json:"registration_endpoint,omitempty"`

	// RECOMMENDED. JSON array containing a list of the OAuth 2.0 scope values
	ScopesSupported []string `json:"scopes_supported,omitempty"`

	// REQUIRED. JSON array containing a list of the OAuth 2.0 response_type values
	ResponseTypesSupported []string `json:"response_types_supported"`

	// OPTIONAL. JSON array containing a list of the OAuth 2.0 response_mode values
	ResponseModesSupported []string `json:"response_modes_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of the OAuth 2.0 grant type values
	GrantTypesSupported []string `json:"grant_types_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of client authentication methods
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`

	// OPTIONAL. JSON array containing a list of the JWS signing algorithms
	TokenEndpointAuthSigningAlgValuesSupported []string `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`

	// OPTIONAL. URL of a page containing human-readable information
	ServiceDocumentation string `json:"service_documentation,omitempty"`

	// OPTIONAL. Languages and scripts supported for the user interface
	UILocalesSupported []string `json:"ui_locales_supported,omitempty"`

	// OPTIONAL. URL for authorization server's requirements on client data usage
	OpPolicyURI string `json:"op_policy_uri,omitempty"`

	// OPTIONAL. URL for authorization server's terms of service
	OpTosURI string `json:"op_tos_uri,omitempty"`

	// OPTIONAL. URL of the authorization server's OAuth 2.0 revocation endpoint
	RevocationEndpoint string `json:"revocation_endpoint,omitempty"`

	// OPTIONAL. JSON array containing client authentication methods for revocation endpoint
	RevocationEndpointAuthMethodsSupported []string `json:"revocation_endpoint_auth_methods_supported,omitempty"`

	// OPTIONAL. JSON array containing JWS signing algorithms for revocation endpoint
	RevocationEndpointAuthSigningAlgValuesSupported []string `json:"revocation_endpoint_auth_signing_alg_values_supported,omitempty"`

	// OPTIONAL. URL of the authorization server's OAuth 2.0 introspection endpoint
	IntrospectionEndpoint string `json:"introspection_endpoint,omitempty"`

	// OPTIONAL. JSON array containing client authentication methods for introspection endpoint
	IntrospectionEndpointAuthMethodsSupported []string `json:"introspection_endpoint_auth_methods_supported,omitempty"`

	// OPTIONAL. JSON array containing JWS signing algorithms for introspection endpoint
	IntrospectionEndpointAuthSigningAlgValuesSupported []string `json:"introspection_endpoint_auth_signing_alg_values_supported,omitempty"`

	// OPTIONAL. JSON array containing PKCE code challenge methods
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported,omitempty"`

	// OPTIONAL. Boolean indicating whether the client ID metadata document is supported
	ClientIDMetadataDocumentSupported bool `json:"client_id_metadata_document_supported,omitempty"`
}

// ClientRegistrationMetadata represents OAuth 2.0 Dynamic Client Registration metadata
// as defined in RFC 7591, merged from protected resource and authorization server metadata
type ClientRegistrationMetadata struct {
	// Array of redirection URI strings for use in redirect-based flows
	RedirectURIs []string `json:"redirect_uris,omitempty"`

	// String indicator of the requested authentication method for the token endpoint
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`

	// Array of OAuth 2.0 grant type strings that the client can use at the token endpoint
	GrantTypes []string `json:"grant_types,omitempty"`

	// Array of the OAuth 2.0 response type strings that the client can use at the authorization endpoint
	ResponseTypes []string `json:"response_types,omitempty"`

	// Human-readable string name of the client to be presented to the end-user during authorization
	ClientName string `json:"client_name,omitempty"`

	// URL string of a web page providing information about the client
	ClientURI string `json:"client_uri,omitempty"`

	// URL string that references a logo for the client
	LogoURI string `json:"logo_uri,omitempty"`

	// String containing a space-separated list of scope values
	Scope string `json:"scope,omitempty"`

	// Array of strings representing ways to contact people responsible for this client
	Contacts []string `json:"contacts,omitempty"`

	// URL string that points to a human-readable terms of service document for the client
	TosURI string `json:"tos_uri,omitempty"`

	// URL string that points to a human-readable privacy policy document
	PolicyURI string `json:"policy_uri,omitempty"`

	// URL string referencing the client's JSON Web Key (JWK) Set document
	JwksURI string `json:"jwks_uri,omitempty"`

	// Client's JSON Web Key Set document value
	Jwks any `json:"jwks,omitempty"`

	// A unique identifier string assigned by the client developer or software publisher
	SoftwareID string `json:"software_id,omitempty"`

	// A version identifier string for the client software identified by "software_id"
	SoftwareVersion string `json:"software_version,omitempty"`
}

// AuthServerMetadataToClientRegistration converts an AuthorizationServerMetadata to a ClientRegistrationMetadata for dynamic registration.
func AuthServerMetadataToClientRegistration(authServer AuthorizationServerMetadata, clientName, redirectURL, scope string) ClientRegistrationMetadata {
	merged := ClientRegistrationMetadata{}

	// Set default values based on OAuth 2.0 specifications

	// token_endpoint_auth_method: default is "client_secret_basic" if not specified
	if len(authServer.TokenEndpointAuthMethodsSupported) > 0 {
		merged.TokenEndpointAuthMethod = authServer.TokenEndpointAuthMethodsSupported[0]
	} else {
		merged.TokenEndpointAuthMethod = "client_secret_basic"
	}

	merged.GrantTypes = supportedClientGrantTypes(authServer.GrantTypesSupported)

	// response_types: default is "code" if not specified
	if len(authServer.ResponseTypesSupported) > 0 {
		merged.ResponseTypes = authServer.ResponseTypesSupported
	} else {
		merged.ResponseTypes = []string{"code"}
	}

	if scope != "" {
		merged.Scope = scope
	}
	if clientName != "" {
		merged.ClientName = clientName
	}
	if redirectURL != "" {
		merged.RedirectURIs = []string{redirectURL}
	}

	return merged
}

func supportedClientGrantTypes(grantTypesSupported []string) []string {
	supported := make(map[string]struct{}, len(grantTypesSupported))
	for _, grantType := range grantTypesSupported {
		supported[grantType] = struct{}{}
	}

	var grantTypes []string
	for _, grantType := range []string{"authorization_code", "refresh_token"} {
		if _, ok := supported[grantType]; ok {
			grantTypes = append(grantTypes, grantType)
		}
	}

	return grantTypes
}

// clientRegistrationResponse represents OAuth 2.0 Dynamic Client Registration Response
// as defined in RFC 7591
type clientRegistrationResponse struct {
	// REQUIRED. OAuth 2.0 client identifier string. It SHOULD NOT be
	// currently valid for any other registered client, though an
	// authorization server MAY issue the same client identifier to
	// multiple instances of a registered client at its discretion.
	ClientID string `json:"client_id"`

	// OPTIONAL. OAuth 2.0 client secret string. If issued, this MUST
	// be unique for each "client_id" and SHOULD be unique for multiple
	// instances of a client using the same "client_id". This value is
	// used by confidential clients to authenticate to the token
	// endpoint, as described in OAuth 2.0 [RFC6749], Section 2.3.1.
	ClientSecret string `json:"client_secret,omitempty"`

	// OPTIONAL. Time at which the client identifier was issued. The
	// time is represented as the number of seconds from
	// 1970-01-01T00:00:00Z as measured in UTC until the date/time of
	// issuance.
	ClientIDIssuedAt *int64 `json:"client_id_issued_at,omitempty"`

	// REQUIRED if "client_secret" is issued. Time at which the client
	// secret will expire or 0 if it will not expire. The time is
	// represented as the number of seconds from 1970-01-01T00:00:00Z as
	// measured in UTC until the date/time of expiration.
	ClientSecretExpiresAt *int64 `json:"client_secret_expires_at,omitempty"`
}

// parseClientRegistrationResponse parses OAuth 2.0 Dynamic Client Registration Response
// from a reader containing JSON data as defined in RFC 7591
func parseClientRegistrationResponse(reader io.Reader) (clientRegistrationResponse, error) {
	var response clientRegistrationResponse
	if err := json.NewDecoder(reader).Decode(&response); err != nil {
		return response, fmt.Errorf("failed to decode client registration response: %w", err)
	}

	// Validate required fields
	if response.ClientID == "" {
		return response, fmt.Errorf("client_id is required but not provided")
	}

	return response, nil
}
