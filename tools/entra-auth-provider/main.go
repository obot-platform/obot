package main

import (
	"context"
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
	"github.com/obot-platform/obot-entraid/tools/entra-auth-provider/pkg/profile"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
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
	AuthCookieSecret         string `env:"OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains         string `env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS" default:"*"`
	AuthTokenRefreshDuration string `env:"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION" optional:"true" default:"1h"`
	AllowedGroups            string `env:"OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS" optional:"true"`
	AllowedTenants           string `env:"OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS" optional:"true"`
}

// GraphClient for Microsoft Graph API requests
var graphClient = &http.Client{
	Timeout: 30 * time.Second,
}

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
		oauthProxyOpts.Session.Type = options.PostgresSessionStoreType
		oauthProxyOpts.Session.Postgres.ConnectionDSN = opts.PostgresConnectionDSN
		oauthProxyOpts.Session.Postgres.TableNamePrefix = "entra_"
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
		fmt.Printf("ERROR: entra-auth-provider: failed to validate options: %v\n", err)
		os.Exit(1)
	}

	// Create OAuth proxy
	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		fmt.Printf("ERROR: entra-auth-provider: failed to create oauth2 proxy: %v\n", err)
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	// Parse allowed groups for filtering
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

	// State endpoint - returns auth state with token refresh support
	mux.HandleFunc("/obot-get-state", getState(oauthProxy, allowedGroups))

	// User info endpoint - fetches profile from Microsoft Graph
	mux.HandleFunc("/obot-get-user-info", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := fetchUserProfile(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch user info: %v", err), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(userInfo)
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

	// Groups endpoint - return 404 as groups are fetched via getState instead
	// (The gateway doesn't provide an access token to this endpoint, so we can't fetch groups here)
	mux.HandleFunc("/obot-list-user-auth-groups", func(w http.ResponseWriter, r *http.Request) {
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
func getState(p *oauth2proxy.OAuthProxy, allowedGroups []string) http.HandlerFunc {
	// Cache for user groups to avoid repeated Graph API calls
	groupCache := expirable.NewLRU[string, []string](5000, nil, time.Hour)

	return func(w http.ResponseWriter, r *http.Request) {
		// Get base state from oauth2-proxy
		ss, err := getSerializableStateFromRequest(p, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get state: %v", err), http.StatusInternalServerError)
			return
		}

		// Extract Azure OID from ID token and set as User
		// This is required because oauth2-proxy's azure provider doesn't populate ss.User correctly
		// (known bug #3165: userIDClaim setting doesn't work)
		if ss.IDToken != "" {
			userProfile, err := profile.ParseIDToken(ss.IDToken)
			if err != nil {
				fmt.Printf("WARNING: entra-auth-provider: failed to parse ID token: %v\n", err)
			} else {
				// Set User to Azure Object ID (stable identifier)
				ss.User = userProfile.OID
				// Set PreferredUsername to the human-readable UPN from the token
				// This is used for display purposes in the UI
				if userProfile.PreferredUsername != "" {
					ss.PreferredUsername = userProfile.PreferredUsername
				} else if userProfile.Email != "" {
					ss.PreferredUsername = userProfile.Email
				}
			}
		}

		// Enrich with groups from Microsoft Graph
		if ss.AccessToken != "" {
			userID := ss.User
			if userID == "" {
				userID = ss.Email
			}

			// Check cache first
			if groups, ok := groupCache.Get(userID); ok {
				ss.Groups = groups
			} else {
				// Fetch groups from Graph API
				groups, err := fetchUserGroups(r.Context(), ss.AccessToken)
				if err != nil {
					// Log error but don't fail - groups are optional
					fmt.Printf("WARNING: entra-auth-provider: failed to fetch groups for %s: %v\n", userID, err)
				} else {
					// Apply group filtering if configured
					if len(allowedGroups) > 0 {
						groups = filterGroups(groups, allowedGroups)
					}
					ss.Groups = groups
					groupCache.Add(userID, groups)
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

// fetchUserProfile fetches user profile from Microsoft Graph API
func fetchUserProfile(ctx context.Context, authHeader string) (map[string]interface{}, error) {
	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphAPIBaseURL+"/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := graphClient.Do(req)
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

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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

		resp, err := graphClient.Do(req)
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

// filterGroups returns only groups that are in the allowed list
func filterGroups(groups []string, allowed []string) []string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, g := range allowed {
		allowedSet[g] = true
	}

	var filtered []string
	for _, g := range groups {
		if allowedSet[g] {
			filtered = append(filtered, g)
		}
	}
	return filtered
}
