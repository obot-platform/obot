package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	sessionsapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/obot-platform/obot-entraid/tools/keycloak-auth-provider/pkg/profile"
	"github.com/obot-platform/tools/auth-providers-common/pkg/database"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/groups"
	"github.com/obot-platform/tools/auth-providers-common/pkg/secrets"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
	"github.com/sahilm/fuzzy"
)

// keycloakClient for Keycloak Admin API requests
var keycloakClient = &http.Client{
	Timeout: 30 * time.Second,
}

// adminTokenCache caches service account access tokens for Client Credentials flow
// Key: realm name, Value: access token
// TTL set to 4 minutes (Keycloak tokens typically valid for 5 minutes)
var adminTokenCache *expirable.LRU[string, string]

type Options struct {
	ClientID                 string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID"`
	ClientSecret             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET"`
	KeycloakURL              string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_URL"`   // e.g., https://keycloak.example.com
	KeycloakRealm            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_REALM"` // e.g., "obot"
	ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
	PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`
	// Cookie secret - provider-specific takes precedence, falls back to shared secret
	AuthCookieSecret         string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains         string `env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS" default:"*"`
	AuthTokenRefreshDuration string `env:"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION" optional:"true" default:"1h"`
	AllowedGroups            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS" optional:"true"`
	AllowedRoles             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES" optional:"true"`
	GroupCacheTTL            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_GROUP_CACHE_TTL" optional:"true" default:"1h"`
	// Admin service account credentials for Keycloak Admin API (optional - uses ClientID/ClientSecret if not provided)
	AdminClientID     string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_ID" optional:"true"`
	AdminClientSecret string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_SECRET" optional:"true"`
}

// sessionManagerAdapter implements state.SessionManager interface
// This adapter wraps OAuthProxy methods via closures to satisfy the interface
// without needing to import the OAuthProxy type (which is in package main)
type sessionManagerAdapter struct {
	loadSession func(*http.Request) (*sessionsapi.SessionState, error)
	serveHTTP   func(http.ResponseWriter, *http.Request)
	cookieOpts  *options.Cookie
}

func (s *sessionManagerAdapter) LoadCookiedSession(r *http.Request) (*sessionsapi.SessionState, error) {
	return s.loadSession(r)
}

func (s *sessionManagerAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serveHTTP(w, r)
}

func (s *sessionManagerAdapter) GetCookieOptions() *options.Cookie {
	return s.cookieOpts
}

func main() {
	var opts Options
	if err := env.LoadEnvForStruct(&opts); err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to load options: %v\n", err)
		os.Exit(1)
	}

	// Validate required Keycloak configuration
	if opts.KeycloakURL == "" {
		fmt.Printf("ERROR: keycloak-auth-provider: OBOT_KEYCLOAK_AUTH_PROVIDER_URL is required\n")
		os.Exit(1)
	}
	if opts.KeycloakRealm == "" {
		fmt.Printf("ERROR: keycloak-auth-provider: OBOT_KEYCLOAK_AUTH_PROVIDER_REALM is required\n")
		os.Exit(1)
	}

	refreshDuration, err := time.ParseDuration(opts.AuthTokenRefreshDuration)
	if err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to parse token refresh duration: %v\n", err)
		os.Exit(1)
	}

	if refreshDuration < 0 {
		fmt.Printf("ERROR: keycloak-auth-provider: token refresh duration must be greater than 0\n")
		os.Exit(1)
	}

	// Validate cookie secret entropy and format
	if err := secrets.ValidateCookieSecret(opts.AuthCookieSecret); err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: %v\n", err)
		fmt.Printf("Generate a valid secret with: openssl rand -base64 32\n")
		fmt.Printf("Or set OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET for provider-specific secret\n")
		os.Exit(1)
	}

	cookieSecret, err := base64.StdEncoding.DecodeString(opts.AuthCookieSecret)
	if err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to decode cookie secret: %v\n", err)
		os.Exit(1)
	}

	// Normalize Keycloak URL (remove trailing slash)
	keycloakURL := strings.TrimSuffix(opts.KeycloakURL, "/")

	// Build OIDC issuer URL for Keycloak
	// Keycloak issuer format: https://{host}/realms/{realm}
	oidcIssuerURL := fmt.Sprintf("%s/realms/%s", keycloakURL, opts.KeycloakRealm)

	// Configure oauth2-proxy with Keycloak OIDC provider
	legacyOpts := options.NewLegacyOptions()
	legacyOpts.LegacyProvider.ProviderType = "keycloak-oidc"
	legacyOpts.LegacyProvider.ProviderName = "keycloak-oidc"
	legacyOpts.LegacyProvider.ClientID = opts.ClientID
	legacyOpts.LegacyProvider.ClientSecret = opts.ClientSecret
	legacyOpts.LegacyProvider.OIDCIssuerURL = oidcIssuerURL
	// Request OIDC scopes - add 'groups' if Keycloak is configured with groups scope
	legacyOpts.LegacyProvider.Scope = "openid email profile offline_access"

	// If allowed groups are configured, add groups scope
	if opts.AllowedGroups != "" {
		legacyOpts.LegacyProvider.Scope = "openid email profile offline_access groups"
	}

	oauthProxyOpts, err := legacyOpts.ToOptions()
	if err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to convert legacy options to new options: %v\n", err)
		os.Exit(1)
	}

	// Enable PKCE with S256 method (required by Keycloak and MCP 2025-11-25 spec)
	oauthProxyOpts.Providers[0].CodeChallengeMethod = "S256"

	// Configure allowed roles if specified
	if opts.AllowedRoles != "" {
		roles := strings.Split(opts.AllowedRoles, ",")
		for i := range roles {
			roles[i] = strings.TrimSpace(roles[i])
		}
		oauthProxyOpts.Providers[0].KeycloakConfig.Roles = roles
	}

	// Configure allowed groups if specified (for Keycloak group-based access)
	if opts.AllowedGroups != "" {
		groups := strings.Split(opts.AllowedGroups, ",")
		for i := range groups {
			groups[i] = strings.TrimSpace(groups[i])
		}
		oauthProxyOpts.Providers[0].KeycloakConfig.Groups = groups
	}

	// Server configuration
	oauthProxyOpts.Server.BindAddress = ""
	oauthProxyOpts.MetricsServer.BindAddress = ""

	// Session storage configuration
	if opts.PostgresConnectionDSN != "" {
		fmt.Printf("INFO: keycloak-auth-provider: validating PostgreSQL connection...\n")

		if err := database.ValidatePostgresConnection(opts.PostgresConnectionDSN); err != nil {
			fmt.Printf("ERROR: keycloak-auth-provider: PostgreSQL connection failed: %v\n", err)
			fmt.Printf("ERROR: Set session storage to PostgreSQL but cannot connect\n")
			fmt.Printf("ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN\n")
			os.Exit(1)
		}

		fmt.Printf("INFO: keycloak-auth-provider: PostgreSQL connection validated successfully\n")

		oauthProxyOpts.Session.Type = options.PostgresSessionStoreType
		oauthProxyOpts.Session.Postgres.ConnectionDSN = opts.PostgresConnectionDSN
		oauthProxyOpts.Session.Postgres.TableNamePrefix = "keycloak_"

		fmt.Printf("INFO: keycloak-auth-provider: using PostgreSQL session storage (table prefix: keycloak_)\n")
	} else {
		fmt.Printf("INFO: keycloak-auth-provider: using cookie-only session storage\n")
		fmt.Printf("WARNING: Cookie-only sessions do not persist across pod restarts\n")
	}

	// Cookie configuration
	oauthProxyOpts.Cookie.Refresh = refreshDuration
	oauthProxyOpts.Cookie.Name = "obot_access_token"
	oauthProxyOpts.Cookie.Secret = string(cookieSecret)
	oauthProxyOpts.Cookie.CSRFExpire = 30 * time.Minute

	// Parse and validate server URL for secure cookie determination
	parsedURL, err := url.Parse(opts.ObotServerURL)
	if err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: invalid OBOT_SERVER_PUBLIC_URL: %v\n", err)
		os.Exit(1)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		fmt.Printf("ERROR: keycloak-auth-provider: OBOT_SERVER_PUBLIC_URL must have http or https scheme\n")
		os.Exit(1)
	}

	// Secure cookies configuration with fail-safe default
	// Allow insecure cookies ONLY if explicitly enabled via environment variable
	insecureCookies := os.Getenv("OBOT_AUTH_INSECURE_COOKIES") == "true"
	isHTTPS := parsedURL.Scheme == "https"

	if !isHTTPS && !insecureCookies {
		fmt.Printf("ERROR: keycloak-auth-provider: OBOT_SERVER_PUBLIC_URL must use https:// scheme\n")
		fmt.Printf("ERROR: For local development, set OBOT_AUTH_INSECURE_COOKIES=true (NOT for production)\n")
		os.Exit(1)
	}

	oauthProxyOpts.Cookie.Secure = isHTTPS

	if !isHTTPS {
		fmt.Printf("WARNING: keycloak-auth-provider: insecure cookies enabled - DO NOT use in production\n")
	}

	// Set additional cookie security flags
	oauthProxyOpts.Cookie.HTTPOnly = true
	oauthProxyOpts.Cookie.SameSite = "Lax" // Prevents CSRF while allowing OAuth redirects

	// Set cookie domain and path explicitly
	oauthProxyOpts.Cookie.Domains = []string{parsedURL.Hostname()}
	oauthProxyOpts.Cookie.Path = "/"

	// Allow environment variable overrides for advanced configurations
	if cookieDomain := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_DOMAIN"); cookieDomain != "" {
		oauthProxyOpts.Cookie.Domains = []string{cookieDomain}
	}

	if cookiePath := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_PATH"); cookiePath != "" {
		oauthProxyOpts.Cookie.Path = cookiePath
	}

	if sameSite := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_SAMESITE"); sameSite != "" {
		oauthProxyOpts.Cookie.SameSite = sameSite
	}

	fmt.Printf("INFO: keycloak-auth-provider: cookie configuration:\n")
	fmt.Printf("  - Name: %s\n", oauthProxyOpts.Cookie.Name)
	fmt.Printf("  - Domains: %v\n", oauthProxyOpts.Cookie.Domains)
	fmt.Printf("  - Path: %s\n", oauthProxyOpts.Cookie.Path)
	fmt.Printf("  - Secure: %v\n", oauthProxyOpts.Cookie.Secure)
	fmt.Printf("  - HTTPOnly: %v\n", oauthProxyOpts.Cookie.HTTPOnly)
	fmt.Printf("  - SameSite: %s\n", oauthProxyOpts.Cookie.SameSite)

	// Templates path
	oauthProxyOpts.Templates.Path = os.Getenv("GPTSCRIPT_TOOL_DIR") + "/../auth-providers-common/templates"

	// Redirect URL configuration
	oauthProxyOpts.RawRedirectURL = opts.ObotServerURL + "/"

	// Email domain restrictions
	if opts.AuthEmailDomains != "" {
		emailDomains := strings.Split(opts.AuthEmailDomains, ",")
		for i := range emailDomains {
			emailDomains[i] = strings.TrimSpace(emailDomains[i])
		}
		oauthProxyOpts.EmailDomains = emailDomains
	}

	// Disable verbose logging
	oauthProxyOpts.Logging.RequestEnabled = false
	oauthProxyOpts.Logging.AuthEnabled = false
	oauthProxyOpts.Logging.StandardEnabled = false

	// Validate options
	if err = validation.Validate(oauthProxyOpts); err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to validate options: %v\n", err)
		os.Exit(1)
	}

	// Create OAuth proxy
	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to create oauth2 proxy: %v\n", err)
		os.Exit(1)
	}

	// Create SessionManager adapter for state package
	// This adapter wraps the OAuthProxy instance to satisfy the state.SessionManager interface
	// We use a closure-based approach because OAuthProxy is in package main and cannot be imported
	sessionManager := &sessionManagerAdapter{
		loadSession: func(r *http.Request) (*sessionsapi.SessionState, error) {
			return oauthProxy.LoadCookiedSession(r)
		},
		serveHTTP: func(w http.ResponseWriter, r *http.Request) {
			oauthProxy.ServeHTTP(w, r)
		},
		cookieOpts: oauthProxy.CookieOptions,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	// Parse cache TTL with default
	groupCacheTTL, err := time.ParseDuration(opts.GroupCacheTTL)
	if err != nil {
		fmt.Printf("WARNING: keycloak-auth-provider: invalid GROUP_CACHE_TTL '%s', using default 1h\\n", opts.GroupCacheTTL)
		groupCacheTTL = time.Hour
	}

	// Parse allowed groups for filtering (if groups come from token claims)
	var allowedGroups []string
	if opts.AllowedGroups != "" {
		allowedGroups = strings.Split(opts.AllowedGroups, ",")
		for i := range allowedGroups {
			allowedGroups[i] = strings.TrimSpace(allowedGroups[i])
		}
	}

	// Initialize admin token cache with 4-minute TTL (Keycloak tokens typically valid for 5 minutes)
	// Capacity of 5 allows caching tokens for up to 5 realms
	adminTokenCache = expirable.NewLRU[string, string](5, nil, 4*time.Minute)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Root endpoint - returns daemon address (required by obot)
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
	})

	// State endpoint - returns auth state with group enrichment from token claims
	mux.HandleFunc("/obot-get-state", getState(sessionManager, allowedGroups, groupCacheTTL))

	// User info endpoint - fetches profile from Keycloak userinfo endpoint
	mux.HandleFunc("/obot-get-user-info", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := fetchUserProfile(r.Context(), r.Header.Get("Authorization"), oidcIssuerURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch user info: %v", err), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(userInfo)
	})

	// Icon URL endpoint - returns user's profile picture URL
	// Required by auth-providers.md spec
	mux.HandleFunc("/obot-get-icon-url", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := fetchUserProfile(r.Context(), r.Header.Get("Authorization"), oidcIssuerURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch icon URL: %v", err), http.StatusInternalServerError)
			return
		}

		// Return JSON response as required by auth-providers.md spec
		type iconResponse struct {
			IconURL string `json:"iconURL"`
		}

		iconURL := ""
		if url, ok := userInfo["icon_url"].(string); ok {
			iconURL = url
		}

		if err := json.NewEncoder(w).Encode(iconResponse{IconURL: iconURL}); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// List auth groups endpoint - returns all groups from Keycloak for admin group discovery
	// Uses service account Client Credentials flow since gateway doesn't pass Authorization header
	mux.HandleFunc("/obot-list-auth-groups", listAuthGroupsKeycloak(opts, keycloakURL, allowedGroups))

	// Groups endpoint - return 404 as groups are extracted from token claims in getState
	mux.HandleFunc("/obot-list-user-auth-groups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Catch-all route - delegates to oauth2-proxy for OAuth flow handling
	mux.HandleFunc("/", oauthProxy.ServeHTTP)

	fmt.Printf("listening on 127.0.0.1:%s\n", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("ERROR: keycloak-auth-provider: failed to listen and serve: %v\n", err)
		os.Exit(1)
	}
}

// getState returns an HTTP handler that wraps the state.ObotGetState with group enrichment from Keycloak token
func getState(sm state.SessionManager, allowedGroups []string, groupCacheTTL time.Duration) http.HandlerFunc {
	// Cache for user groups to avoid repeated token parsing
	groupCache := expirable.NewLRU[string, []string](5000, nil, groupCacheTTL)

	return func(w http.ResponseWriter, r *http.Request) {
		// Get base state from oauth2-proxy
		ss, err := getSerializableStateFromRequest(sm, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get state: %v", err), http.StatusInternalServerError)
			return
		}

		// CRITICAL: ID token parsing is required for reliable user identification
		// Without it, we cannot guarantee consistent ProviderUserID across sessions
		if ss.IDToken == "" {
			http.Error(w, "missing ID token - cannot authenticate user", http.StatusUnauthorized)
			return
		}

		userProfile, err := profile.ParseIDToken(ss.IDToken)
		if err != nil {
			fmt.Printf("ERROR: keycloak-auth-provider: failed to parse ID token: %v\n", err)
			http.Error(w, fmt.Sprintf("failed to parse ID token: %v", err), http.StatusInternalServerError)
			return
		}

		// Set User to Keycloak subject (stable identifier)
		ss.User = userProfile.Subject
		// Set PreferredUsername from token
		if userProfile.PreferredUsername != "" {
			ss.PreferredUsername = userProfile.PreferredUsername
		} else if userProfile.Email != "" {
			ss.PreferredUsername = userProfile.Email
		}

		// Extract groups from token claims (if Keycloak is configured to include them)
		userID := ss.User
		if userID == "" {
			userID = ss.Email
		}

		// Check cache first
		if cachedGroups, ok := groupCache.Get(userID); ok {
			ss.Groups = cachedGroups
		} else if len(userProfile.Groups) > 0 {
			filteredGroups := userProfile.Groups
			// Apply group filtering if configured
			if len(allowedGroups) > 0 {
				filteredGroups = groups.Filter(filteredGroups, allowedGroups)
			}
			ss.Groups = filteredGroups
			groupCache.Add(userID, filteredGroups)
		}

		if err = json.NewEncoder(w).Encode(ss); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode state: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// getSerializableStateFromRequest decodes the request and gets state from oauth2-proxy
func getSerializableStateFromRequest(sm state.SessionManager, r *http.Request) (state.SerializableState, error) {
	var sr state.SerializableRequest
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		return state.SerializableState{}, fmt.Errorf("failed to decode request body: %v", err)
	}

	reqObj, err := http.NewRequest(sr.Method, sr.URL, nil)
	if err != nil {
		return state.SerializableState{}, fmt.Errorf("failed to create request object: %v", err)
	}
	reqObj.Header = sr.Header

	return state.GetSerializableState(sm, reqObj)
}

// fetchUserProfile fetches user profile from Keycloak's userinfo endpoint
func fetchUserProfile(ctx context.Context, authHeader, issuerURL string) (map[string]any, error) {
	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Keycloak userinfo endpoint: {issuer}/protocol/openid-connect/userinfo
	userinfoURL := issuerURL + "/protocol/openid-connect/userinfo"

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Map 'picture' to 'icon_url' for obot compatibility (same as EntraID provider)
	// obot's UpdateProfileIfNeeded looks for profile["icon_url"]
	if picture, ok := result["picture"].(string); ok && picture != "" {
		result["icon_url"] = picture
	} else if email, ok := result["email"].(string); ok && email != "" {
		// Gravatar fallback using email hash for identicon avatars
		hash := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
		result["icon_url"] = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon&s=200", hash)
	}

	return result, nil
}

// getKeycloakAdminToken retrieves a service account access token using Client Credentials flow.
// Uses AdminClientID/AdminClientSecret if provided, otherwise falls back to ClientID/ClientSecret.
// Tokens are cached with a 4-minute TTL (Keycloak tokens typically valid for 5 minutes).
func getKeycloakAdminToken(ctx context.Context, opts Options, keycloakURL string) (string, error) {
	// Determine which credentials to use
	clientID := opts.ClientID
	clientSecret := opts.ClientSecret
	if opts.AdminClientID != "" && opts.AdminClientSecret != "" {
		clientID = opts.AdminClientID
		clientSecret = opts.AdminClientSecret
	}

	// Check cache first
	cacheKey := opts.KeycloakRealm
	if token, ok := adminTokenCache.Get(cacheKey); ok {
		return token, nil
	}

	// Request new token using Client Credentials flow
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, opts.KeycloakRealm)
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := keycloakClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	// Cache the token
	adminTokenCache.Add(cacheKey, tokenResp.AccessToken)

	return tokenResp.AccessToken, nil
}

// fetchAllKeycloakGroups retrieves all groups from Keycloak Admin API recursively.
// Returns groups filtered by allowedGroups if provided.
// Includes group descriptions from attributes if available.
func fetchAllKeycloakGroups(ctx context.Context, token, keycloakURL, realm, _ string, allowedGroups []string) (state.GroupInfoList, error) {
	var allGroups state.GroupInfoList

	// Keycloak Admin API endpoint for groups
	apiURL := fmt.Sprintf("%s/admin/realms/%s/groups?briefRepresentation=false", keycloakURL, realm)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := keycloakClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keycloak API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Keycloak group structure with recursive subgroups
	type KeycloakGroup struct {
		ID         string              `json:"id"`
		Name       string              `json:"name"`
		Path       string              `json:"path"`
		Attributes map[string][]string `json:"attributes,omitempty"`
		SubGroups  []KeycloakGroup     `json:"subGroups,omitempty"`
	}

	var groups []KeycloakGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Recursively flatten group hierarchy
	var flattenGroups func([]KeycloakGroup)
	flattenGroups = func(groupList []KeycloakGroup) {
		for _, g := range groupList {
			// Use path for display name (shows hierarchy like /Parent/Child)
			displayName := g.Path
			if displayName == "" || displayName == "/" {
				displayName = g.Name
			}
			// Remove leading slash for cleaner display
			displayName = strings.TrimPrefix(displayName, "/")

			// Extract description from attributes
			var description *string
			if desc, ok := g.Attributes["description"]; ok && len(desc) > 0 && desc[0] != "" {
				description = &desc[0]
			}

			allGroups = append(allGroups, state.GroupInfo{
				ID:          g.ID,
				Name:        displayName,
				Description: description,
			})

			// Process subgroups recursively
			if len(g.SubGroups) > 0 {
				flattenGroups(g.SubGroups)
			}
		}
	}

	flattenGroups(groups)

	// Filter by allowed groups if specified
	if len(allowedGroups) > 0 {
		allGroups = allGroups.FilterByAllowed(allowedGroups)
	}

	return allGroups, nil
}

// listAuthGroupsKeycloak handles the /obot-list-auth-groups endpoint for Keycloak.
// Supports optional "name" query parameter for fuzzy searching.
func listAuthGroupsKeycloak(opts Options, keycloakURL string, allowedGroups []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nameFilter := r.URL.Query().Get("name")

		// Get service account access token
		token, err := getKeycloakAdminToken(r.Context(), opts, keycloakURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get access token: %v", err), http.StatusInternalServerError)
			return
		}

		// Fetch all groups
		groups, err := fetchAllKeycloakGroups(r.Context(), token, keycloakURL, opts.KeycloakRealm, nameFilter, allowedGroups)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch groups: %v", err), http.StatusInternalServerError)
			return
		}

		// Apply client-side fuzzy search if name filter is provided
		if nameFilter != "" {
			var groupNames []string
			for _, g := range groups {
				groupNames = append(groupNames, g.Name)
			}

			// Use fuzzy matching to rank results by relevance
			matches := fuzzy.Find(nameFilter, groupNames)

			// Build result list in relevance order
			var rankedGroups state.GroupInfoList
			for _, match := range matches {
				rankedGroups = append(rankedGroups, groups[match.Index])
			}
			groups = rankedGroups
		}

		// Return groups as JSON
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(groups); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		}
	}
}
