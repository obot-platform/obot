package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/oauth2"
)

var (
	// ErrOAuthNotRequired reports that an MCP server accepted an unauthenticated connection and listed its tools without
	// asking for authorization, and publishes no OAuth protected resource metadata, so there is no OAuth to authorize for
	// it (SessionManager.StartOAuthAuthorization).
	ErrOAuthNotRequired = errors.New("the MCP server does not require OAuth")

	// strictEgressClient is the fallback for OAuth token requests when the caller passes no
	// client. Callers should pass the client from SessionManager.OAuthClientForServer, which
	// uses the server's tunnel when it has one, so metadata and tokens take the same path.
	// This fallback only ensures a server-supplied token endpoint is never reached with
	// http.DefaultClient, which applies no egress policy.
	strictEgressClient = safehttp.NewClient(safehttp.Options{
		BlockLoopback:  true,
		BlockPrivateIP: true,
		BlockLinkLocal: true,
		Timeout:        30 * time.Second,
	})
)

// assumeOAuthRequiredTransport synthesizes the initialize challenge used to
// begin standard OAuth metadata discovery. All subsequent metadata requests use
// the original, policy-enforcing transport.
type assumeOAuthRequiredTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	done bool
}

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
	NewState(context.Context, *oauth2.Config, string, string) (string, <-chan CallbackPayload, error)
}

type ClientCredLookup interface {
	Lookup(context.Context) (string, string, error)
}

type oauth struct {
	redirectURL              string
	clientName               string
	serverName               string
	serverURL                string
	clientIDMetadataDocument string
	currentToken             oauth2.Token

	// httpClient performs discovery, client registration, and token exchange and refresh.
	// Those URLs are server-supplied, so it applies the egress policy and carries none of
	// the server's headers (see SessionManager.OAuthClientForServer).
	httpClient      *http.Client
	callbackHandler CallbackHandler
	clientLookup    ClientCredLookup
	tokenStorage    TokenStorage
}

// oauthAuthorization is shared by preflight and captured challenges after metadata discovery.
type oauthAuthorization struct {
	authURL         string
	callback        <-chan CallbackPayload
	verifier        string
	conf            *oauth2.Config
	discovery       oauthMetadataDiscovery
	hasClientSecret bool
}

// oauthProbe is how an MCP server answered the unauthenticated requests that probeOAuthChallenge sends it.
type oauthProbe struct {
	// challenged reports that the server answered a request with 401, and header is the WWW-Authenticate header of that
	// answer, which may be empty.
	challenged bool
	header     string
	// initialized reports that the server accepted the unauthenticated initialize request.
	initialized bool
	// initializeStatus is the status the server answered initialize with when it neither accepted nor challenged it.
	initializeStatus int
	// listFailure is set when the server answered tools/list with neither a result nor a challenge, so whether it asks
	// for OAuth is unknown.
	listFailure error
}

type oauthMetadataDiscovery struct {
	ProtectedResourceURL              string
	ResourceURL                       string
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
	ResourceURL                       string          `json:"resourceUrl,omitempty"`
	AuthorizationServerMetadataURL    string          `json:"authorizationServerMetadataUrl,omitempty"`
	ProtectedResourceMetadata         json.RawMessage `json:"protectedResourceMetadata,omitempty"`
	AuthorizationServerMetadata       json.RawMessage `json:"authorizationServerMetadata,omitempty"`
	ClientRegistration                json.RawMessage `json:"clientRegistration,omitempty"`
	DynamicClientRegistration         bool            `json:"dynamicClientRegistration,omitempty"`
	ClientIDMetadataDocumentSupported bool            `json:"clientIdMetadataDocumentSupported,omitempty"`
}

type resourceIdentifier string

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

type authorizationErrorOAuthHandler struct{}

func (authorizationErrorOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return nil, nil
}

func (authorizationErrorOAuthHandler) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	authErr := &mmmcp.AuthorizationError{StatusCode: resp.StatusCode}
	challenges, err := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	if err != nil {
		return authErr
	}
	for _, challenge := range challenges {
		if challenge.Scheme != "bearer" {
			continue
		}
		if authErr.ResourceMetadata == "" {
			authErr.ResourceMetadata = challenge.Params["resource_metadata"]
		}
		if authErr.Scope == "" {
			authErr.Scope = challenge.Params["scope"]
			authErr.Scopes = strings.Fields(authErr.Scope)
		}
		if authErr.ErrorCode == "" {
			authErr.ErrorCode = challenge.Params["error"]
		}
		if authErr.ErrorDescription == "" {
			authErr.ErrorDescription = challenge.Params["error_description"]
		}
	}
	return authErr
}

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
	// Discovery follows server-supplied URLs, so use the OAuth client: the server's tunnel when
	// it has one, otherwise the egress policy, and never the server's configured headers.
	oauthClient, err := m.OAuthClientForServer(serverConfig)
	if err != nil {
		return OAuthMetadata{}, err
	}
	httpClient := *oauthClient
	httpClient.Timeout = 5 * time.Second

	return getOAuthMetadataWithClient(ctx, &httpClient, serverConfig, clientName, redirectURL, assumeOAuthRequired)
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

func newOAuth(httpClient *http.Client, callbackHandler CallbackHandler, clientLookup ClientCredLookup, tokenStorage TokenStorage, serverName, serverURL, clientName, redirectURL, clientIDMetadataDocument string) *oauth {
	return &oauth{
		serverName:               serverName,
		serverURL:                serverURL,
		clientName:               clientName,
		redirectURL:              redirectURL,
		clientIDMetadataDocument: clientIDMetadataDocument,
		callbackHandler:          callbackHandler,
		httpClient:               httpClient,
		clientLookup:             clientLookup,
		tokenStorage:             tokenStorage,
	}
}

func (o *oauth) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	return o.tokenStorage.TokenSource(withOAuthHTTPClient(ctx, o.httpClient))
}

func (o *oauth) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	// This function is responsible for closing the response body.
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	start, err := o.startAuthorization(ctx, req.URL.String(), resp.Header.Get("WWW-Authenticate"))
	if err != nil {
		return err
	}

	authURL, ch, verifier, conf, discovery := start.authURL, start.callback, start.verifier, start.conf, start.discovery
	connectURL := req.URL.String()
	authorizationServerMetadata := discovery.AuthorizationServerMetadata
	resourceURL := ResolveOAuthResourceURL(authorizationServerMetadata.AuthorizationEndpoint, discovery.ResourceURL, connectURL)

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

	tok, err := ExchangeOAuthToken(withOAuthHTTPClient(ctx, o.httpClient), conf, cb.Code, verifier, resourceURL)
	if err != nil {
		slog.Warn("oauth code exchange failed",
			"server", o.serverName,
			"connect_url", connectURL,
			"token_endpoint", conf.Endpoint.TokenURL,
			"token_endpoint_auth_method", discovery.ClientRegistration.TokenEndpointAuthMethod,
			"has_client_secret", start.hasClientSecret,
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

// discoverOAuthMetadata discovers OAuth metadata for the MCP server at baseURL. The protected
// resource metadata must name a resource that belongs to resourceCheckURL, the server's own URL,
// which differs from baseURL when the server is reached through a tunnel.
func discoverOAuthMetadata(ctx context.Context, client *http.Client, baseURL, resourceCheckURL, authenticateHeader, clientName, redirectURL string) (oauthMetadataDiscovery, bool, error) {
	finalResourceMetadataURL, protectedResourceMetadataJSON, scope, ok, err := findProtectedResourceMetadata(ctx, client, baseURL, authenticateHeader)
	if err != nil {
		return oauthMetadataDiscovery{}, false, err
	}

	var protectedResourceMetadata protectedResourceMetadata
	if ok {
		protectedResourceMetadata, err = parseProtectedResourceMetadata(bytes.NewReader(protectedResourceMetadataJSON))
		if err != nil {
			return oauthMetadataDiscovery{}, false, fmt.Errorf("failed to parse protected resource metadata: %w", err)
		}
		if err := validateProtectedResource(string(protectedResourceMetadata.Resource), resourceCheckURL); err != nil {
			return oauthMetadataDiscovery{}, false, err
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

	// Some servers (e.g. Vercel) only issue refresh tokens for offline_access but leave it out of the
	// resource's scopes, so request it whenever the authorization server supports refreshing.
	// An empty scope is left alone so the server's default scopes still apply.
	if scope != "" && !slices.Contains(strings.Fields(scope), "offline_access") &&
		slices.Contains(authorizationServerMetadata.ScopesSupported, "offline_access") &&
		slices.Contains(authorizationServerMetadata.GrantTypesSupported, "refresh_token") {
		scope += " offline_access"
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
		ResourceURL:                       string(protectedResourceMetadata.Resource),
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

// findProtectedResourceMetadata looks for the OAuth protected resource metadata (RFC 9728) of the MCP server at baseURL:
// at the URL that authenticateHeader's Bearer challenge names, or, when it names none, at the well-known locations for
// baseURL, in order (oauthResourceMetadataURLs). It returns the URL it looked at last, which is where it found the
// metadata when found is true, the metadata as served, unparsed, and the scope the challenge asked for. Metadata that is
// not found, at a location that answers 4xx, is not an error; a location that answers 5xx is.
func findProtectedResourceMetadata(ctx context.Context, client *http.Client, baseURL, authenticateHeader string) (metadataURL *url.URL, metadata json.RawMessage, scope string, found bool, err error) {
	metadataURLs, scope, err := oauthResourceMetadataURLs(baseURL, authenticateHeader)
	if err != nil {
		return nil, nil, "", false, err
	}
	for _, metadataURL = range metadataURLs {
		slog.Info("fetching protected resource metadata", "url", metadataURL)
		metadata, found, err = getOAuthMetadataJSON(ctx, client, metadataURL.String())
		if err != nil {
			return nil, nil, "", false, fmt.Errorf("failed to get protected resource metadata: %w", err)
		}
		if found {
			break
		}
	}
	return metadataURL, metadata, scope, found, nil
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

func tokenEndpointAuthStyle(tokenEndpointAuthMethod string, hasClientSecret, staticClient bool) oauth2.AuthStyle {
	if staticClient {
		return oauth2.AuthStyleAutoDetect
	}

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

func (o *oauth) resolveClientInfo(ctx context.Context, serverName string, discovery oauthMetadataDiscovery) (clientRegistrationResponse, bool, error) {
	authorizationServerMetadata := discovery.AuthorizationServerMetadata
	protectedResourceMetadata := discovery.ProtectedResourceMetadata

	// Prefer configured static credentials: CIMD support does not guarantee the upstream accepts our document URL.
	var (
		clientInfo clientRegistrationResponse
		lookupErr  error
	)
	if o.clientLookup != nil {
		clientInfo.ClientID, clientInfo.ClientSecret, lookupErr = o.clientLookup.Lookup(ctx)
	}
	if lookupErr == nil && clientInfo.ClientID != "" {
		slog.Info("using static oauth client credentials", "server", serverName, "authorization_server", protectedResourceMetadata.AuthorizationServers[0])
		return clientInfo, true, nil
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

	if authorizationServerMetadata.ClientIDMetadataDocumentSupported && o.clientIDMetadataDocument != "" {
		slog.Info("using oauth client ID metadata document", "server", serverName, "client_id", o.clientIDMetadataDocument)
		return clientRegistrationResponse{
			ClientID: o.clientIDMetadataDocument,
		}, false, nil
	}

	// Without static credentials or CIMD, register a client dynamically.
	clientInfo, err := registerOAuthClient(ctx, o.httpClient, serverName, authorizationServerMetadata, discovery.ClientRegistration)
	if err != nil {
		slog.Warn("oauth dynamic client registration failed",
			"server", serverName,
			"authorization_server", protectedResourceMetadata.AuthorizationServers[0],
			"registration_endpoint", authorizationServerMetadata.RegistrationEndpoint,
			"error", err)
		if lookupErr != nil {
			return clientRegistrationResponse{}, false, fmt.Errorf("%w - static OAuth client lookup also failed: %v", err, lookupErr)
		}
		return clientRegistrationResponse{}, false, err
	}

	return clientInfo, false, nil
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

	discovery, ok, err := discoverOAuthMetadata(ctx, httpClient, server.URL, server.URL, authenticateHeader, clientName, redirectURL)
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
		ResourceURL:                       discovery.ResourceURL,
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
func GetOAuthAuthorizationURL(ctx context.Context, callbackHandler CallbackHandler, conf *oauth2.Config, authorizationEndpoint, resourceURL string) (string, <-chan CallbackPayload, string, error) {
	// use PKCE to protect against CSRF attacks
	// https://www.ietf.org/archive/id/draft-ietf-oauth-security-topics-22.html#name-countermeasures-6
	verifier := oauth2.GenerateVerifier()

	resourceURL = OAuthResourceURL(authorizationEndpoint, resourceURL)
	state, ch, err := callbackHandler.NewState(ctx, conf, resourceURL, verifier)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to create state: %w", err)
	}

	authURL, err := AuthCodeURL(conf, authorizationEndpoint, resourceURL, state, verifier)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to generate auth code URL: %w", err)
	}

	return authURL, ch, verifier, nil
}

// withOAuthHTTPClient returns ctx carrying client as the HTTP client that
// golang.org/x/oauth2 uses for token exchange and refresh requests.
func withOAuthHTTPClient(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}

// oauthHTTPClientContext returns ctx with an HTTP client for oauth2 set, keeping the
// caller's client when it has one and falling back to strictEgressClient otherwise.
func oauthHTTPClientContext(ctx context.Context) context.Context {
	if _, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		return ctx
	}
	return withOAuthHTTPClient(ctx, strictEgressClient)
}

// ExchangeOAuthToken exchanges an OAuth authorization code for a token. The token
// endpoint is server-supplied, so the request goes through the HTTP client in ctx
// (see withOAuthHTTPClient) or, without one, through strictEgressClient.
func ExchangeOAuthToken(ctx context.Context, conf *oauth2.Config, code, verifier, resourceURL string) (*oauth2.Token, error) {
	ctx = oauthHTTPClientContext(ctx)
	options := []oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}
	if resourceURL = OAuthResourceURL(conf.Endpoint.AuthURL, resourceURL); resourceURL != "" {
		options = append(options, oauth2.SetAuthURLParam("resource", resourceURL))
	}
	tok, err := conf.Exchange(ctx, code, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return tok, nil
}

// oauthProbeInitializeRequest is the body of the unauthenticated initialize request that Obot sends an MCP server to
// learn whether it asks for OAuth.
func oauthProbeInitializeRequest() ([]byte, error) {
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
		return nil, fmt.Errorf("failed to marshal initialize request: %w", err)
	}
	return body, nil
}

func wwwAuthenticateFromInitialize(ctx context.Context, httpClient *http.Client, server ServerConfig) (string, bool, error) {
	body, err := oauthProbeInitializeRequest()
	if err != nil {
		return "", false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

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

// probeOAuthChallenge learns whether the MCP server at serverURL asks for OAuth, from unauthenticated requests sent with
// client, which must carry none of the server's configured headers and no token (SessionManager.OAuthClientForServer):
//
//  1. An initialize request. A server that answers it with 401 is challenged there; one that answers with anything but
//     200 or 401 is neither initialized nor challenged.
//  2. If it was accepted, the initialized notification and then a tools/list request, in the session it started, if
//     any, with the protocol version it agreed on. Some servers let anyone initialize, and only challenge once tools are
//     requested. A 401 to either is the challenge. tools/list answered with anything else but success sets listFailure.
//
// The session is always ended with a DELETE, once the requests in it are done or have failed, even if ctx is done by
// then. Every request, the DELETE included, is bounded by client's timeout, so a server that never answers fails the
// probe once that runs out. A request that fails before it is answered fails the probe.
func probeOAuthChallenge(ctx context.Context, client *http.Client, serverURL string) (oauthProbe, error) {
	initialize, err := oauthProbeInitializeRequest()
	if err != nil {
		return oauthProbe{}, err
	}
	resp, err := sendOAuthProbeMessage(ctx, client, serverURL, "", "", initialize)
	if err != nil {
		return oauthProbe{}, fmt.Errorf("initialize: %w", err)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	protocolVersion := negotiatedProtocolVersion(resp)
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return oauthProbe{challenged: true, header: resp.Header.Get("WWW-Authenticate")}, nil
	default:
		return oauthProbe{initializeStatus: resp.StatusCode}, nil
	}

	probe := oauthProbe{initialized: true}
	if sessionID != "" {
		// Not bound to ctx, so that a request that ran out of time still ends its session.
		defer endOAuthProbeSession(context.WithoutCancel(ctx), client, serverURL, sessionID, protocolVersion)
	}
	for _, message := range []struct {
		method string
		body   string
	}{
		{method: "notifications/initialized", body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`},
		{method: "tools/list", body: `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`},
	} {
		resp, err := sendOAuthProbeMessage(ctx, client, serverURL, sessionID, protocolVersion, []byte(message.body))
		if err != nil {
			return oauthProbe{}, fmt.Errorf("%s: %w", message.method, err)
		}
		// Only the status and the challenge are needed, so the body, which may be a stream, is never waited for.
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			probe.challenged, probe.header = true, resp.Header.Get("WWW-Authenticate")
			return probe, nil
		case message.method == "tools/list" && (resp.StatusCode < 200 || resp.StatusCode > 299):
			probe.listFailure = fmt.Errorf("tools/list was answered with status %d", resp.StatusCode)
		}
	}
	return probe, nil
}

// sendOAuthProbeMessage posts one JSON-RPC message of probeOAuthChallenge to the MCP server at serverURL, in session
// sessionID if it is set, with the protocol version header if one was agreed on, and no other header than the
// transport's own.
func sendOAuthProbeMessage(ctx context.Context, client *http.Client, serverURL, sessionID, protocolVersion string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if protocolVersion != "" {
		req.Header.Set("Mcp-Protocol-Version", protocolVersion)
	}
	return client.Do(req)
}

// endOAuthProbeSession ends the MCP session sessionID that probeOAuthChallenge started. A failure is only logged: the
// probe already has its answer, and the server expires the session on its own.
func endOAuthProbeSession(ctx context.Context, client *http.Client, serverURL, sessionID, protocolVersion string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, serverURL, http.NoBody)
	if err != nil {
		slog.Warn("Failed to end the MCP session of an OAuth probe", "error", err)
		return
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	if protocolVersion != "" {
		req.Header.Set("Mcp-Protocol-Version", protocolVersion)
	}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("Failed to end the MCP session of an OAuth probe", "error", err)
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// negotiatedProtocolVersion returns the protocol version that resp, a server's answer to initialize, agreed on, from
// its JSON or event-stream body, or "" if it names none that can be sent back in a header. It reads only as much of the
// body as it needs, and at most 1 MiB.
func negotiatedProtocolVersion(resp *http.Response) string {
	var message struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	valid := func(version string) bool {
		return version != "" && len(version) <= 64 && !strings.ContainsFunc(version, func(r rune) bool {
			return (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' && r != '.' && r != '_'
		})
	}
	body := io.LimitReader(resp.Body, 1<<20)
	if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType == "text/event-stream" {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data:")
			if ok && json.Unmarshal([]byte(data), &message) == nil && message.Result.ProtocolVersion != "" {
				if valid(message.Result.ProtocolVersion) {
					return message.Result.ProtocolVersion
				}
				return ""
			}
		}
		return ""
	}
	if json.NewDecoder(body).Decode(&message) != nil || !valid(message.Result.ProtocolVersion) {
		return ""
	}
	return message.Result.ProtocolVersion
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

// ParseOAuthResourceURL returns the resource identifier from protected-resource metadata.
func ParseOAuthResourceURL(metadata json.RawMessage) (string, error) {
	parsed, err := parseProtectedResourceMetadata(bytes.NewReader(metadata))
	if err != nil {
		return "", err
	}
	return string(parsed.Resource), nil
}

// validateProtectedResource rejects protected resource metadata whose resource identifier
// does not belong to the MCP server Obot connected to. RFC 9728 says a client must not use
// metadata whose resource does not match the resource it requested. Without this check, a
// server could name another service as the resource, and Obot would obtain a token for that
// service and send it to the server. This follows the MCP TypeScript SDK's
// checkResourceAllowed: the resource must have the same origin as the connect URL, and its
// path must be the connect URL's path or a parent of it, so a server can identify itself by
// its origin but not by another service's path on the same host.
func validateProtectedResource(resource, connectURL string) error {
	r, err := url.Parse(resource)
	if err != nil {
		return fmt.Errorf("invalid resource %q in protected resource metadata: %w", resource, err)
	}
	c, err := url.Parse(connectURL)
	if err != nil {
		return fmt.Errorf("failed to parse MCP URL: %w", err)
	}

	// RFC 8707 forbids a fragment in a resource identifier. A query can identify a different
	// tenant on the same path, so a resource that has one must match the server URL's query.
	if strings.Contains(resource, "#") {
		return fmt.Errorf("protected resource metadata resource %q must not include a fragment", resource)
	}
	if r.RawQuery != "" && r.RawQuery != c.RawQuery {
		return fmt.Errorf("protected resource metadata resource %q has a query that does not match MCP server URL %q", resource, connectURL)
	}
	if hasAmbiguousPath(c) {
		return fmt.Errorf("MCP server URL %q has a path that cannot be safely compared for OAuth: remove any percent-encoding, \";\", \"\\\", or \".\" or \"..\" segments", connectURL)
	}
	if hasAmbiguousPath(r) {
		return fmt.Errorf("protected resource metadata resource %q has a path that cannot be safely compared for OAuth: it contains percent-encoding, \";\", \"\\\", or a \".\" or \"..\" segment", resource)
	}
	if !strings.EqualFold(r.Scheme, c.Scheme) ||
		!strings.EqualFold(r.Hostname(), c.Hostname()) ||
		urlPort(r) != urlPort(c) ||
		!isParentOrSamePath(r.EscapedPath(), c.EscapedPath()) {
		return fmt.Errorf("protected resource metadata resource %q does not match MCP server URL %q", resource, connectURL)
	}
	return nil
}

// hasAmbiguousPath reports whether u's path could be read as a different path by the servers
// and proxies between Obot and the MCP server. The prefix check compares paths literally, but
// those servers resolve "." and ".." segments, and some first decode percent-encoding, treat a
// backslash as "/", or strip ";" parameters. Rather than try to predict every normalization,
// reject any path with a dot segment, a percent-encoded character, a backslash, or a ";".
func hasAmbiguousPath(u *url.URL) bool {
	if strings.ContainsAny(u.EscapedPath(), `%;\`) {
		return true
	}
	segments := strings.Split(u.Path, "/")
	return slices.Contains(segments, ".") || slices.Contains(segments, "..")
}

// isParentOrSamePath reports whether resourcePath is connectPath or one of its parents, at a
// segment boundary. A trailing slash is ignored on both paths, so "/mcp" and "/mcp/" match each
// other. This is looser than the MCP TypeScript SDK, which rejects a resource of "/mcp/" for an
// endpoint at "/mcp", but both paths belong to the same server.
func isParentOrSamePath(resourcePath, connectPath string) bool {
	if !strings.HasSuffix(resourcePath, "/") {
		resourcePath += "/"
	}
	if !strings.HasSuffix(connectPath, "/") {
		connectPath += "/"
	}
	return strings.HasPrefix(connectPath, resourcePath)
}

// urlPort returns u's port as a canonical number, or the default port for its scheme.
func urlPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err == nil {
			return strconv.Itoa(n)
		}
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// These parameters belong to one Bearer challenge, as parsed by the SDK.
func parseResourceMetadata(authenticateHeader string) string {
	return bearerParameter(authenticateHeader, "resource_metadata")
}

func parseScopeFromAuthenticateHeader(authenticateHeader string) string {
	return bearerParameter(authenticateHeader, "scope")
}

func bearerParameter(header, parameter string) string {
	challenges, err := oauthex.ParseWWWAuthenticate([]string{header})
	if err != nil {
		return ""
	}

	for _, challenge := range challenges {
		if strings.EqualFold(challenge.Scheme, "bearer") {
			return challenge.Params[parameter]
		}
	}

	return ""
}

// AuthCodeURL returns the authorization code URL for the given configuration and resource URL.
func AuthCodeURL(conf *oauth2.Config, urlFromMetadata, resourceURL, state, verifier string) (string, error) {
	authEndpoint, err := url.Parse(urlFromMetadata)
	if err != nil {
		return "", fmt.Errorf("failed to parse authorization endpoint: %w", err)
	}

	// Redirect user to consent page to ask for permission for the scopes specified above.
	authCodeURLOpts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if resourceURL = OAuthResourceURL(authEndpoint.String(), resourceURL); resourceURL != "" {
		// Entra does not like the resource parameter, and including it will often cause things to fail.
		// VSCode does something similar to this.
		authCodeURLOpts = append(authCodeURLOpts, oauth2.SetAuthURLParam("resource", resourceURL))
	}
	if authEndpoint.Host != "mcp.zoho.com" {
		// Zoho doesn't support the access_type parameter
		authCodeURLOpts = append(authCodeURLOpts, oauth2.AccessTypeOffline)
	}
	if strings.EqualFold(authEndpoint.Hostname(), "accounts.google.com") {
		// Google needs renewed consent to issue refresh tokens on repeat authorization.
		// https://developers.google.com/identity/protocols/oauth2/web-server#offline
		authCodeURLOpts = append(authCodeURLOpts, oauth2.SetAuthURLParam("prompt", "consent"))
	}

	return conf.AuthCodeURL(state, authCodeURLOpts...), nil
}

// OAuthResourceURL returns the protected resource identifier when the
// authorization server supports the RFC 8707 resource parameter. Microsoft
// Entra rejects this parameter, so it must be omitted throughout the flow.
func OAuthResourceURL(authorizationEndpoint, resourceURL string) string {
	authEndpoint, err := url.Parse(authorizationEndpoint)
	if err == nil {
		switch strings.ToLower(authEndpoint.Hostname()) {
		case "login.microsoftonline.com", "login.microsoftonline.us", "login.partner.microsoftonline.cn":
			return ""
		}
	}
	return resourceURL
}

// ResolveOAuthResourceURL prefers the discovered protected-resource identifier
// and falls back to the MCP connection URL before applying provider-specific handling.
func ResolveOAuthResourceURL(authorizationEndpoint, discoveredResourceURL, connectionURL string) string {
	if discoveredResourceURL == "" {
		discoveredResourceURL = connectionURL
	}
	return OAuthResourceURL(authorizationEndpoint, discoveredResourceURL)
}

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

func (o *oauth) startAuthorization(ctx context.Context, connectURL, authenticateHeader string) (oauthAuthorization, error) {
	if o.callbackHandler == nil || o.redirectURL == "" {
		return oauthAuthorization{}, fmt.Errorf("oauth callback server is not configured")
	}

	slog.Info("starting oauth flow", "server", o.serverName, "connect_url", connectURL)

	resourceCheckURL := o.serverURL
	if resourceCheckURL == "" {
		resourceCheckURL = connectURL
	}

	discovery, ok, err := discoverOAuthMetadata(ctx, o.httpClient, connectURL, resourceCheckURL, authenticateHeader, o.clientName, o.redirectURL)
	if err != nil {
		slog.Warn("oauth metadata discovery failed", "server", o.serverName, "connect_url", connectURL, "error", err)

		return oauthAuthorization{}, err
	}

	if !ok {
		slog.Warn("oauth metadata discovery did not find authorization server metadata", "server", o.serverName, "connect_url", connectURL)

		return oauthAuthorization{}, fmt.Errorf("failed to get authorization server metadata")
	}

	authorizationServerMetadata := discovery.AuthorizationServerMetadata
	slog.Info("resolved oauth scope for server", "server", o.serverName, "scope", discovery.ClientRegistration.Scope)
	slog.Info("resolved authorization server", "server", o.serverName, "authorization_server", discovery.AuthorizationServerURL)

	if o.tokenStorage != nil {
		existing, _, err := o.tokenStorage.GetTokenConfig(ctx)
		if err != nil {
			return oauthAuthorization{}, err
		}

		// Replacing a grant must retain its scopes, but never carry them to a different provider.
		if existing != nil && existing.Endpoint.AuthURL == authorizationServerMetadata.AuthorizationEndpoint && existing.Endpoint.TokenURL == authorizationServerMetadata.TokenEndpoint {
			scopes := strings.Fields(discovery.ClientRegistration.Scope)
			for _, scope := range existing.Scopes {
				if !slices.Contains(scopes, scope) {
					scopes = append(scopes, scope)
				}
			}

			discovery.ClientRegistration.Scope = strings.Join(scopes, " ")
		}
	}

	clientInfo, staticClient, err := o.resolveClientInfo(ctx, o.serverName, discovery)
	if err != nil {
		return oauthAuthorization{}, err
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

	conf.Endpoint.AuthStyle = tokenEndpointAuthStyle(discovery.ClientRegistration.TokenEndpointAuthMethod, clientInfo.ClientSecret != "", staticClient)
	resourceURL := ResolveOAuthResourceURL(authorizationServerMetadata.AuthorizationEndpoint, discovery.ResourceURL, connectURL)
	authURL, ch, verifier, err := GetOAuthAuthorizationURL(ctx, o.callbackHandler, conf, authorizationServerMetadata.AuthorizationEndpoint, resourceURL)
	if err != nil {
		return oauthAuthorization{}, err
	}

	return oauthAuthorization{
		authURL:         authURL,
		callback:        ch,
		verifier:        verifier,
		conf:            conf,
		discovery:       discovery,
		hasClientSecret: clientInfo.ClientSecret != "",
	}, nil
}

// StartOAuthAuthorization starts a new OAuth authorization for server and returns the URL to authorize at, whether or
// not a token is stored for it, so that an administrator can replace a grant that still works. The authorization ends
// at opt.CallbackHandler's callback, which stores the token. Client credentials are resolved as for any other
// authorization: static credentials from opt.ClientLookup first, then a client ID metadata document, then dynamic
// registration. Its only caller is the start of a vMCP profile's OAuth grant, which an administrator asks for.
//
// It learns how the server asks for OAuth from unauthenticated requests sent with the OAuth client, which carries none
// of the server's configured headers and no token (probeOAuthChallenge), and starts from, in order:
//
//  1. The challenge to the initialize request, as it always has.
//  2. The challenge to the initialized notification or to tools/list, in the session that initialize started, for a
//     server that lets anyone initialize and only challenges once tools are requested. The authorization asks for the
//     scope that challenge names.
//  3. For a server that challenged neither, the protected resource metadata (RFC 9728) it publishes at the well-known
//     location for its URL, which covers a server that only challenges when a tool is called. An administrator asked
//     to authorize the server, and its metadata says that it is protected, so authorizing it is safe.
//
// A server that accepted initialize, challenged neither request, and publishes no protected resource metadata does not
// use OAuth, and is refused with ErrOAuthNotRequired, unless staticOAuth says that it requires administrator static
// OAuth, which then starts from the well-known metadata locations as before. So is a server that answered tools/list
// with an error, except that the error is returned instead, since whether it uses OAuth is unknown. A server that
// answered initialize with neither success nor a challenge starts from the well-known metadata locations, as before.
func (m *SessionManager) StartOAuthAuthorization(ctx context.Context, server ServerConfig, opt ClientOption, staticOAuth bool) (string, error) {
	client, err := m.OAuthClientForServer(server)
	if err != nil {
		return "", err
	}
	probe, err := probeOAuthChallenge(ctx, client, server.URL)
	if err != nil {
		return "", fmt.Errorf("failed to reach the MCP server to authorize: %w", err)
	}

	var resourceMetadata, scope string
	switch {
	case probe.challenged:
		resourceMetadata, scope = parseResourceMetadata(probe.header), parseScopeFromAuthenticateHeader(probe.header)
	case probe.initialized && !staticOAuth:
		metadataURL, _, _, found, err := findProtectedResourceMetadata(ctx, client, server.URL, "")
		if err != nil {
			return "", err
		}
		if !found {
			if probe.listFailure != nil {
				return "", fmt.Errorf("failed to reach the MCP server to authorize: %w", probe.listFailure)
			}
			return "", ErrOAuthNotRequired
		}
		// Discovery reads the metadata again from where it was found, and checks that it is for this server.
		resourceMetadata = metadataURL.String()
	}
	// Without resource_metadata, discovery looks at the well-known metadata locations.
	return startOAuthFromChallenge(ctx, client, server, opt, resourceMetadata, scope)
}

// ProbeOAuthRequirement learns whether the remote MCP server at serverURL, reached through the tunnel tunnelName if it
// is set, asks its callers for OAuth, for the vMCP controller, which records the answer for agent readiness and profile
// status to read (pkg/controller/handlers/vmcp). It never runs on a request. It sends the same unauthenticated requests
// as the start of a profile grant (probeOAuthChallenge), with the OAuth client, which carries none of the server's
// configured headers and no token, and ends any session it opens. Every request is bounded by the client's timeout, and
// all of them by ctx.
//
// The server asks for OAuth (true) when:
//
//   - It answered initialize, the initialized notification or tools/list with 401, and Obot's OAuth discovery would
//     start an authorization from that challenge: it finds the protected resource metadata that the challenge, or
//     else the well-known location for serverURL, names, and the authorization server's metadata. A 401 from which
//     no authorization could start is not taken for OAuth, and is an error.
//   - It challenged neither, and publishes protected resource metadata (RFC 9728) at the well-known location for
//     serverURL, as a server that only challenges once a tool is called does.
//
// It does not (false) when it accepted initialize and listed its tools without a challenge, and publishes no protected
// resource metadata. Anything else is an error, and whether it asks for OAuth is unknown: a request that failed or
// timed out, initialize answered with neither success nor 401, tools/list answered with an error, or metadata that
// could not be read. The error may hold serverURL.
func (m *SessionManager) ProbeOAuthRequirement(ctx context.Context, serverURL, tunnelName string) (bool, error) {
	client, err := m.OAuthClientForServer(ServerConfig{URL: serverURL, TunnelName: tunnelName})
	if err != nil {
		return false, err
	}
	probe, err := probeOAuthChallenge(ctx, client, serverURL)
	if err != nil {
		return false, err
	}
	if probe.challenged {
		// Discovery reads the challenge as the start of an authorization would (startOAuthFromChallenge).
		_, ok, err := discoverOAuthMetadata(ctx, client, serverURL, serverURL, probe.header, "", "")
		switch {
		case err != nil:
			return false, fmt.Errorf("discover the OAuth metadata the server's challenge names: %w", err)
		case !ok:
			return false, errors.New("the server answered with 401, but publishes no OAuth authorization server metadata")
		}
		return true, nil
	}
	_, _, _, found, err := findProtectedResourceMetadata(ctx, client, serverURL, "")
	switch {
	case err != nil:
		return false, err
	case found:
		return true, nil
	case !probe.initialized:
		return false, fmt.Errorf("initialize was answered with status %d", probe.initializeStatus)
	case probe.listFailure != nil:
		return false, probe.listFailure
	}
	return false, nil
}

// StartOAuthFromChallenge starts OAuth from an actual challenge without issuing any MCP operation.
func (m *SessionManager) StartOAuthFromChallenge(ctx context.Context, server ServerConfig, opt ClientOption, resourceMetadata, scope string) (string, error) {
	client, err := m.OAuthClientForServer(server)
	if err != nil {
		return "", err
	}

	return startOAuthFromChallenge(ctx, client, server, opt, resourceMetadata, scope)
}

func startOAuthFromChallenge(ctx context.Context, client *http.Client, server ServerConfig, opt ClientOption, resourceMetadata, scope string) (string, error) {
	o := newOAuth(client, opt.CallbackHandler, opt.ClientLookup, opt.TokenStorage, server.MCPServerName, server.URL, opt.OAuthClientName, opt.OAuthRedirectURL, opt.OAuthClientIDMetadataDocument)

	// Use a canonical header only to enter the existing metadata discovery path.
	// Empty resource_metadata uses safe well-known fallback, never initialize.
	header := "Bearer scope=" + strconv.Quote(scope)
	if resourceMetadata != "" {
		header += ", resource_metadata=" + strconv.Quote(resourceMetadata)
	}

	start, err := o.startAuthorization(ctx, server.URL, header)
	if err != nil {
		return "", err
	}

	return start.authURL, nil
}
