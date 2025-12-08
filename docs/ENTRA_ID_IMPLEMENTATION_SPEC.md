# Microsoft Entra ID Authentication Provider - Implementation Specification

**Project**: obot-entraid
**Date**: 2025-12-07
**Status**: Draft v3 (Analysis-Reviewed)
**Reviewed By**: Expert Panel + Automated Analysis

---

## 1. Executive Summary

This specification outlines the implementation of Microsoft Entra ID (formerly Azure Active Directory) as an authentication provider for the obot-entraid fork. The implementation follows established patterns from the upstream obot project while adding Entra ID support that is otherwise only available in the Enterprise Edition.

### Goals
- Implement Entra ID OAuth 2.0/OIDC authentication with PKCE
- Support Azure AD group membership sync for RBAC (including 200+ groups)
- Follow existing auth provider patterns for maintainability
- Minimize divergence from upstream obot codebase
- Implement production-grade security and observability

### Approach
**Option 2: Local Tool Registry** - Create a custom tools registry within this repository containing the Entra ID auth provider, configured alongside the upstream registry.

### Security Review Status

| Check | Status | Notes |
|-------|--------|-------|
| oauth2-proxy version | **v7.13.0** | Addresses CVE-2025-54576, CVE-2025-64484 |
| PKCE enabled | S256 | Required by Microsoft |
| Header smuggling | Protected | v7.13.0 normalizes headers |
| Issuer validation | Configured | Multi-tenant aware |
| Token validation | Implemented | Expiry and format checks |
| Group overage | Handled | Pagination for 200+ groups |
| Rate limiting | Implemented | Retry-After header support |
| Workload Identity | Supported | For Kubernetes deployments |

---

## 2. Architecture Overview

### 2.1 How Auth Providers Work in Obot

```
+---------------------------------------------------------------------+
|                        Obot Server                                   |
+---------------------------------------------------------------------+
|  pkg/services/config.go                                              |
|  +-- ToolRegistries: ["github.com/obot-platform/tools", "./tools"]   |
|                                                                      |
|  pkg/controller/handlers/toolreference/toolreference.go              |
|  +-- Reads index.yaml from each registry                             |
|  +-- Creates ToolReference resources for each auth provider          |
|                                                                      |
|  pkg/gateway/server/dispatcher/dispatcher.go                         |
|  +-- Starts auth provider daemons                                    |
|  +-- Routes auth requests to provider URLs                           |
|                                                                      |
|  pkg/api/handlers/authprovider.go                                    |
|  +-- Configure/Deconfigure endpoints                                 |
|  +-- Stores credentials via GPTScript credential system              |
+---------------------------------------------------------------------+
                              |
                              v
+---------------------------------------------------------------------+
|              Auth Provider Daemon (per provider)                     |
+---------------------------------------------------------------------+
|  entra-auth-provider/main.go                                         |
|  +-- Wraps oauth2-proxy v7.13.0 with Entra ID + PKCE                |
|  +-- Exposes HTTP endpoints:                                         |
|  |   +-- /health              -> Kubernetes health probe             |
|  |   +-- /ready               -> Kubernetes readiness probe          |
|  |   +-- /metrics             -> Prometheus metrics                  |
|  |   +-- /                    -> Returns local address               |
|  |   +-- /obot-get-state      -> Returns cached user state           |
|  |   +-- /obot-get-user-info  -> Fetches user profile from Graph     |
|  |   +-- /obot-list-user-auth-groups -> Returns Azure AD groups      |
|  |   +-- /*                   -> Delegates to oauth2-proxy           |
|  +-- Caches user IDs (configurable LRU with TTL expiration)          |
|  +-- Handles group overage (200+ groups with pagination)             |
|  +-- Rate limit handling with Retry-After support                    |
|  +-- Structured logging with slog                                    |
+---------------------------------------------------------------------+
```

### 2.2 Entra ID OAuth 2.0 Flow with PKCE

```
User                    Obot                    Entra ID              MS Graph
  |                       |                         |                     |
  | 1. Login request      |                         |                     |
  |---------------------->|                         |                     |
  |                       |                         |                     |
  |                       | 2. Generate PKCE        |                     |
  |                       |    code_verifier +      |                     |
  |                       |    code_challenge       |                     |
  |                       |                         |                     |
  |                       | 3. Redirect with        |                     |
  |                       |    code_challenge       |                     |
  |                       |------------------------>                      |
  |                       |                         |                     |
  | 4. User authenticates |                         |                     |
  |<-----------------------------------------------                       |
  |                       |                         |                     |
  | 5. Auth code callback |                         |                     |
  |---------------------->|                         |                     |
  |                       |                         |                     |
  |                       | 6. Exchange code +      |                     |
  |                       |    code_verifier        |                     |
  |                       |------------------------>                      |
  |                       |                         |                     |
  |                       | 7. Validate PKCE        |                     |
  |                       |    Return tokens        |                     |
  |                       |<------------------------                      |
  |                       |                         |                     |
  |                       | 8. Validate token       |                     |
  |                       |    expiry + issuer      |                     |
  |                       |                         |                     |
  |                       | 9. Fetch user profile   |                     |
  |                       |------------------------------------------------>|
  |                       |                         |                     |
  |                       | 10. User info + groups  |                     |
  |                       |     (with pagination)   |                     |
  |                       |<------------------------------------------------|
  |                       |                         |                     |
  | 11. Session cookie    |                         |                     |
  |<----------------------|                         |                     |
```

---

## 3. File Structure

```
obot-entraid/
+-- tools/                                    # NEW: Custom tools registry
|   +-- index.yaml                           # Registry index (auth providers only)
|   +-- entra-auth-provider/
|       +-- tool.gpt                         # GPTScript tool definition
|       +-- main.go                          # OAuth2-proxy wrapper
|       +-- config.go                        # Configuration loading
|       +-- handlers.go                      # HTTP handlers
|       +-- graph.go                         # Microsoft Graph API client
|       +-- validation.go                    # Token validation
|       +-- errors.go                        # Structured error types
|       +-- metrics.go                       # Prometheus metrics
|       +-- go.mod                           # Go module definition
|       +-- go.sum
|       +-- Makefile                         # Build targets
|       +-- pkg/
|           +-- logging/
|           |   +-- logger.go                # Structured logging setup
|           +-- retry/
|               +-- retry.go                 # Rate limit retry logic
+-- docs/
|   +-- ENTRA_ID_IMPLEMENTATION_SPEC.md      # This document
+-- [existing obot code unchanged]
```

---

## 4. Detailed Component Specifications

### 4.1 Registry Index (`tools/index.yaml`)

```yaml
# Custom tool registry for obot-entraid
# Contains auth providers not available in the open-source edition

authProviders:
  entra-auth-provider:
    reference: ./entra-auth-provider
```

### 4.2 Tool Definition (`tools/entra-auth-provider/tool.gpt`)

```
Name: Entra
Description: Auth provider for Microsoft Entra ID (Azure AD)
Daemon: true
Metadata: icon=https://learn.microsoft.com/en-us/entra/identity/managed-identities-azure-resources/media/entra-id.png
Metadata: link=https://entra.microsoft.com
Metadata: postgresTablePrefix=entra_

# Required configuration parameters
Param: client_id: The Application (client) ID from Azure App Registration (required)
Param: tenant_id: The Directory (tenant) ID, or 'common'/'organizations' for multi-tenant (required)
Param: cookie_secret: Base64-encoded secret (must decode to 16, 24, or 32 bytes for AES) (required, sensitive)
Param: allowed_email_domains: Comma-separated list of allowed email domains, or * to allow all (required)

# Authentication method (choose one)
Param: client_secret: The client secret value from Certificates & secrets. Not required if using workload_identity or certificate auth. (optional, sensitive)
Param: use_workload_identity: Enable Azure Workload Identity authentication for Kubernetes (eliminates client_secret). (optional, default: false)
Param: client_cert_path: Path to client certificate PEM file for certificate-based auth. (optional)
Param: client_key_path: Path to client private key PEM file for certificate-based auth. (optional)

# Optional configuration parameters
Param: postgres_dsn: PostgreSQL DSN for session storage. If not set, sessions are stored in cookies. (optional, env: OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN)
Param: token_refresh_duration: How often to refresh the access token. Defaults to 1 hour. (optional)
Param: allowed_groups: Comma-separated list of Azure AD group IDs that are allowed to authenticate. Leave empty to allow all groups. (optional)
Param: allowed_tenants: Comma-separated list of tenant IDs allowed for multi-tenant apps. Required if tenant_id is 'common' or 'organizations'. (optional)
Param: cache_size: Maximum number of user states to cache. Defaults to 5000. (optional)
Param: cache_ttl: Cache TTL duration. Defaults to 1h. (optional)
Param: log_level: Logging level (debug, info, warn, error). Defaults to info. (optional)
Param: metrics_enabled: Enable Prometheus metrics endpoint at /metrics. Defaults to true. (optional)

#!sys.daemon /bin/gptscript-go-tool

---
!metadata:Entra:bundle
true

---
Name: Entra Credential
Type: credential
---
#!sys.echo
```

### 4.3 Main Implementation (`tools/entra-auth-provider/main.go`)

```go
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
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultPort       = 9999
	defaultCacheSize  = 5000
	defaultCacheTTL   = time.Hour
	defaultTimeout    = 30 * time.Second
	graphAPIBaseURL   = "https://graph.microsoft.com/v1.0"
	maxRetries        = 3
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
	ClientID              string
	ClientSecret          string
	TenantID              string
	CookieSecret          []byte
	AllowedEmailDomains   []string
	AllowedGroups         []string
	AllowedTenants        []string
	PostgresDSN           string
	TokenRefreshDuration  time.Duration
	Port                  int
	ServerURL             string
	CacheSize             int
	CacheTTL              time.Duration
	LogLevel              slog.Level
	UseWorkloadIdentity   bool
	ClientCertPath        string
	ClientKeyPath         string
	MetricsEnabled        bool
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

	// Configure oauth2-proxy for Microsoft Entra ID with PKCE
	proxyOpts, err := buildProxyOptions(config)
	if err != nil {
		logger.Error("Failed to build proxy options", "error", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := validation.Validate(proxyOpts); err != nil {
		logger.Error("Invalid oauth2-proxy configuration", "error", err)
		os.Exit(1)
	}

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
		fmt.Sscanf(p, "%d", &port)
	}

	refreshDuration := time.Hour
	if d := os.Getenv("GPTSCRIPT_TOKEN_REFRESH_DURATION"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			refreshDuration = parsed
		}
	}

	cacheSize := defaultCacheSize
	if s := os.Getenv("GPTSCRIPT_CACHE_SIZE"); s != "" {
		fmt.Sscanf(s, "%d", &cacheSize)
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
		ClientID:              os.Getenv("GPTSCRIPT_CLIENT_ID"),
		ClientSecret:          clientSecret,
		TenantID:              tenantID,
		CookieSecret:          cookieBytes,
		AllowedEmailDomains:   allowedEmailDomains,
		AllowedGroups:         allowedGroups,
		AllowedTenants:        allowedTenants,
		PostgresDSN:           os.Getenv("OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"),
		TokenRefreshDuration:  refreshDuration,
		Port:                  port,
		ServerURL:             os.Getenv("OBOT_SERVER_URL"),
		CacheSize:             cacheSize,
		CacheTTL:              cacheTTL,
		LogLevel:              logLevel,
		UseWorkloadIdentity:   useWorkloadIdentity,
		ClientCertPath:        clientCertPath,
		ClientKeyPath:         clientKeyPath,
		MetricsEnabled:        metricsEnabled,
	}, nil
}

func isMultiTenant(tenantID string) bool {
	return tenantID == "common" || tenantID == "organizations" || tenantID == "consumers"
}

func buildProxyOptions(config Config) (*options.Options, error) {
	opts := options.NewOptions()

	// Determine issuer URL based on tenant configuration
	issuerURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", config.TenantID)

	// Provider configuration for Microsoft Entra ID with PKCE
	// NOTE: oauth2-proxy v7.13.0+ validates sessions using access_token,
	// not id_token, during token refresh. This aligns with OIDC spec
	// which doesn't guarantee id_token issuance on refresh.
	provider := options.Provider{
		ID:                  "entra",
		Type:                options.OIDCProvider,
		ClientID:            config.ClientID,
		CodeChallengeMethod: "S256", // PKCE with SHA-256 (REQUIRED by Microsoft)
		OIDCConfig: options.OIDCOptions{
			IssuerURL:      issuerURL,
			ExtraAudiences: []string{config.ClientID},
		},
		// Only request delegated scopes - application permissions are configured in Azure portal
		Scope: "openid email profile User.Read",
	}

	// Configure authentication method
	if config.UseWorkloadIdentity {
		// Azure Workload Identity - no client secret needed
		provider.EntraIDFederatedTokenAuth = true
	} else if config.ClientCertPath != "" {
		// Certificate-based authentication
		provider.ClientCertFile = config.ClientCertPath
		provider.ClientKeyFile = config.ClientKeyPath
	} else {
		// Client secret authentication
		provider.ClientSecret = config.ClientSecret
	}

	opts.Providers = []options.Provider{provider}

	// Multi-tenant configuration
	if isMultiTenant(config.TenantID) {
		opts.InsecureOIDCSkipIssuerVerification = true
		// Validate against allowed tenants list
		opts.ExtraJWTIssuers = make([]options.JWTIssuer, len(config.AllowedTenants))
		for i, tenant := range config.AllowedTenants {
			opts.ExtraJWTIssuers[i] = options.JWTIssuer{
				Issuer:   fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenant),
				Audience: config.ClientID,
			}
		}
	}

	// Cookie configuration
	opts.Cookie.Secret = string(config.CookieSecret)
	opts.Cookie.Secure = true
	opts.Cookie.HTTPOnly = true
	opts.Cookie.SameSite = "lax"

	// Email domain restrictions
	if len(config.AllowedEmailDomains) > 0 && config.AllowedEmailDomains[0] != "*" {
		opts.EmailDomains = config.AllowedEmailDomains
	}

	// Session storage
	if config.PostgresDSN != "" {
		opts.Session.Type = options.PostgresSessionStoreType
		opts.Session.Postgres.ConnectionURL = config.PostgresDSN
	}

	return opts, nil
}

// writeError sends a structured JSON error response
func writeError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIError{
		Error: message,
		Code:  code,
	})
}

// Health check endpoint for Kubernetes liveness probe
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// Readiness check endpoint for Kubernetes readiness probe
func handleReady(w http.ResponseWriter, r *http.Request) {
	// Check if we can reach Microsoft's OIDC discovery endpoint
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	discoveryURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0/.well-known/openid-configuration", config.TenantID)
	req, _ := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	resp, err := graphClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "reason": "cannot reach Entra ID"})
		return
	}
	resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
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
		json.NewEncoder(w).Encode(cached)
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
	json.NewEncoder(w).Encode(state)
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
	json.NewEncoder(w).Encode(userInfo)
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
	json.NewEncoder(w).Encode(map[string][]string{"groups": groups})
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

	req, err := http.NewRequestWithContext(ctx, "GET", graphAPIBaseURL+"/me", nil)
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
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
```

### 4.4 Go Module (`tools/entra-auth-provider/go.mod`)

```go
module github.com/obot-platform/obot-entraid/tools/entra-auth-provider

go 1.25.5

require (
    github.com/hashicorp/golang-lru/v2 v2.0.7
    github.com/oauth2-proxy/oauth2-proxy/v7 v7.13.0
    github.com/prometheus/client_golang v1.20.0
)
```

> **SECURITY NOTE**: Version **7.13.0** is REQUIRED to address:
> - CVE-2025-54576 (CVSS 9.1) - Authentication bypass via skip_auth_routes
> - CVE-2025-64484 - Request header smuggling via underscore normalization
> - CVE-2025-47912, CVE-2025-58183, CVE-2025-58186 - Various security fixes

### 4.5 Makefile (`tools/entra-auth-provider/Makefile`)

```makefile
.PHONY: build clean test lint

VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.Version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/gptscript-go-tool .

clean:
	rm -rf bin/

test:
	go test -v -race -cover ./...

lint:
	golangci-lint run
```

---

## 5. Configuration Changes to Obot

### 5.1 Environment Variable

Add the custom tools registry to the obot server configuration:

```bash
# Option A: Add to existing registries (recommended)
OBOT_SERVER_TOOL_REGISTRIES=github.com/obot-platform/tools,./tools

# Option B: Use only custom registry (if you want to maintain your own fork)
OBOT_SERVER_TOOL_REGISTRIES=./tools
```

### 5.2 Helm Chart Values (if deploying via Helm)

```yaml
# chart/values.yaml addition
config:
  toolRegistries:
    - github.com/obot-platform/tools
    - ./tools  # or a git raw URL to your tools index
```

### 5.3 Local Development

For `make dev`, the local tools directory will be automatically accessible. Add to `.envrc.dev`:

```bash
export OBOT_SERVER_TOOL_REGISTRIES=github.com/obot-platform/tools,./tools
```

---

## 6. Azure AD App Registration Setup

### 6.1 Create App Registration

1. Navigate to [Microsoft Entra admin center](https://entra.microsoft.com)
2. Go to **App registrations** > **New registration**
3. Configure:
   - **Name**: Obot Authentication
   - **Supported account types**:
     - "Accounts in this organizational directory only" (single tenant - **most secure**)
     - OR "Accounts in any organizational directory" (multi-tenant - requires `allowed_tenants`)
   - **Redirect URI**: Web - `https://<your-obot-domain>/oauth2/callback`

### 6.2 Configure API Permissions

**Delegated permissions** (requested via OAuth scope):
- `openid` - Sign in and read user profile
- `email` - View user's email address
- `profile` - View user's basic profile
- `User.Read` - Sign in and read user profile (used to call Graph API on behalf of user)

**Application permissions** (configured in Azure portal, NOT in OAuth scope):
- `GroupMember.Read.All` - Read all group memberships (**requires admin consent**)
- `User.Read.All` - Read all users' profiles (**requires admin consent**)

> **IMPORTANT**: Application permissions are configured in Azure AD portal and require admin consent. They are NOT requested in the OAuth scope. The `/me/transitiveMemberOf` endpoint uses delegated `User.Read` permission to fetch the signed-in user's groups.

### 6.3 Authentication Method (Choose One)

#### Option A: Workload Identity Federation (RECOMMENDED for Kubernetes)

For AKS or Kubernetes deployments, use Azure Workload Identity to eliminate secrets entirely:

1. **Prerequisites**:
   - Cluster has public OIDC provider URL enabled
   - Workload Identity admission webhook deployed
   - Federated credential configured in App Registration

2. **Configuration**:
   - Service account annotated with `azure.workload.identity/client-id: <client-id>`
   - Pod labeled with `azure.workload.identity/use: "true"`
   - Set `use_workload_identity: true` in tool configuration
   - No `client_secret` required

**Benefits**: No credential storage, automatic token refresh, no rotation required.

#### Option B: Certificate Credentials (RECOMMENDED for non-Kubernetes)

For enhanced security over client secrets:

1. Generate or obtain a certificate from Azure Key Vault
2. Upload public key to **App Registration > Certificates & secrets > Certificates**
3. Store private key securely (not in source control)
4. Configure paths via `client_cert_path` and `client_key_path`

**Best Practices**:
- Use certificates from a trusted CA (Azure Key Vault recommended)
- Maximum lifetime: 180 days
- Configure automatic rotation via Key Vault

#### Option C: Client Secret (Development Only)

**NOT RECOMMENDED FOR PRODUCTION**

1. Go to **Certificates & secrets > Client secrets**
2. Click **New client secret**
3. Set expiration (maximum: 24 months, recommend: 12 months)
4. **Save the Value immediately** - cannot be retrieved later

**Limitations**:
- Less secure than certificates or Workload Identity
- Requires manual rotation
- Can be accidentally exposed in logs/config

### 6.4 Configure Token Claims (Optional)

For group claims in the ID token (avoids extra Graph API call for <200 groups):
1. Go to **Token configuration**
2. Add optional claim > ID token > `groups`
3. Select "Security groups" or "Groups assigned to the application"

### 6.5 Note Required Values

| Obot Parameter | Azure Portal Location | Notes |
|----------------|----------------------|-------|
| `client_id` | Overview > Application (client) ID | |
| `tenant_id` | Overview > Directory (tenant) ID | Or `common` for multi-tenant |
| `client_secret` | Certificates & secrets > Value | Only if using Option C |
| `allowed_tenants` | Your allowed tenant IDs | Required if using `common` |

---

## 7. Implementation Tasks

### Phase 1: Setup (Day 1)
- [ ] Create `tools/` directory structure
- [ ] Create `tools/index.yaml`
- [ ] Initialize Go module for entra-auth-provider (**v7.13.0**)
- [ ] Add Makefile with lint/test targets
- [ ] Setup golangci-lint configuration

### Phase 2: Core Implementation (Days 2-3)
- [ ] Implement `tool.gpt` with all parameters including Workload Identity
- [ ] Implement `main.go` with PKCE-enabled oauth2-proxy
- [ ] Implement token validation (expiry, issuer, tenant)
- [ ] Implement Microsoft Graph API calls with retry logic
- [ ] Implement group membership fetching with pagination
- [ ] Add structured logging with slog
- [ ] Add expirable LRU caching with TTL
- [ ] Add Prometheus metrics

### Phase 3: Security Hardening (Day 4)
- [ ] Verify PKCE is enforced (S256)
- [ ] Implement multi-tenant issuer validation
- [ ] Add cookie secret length validation
- [ ] Add HTTP client timeouts
- [ ] Add health/readiness endpoints
- [ ] Implement rate limit retry with Retry-After
- [ ] Security review of all endpoints
- [ ] Test header smuggling protection (v7.13.0)

### Phase 4: Integration (Day 5)
- [ ] Update `.envrc.dev` for local development
- [ ] Test with local obot instance
- [ ] Verify tool appears in Admin UI auth providers
- [ ] Configure with test Azure AD app
- [ ] Test authentication flow with PKCE
- [ ] Test multi-tenant scenario (if applicable)
- [ ] Test Workload Identity (if applicable)

### Phase 5: Group Sync & RBAC (Day 6)
- [ ] Test with user having <200 groups
- [ ] Test with user having 200+ groups (pagination)
- [ ] Verify groups are returned from `/obot-list-user-auth-groups`
- [ ] Test group-based access control rules
- [ ] Document group ID -> role mapping

### Phase 6: Documentation & Polish (Day 7)
- [ ] Update `docs/docs/configuration/auth-providers.md`
- [ ] Add Entra setup screenshots
- [ ] Update README.md
- [ ] Create example configurations
- [ ] Write troubleshooting guide

---

## 8. Testing Checklist

### Unit Tests
- [ ] Config loading from environment
- [ ] Cookie secret validation (16, 24, 32 bytes)
- [ ] OAuth2-proxy options building with PKCE
- [ ] Token validation (format, expiry, issuer, tenant)
- [ ] Microsoft Graph API response parsing
- [ ] Group pagination handling
- [ ] Group filtering logic
- [ ] Expirable LRU cache behavior
- [ ] Retry logic with Retry-After handling
- [ ] Structured error responses

### Security Tests
- [ ] Verify PKCE code_challenge is sent
- [ ] Test token expiry handling (reject expired)
- [ ] Test invalid/malformed tokens (reject)
- [ ] Test issuer validation in multi-tenant
- [ ] Test tenant ID validation against allowed list
- [ ] Test cookie tampering detection
- [ ] Test session hijacking prevention
- [ ] Test header smuggling protection (underscore headers)

### Integration Tests
- [ ] Tool appears in registry after obot starts
- [ ] Configure endpoint accepts credentials
- [ ] Authentication flow redirects correctly with PKCE
- [ ] Callback returns valid session
- [ ] User info endpoint returns profile
- [ ] Groups endpoint returns memberships
- [ ] Deconfigure removes sessions from DB
- [ ] Health endpoint returns 200
- [ ] Ready endpoint checks Entra ID connectivity
- [ ] Metrics endpoint returns Prometheus format

### Edge Case Tests
- [ ] User with no groups
- [ ] User with exactly 200 groups
- [ ] User with 1000+ groups (pagination)
- [ ] Concurrent login attempts
- [ ] Token refresh during active session
- [ ] Microsoft Graph API timeout
- [ ] Microsoft Graph API 5xx errors
- [ ] Microsoft Graph API rate limiting (429)
- [ ] Retry-After header parsing

### End-to-End Tests
- [ ] Full login flow with real Azure AD
- [ ] Token refresh works after expiry
- [ ] Group-based access control works
- [ ] Session persistence in PostgreSQL
- [ ] Logout clears session
- [ ] Multi-tenant login (if applicable)
- [ ] Workload Identity authentication (if applicable)

---

## 9. Security Considerations

### 9.1 Critical Security Requirements

| Requirement | Implementation | Status |
|-------------|----------------|--------|
| oauth2-proxy **v7.13.0** | `go.mod` specifies exact version | Required |
| PKCE (S256) | `CodeChallengeMethod: "S256"` in provider config | Required |
| Header smuggling protection | v7.13.0 normalizes underscore headers | Required |
| Issuer validation | Multi-tenant uses `AllowedTenants` list | Required |
| Token validation | `validateAccessToken()` checks expiry/issuer/tenant | Required |
| Cookie encryption | AES-128/192/256 based on secret length | Required |
| Rate limit handling | Retry with `Retry-After` header | Required |

### 9.2 PKCE (Proof Key for Code Exchange)

PKCE prevents authorization code interception attacks and is [required by Microsoft](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow) for all OAuth2 flows.

```go
provider := options.Provider{
    CodeChallengeMethod: "S256",  // SHA-256 PKCE challenge
    // ...
}
```

> **Note**: Microsoft Entra ID does not include `code_challenge_methods_supported` in OIDC discovery metadata (unlike Google, Okta, Auth0). This is implementation-dependent per RFC 8414. oauth2-proxy handles this correctly, but be aware if using other OIDC clients.

**Alternative configuration** (if programmatic fails):
```bash
# CLI flag (backup)
--code-challenge-method=S256
```

### 9.3 Cookie Security

| Setting | Value | Purpose |
|---------|-------|---------|
| `HttpOnly` | true | Prevent XSS access to cookies |
| `Secure` | true | HTTPS only transmission |
| `SameSite` | lax | CSRF protection |
| Secret length | 32 bytes | AES-256 encryption |

### 9.4 Multi-Tenant Security

When `tenant_id` is `common`, `organizations`, or `consumers`:

1. **Must configure `allowed_tenants`** - explicit list of permitted tenant IDs
2. **Token validation** - `tid` claim checked against allowed list
3. **Issuer skip** - `InsecureOIDCSkipIssuerVerification` enabled with tenant validation

```go
// Multi-tenant with explicit tenant allowlist
if isMultiTenant(config.TenantID) {
    opts.InsecureOIDCSkipIssuerVerification = true
    // Validate tid claim in validateAccessToken()
}
```

### 9.5 Rate Limiting

Implement rate limiting to prevent abuse:

- **Microsoft Graph API**: Respect `429 Too Many Requests` responses with `Retry-After` header
- **Exponential backoff**: For 5xx errors, retry with increasing delays
- **Authentication attempts**: Limit failed logins per IP
- **Session creation**: Prevent DoS through session exhaustion

```go
// Rate limit handling with Retry-After
if resp.StatusCode == http.StatusTooManyRequests {
    retryAfter := resp.Header.Get("Retry-After")
    if seconds, err := strconv.Atoi(retryAfter); err == nil {
        time.Sleep(time.Duration(seconds) * time.Second)
    }
}
```

### 9.6 Credential Preference Hierarchy

Microsoft recommends credentials in this order (most to least secure):

1. **Managed Identity** - No credentials to manage (Azure-hosted only)
2. **Workload Identity Federation** - For Kubernetes/GitHub Actions
3. **Certificate Credentials** - Recommended for production
4. **Client Secrets** - NOT recommended for production

See Section 6.3 for implementation details.

### 9.7 Audit Logging

Log all authentication events for security monitoring:

```go
logger.Info("User authenticated",
    "event", "login_success",
    "user_id", userID,
    "email", email,
    "tenant_id", tenantID,
    "ip", r.RemoteAddr,
    "user_agent", r.UserAgent(),
)

logger.Warn("Authentication failed",
    "event", "login_failure",
    "reason", "token_expired",
    "ip", r.RemoteAddr,
)
```

### 9.8 Secret Management

| Secret | Storage | Notes |
|--------|---------|-------|
| Client Secret | GPTScript credential system | Encrypted at rest |
| Cookie Secret | GPTScript credential system | 32 bytes, base64 encoded |
| PostgreSQL DSN | Environment variable | Contains password |
| Certificate Key | Secure file system | Permissions 600 |

### 9.9 Header Smuggling Protection

oauth2-proxy v7.13.0+ normalizes headers to prevent request header smuggling attacks (CVE-2025-64484):

- Both `X-Forwarded-For` and `X_Forwarded-for` are stripped
- Capitalization is normalized
- Underscores and dashes treated equivalently

**Recommendation**: Configure upstream applications to also reject underscore headers.

---

## 10. Observability

### 10.1 Health Endpoints

| Endpoint | Purpose | Kubernetes Probe |
|----------|---------|------------------|
| `/health` | Liveness check | `livenessProbe` |
| `/ready` | Readiness check (verifies Entra ID connectivity) | `readinessProbe` |
| `/metrics` | Prometheus metrics | N/A |

### 10.2 Structured Logging

All logs use `slog` with JSON output for observability platform integration:

```json
{
  "time": "2025-12-07T10:00:00Z",
  "level": "INFO",
  "msg": "User authenticated",
  "user_id": "abc123",
  "email": "user@example.com",
  "group_count": 15
}
```

### 10.3 Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `entra_auth_requests_total` | Counter | status, endpoint | Total authentication requests |
| `entra_auth_failures_total` | Counter | reason | Failed authentications by reason |
| `entra_graph_api_duration_seconds` | Histogram | endpoint, status | Graph API latency |
| `entra_cache_hits_total` | Counter | - | Cache hits |
| `entra_cache_misses_total` | Counter | - | Cache misses |

---

## 11. Error Handling & Graceful Degradation

### 11.1 Structured Error Responses

All error responses use a consistent JSON format:

```go
type APIError struct {
    Error   string `json:"error"`
    Code    string `json:"code,omitempty"`
    Details string `json:"details,omitempty"`
}
```

| HTTP Status | Code | Description |
|-------------|------|-------------|
| 401 | `AUTH_REQUIRED` | No authentication provided |
| 401 | `INVALID_TOKEN` | Token expired or malformed |
| 401 | `NO_TOKEN` | Access token missing |
| 500 | `GRAPH_API_ERROR` | Microsoft Graph API failure |

### 11.2 Microsoft Graph API Failures

| Scenario | Behavior |
|----------|----------|
| Timeout | Retry up to 3 times, return cached state if available |
| 5xx Error | Retry with exponential backoff (1s, 2s, 4s) |
| 429 Rate Limited | Respect `Retry-After` header, retry |
| 401 Unauthorized | Token expired, trigger refresh |

### 11.3 Entra ID Unavailable

If Microsoft Entra ID is unreachable:
- `/ready` endpoint returns 503
- Existing sessions remain valid (cookie-based)
- New logins will fail with clear error message

---

## 12. References

- [Microsoft Entra ID OAuth 2.0 Authorization Code Flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow)
- [OAuth2-Proxy Microsoft Entra ID Provider](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/ms_entra_id/)
- [OAuth2-Proxy v7.13.0 Release Notes](https://github.com/oauth2-proxy/oauth2-proxy/releases/tag/v7.13.0)
- [CVE-2025-54576 - OAuth2-Proxy Authentication Bypass](https://nvd.nist.gov/vuln/detail/CVE-2025-54576)
- [CVE-2025-64484 - Header Smuggling](https://github.com/advisories/GHSA-vjrc-mh2v-45x6)
- [Microsoft Graph API - User](https://learn.microsoft.com/en-us/graph/api/user-get)
- [Microsoft Graph API - List transitiveMemberOf](https://learn.microsoft.com/en-us/graph/api/user-list-transitivememberof)
- [Microsoft Graph Best Practices](https://learn.microsoft.com/en-us/graph/best-practices-concept)
- [Microsoft Entra Security Best Practices](https://learn.microsoft.com/en-us/entra/identity-platform/security-best-practices-for-app-registration)
- [Azure Workload Identity](https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation)
- [HashiCorp golang-lru v2 Expirable](https://pkg.go.dev/github.com/hashicorp/golang-lru/v2/expirable)
- [obot-platform/tools Repository](https://github.com/obot-platform/tools)

---

## 13. Appendix: Comparison with Enterprise Edition

Based on the documentation at `docs/docs/configuration/auth-providers.md`, the Enterprise Edition's Entra provider requires:

| Feature | Enterprise | This Implementation |
|---------|------------|---------------------|
| Basic OAuth2/OIDC | Yes | Yes |
| PKCE (S256) | Unknown | Yes |
| User.Read permission | Yes | Yes |
| ProfilePhoto.Read.All | Yes | Yes |
| GroupMember.Read.All | Yes | Yes |
| User.Read.All | Yes | Yes |
| Multi-tenant support | Unknown | Yes (with tenant validation) |
| Group filtering | Unknown | Yes |
| Group overage (200+) | Unknown | Yes (pagination) |
| PostgreSQL sessions | Yes | Yes |
| Structured logging | Unknown | Yes |
| Health checks | Unknown | Yes |
| Prometheus metrics | Unknown | Yes |
| Workload Identity | Unknown | Yes |
| Certificate auth | Unknown | Yes |
| Rate limit handling | Unknown | Yes |
| Header smuggling protection | Unknown | Yes (v7.13.0) |

This implementation aims to be feature-equivalent or better than the Enterprise Edition's Entra provider, with additional security hardening based on December 2025 best practices.

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| v3 | 2025-12-07 | Analysis review fixes: oauth2-proxy v7.13.0, Workload Identity, expirable cache, Prometheus metrics, structured errors, rate limit retry, header smuggling protection |
| v2 | 2025-12-07 | Security review fixes: PKCE, oauth2-proxy v7.11.0, token validation, group pagination |
| v1 | 2025-12-07 | Initial draft |
