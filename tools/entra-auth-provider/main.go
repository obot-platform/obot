package main

import (
	"context"
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
	"github.com/obot-platform/obot-entraid/tools/entra-auth-provider/pkg/profile"
	"github.com/obot-platform/tools/auth-providers-common/pkg/database"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/groups"
	"github.com/obot-platform/tools/auth-providers-common/pkg/ratelimit"
	"github.com/obot-platform/tools/auth-providers-common/pkg/secrets"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
	"github.com/sahilm/fuzzy"
)

const (
	graphAPIBaseURL = "https://graph.microsoft.com/v1.0"
)

type Options struct {
	ClientID                 string `env:"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID"`
	ClientSecret             string `env:"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET"`
	TenantID                 string `env:"OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID"`
	ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
	PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`
	// Cookie secret - provider-specific takes precedence, falls back to shared secret
	AuthCookieSecret         string `env:"OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains         string `env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS" default:"*"`
	AuthTokenRefreshDuration string `env:"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION" optional:"true" default:"1h"`
	AllowedGroups            string `env:"OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS" optional:"true"`
	AllowedTenants           string `env:"OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS" optional:"true"`
	GroupCacheTTL            string `env:"OBOT_ENTRA_AUTH_PROVIDER_GROUP_CACHE_TTL" optional:"true" default:"1h"`
	IconCacheTTL             string `env:"OBOT_ENTRA_AUTH_PROVIDER_ICON_CACHE_TTL" optional:"true" default:"24h"`
	// Admin credentials for Client Credentials flow (optional - uses ClientID/ClientSecret if not provided)
	AdminClientID     string `env:"OBOT_ENTRA_AUTH_PROVIDER_ADMIN_CLIENT_ID" optional:"true"`
	AdminClientSecret string `env:"OBOT_ENTRA_AUTH_PROVIDER_ADMIN_CLIENT_SECRET" optional:"true"`
}

// GraphClient for Microsoft Graph API requests
var graphClient = &http.Client{
	Timeout: 30 * time.Second,
}

// iconCache caches user profile pictures to reduce Graph API calls
// Key: user OID, Value: base64 data URL
// Initialized in main() with configurable TTL
var iconCache *expirable.LRU[string, string]

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

// appTokenCache caches app-only access tokens for Client Credentials flow
// Key: tenantID, Value: access token
// TTL set to 55 minutes (Microsoft Graph tokens valid for 60-75 minutes)
var appTokenCache *expirable.LRU[string, string]

func main() {
	var opts Options
	if err := env.LoadEnvForStruct(&opts); err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to load options: %v\n", err)
		os.Exit(1)
	}

	// Validate multi-tenant configuration
	if isMultiTenant(opts.TenantID) && opts.AllowedTenants == "" {
		fmt.Printf("ERROR: entra-auth-provider: OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS is required when tenant_id is 'common' or 'organizations'\n")
		os.Exit(1)
	}

	refreshDuration, err := time.ParseDuration(opts.AuthTokenRefreshDuration)
	if err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to parse token refresh duration: %v\n", err)
		os.Exit(1)
	}

	if refreshDuration < 0 {
		fmt.Printf("ERROR: entra-auth-provider: token refresh duration must be greater than 0\n")
		os.Exit(1)
	}

	// Validate cookie secret entropy and format
	if err := secrets.ValidateCookieSecret(opts.AuthCookieSecret); err != nil {
		fmt.Printf("ERROR: entra-auth-provider: %v\n", err)
		fmt.Printf("Generate a valid secret with: openssl rand -base64 32\n")
		fmt.Printf("Or set OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET for provider-specific secret\n")
		os.Exit(1)
	}

	cookieSecret, err := base64.StdEncoding.DecodeString(opts.AuthCookieSecret)
	if err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to decode cookie secret: %v\n", err)
		os.Exit(1)
	}

	// Configure oauth2-proxy with Azure/Entra ID provider
	legacyOpts := options.NewLegacyOptions()
	legacyOpts.LegacyProvider.ProviderType = "azure"
	legacyOpts.LegacyProvider.ProviderName = "azure"
	legacyOpts.LegacyProvider.ClientID = opts.ClientID
	legacyOpts.LegacyProvider.ClientSecret = opts.ClientSecret
	legacyOpts.LegacyProvider.AzureTenant = opts.TenantID
	// Set OIDC issuer URL for Azure AD v2.0 endpoint
	legacyOpts.LegacyProvider.OIDCIssuerURL = fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", opts.TenantID)
	// Request OIDC scopes only - Graph API permissions should be configured in Azure App Registration
	// Note: Don't mix .default with resource-specific scopes (Azure doesn't allow it)
	legacyOpts.LegacyProvider.Scope = "openid email profile offline_access"

	oauthProxyOpts, err := legacyOpts.ToOptions()
	if err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to convert legacy options to new options: %v\n", err)
		os.Exit(1)
	}

	// For multi-tenant apps, skip OIDC issuer verification since Azure returns {tenantid} template
	if isMultiTenant(opts.TenantID) {
		oauthProxyOpts.Providers[0].OIDCConfig.InsecureSkipIssuerVerification = true
	}

	// Server configuration
	oauthProxyOpts.Server.BindAddress = ""
	oauthProxyOpts.MetricsServer.BindAddress = ""

	// Session storage configuration
	if opts.PostgresConnectionDSN != "" {
		fmt.Printf("INFO: entra-auth-provider: validating PostgreSQL connection...\n")

		if err := database.ValidatePostgresConnection(opts.PostgresConnectionDSN); err != nil {
			fmt.Printf("ERROR: entra-auth-provider: PostgreSQL connection failed: %v\n", err)
			fmt.Printf("ERROR: Set session storage to PostgreSQL but cannot connect\n")
			fmt.Printf("ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN\n")
			os.Exit(1)
		}

		fmt.Printf("INFO: entra-auth-provider: PostgreSQL connection validated successfully\n")

		oauthProxyOpts.Session.Type = options.PostgresSessionStoreType
		oauthProxyOpts.Session.Postgres.ConnectionDSN = opts.PostgresConnectionDSN
		oauthProxyOpts.Session.Postgres.TableNamePrefix = "entra_"

		fmt.Printf("INFO: entra-auth-provider: using PostgreSQL session storage (table prefix: entra_)\n")
	} else {
		fmt.Printf("INFO: entra-auth-provider: using cookie-only session storage\n")
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
		fmt.Printf("ERROR: entra-auth-provider: invalid OBOT_SERVER_PUBLIC_URL: %v\n", err)
		os.Exit(1)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		fmt.Printf("ERROR: entra-auth-provider: OBOT_SERVER_PUBLIC_URL must have http or https scheme\n")
		os.Exit(1)
	}

	// Secure cookies configuration with fail-safe default
	// Allow insecure cookies ONLY if explicitly enabled via environment variable
	insecureCookies := os.Getenv("OBOT_AUTH_INSECURE_COOKIES") == "true"
	isHTTPS := parsedURL.Scheme == "https"

	if !isHTTPS && !insecureCookies {
		fmt.Printf("ERROR: entra-auth-provider: OBOT_SERVER_PUBLIC_URL must use https:// scheme\n")
		fmt.Printf("ERROR: For local development, set OBOT_AUTH_INSECURE_COOKIES=true (NOT for production)\n")
		os.Exit(1)
	}

	oauthProxyOpts.Cookie.Secure = isHTTPS

	if !isHTTPS {
		fmt.Printf("WARNING: entra-auth-provider: insecure cookies enabled - DO NOT use in production\n")
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

	fmt.Printf("INFO: entra-auth-provider: cookie configuration:\n")
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
		fmt.Printf("ERROR: entra-auth-provider: failed to validate options: %v\n", err)
		os.Exit(1)
	}

	// Create OAuth proxy
	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to create oauth2 proxy: %v\n", err)
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

	// Parse cache TTLs with defaults
	groupCacheTTL, err := time.ParseDuration(opts.GroupCacheTTL)
	if err != nil {
		fmt.Printf("WARNING: entra-auth-provider: invalid GROUP_CACHE_TTL '%s', using default 1h\\n", opts.GroupCacheTTL)
		groupCacheTTL = time.Hour
	}

	iconCacheTTL, err := time.ParseDuration(opts.IconCacheTTL)
	if err != nil {
		fmt.Printf("WARNING: entra-auth-provider: invalid ICON_CACHE_TTL '%s', using default 24h\\n", opts.IconCacheTTL)
		iconCacheTTL = 24 * time.Hour
	}

	// Initialize icon cache with configured TTL
	iconCache = expirable.NewLRU[string, string](10000, nil, iconCacheTTL)

	// Initialize app token cache with 55-minute TTL (Microsoft Graph tokens valid for 60-75 minutes)
	// Capacity of 10 allows caching tokens for up to 10 tenants in multi-tenant scenarios
	appTokenCache = expirable.NewLRU[string, string](10, nil, 55*time.Minute)

	// Parse allowed groups for filtering
	var allowedGroups []string
	if opts.AllowedGroups != "" {
		allowedGroups = strings.Split(opts.AllowedGroups, ",")
		for i := range allowedGroups {
			allowedGroups[i] = strings.TrimSpace(allowedGroups[i])
		}
	}

	// Parse allowed tenants for multi-tenant runtime validation
	var allowedTenantSet map[string]bool
	if opts.AllowedTenants != "" && opts.AllowedTenants != "*" {
		allowedTenantSet = make(map[string]bool)
		for _, tenant := range strings.Split(opts.AllowedTenants, ",") {
			allowedTenantSet[strings.TrimSpace(tenant)] = true
		}
	}

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Root endpoint - returns daemon address (required by obot)
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
	})

	// State endpoint - returns auth state with token refresh support
	mux.HandleFunc("/obot-get-state", getState(sessionManager, allowedGroups, allowedTenantSet, groupCacheTTL))

	// User info endpoint - fetches profile from Microsoft Graph
	mux.HandleFunc("/obot-get-user-info", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := fetchUserProfile(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch user info: %v", err), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(userInfo)
	})

	// Icon URL endpoint - returns user's profile picture URL from Microsoft Graph
	mux.HandleFunc("/obot-get-icon-url", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing Authorization header", http.StatusBadRequest)
			return
		}

		// Extract access token from "Bearer <token>"
		accessToken := strings.TrimPrefix(auth, "Bearer ")
		if accessToken == "" || accessToken == auth {
			http.Error(w, "invalid Authorization header format", http.StatusBadRequest)
			return
		}

		iconURL, err := profile.FetchUserIconURL(r.Context(), accessToken)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch icon URL: %v", err), http.StatusInternalServerError)
			return
		}

		// Return JSON response as required by auth-providers.md spec
		type iconResponse struct {
			IconURL string `json:"iconURL"`
		}

		if err := json.NewEncoder(w).Encode(iconResponse{IconURL: iconURL}); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// List auth groups endpoint - returns all groups from identity provider for admin group discovery
	// Uses Client Credentials flow (app-only authentication) since gateway doesn't pass Authorization header
	mux.HandleFunc("/obot-list-auth-groups", listAuthGroups(opts, allowedGroups))

	// Groups endpoint - return 404 as groups are fetched via getState instead
	// (The gateway doesn't provide an access token to this endpoint, so we can't fetch groups here)
	mux.HandleFunc("/obot-list-user-auth-groups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Catch-all route - delegates to oauth2-proxy for OAuth flow handling
	mux.HandleFunc("/", oauthProxy.ServeHTTP)

	fmt.Printf("listening on 127.0.0.1:%s\n", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("ERROR: entra-auth-provider: failed to listen and serve: %v\n", err)
		os.Exit(1)
	}
}

func isMultiTenant(tenantID string) bool {
	return tenantID == "common" || tenantID == "organizations" || tenantID == "consumers"
}

// getState returns an HTTP handler that wraps the state.ObotGetState with group enrichment
func getState(sm state.SessionManager, allowedGroups []string, allowedTenants map[string]bool, groupCacheTTL time.Duration) http.HandlerFunc {
	// Cache for user groups to avoid repeated Graph API calls
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
		// This prevents admin/owner permission loss after re-login (see commit 1e7fb26c)
		if ss.IDToken == "" {
			http.Error(w, "missing ID token - cannot authenticate user", http.StatusUnauthorized)
			return
		}

		userProfile, err := profile.ParseIDToken(ss.IDToken)
		if err != nil {
			fmt.Printf("ERROR: entra-auth-provider: failed to parse ID token: %v\n", err)
			http.Error(w, fmt.Sprintf("failed to parse ID token: %v", err), http.StatusInternalServerError)
			return
		}

		// Validate tenant if restrictions are configured (multi-tenant mode)
		if allowedTenants != nil && !allowedTenants[userProfile.TenantID] {
			fmt.Printf("ERROR: entra-auth-provider: rejected login from unauthorized tenant: %s\n", userProfile.TenantID)
			http.Error(w, "tenant not allowed", http.StatusForbidden)
			return
		}

		// Set User to Azure Object ID (stable identifier)
		ss.User = userProfile.OID

		// Set PreferredUsername to the human-readable UPN from the token
		// This is used for display purposes in the UI
		if userProfile.PreferredUsername != "" {
			ss.PreferredUsername = userProfile.PreferredUsername
		} else if userProfile.Email != "" {
			ss.PreferredUsername = userProfile.Email
		}

		// Enrich with groups from Microsoft Graph
		if ss.AccessToken != "" {
			userID := ss.User
			if userID == "" {
				userID = ss.Email
			}

			// Check cache first
			if cachedGroups, ok := groupCache.Get(userID); ok {
				ss.Groups = cachedGroups
			} else {
				// Fetch groups from Graph API
				fetchedGroups, err := fetchUserGroups(r.Context(), ss.AccessToken)
				if err != nil {
					// Log error but don't fail - groups are optional
					fmt.Printf("WARNING: entra-auth-provider: failed to fetch groups for %s: %v\n", userID, err)
				} else {
					// Apply group filtering if configured
					if len(allowedGroups) > 0 {
						fetchedGroups = groups.Filter(fetchedGroups, allowedGroups)
					}
					ss.Groups = fetchedGroups
					groupCache.Add(userID, fetchedGroups)
				}
			}
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

// fetchUserProfile fetches user profile from Microsoft Graph API
// Returns a map with field names that obot expects: "name" for display name, "icon_url" for profile picture
func fetchUserProfile(ctx context.Context, authHeader string) (map[string]any, error) {
	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphAPIBaseURL+"/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := ratelimit.DoWithRetry(ctx, graphClient, req, ratelimit.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("graph API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graph API returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Map displayName to "name" for obot compatibility
	// obot's UpdateProfileIfNeeded looks for profile["name"] for entra-auth-provider
	if displayName, ok := result["displayName"].(string); ok {
		result["name"] = displayName
	}

	// Get user OID for cache key
	var cacheKey string
	if id, ok := result["id"].(string); ok {
		cacheKey = id
	}

	// Check icon cache first to avoid redundant Graph API calls
	if cacheKey != "" {
		if cachedIcon, ok := iconCache.Get(cacheKey); ok {
			result["icon_url"] = cachedIcon
			return result, nil
		}
	}

	// Fetch and add icon URL for obot compatibility
	// obot's UpdateProfileIfNeeded looks for profile["icon_url"] for entra-auth-provider
	iconURL, err := profile.FetchUserIconURL(ctx, accessToken)
	if err != nil {
		// Log but don't fail - icon is optional
		fmt.Printf("WARNING: failed to fetch icon URL: %v\n", err)
	} else if iconURL != "" {
		result["icon_url"] = iconURL
		// Cache the icon for future requests
		if cacheKey != "" {
			iconCache.Add(cacheKey, iconURL)
		}
	}

	return result, nil
}

// fetchUserGroups fetches user's group memberships from Microsoft Graph API with pagination
func fetchUserGroups(ctx context.Context, accessToken string) ([]string, error) {
	var allGroups []string

	// Use transitiveMemberOf for complete group hierarchy including nested groups
	url := graphAPIBaseURL + "/me/transitiveMemberOf/microsoft.graph.group?$count=true&$select=id,displayName&$top=999"

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("ConsistencyLevel", "eventual") // Required for $count and advanced query

		resp, err := ratelimit.DoWithRetry(ctx, graphClient, req, ratelimit.DefaultConfig())
		if err != nil {
			return nil, fmt.Errorf("graph API request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("graph API returned %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Value []struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, g := range result.Value {
			allGroups = append(allGroups, g.ID)
		}

		// Follow pagination
		url = result.NextLink
	}

	return allGroups, nil
}

// getAppAccessToken retrieves an app-only access token using Client Credentials flow.
// Uses AdminClientID/AdminClientSecret if provided, otherwise falls back to ClientID/ClientSecret.
// Tokens are cached with a 55-minute TTL to minimize API calls.
func getAppAccessToken(ctx context.Context, opts Options) (string, error) {
	// Determine which credentials to use
	clientID := opts.ClientID
	clientSecret := opts.ClientSecret
	if opts.AdminClientID != "" && opts.AdminClientSecret != "" {
		clientID = opts.AdminClientID
		clientSecret = opts.AdminClientSecret
	}

	// Check cache first
	cacheKey := opts.TenantID
	if token, ok := appTokenCache.Get(cacheKey); ok {
		return token, nil
	}

	// Request new token using Client Credentials flow
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", opts.TenantID)
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("scope", "https://graph.microsoft.com/.default")
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ratelimit.DoWithRetry(ctx, graphClient, req, ratelimit.DefaultConfig())
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
	appTokenCache.Add(cacheKey, tokenResp.AccessToken)

	return tokenResp.AccessToken, nil
}

// fetchAllGroups retrieves all groups from Microsoft Graph API with optional name filtering.
// Returns groups filtered by allowedGroups if provided.
// Includes group descriptions for better identification.
func fetchAllGroups(ctx context.Context, token, nameFilter string, allowedGroups []string) (state.GroupInfoList, error) {
	var allGroups state.GroupInfoList

	// Build Graph API URL with query parameters
	params := url.Values{}
	params.Set("$select", "id,displayName,description")
	params.Set("$top", "999")

	// Apply server-side filtering for security and M365 groups
	filter := "securityEnabled eq true or groupTypes/any(c:c eq 'Unified')"

	// Add name filter if provided (server-side)
	if nameFilter != "" {
		// Escape single quotes in the filter string
		escapedName := strings.ReplaceAll(nameFilter, "'", "''")
		nameClause := fmt.Sprintf("startsWith(displayName, '%s')", escapedName)
		filter = fmt.Sprintf("(%s) and %s", filter, nameClause)
	}

	params.Set("$filter", filter)

	apiURL := fmt.Sprintf("%s/groups?%s", graphAPIBaseURL, params.Encode())

	// Paginate through all results
	for apiURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := ratelimit.DoWithRetry(ctx, graphClient, req, ratelimit.DefaultConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to fetch groups: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("graph API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Value []struct {
				ID          string  `json:"id"`
				DisplayName string  `json:"displayName"`
				Description *string `json:"description"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, g := range result.Value {
			allGroups = append(allGroups, state.GroupInfo{
				ID:          g.ID,
				Name:        g.DisplayName,
				Description: g.Description,
			})
		}

		// Follow pagination
		apiURL = result.NextLink
	}

	// Filter by allowed groups if specified
	if len(allowedGroups) > 0 {
		allGroups = allGroups.FilterByAllowed(allowedGroups)
	}

	return allGroups, nil
}

// listAuthGroups handles the /obot-list-auth-groups endpoint.
// Supports optional "name" query parameter for fuzzy searching.
func listAuthGroups(opts Options, allowedGroups []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nameFilter := r.URL.Query().Get("name")

		// Get app-only access token
		token, err := getAppAccessToken(r.Context(), opts)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get access token: %v", err), http.StatusInternalServerError)
			return
		}

		// Fetch all groups (with server-side name filtering if provided)
		groups, err := fetchAllGroups(r.Context(), token, nameFilter, allowedGroups)
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
