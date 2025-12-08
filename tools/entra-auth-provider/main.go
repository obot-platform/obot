package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultPort      = 9999
	defaultCacheSize = 5000
	defaultCacheTTL  = time.Hour
	defaultTimeout   = 30 * time.Second
	graphAPIBaseURL  = "https://graph.microsoft.com/v1.0"
	maxRetries       = 3
)

// Prometheus metrics
var (
	authRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "entra_auth_requests_total",
			Help: "Total authentication requests",
		},
		[]string{"status", "endpoint"},
	)

	authFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "entra_auth_failures_total",
			Help: "Failed authentication attempts by reason",
		},
		[]string{"reason"},
	)

	graphAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "entra_graph_api_duration_seconds",
			Help:    "Microsoft Graph API request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "status"},
	)

	cacheHitsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "entra_cache_hits_total",
			Help: "Total cache hits",
		},
	)

	cacheMissesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "entra_cache_misses_total",
			Help: "Total cache misses",
		},
	)
)

func init() {
	prometheus.MustRegister(authRequestsTotal)
	prometheus.MustRegister(authFailuresTotal)
	prometheus.MustRegister(graphAPILatency)
	prometheus.MustRegister(cacheHitsTotal)
	prometheus.MustRegister(cacheMissesTotal)
}

// APIError represents a structured error response
type APIError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// Config holds all configuration from environment variables
type Config struct {
	ClientID             string
	ClientSecret         string
	TenantID             string
	CookieSecret         []byte
	AllowedEmailDomains  []string
	AllowedGroups        []string
	AllowedTenants       []string
	PostgresDSN          string
	TokenRefreshDuration time.Duration
	Port                 int
	ServerURL            string
	CacheSize            int
	CacheTTL             time.Duration
	LogLevel             slog.Level
	UseWorkloadIdentity  bool
	ClientCertPath       string
	ClientKeyPath        string
	MetricsEnabled       bool
}

// UserState represents cached user authentication state
type UserState struct {
	UserID   string   `json:"userId"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Groups   []string `json:"groups,omitempty"`
	TenantID string   `json:"tenantId,omitempty"`
}

var (
	// Expirable LRU cache with built-in TTL support
	stateCache *expirable.LRU[string, UserState]
	config     Config
	logger     *slog.Logger

	// HTTP client with timeouts for Microsoft Graph API
	graphClient = &http.Client{
		Timeout: defaultTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

func main() {
	// Load and validate configuration
	var err error
	config, err = loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Setup structured logging
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: config.LogLevel,
	}))
	slog.SetDefault(logger)

	// Initialize expirable LRU cache with built-in TTL
	stateCache = expirable.NewLRU[string, UserState](config.CacheSize, nil, config.CacheTTL)

	// Create HTTP server with custom handlers
	mux := http.NewServeMux()

	// Health and readiness endpoints (Kubernetes probes)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady)

	// Prometheus metrics endpoint
	if config.MetricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// Obot-specific endpoints
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/obot-get-state", handleGetState)
	mux.HandleFunc("/obot-get-user-info", handleGetUserInfo)
	mux.HandleFunc("/obot-list-user-auth-groups", handleListGroups)

	// Create server with timeouts
	addr := fmt.Sprintf("127.0.0.1:%d", config.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown handling
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Error("Server forced to shutdown", "error", err)
		}
		close(done)
	}()

	logger.Info("Starting Entra ID auth provider",
		"address", addr,
		"tenant_id", config.TenantID,
		"multi_tenant", isMultiTenant(config.TenantID),
		"workload_identity", config.UseWorkloadIdentity,
		"metrics_enabled", config.MetricsEnabled,
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}

	<-done
	logger.Info("Server stopped")
}

func loadConfig() (Config, error) {
	port := defaultPort
	if p := os.Getenv("PORT"); p != "" {
		if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
			return Config{}, fmt.Errorf("invalid PORT: %w", err)
		}
	}

	refreshDuration := time.Hour
	if d := os.Getenv("GPTSCRIPT_TOKEN_REFRESH_DURATION"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			refreshDuration = parsed
		}
	}

	cacheSize := defaultCacheSize
	if s := os.Getenv("GPTSCRIPT_CACHE_SIZE"); s != "" {
		if _, err := fmt.Sscanf(s, "%d", &cacheSize); err != nil {
			return Config{}, fmt.Errorf("invalid GPTSCRIPT_CACHE_SIZE: %w", err)
		}
	}

	cacheTTL := defaultCacheTTL
	if t := os.Getenv("GPTSCRIPT_CACHE_TTL"); t != "" {
		if parsed, err := time.ParseDuration(t); err == nil {
			cacheTTL = parsed
		}
	}

	logLevel := slog.LevelInfo
	if l := os.Getenv("GPTSCRIPT_LOG_LEVEL"); l != "" {
		switch strings.ToLower(l) {
		case "debug":
			logLevel = slog.LevelDebug
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		}
	}

	// Check for workload identity
	useWorkloadIdentity := os.Getenv("GPTSCRIPT_USE_WORKLOAD_IDENTITY") == "true"

	// Validate and decode cookie secret
	cookieSecretB64 := os.Getenv("GPTSCRIPT_COOKIE_SECRET")
	if cookieSecretB64 == "" {
		return Config{}, errors.New("GPTSCRIPT_COOKIE_SECRET is required")
	}

	cookieBytes, err := base64.StdEncoding.DecodeString(cookieSecretB64)
	if err != nil {
		return Config{}, fmt.Errorf("GPTSCRIPT_COOKIE_SECRET must be valid base64: %w", err)
	}

	switch len(cookieBytes) {
	case 16, 24, 32:
		// Valid AES key lengths
	default:
		return Config{}, fmt.Errorf("GPTSCRIPT_COOKIE_SECRET must decode to 16, 24, or 32 bytes (got %d)", len(cookieBytes))
	}

	// Validate authentication method
	clientSecret := os.Getenv("GPTSCRIPT_CLIENT_SECRET")
	clientCertPath := os.Getenv("GPTSCRIPT_CLIENT_CERT_PATH")
	clientKeyPath := os.Getenv("GPTSCRIPT_CLIENT_KEY_PATH")

	if !useWorkloadIdentity && clientSecret == "" && clientCertPath == "" {
		return Config{}, errors.New("one of GPTSCRIPT_CLIENT_SECRET, GPTSCRIPT_USE_WORKLOAD_IDENTITY=true, or GPTSCRIPT_CLIENT_CERT_PATH must be set")
	}

	// Parse allowed groups
	var allowedGroups []string
	if groups := os.Getenv("GPTSCRIPT_ALLOWED_GROUPS"); groups != "" {
		allowedGroups = strings.Split(groups, ",")
		for i := range allowedGroups {
			allowedGroups[i] = strings.TrimSpace(allowedGroups[i])
		}
	}

	// Parse allowed tenants (required for multi-tenant)
	var allowedTenants []string
	tenantID := os.Getenv("GPTSCRIPT_TENANT_ID")
	if isMultiTenant(tenantID) {
		tenantsEnv := os.Getenv("GPTSCRIPT_ALLOWED_TENANTS")
		if tenantsEnv == "" {
			return Config{}, errors.New("GPTSCRIPT_ALLOWED_TENANTS is required when tenant_id is 'common' or 'organizations'")
		}
		allowedTenants = strings.Split(tenantsEnv, ",")
		for i := range allowedTenants {
			allowedTenants[i] = strings.TrimSpace(allowedTenants[i])
		}
	}

	// Parse allowed email domains
	var allowedEmailDomains []string
	if domains := os.Getenv("GPTSCRIPT_ALLOWED_EMAIL_DOMAINS"); domains != "" {
		allowedEmailDomains = strings.Split(domains, ",")
		for i := range allowedEmailDomains {
			allowedEmailDomains[i] = strings.TrimSpace(allowedEmailDomains[i])
		}
	}

	// Metrics enabled by default
	metricsEnabled := true
	if m := os.Getenv("GPTSCRIPT_METRICS_ENABLED"); m == "false" {
		metricsEnabled = false
	}

	return Config{
		ClientID:             os.Getenv("GPTSCRIPT_CLIENT_ID"),
		ClientSecret:         clientSecret,
		TenantID:             tenantID,
		CookieSecret:         cookieBytes,
		AllowedEmailDomains:  allowedEmailDomains,
		AllowedGroups:        allowedGroups,
		AllowedTenants:       allowedTenants,
		PostgresDSN:          os.Getenv("OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"),
		TokenRefreshDuration: refreshDuration,
		Port:                 port,
		ServerURL:            os.Getenv("OBOT_SERVER_URL"),
		CacheSize:            cacheSize,
		CacheTTL:             cacheTTL,
		LogLevel:             logLevel,
		UseWorkloadIdentity:  useWorkloadIdentity,
		ClientCertPath:       clientCertPath,
		ClientKeyPath:        clientKeyPath,
		MetricsEnabled:       metricsEnabled,
	}, nil
}

func isMultiTenant(tenantID string) bool {
	return tenantID == "common" || tenantID == "organizations" || tenantID == "consumers"
}

// writeError sends a structured JSON error response
func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{
		Error: message,
		Code:  code,
	})
}

// Health check endpoint for Kubernetes liveness probe
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// Readiness check endpoint for Kubernetes readiness probe
func handleReady(w http.ResponseWriter, r *http.Request) {
	// Check if we can reach Microsoft's OIDC discovery endpoint
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	discoveryURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0/.well-known/openid-configuration", config.TenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "reason": "cannot create request"})
		return
	}

	resp, err := graphClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "reason": "cannot reach Entra ID"})
		return
	}
	resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func handleRoot(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(w, "http://127.0.0.1:%d", config.Port)
}

func handleGetState(w http.ResponseWriter, r *http.Request) {
	authRequestsTotal.WithLabelValues("attempt", "get-state").Inc()

	// Extract user from oauth2-proxy session headers
	email := r.Header.Get("X-Forwarded-Email")
	userID := r.Header.Get("X-Forwarded-User")

	if userID == "" || email == "" {
		logger.Debug("Unauthenticated request to get-state")
		authFailuresTotal.WithLabelValues("not_authenticated").Inc()
		writeError(w, http.StatusUnauthorized, "not authenticated", "AUTH_REQUIRED")
		return
	}

	// Check cache first (expirable cache handles TTL automatically)
	if cached, ok := stateCache.Get(userID); ok {
		cacheHitsTotal.Inc()
		logger.Debug("Returning cached state", "user_id", userID)
		authRequestsTotal.WithLabelValues("success", "get-state").Inc()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cached)
		return
	}
	cacheMissesTotal.Inc()

	// Validate access token before using it
	accessToken := r.Header.Get("X-Forwarded-Access-Token")
	if err := validateAccessToken(accessToken); err != nil {
		logger.Warn("Invalid access token", "error", err, "user_id", userID)
		authFailuresTotal.WithLabelValues("invalid_token").Inc()
		writeError(w, http.StatusUnauthorized, "invalid access token", "INVALID_TOKEN")
		return
	}

	// Fetch fresh state
	state := UserState{
		UserID: userID,
		Email:  email,
		Name:   r.Header.Get("X-Forwarded-Preferred-Username"),
	}

	// Fetch groups from Microsoft Graph (with pagination and retry for 200+ groups)
	groups, err := fetchUserGroupsWithRetry(r.Context(), accessToken)
	if err != nil {
		logger.Warn("Failed to fetch groups", "error", err, "user_id", userID)
		// Continue without groups - don't fail the entire request
	} else {
		// Apply group filtering if configured
		if len(config.AllowedGroups) > 0 {
			state.Groups = filterGroups(groups, config.AllowedGroups)
		} else {
			state.Groups = groups
		}
	}

	// Cache the state (TTL handled by expirable cache)
	stateCache.Add(userID, state)

	logger.Info("User state retrieved",
		"user_id", userID,
		"email", email,
		"group_count", len(state.Groups),
	)

	authRequestsTotal.WithLabelValues("success", "get-state").Inc()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	authRequestsTotal.WithLabelValues("attempt", "get-user-info").Inc()

	accessToken := r.Header.Get("X-Forwarded-Access-Token")
	if accessToken == "" {
		authFailuresTotal.WithLabelValues("no_token").Inc()
		writeError(w, http.StatusUnauthorized, "no access token", "NO_TOKEN")
		return
	}

	// Validate token before use
	if err := validateAccessToken(accessToken); err != nil {
		logger.Warn("Invalid access token for user info", "error", err)
		authFailuresTotal.WithLabelValues("invalid_token").Inc()
		writeError(w, http.StatusUnauthorized, "invalid access token", "INVALID_TOKEN")
		return
	}

	// Fetch from Microsoft Graph API with retry
	userInfo, err := fetchUserProfileWithRetry(r.Context(), accessToken)
	if err != nil {
		logger.Error("Failed to fetch user profile", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error(), "GRAPH_API_ERROR")
		return
	}

	authRequestsTotal.WithLabelValues("success", "get-user-info").Inc()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(userInfo)
}

func handleListGroups(w http.ResponseWriter, r *http.Request) {
	authRequestsTotal.WithLabelValues("attempt", "list-groups").Inc()

	accessToken := r.Header.Get("X-Forwarded-Access-Token")
	if accessToken == "" {
		authFailuresTotal.WithLabelValues("no_token").Inc()
		writeError(w, http.StatusUnauthorized, "no access token", "NO_TOKEN")
		return
	}

	// Validate token before use
	if err := validateAccessToken(accessToken); err != nil {
		logger.Warn("Invalid access token for list groups", "error", err)
		authFailuresTotal.WithLabelValues("invalid_token").Inc()
		writeError(w, http.StatusUnauthorized, "invalid access token", "INVALID_TOKEN")
		return
	}

	groups, err := fetchUserGroupsWithRetry(r.Context(), accessToken)
	if err != nil {
		logger.Error("Failed to fetch groups", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error(), "GRAPH_API_ERROR")
		return
	}

	// Apply group filtering if configured
	if len(config.AllowedGroups) > 0 {
		groups = filterGroups(groups, config.AllowedGroups)
	}

	authRequestsTotal.WithLabelValues("success", "list-groups").Inc()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{"groups": groups})
}

// validateAccessToken performs basic validation on the access token
func validateAccessToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}

	// JWT format check
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid token format")
	}

	// Decode claims (middle part)
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("failed to decode token claims: %w", err)
	}

	var claims struct {
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
		Aud string `json:"aud"`
		Tid string `json:"tid"`
	}

	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return fmt.Errorf("failed to parse token claims: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > claims.Exp {
		return errors.New("token expired")
	}

	// Validate issuer for multi-tenant
	if isMultiTenant(config.TenantID) && len(config.AllowedTenants) > 0 {
		tenantAllowed := false
		for _, allowed := range config.AllowedTenants {
			if claims.Tid == allowed {
				tenantAllowed = true
				break
			}
		}
		if !tenantAllowed {
			return fmt.Errorf("tenant %s not in allowed list", claims.Tid)
		}
	}

	return nil
}

// fetchWithRetry performs an HTTP request with retry logic for rate limiting
func fetchWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Clone request for retry
			req = req.Clone(ctx)
		}

		resp, err := graphClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Handle rate limiting (429 Too Many Requests)
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			retryAfter := resp.Header.Get("Retry-After")
			delay := 1 * time.Second // default delay

			if retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					delay = time.Duration(seconds) * time.Second
				}
			}

			logger.Warn("Rate limited by Graph API, retrying",
				"attempt", attempt+1,
				"retry_after", delay,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		// Handle 5xx errors with exponential backoff
		if resp.StatusCode >= 500 && attempt < maxRetries {
			resp.Body.Close()
			delay := time.Duration(1<<attempt) * time.Second
			logger.Warn("Graph API server error, retrying",
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"delay", delay,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("max retries exceeded")
}

func fetchUserProfileWithRetry(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	start := time.Now()
	defer func() {
		graphAPILatency.WithLabelValues("/me", "completed").Observe(time.Since(start).Seconds())
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphAPIBaseURL+"/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(ctx, req)
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

// fetchUserGroupsWithRetry handles the 200+ groups scenario with pagination and retry
func fetchUserGroupsWithRetry(ctx context.Context, accessToken string) ([]string, error) {
	start := time.Now()
	defer func() {
		graphAPILatency.WithLabelValues("/transitiveMemberOf", "completed").Observe(time.Since(start).Seconds())
	}()

	var allGroups []string

	// Use transitiveMemberOf for complete group hierarchy including nested groups
	// IMPORTANT: ConsistencyLevel: eventual is REQUIRED for $count and $top>100
	url := graphAPIBaseURL + "/me/transitiveMemberOf/microsoft.graph.group?$count=true&$select=id,displayName&$top=999"

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("ConsistencyLevel", "eventual") // REQUIRED for $count and advanced query

		resp, err := fetchWithRetry(ctx, req)
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
			Count    int    `json:"@odata.count"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, g := range result.Value {
			allGroups = append(allGroups, g.ID)
		}

		// Follow pagination
		url = result.NextLink

		logger.Debug("Fetched group page",
			"count", len(result.Value),
			"total", result.Count,
			"has_more", url != "",
		)
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
