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
	"os"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/obot-platform/obot-entraid/tools/keycloak-auth-provider/pkg/profile"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/groups"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
)

type Options struct {
	ClientID                 string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID"`
	ClientSecret             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET"`
	KeycloakURL              string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_URL"`   // e.g., https://keycloak.example.com
	KeycloakRealm            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_REALM"` // e.g., "obot"
	ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
	PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`
	AuthCookieSecret         string `env:"OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains         string `env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS" default:"*"`
	AuthTokenRefreshDuration string `env:"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION" optional:"true" default:"1h"`
	AllowedGroups            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS" optional:"true"`
	AllowedRoles             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES" optional:"true"`
	GroupCacheTTL            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_GROUP_CACHE_TTL" optional:"true" default:"1h"`
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
		oauthProxyOpts.Session.Type = options.PostgresSessionStoreType
		oauthProxyOpts.Session.Postgres.ConnectionDSN = opts.PostgresConnectionDSN
		oauthProxyOpts.Session.Postgres.TableNamePrefix = "keycloak_"
	}

	// Cookie configuration
	oauthProxyOpts.Cookie.Refresh = refreshDuration
	oauthProxyOpts.Cookie.Name = "obot_access_token"
	oauthProxyOpts.Cookie.Secret = string(cookieSecret)
	oauthProxyOpts.Cookie.Secure = strings.HasPrefix(opts.ObotServerURL, "https://")
	oauthProxyOpts.Cookie.CSRFExpire = 30 * time.Minute

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

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Root endpoint - returns daemon address (required by obot)
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
	})

	// State endpoint - returns auth state with group enrichment from token claims
	mux.HandleFunc("/obot-get-state", getState(oauthProxy, allowedGroups, groupCacheTTL))

	// User info endpoint - fetches profile from Keycloak userinfo endpoint
	mux.HandleFunc("/obot-get-user-info", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := fetchUserProfile(r.Context(), r.Header.Get("Authorization"), oidcIssuerURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch user info: %v", err), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(userInfo)
	})

	// Groups endpoint - return 404 as groups are extracted from token claims in getState
	mux.HandleFunc("/obot-list-user-auth-groups", func(w http.ResponseWriter, r *http.Request) {
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
func getState(p *oauth2proxy.OAuthProxy, allowedGroups []string, groupCacheTTL time.Duration) http.HandlerFunc {
	// Cache for user groups to avoid repeated token parsing
	groupCache := expirable.NewLRU[string, []string](5000, nil, groupCacheTTL)

	return func(w http.ResponseWriter, r *http.Request) {
		// Get base state from oauth2-proxy
		ss, err := getSerializableStateFromRequest(p, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get state: %v", err), http.StatusInternalServerError)
			return
		}

		// Extract user info from ID token
		// Keycloak includes 'sub' (subject) as the user identifier
		if ss.IDToken != "" {
			userProfile, err := profile.ParseIDToken(ss.IDToken)
			if err != nil {
				fmt.Printf("WARNING: keycloak-auth-provider: failed to parse ID token: %v\n", err)
			} else {
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
			}
		}

		if err = json.NewEncoder(w).Encode(ss); err != nil {
			http.Error(w, fmt.Sprintf("failed to encode state: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// getSerializableStateFromRequest decodes the request and gets state from oauth2-proxy
func getSerializableStateFromRequest(p *oauth2proxy.OAuthProxy, r *http.Request) (state.SerializableState, error) {
	var sr state.SerializableRequest
	if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
		return state.SerializableState{}, fmt.Errorf("failed to decode request body: %v", err)
	}

	reqObj, err := http.NewRequest(sr.Method, sr.URL, nil)
	if err != nil {
		return state.SerializableState{}, fmt.Errorf("failed to create request object: %v", err)
	}
	reqObj.Header = sr.Header

	return state.GetSerializableState(p, reqObj)
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

	// Add Gravatar fallback for icon_url since Keycloak doesn't have built-in profile pictures
	// Uses email hash to generate consistent identicon avatars
	if email, ok := result["email"].(string); ok && email != "" {
		// Check if icon_url is not already set (e.g., from Keycloak custom attribute)
		if _, hasIcon := result["icon_url"]; !hasIcon {
			hash := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
			result["icon_url"] = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon&s=200", hash)
		}
	}

	return result, nil
}
