# Authentication Security Implementation Guide

## Document Overview

**Created:** 2026-01-12
**Based On:** Comprehensive Security Analysis of obot-entraid Authentication System
**Scope:** Custom authentication providers (Keycloak, Microsoft Entra ID)
**Priority:** CRITICAL - Contains security vulnerabilities requiring immediate attention

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Critical Security Vulnerabilities](#critical-security-vulnerabilities)
3. [High Priority Issues](#high-priority-issues)
4. [Integration Testing Requirements](#integration-testing-requirements)
5. [Token Refresh Configuration](#token-refresh-configuration)
6. [Implementation Roadmap](#implementation-roadmap)
7. [Code Examples and Fixes](#code-examples-and-fixes)
8. [Monitoring and Observability](#monitoring-and-observability)
9. [Testing Strategy](#testing-strategy)
10. [Future Enhancements and Security Considerations](#future-enhancements-and-security-considerations)
11. [References](#references)

---

## Executive Summary

This guide provides actionable implementation instructions for addressing security vulnerabilities and gaps identified in the obot-entraid authentication system during a comprehensive security analysis conducted on 2026-01-12.

### Key Findings

- **3 CRITICAL vulnerabilities** requiring immediate fixes
- **5 HIGH priority issues** requiring near-term resolution
- **10 missing integration test scenarios** creating production risk
- **Token refresh error handling** requires improvement
- **Observability gaps** preventing proactive issue detection

### Immediate Actions Required

1. Fix Entra ID non-fatal ID token parsing error (CRITICAL)
2. Expand token refresh error handling (CRITICAL)
3. Add TLS validation for cookie Secure flag (CRITICAL)
4. Implement basic monitoring metrics (HIGH)
5. Create integration test suite (HIGH)

---

## Critical Security Vulnerabilities

### CRITICAL-1: Entra ID Non-Fatal ID Token Parsing Error

#### Problem Statement

**Location:** `tools/entra-auth-provider/main.go:306-328`

The Entra ID authentication provider uses non-fatal error handling for ID token parsing failures. This causes:
- Admin/owner permission loss after re-login
- Duplicate user account creation with inconsistent ProviderUserID
- Silent failures masking OAuth configuration issues

This is the **EXACT SAME BUG** that was fixed in Keycloak (commit 1e7fb26c) but was not applied to Entra ID.

#### Current Code (VULNERABLE)

```go
// tools/entra-auth-provider/main.go:306-328
if ss.IDToken != "" {
    userProfile, err := profile.ParseIDToken(ss.IDToken)
    if err != nil {
        fmt.Printf("WARNING: entra-auth-provider: failed to parse ID token: %v\n", err)
        // BUG: Continues without setting ss.User - causes identity mismatch
    } else {
        // Validate tenant if restrictions are configured (multi-tenant mode)
        if allowedTenants != nil && !allowedTenants[userProfile.TenantID] {
            fmt.Printf("WARNING: entra-auth-provider: rejected login from unauthorized tenant: %s\n", userProfile.TenantID)
            http.Error(w, "tenant not allowed", http.StatusForbidden)
            return
        }

        // Set User to Azure Object ID (stable identifier)
        ss.User = userProfile.OID
        // Set PreferredUsername to the human-readable UPN from the token
        if userProfile.PreferredUsername != "" {
            ss.PreferredUsername = userProfile.PreferredUsername
        } else if userProfile.Email != "" {
            ss.PreferredUsername = userProfile.Email
        }
    }
}
```

#### Fixed Code (REQUIRED)

```go
// tools/entra-auth-provider/main.go:306-328 (UPDATED)

// CRITICAL: ID token parsing is required for reliable user identification
// Without it, we cannot guarantee consistent ProviderUserID across sessions
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
if userProfile.PreferredUsername != "" {
    ss.PreferredUsername = userProfile.PreferredUsername
} else if userProfile.Email != "" {
    ss.PreferredUsername = userProfile.Email
}
```

#### Implementation Steps

1. **Update `tools/entra-auth-provider/main.go`:**
   - Locate the `getState` function (lines 290-362)
   - Replace lines 306-328 with the fixed code above
   - Change `WARNING` to `ERROR` in log statements
   - Add explicit check for empty ID token
   - Return HTTP 401 if ID token is missing
   - Return HTTP 500 if ID token parsing fails

2. **Test the fix:**
   ```bash
   cd tools/entra-auth-provider
   go test -v ./...
   go build -o entra-auth-provider .
   ```

3. **Verify behavior:**
   - Authentication MUST fail if ID token is missing
   - Authentication MUST fail if ID token parsing fails
   - Error messages must be logged as ERROR (not WARNING)
   - User profile information must be complete or authentication fails

4. **Update integration tests:**
   - Add test case for missing ID token (should return 401)
   - Add test case for malformed ID token (should return 500)
   - Add test case for successful ID token parsing (should set ss.User)

#### Verification Checklist

- [ ] Code changes applied to `tools/entra-auth-provider/main.go`
- [ ] Unit tests pass (`go test ./...`)
- [ ] Build succeeds (`go build`)
- [ ] Error messages changed from WARNING to ERROR
- [ ] ID token validation is mandatory (fails if missing)
- [ ] `ss.User` is always set or authentication fails
- [ ] Commit message follows pattern from auth_fix_jan2026
- [ ] Integration test added for regression prevention

#### Related Files

- `tools/keycloak-auth-provider/main.go` (reference implementation - CORRECT)
- `tools/auth-providers-common/pkg/profile/profile.go` (ParseIDToken function)
- Memory: `auth_fix_jan2026` (original Keycloak fix documentation)

---

### CRITICAL-2: Incomplete Token Refresh Error Handling

#### Problem Statement

**Location:** `pkg/proxy/proxy.go:283-286`

Token refresh failures are not properly converted to `ErrInvalidSession`, causing HTTP 500 errors instead of redirecting users to login. This creates poor user experience and makes OAuth misconfigurations difficult to diagnose.

#### Current Code (VULNERABLE)

```go
// pkg/proxy/proxy.go:283-286
if stateResponse.StatusCode == http.StatusInternalServerError &&
   (strings.Contains(string(body), "record not found") ||
    strings.Contains(string(body), "session ticket cookie failed validation")) {
    return nil, false, ErrInvalidSession
}
```

**Problem:** Only handles specific error messages. Token refresh failures like:
- "refreshing token returned 401: ..."
- "refreshing token returned unexpected status 500: ..."
- "REFRESH_TOKEN_ERROR: invalid_token"
- "RESTART_AUTHENTICATION_ERROR: cookie_not_found"

...are NOT converted to `ErrInvalidSession` and cause HTTP 500 responses.

#### Fixed Code (REQUIRED)

```go
// pkg/proxy/proxy.go:283-295 (UPDATED)

// Handle session-related errors that should redirect to login
if stateResponse.StatusCode == http.StatusInternalServerError {
    bodyStr := string(body)

    // List of error patterns that indicate invalid/expired session
    sessionErrors := []string{
        "record not found",
        "session ticket cookie failed validation",
        "refreshing token returned",           // Token refresh failures
        "REFRESH_TOKEN_ERROR",                 // OAuth2-proxy token refresh error
        "RESTART_AUTHENTICATION_ERROR",        // OAuth2-proxy auth restart error
        "invalid_token",                       // OAuth2 standard error code
        "failed to refresh token",             // State.go refresh error
    }

    for _, errPattern := range sessionErrors {
        if strings.Contains(bodyStr, errPattern) {
            return nil, false, ErrInvalidSession
        }
    }
}
```

#### Implementation Steps

1. **Update `pkg/proxy/proxy.go`:**
   - Locate the `authenticateRequest` method (lines 256-316)
   - Replace lines 283-286 with the fixed code above
   - Add comprehensive error pattern matching

2. **Add structured error handling:**
   ```go
   // pkg/proxy/errors.go (NEW FILE)
   package proxy

   import "strings"

   // IsSessionError determines if an error indicates an invalid session
   func IsSessionError(statusCode int, body string) bool {
       if statusCode != http.StatusInternalServerError {
           return false
       }

       sessionErrorPatterns := []string{
           "record not found",
           "session ticket cookie failed validation",
           "refreshing token returned",
           "REFRESH_TOKEN_ERROR",
           "RESTART_AUTHENTICATION_ERROR",
           "invalid_token",
           "failed to refresh token",
       }

       for _, pattern := range sessionErrorPatterns {
           if strings.Contains(body, pattern) {
               return true
           }
       }

       return false
   }
   ```

3. **Update authenticateRequest to use helper:**
   ```go
   // pkg/proxy/proxy.go:283-286 (UPDATED)
   if IsSessionError(stateResponse.StatusCode, string(body)) {
       return nil, false, ErrInvalidSession
   }
   ```

4. **Add logging for diagnostics:**
   ```go
   if IsSessionError(stateResponse.StatusCode, string(body)) {
       log.WithFields(log.Fields{
           "provider": p.name,
           "status_code": stateResponse.StatusCode,
           "error_body": string(body),
       }).Warn("session error detected, redirecting to login")
       return nil, false, ErrInvalidSession
   }
   ```

#### Implementation Steps (Detailed)

1. **Create error helper file:**
   ```bash
   touch pkg/proxy/errors.go
   ```

2. **Implement `IsSessionError` function** (code above)

3. **Update `authenticateRequest` method:**
   - Replace string matching with `IsSessionError` call
   - Add structured logging

4. **Test error handling:**
   ```bash
   cd pkg/proxy
   go test -v ./...
   ```

5. **Create integration test:**
   ```go
   // pkg/proxy/proxy_test.go (NEW)
   func TestAuthenticateRequest_TokenRefreshError(t *testing.T) {
       // Test that token refresh errors return ErrInvalidSession
       // Test that users are redirected to login (not 500 error)
   }
   ```

#### Verification Checklist

- [ ] `pkg/proxy/errors.go` created with `IsSessionError` function
- [ ] `pkg/proxy/proxy.go` updated to use `IsSessionError`
- [ ] All token refresh error patterns are covered
- [ ] Logging added for session error diagnostics
- [ ] Unit tests pass
- [ ] Integration test added for token refresh failures
- [ ] Manual testing with expired tokens confirms redirect to login

#### Root Cause Context

The issue originates in `tools/auth-providers-common/pkg/state/state.go:124-149`:

```go
func refreshToken(p *oauth2proxy.OAuthProxy, r *http.Request) ([]string, error) {
    // ...
    switch w.status {
    case http.StatusOK, http.StatusAccepted:
        return headers, nil
    case http.StatusUnauthorized, http.StatusForbidden:
        return nil, fmt.Errorf("refreshing token returned %d: %s", w.status, w.body)
    default:
        return nil, fmt.Errorf("refreshing token returned unexpected status %d: %s", w.status, w.body)
    }
}
```

These errors propagate to `authenticateRequest` but are not recognized as session errors, causing HTTP 500 responses.

---

### CRITICAL-3: Cookie Secure Flag Vulnerability

#### Problem Statement

**Locations:**
- `tools/entra-auth-provider/main.go:134`
- `tools/keycloak-auth-provider/main.go:152`

The Secure flag for session cookies is set based on **URL string prefix checking** without TLS validation. This can expose session tokens over unencrypted HTTP if the URL is misconfigured.

#### Current Code (VULNERABLE)

```go
// tools/entra-auth-provider/main.go:134
oauthProxyOpts.Cookie.Secure = strings.HasPrefix(opts.ObotServerURL, "https://")

// tools/keycloak-auth-provider/main.go:152
oauthProxyOpts.Cookie.Secure = strings.HasPrefix(opts.ObotServerURL, "https://")
```

**Problems:**
- Misconfigured `OBOT_SERVER_PUBLIC_URL` could expose tokens over HTTP
- No validation that actual traffic is using TLS
- Development environments might accidentally use HTTP in production
- Man-in-the-middle attacks possible

#### Fixed Code (REQUIRED)

```go
// tools/entra-auth-provider/main.go:130-140 (UPDATED)

// Cookie configuration
oauthProxyOpts.Cookie.Refresh = refreshDuration
oauthProxyOpts.Cookie.Name = "obot_access_token"
oauthProxyOpts.Cookie.Secret = string(cookieSecret)
oauthProxyOpts.Cookie.CSRFExpire = 30 * time.Minute

// Parse server URL for secure cookie determination
parsedURL, err := url.Parse(opts.ObotServerURL)
if err != nil {
    fmt.Printf("ERROR: entra-auth-provider: invalid OBOT_SERVER_PUBLIC_URL: %v\n", err)
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
```

#### Implementation Steps

1. **Update both auth providers** (entra and keycloak):

   **File:** `tools/entra-auth-provider/main.go:130-140`
   **File:** `tools/keycloak-auth-provider/main.go:150-160`

   Replace cookie configuration with fixed code above.

2. **Add URL validation:**
   ```go
   // Validate OBOT_SERVER_PUBLIC_URL format
   parsedURL, err := url.Parse(opts.ObotServerURL)
   if err != nil {
       fmt.Printf("ERROR: %s: invalid OBOT_SERVER_PUBLIC_URL: %v\n", providerName, err)
       os.Exit(1)
   }

   if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
       fmt.Printf("ERROR: %s: OBOT_SERVER_PUBLIC_URL must have http or https scheme\n", providerName)
       os.Exit(1)
   }
   ```

3. **Update tool.gpt metadata:**

   **File:** `tools/entra-auth-provider/tool.gpt`
   ```gpt
   Metadata: optionalEnvVars: OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS,OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS,OBOT_ENTRA_AUTH_PROVIDER_GROUP_CACHE_TTL,OBOT_ENTRA_AUTH_PROVIDER_ICON_CACHE_TTL,OBOT_ENTRA_AUTH_PROVIDER_ADMIN_CLIENT_ID,OBOT_ENTRA_AUTH_PROVIDER_ADMIN_CLIENT_SECRET,OBOT_AUTH_INSECURE_COOKIES
   ```

   **File:** `tools/keycloak-auth-provider/tool.gpt`
   ```gpt
   Metadata: optionalEnvVars: OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS,OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES,OBOT_KEYCLOAK_AUTH_PROVIDER_GROUP_CACHE_TTL,OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_ID,OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_SECRET,OBOT_AUTH_INSECURE_COOKIES
   ```

4. **Update Helm chart values:**

   **File:** `chart/values.yaml`
   ```yaml
   # Authentication configuration
   auth:
     # ...

     # Cookie security (only set to true for local development)
     insecureCookies: false  # NEVER enable in production
   ```

5. **Update documentation:**

   **File:** `tools/README.md`
   ```markdown
   ## Environment Variables

   ### Cookie Security

   - `OBOT_AUTH_INSECURE_COOKIES` (optional, default: `false`)
     - Set to `true` to allow HTTP cookies for local development
     - **NEVER enable in production** - exposes session tokens over unencrypted HTTP
     - Only use with `OBOT_SERVER_PUBLIC_URL=http://localhost:...`
   ```

#### Development vs Production Configuration

**Development (Local):**
```bash
export OBOT_SERVER_PUBLIC_URL="http://localhost:8080"
export OBOT_AUTH_INSECURE_COOKIES="true"
```

**Production:**
```bash
export OBOT_SERVER_PUBLIC_URL="https://obot.example.com"
# OBOT_AUTH_INSECURE_COOKIES not set (defaults to false)
```

#### Verification Checklist

- [ ] URL parsing validation added to both providers
- [ ] `OBOT_AUTH_INSECURE_COOKIES` environment variable support added
- [ ] Fail-safe default: HTTPS required unless explicitly disabled
- [ ] WARNING logged when insecure cookies are enabled
- [ ] `HTTPOnly` and `SameSite` flags set explicitly
- [ ] Tool.gpt metadata updated with new optional env var
- [ ] Helm chart values.yaml updated
- [ ] Documentation updated in tools/README.md
- [ ] Manual testing confirms HTTPS enforcement
- [ ] Manual testing confirms dev mode works with insecure flag

#### Security Rationale

**Why fail-safe default?**
- Prevents accidental production deployment with HTTP
- Forces developers to explicitly acknowledge security risk
- Aligns with security best practices (deny by default)

**Why allow insecure mode at all?**
- Local development without HTTPS certificates
- Integration testing environments
- Must be explicitly enabled (not default)

---

## High Priority Issues

### HIGH-1: Cookie Secret Management

#### Problem Statement

**Current Implementation:**
- Single `OBOT_AUTH_PROVIDER_COOKIE_SECRET` shared across all providers
- No rotation mechanism or versioning
- No entropy validation
- Compromise exposes ALL user sessions
- Both Keycloak and Entra sessions use same key

#### Recommended Solution

**Phase 1: Per-Provider Secrets (Immediate)**

1. **Add provider-specific cookie secret support:**

   **File:** `tools/entra-auth-provider/main.go:38,89-93`
   ```go
   type Options struct {
       ClientID                 string `env:"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID"`
       ClientSecret             string `env:"OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET"`
       TenantID                 string `env:"OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID"`
       ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
       PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`

       // Cookie secret - provider-specific or shared fallback
       CookieSecret             string `env:"OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET"`

       // ... rest of fields
   }
   ```

   **File:** `tools/keycloak-auth-provider/main.go:45,85-87`
   ```go
   type Options struct {
       ClientID                 string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID"`
       ClientSecret             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET"`
       KeycloakURL              string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_URL"`
       KeycloakRealm            string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_REALM"`
       ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
       PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`

       // Cookie secret - provider-specific or shared fallback
       CookieSecret             string `env:"OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET"`

       // ... rest of fields
   }
   ```

2. **Add entropy validation:**

   **File:** `tools/auth-providers-common/pkg/secrets/validation.go` (NEW)
   ```go
   package secrets

   import (
       "encoding/base64"
       "fmt"
   )

   const (
       MinSecretBits = 256 // Minimum 256 bits (32 bytes)
       MinSecretBytes = MinSecretBits / 8
   )

   // ValidateCookieSecret ensures the cookie secret has sufficient entropy
   func ValidateCookieSecret(base64Secret string) error {
       decoded, err := base64.StdEncoding.DecodeString(base64Secret)
       if err != nil {
           return fmt.Errorf("cookie secret must be valid base64: %w", err)
       }

       if len(decoded) < MinSecretBytes {
           return fmt.Errorf("cookie secret must be at least %d bytes (%d bits), got %d bytes",
               MinSecretBytes, MinSecretBits, len(decoded))
       }

       return nil
   }

   // GenerateCookieSecret generates a cryptographically secure cookie secret
   func GenerateCookieSecret() (string, error) {
       secret := make([]byte, MinSecretBytes)
       if _, err := rand.Read(secret); err != nil {
           return "", fmt.Errorf("failed to generate random secret: %w", err)
       }
       return base64.StdEncoding.EncodeToString(secret), nil
   }
   ```

3. **Update providers to use validation:**

   **Both providers:**
   ```go
   // Validate and decode cookie secret
   if err := secrets.ValidateCookieSecret(opts.CookieSecret); err != nil {
       fmt.Printf("ERROR: %s: %v\n", providerName, err)
       fmt.Printf("Generate a valid secret with: openssl rand -base64 32\n")
       os.Exit(1)
   }

   cookieSecret, err := base64.StdEncoding.DecodeString(opts.CookieSecret)
   if err != nil {
       fmt.Printf("ERROR: %s: failed to decode cookie secret: %v\n", providerName, err)
       os.Exit(1)
   }
   ```

**Phase 2: Secret Rotation (Future)**

1. **Support multiple secrets with versioning:**

   ```go
   type Options struct {
       // Current cookie secret (required)
       CookieSecret string `env:"OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET,OBOT_AUTH_PROVIDER_COOKIE_SECRET"`

       // Previous cookie secrets for rotation (optional)
       // Comma-separated list of base64-encoded secrets
       PreviousCookieSecrets string `env:"OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS" optional:"true"`
   }
   ```

2. **Implement multi-secret validation:**

   ```go
   // Configure cookie secrets with rotation support
   allSecrets := []string{string(cookieSecret)}

   if opts.PreviousCookieSecrets != "" {
       prevSecrets := strings.Split(opts.PreviousCookieSecrets, ",")
       for _, secret := range prevSecrets {
           decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(secret))
           if err != nil {
               fmt.Printf("WARNING: skipping invalid previous secret: %v\n", err)
               continue
           }
           allSecrets = append(allSecrets, string(decoded))
       }
   }

   oauthProxyOpts.Cookie.Secret = strings.Join(allSecrets, ",")
   ```

3. **Document rotation procedure:**

   **File:** `docs/COOKIE_SECRET_ROTATION.md` (NEW)
   ```markdown
   # Cookie Secret Rotation Guide

   ## Step 1: Generate New Secret
   ```bash
   NEW_SECRET=$(openssl rand -base64 32)
   echo "New secret: $NEW_SECRET"
   ```

   ## Step 2: Add New Secret to Previous Secrets List
   Update environment variables:
   ```bash
   OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS="$OLD_SECRET"
   OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET="$NEW_SECRET"
   ```

   ## Step 3: Deploy Updated Configuration
   - Helm: Update values.yaml and upgrade release
   - Docker: Update environment variables and restart containers

   ## Step 4: Grace Period (7 days recommended)
   - Keep old secret in previous secrets list
   - Allows existing sessions to remain valid during rotation

   ## Step 5: Remove Old Secret
   After grace period, remove old secret from previous secrets list.
   ```

#### Implementation Priority

**Immediate (This Sprint):**
- [ ] Add per-provider cookie secret support (Phase 1, step 1)
- [ ] Add entropy validation (Phase 1, step 2-3)
- [ ] Update documentation with secret generation instructions

**Future (1-2 Months):**
- [ ] Implement secret rotation with versioning (Phase 2)
- [ ] Create rotation documentation and runbook
- [ ] Add monitoring for secret age/rotation compliance

---

### HIGH-2: PostgreSQL Session Storage Configuration

#### Problem Statement

**Current Issues:**
- No connection validation on startup
- No connection pooling configuration exposed
- No health checks or reconnection logic
- Falls back to cookie-only sessions silently if connection fails
- No visibility into session storage mode

#### Recommended Solution

**Phase 1: Connection Validation (Immediate)**

1. **Create database helper package:**

   **File:** `tools/auth-providers-common/pkg/database/postgres.go` (NEW)
   ```go
   package database

   import (
       "context"
       "database/sql"
       "fmt"
       "time"

       _ "github.com/lib/pq" // PostgreSQL driver
   )

   // ValidatePostgresConnection tests PostgreSQL connectivity
   func ValidatePostgresConnection(dsn string) error {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()

       db, err := sql.Open("postgres", dsn)
       if err != nil {
           return fmt.Errorf("failed to open database connection: %w", err)
       }
       defer db.Close()

       if err := db.PingContext(ctx); err != nil {
           return fmt.Errorf("failed to ping database: %w", err)
       }

       return nil
   }

   // GetSessionStorageHealth checks session storage health
   func GetSessionStorageHealth(dsn, tablePrefix string) error {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()

       db, err := sql.Open("postgres", dsn)
       if err != nil {
           return fmt.Errorf("failed to open database connection: %w", err)
       }
       defer db.Close()

       // Check if session table exists
       tableName := tablePrefix + "session_state"
       query := `SELECT EXISTS (
           SELECT FROM information_schema.tables
           WHERE table_name = $1
       )`

       var exists bool
       err = db.QueryRowContext(ctx, query, tableName).Scan(&exists)
       if err != nil {
           return fmt.Errorf("failed to check session table: %w", err)
       }

       if !exists {
           return fmt.Errorf("session table '%s' does not exist", tableName)
       }

       return nil
   }
   ```

2. **Update auth providers to validate connection:**

   **File:** `tools/entra-auth-provider/main.go:124-128` (UPDATED)
   ```go
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
   ```

   Apply similar changes to `tools/keycloak-auth-provider/main.go:144-148`

3. **Add go.mod dependency:**

   **File:** `tools/auth-providers-common/go.mod`
   ```go
   require (
       github.com/lib/pq v1.10.9
       // ... existing dependencies
   )
   ```

**Phase 2: Connection Pooling (Future)**

1. **Add connection pool configuration:**

   **Environment variables:**
   - `OBOT_AUTH_PROVIDER_POSTGRES_MAX_CONNS` (default: 10)
   - `OBOT_AUTH_PROVIDER_POSTGRES_MAX_IDLE_CONNS` (default: 2)
   - `OBOT_AUTH_PROVIDER_POSTGRES_CONN_MAX_LIFETIME` (default: 1h)

2. **Configure oauth2-proxy session options:**
   ```go
   if opts.PostgresConnectionDSN != "" {
       oauthProxyOpts.Session.Postgres.MaxConns = opts.PostgresMaxConns
       oauthProxyOpts.Session.Postgres.MaxIdleConns = opts.PostgresMaxIdleConns
       oauthProxyOpts.Session.Postgres.ConnMaxLifetime = opts.PostgresConnMaxLifetime
   }
   ```

**Phase 3: Health Checks (Future)**

1. **Add periodic health check:**
   ```go
   // Start background health check goroutine
   if opts.PostgresConnectionDSN != "" {
       go func() {
           ticker := time.NewTicker(1 * time.Minute)
           defer ticker.Stop()

           for range ticker.C {
               if err := database.GetSessionStorageHealth(opts.PostgresConnectionDSN, "entra_"); err != nil {
                   log.Errorf("session storage health check failed: %v", err)
                   // Increment metric: session_storage_health_check_failures
               }
           }
       }()
   }
   ```

#### Implementation Priority

**Immediate (This Sprint):**
- [ ] Create database helper package with connection validation
- [ ] Update both providers to validate PostgreSQL on startup
- [ ] Add clear logging for session storage mode (PostgreSQL vs cookie)
- [ ] Add lib/pq dependency to go.mod

**Future (1-2 Months):**
- [ ] Add connection pooling configuration (Phase 2)
- [ ] Implement periodic health checks (Phase 3)
- [ ] Add metrics for session storage health

---

### HIGH-3: No Monitoring or Alerting for Token Refresh

#### Problem Statement

**Current State:**
- Zero observability into token refresh success/failure rates
- OAuth misconfigurations go unnoticed until users report issues
- No early warning for token refresh problems
- Difficult to diagnose production authentication issues

#### Recommended Solution

**Phase 1: Basic Prometheus Metrics (Immediate)**

1. **Create metrics package:**

   **File:** `tools/auth-providers-common/pkg/metrics/metrics.go` (NEW)
   ```go
   package metrics

   import (
       "github.com/prometheus/client_golang/prometheus"
       "github.com/prometheus/client_golang/prometheus/promauto"
   )

   var (
       // TokenRefreshAttempts counts token refresh attempts by provider and result
       TokenRefreshAttempts = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "obot_auth_token_refresh_attempts_total",
               Help: "Total number of token refresh attempts",
           },
           []string{"provider", "result"}, // result: success|failure
       )

       // TokenRefreshDuration tracks token refresh latency
       TokenRefreshDuration = promauto.NewHistogramVec(
           prometheus.HistogramOpts{
               Name: "obot_auth_token_refresh_duration_seconds",
               Help: "Token refresh operation duration",
               Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
           },
           []string{"provider"},
       )

       // AuthenticationAttempts counts authentication attempts
       AuthenticationAttempts = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "obot_auth_authentication_attempts_total",
               Help: "Total number of authentication attempts",
           },
           []string{"provider", "result"}, // result: success|failure|invalid_session
       )

       // SessionStorageErrors counts session storage errors
       SessionStorageErrors = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "obot_auth_session_storage_errors_total",
               Help: "Total number of session storage errors",
           },
           []string{"provider", "operation"}, // operation: save|load|delete
       )

       // CookieDecryptionErrors counts cookie decryption failures
       CookieDecryptionErrors = promauto.NewCounterVec(
           prometheus.CounterOpts{
               Name: "obot_auth_cookie_decryption_errors_total",
               Help: "Total number of cookie decryption errors",
           },
           []string{"provider"},
       )
   )
   ```

2. **Instrument token refresh in state.go:**

   **File:** `tools/auth-providers-common/pkg/state/state.go:104-112` (UPDATED)
   ```go
   func GetSerializableState(p *oauth2proxy.OAuthProxy, r *http.Request, providerName string) (SerializableState, error) {
       state, err := p.LoadCookiedSession(r)
       if err != nil {
           metrics.SessionStorageErrors.WithLabelValues(providerName, "load").Inc()
           return SerializableState{}, fmt.Errorf("failed to load cookied session: %v", err)
       }

       if state == nil {
           return SerializableState{}, fmt.Errorf("state is nil")
       }

       var setCookies []string
       if state.IsExpired() || (p.CookieOptions.Refresh != 0 && state.Age() > p.CookieOptions.Refresh) {
           // Record token refresh attempt
           start := time.Now()

           setCookies, err = refreshToken(p, r)

           duration := time.Since(start).Seconds()
           metrics.TokenRefreshDuration.WithLabelValues(providerName).Observe(duration)

           if err != nil {
               metrics.TokenRefreshAttempts.WithLabelValues(providerName, "failure").Inc()
               return SerializableState{}, fmt.Errorf("failed to refresh token: %v", err)
           }

           metrics.TokenRefreshAttempts.WithLabelValues(providerName, "success").Inc()
       }

       return SerializableState{
           ExpiresOn:         state.ExpiresOn,
           AccessToken:       state.AccessToken,
           IDToken:           state.IDToken,
           PreferredUsername: state.PreferredUsername,
           User:              state.User,
           Email:             state.Email,
           Groups:            state.Groups,
           GroupInfos:        GroupInfoList{},
           SetCookies:        setCookies,
       }, nil
   }
   ```

3. **Expose Prometheus metrics endpoint:**

   **File:** `tools/entra-auth-provider/main.go` (UPDATED)
   ```go
   import (
       "github.com/prometheus/client_golang/prometheus/promhttp"
   )

   // Setup HTTP routes
   mux := http.NewServeMux()

   // Root endpoint - returns daemon address (required by obot)
   mux.HandleFunc("/{$}", func(w http.ResponseWriter, _ *http.Request) {
       _, _ = w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
   })

   // Metrics endpoint
   mux.Handle("/metrics", promhttp.Handler())

   // State endpoint - returns auth state with token refresh support
   mux.HandleFunc("/obot-get-state", getState(oauthProxy, allowedGroups, allowedTenantSet, groupCacheTTL))

   // ... rest of endpoints
   ```

   Apply similar changes to Keycloak provider.

**Phase 2: Structured Logging (Immediate)**

1. **Add structured logging for token refresh:**

   **File:** `tools/auth-providers-common/pkg/state/state.go:124-149` (UPDATED)
   ```go
   func refreshToken(p *oauth2proxy.OAuthProxy, r *http.Request, providerName, userID string) ([]string, error) {
       w := &response{
           headers: make(http.Header),
       }

       req, err := http.NewRequest(r.Method, "/oauth2/auth", nil)
       if err != nil {
           return nil, fmt.Errorf("failed to create refresh request object: %v", err)
       }

       req.Header = r.Header
       p.ServeHTTP(w, req)

       switch w.status {
       case http.StatusOK, http.StatusAccepted:
           var headers []string
           for _, v := range w.Header().Values("Set-Cookie") {
               headers = append(headers, v)
           }

           log.WithFields(log.Fields{
               "provider": providerName,
               "user_id": userID,
               "status": w.status,
           }).Info("token refresh successful")

           return headers, nil

       case http.StatusUnauthorized, http.StatusForbidden:
           log.WithFields(log.Fields{
               "provider": providerName,
               "user_id": userID,
               "status": w.status,
               "error": string(w.body),
           }).Warn("token refresh failed - session invalid")

           return nil, fmt.Errorf("refreshing token returned %d: %s", w.status, w.body)

       default:
           log.WithFields(log.Fields{
               "provider": providerName,
               "user_id": userID,
               "status": w.status,
               "error": string(w.body),
           }).Error("token refresh failed - unexpected status")

           return nil, fmt.Errorf("refreshing token returned unexpected status %d: %s", w.status, w.body)
       }
   }
   ```

**Phase 3: Alerting Rules (Future)**

1. **Create Prometheus alert rules:**

   **File:** `chart/templates/prometheus-rules.yaml` (NEW)
   ```yaml
   apiVersion: monitoring.coreos.com/v1
   kind: PrometheusRule
   metadata:
     name: obot-auth-alerts
     namespace: {{ .Release.Namespace }}
   spec:
     groups:
       - name: obot-authentication
         interval: 30s
         rules:
           # Alert on high token refresh failure rate
           - alert: HighTokenRefreshFailureRate
             expr: |
               (
                 sum(rate(obot_auth_token_refresh_attempts_total{result="failure"}[5m])) by (provider)
                 /
                 sum(rate(obot_auth_token_refresh_attempts_total[5m])) by (provider)
               ) > 0.05
             for: 5m
             labels:
               severity: critical
               component: authentication
             annotations:
               summary: "High token refresh failure rate for {{ $labels.provider }}"
               description: "Token refresh failure rate is {{ $value | humanizePercentage }} (threshold: 5%)"

           # Alert on session storage errors
           - alert: SessionStorageErrors
             expr: |
               rate(obot_auth_session_storage_errors_total[5m]) > 0
             for: 2m
             labels:
               severity: warning
               component: authentication
             annotations:
               summary: "Session storage errors detected for {{ $labels.provider }}"
               description: "{{ $value }} session storage errors per second"

           # Alert on high authentication failure rate
           - alert: HighAuthenticationFailureRate
             expr: |
               (
                 sum(rate(obot_auth_authentication_attempts_total{result="failure"}[5m])) by (provider)
                 /
                 sum(rate(obot_auth_authentication_attempts_total[5m])) by (provider)
               ) > 0.10
             for: 5m
             labels:
               severity: warning
               component: authentication
             annotations:
               summary: "High authentication failure rate for {{ $labels.provider }}"
               description: "Authentication failure rate is {{ $value | humanizePercentage }} (threshold: 10%)"
   ```

#### Implementation Priority

**Immediate (This Sprint):**
- [ ] Create metrics package with Prometheus metrics
- [ ] Instrument token refresh in state.go
- [ ] Expose /metrics endpoint in both providers
- [ ] Add structured logging for token refresh events

**Future (1-2 Months):**
- [ ] Create Prometheus alerting rules (Phase 3)
- [ ] Set up Grafana dashboards for auth metrics
- [ ] Implement alerting integrations (Slack, PagerDuty, etc.)

---

### HIGH-4: No Circuit Breaker for Token Refresh Failures

#### Problem Statement

**Current Behavior:**
- Every request attempts token refresh, even if OAuth provider is down
- No backoff or graceful degradation
- Thundering herd problem during outages

#### Recommended Solution

**Implementation:**

1. **Create circuit breaker package:**

   **File:** `tools/auth-providers-common/pkg/circuitbreaker/circuitbreaker.go` (NEW)
   ```go
   package circuitbreaker

   import (
       "fmt"
       "sync"
       "time"
   )

   type State string

   const (
       StateClosed   State = "closed"   // Normal operation
       StateOpen     State = "open"     // Circuit breaker triggered
       StateHalfOpen State = "half_open" // Testing if service recovered
   )

   type CircuitBreaker struct {
       mu              sync.RWMutex
       state           State
       failures        int
       lastAttempt     time.Time
       lastStateChange time.Time

       // Configuration
       maxFailures     int           // Max failures before opening circuit
       resetTimeout    time.Duration // Time to wait before trying half-open
       halfOpenTimeout time.Duration // Time to wait in half-open before closing
   }

   func NewCircuitBreaker(maxFailures int, resetTimeout, halfOpenTimeout time.Duration) *CircuitBreaker {
       return &CircuitBreaker{
           state:           StateClosed,
           maxFailures:     maxFailures,
           resetTimeout:    resetTimeout,
           halfOpenTimeout: halfOpenTimeout,
           lastStateChange: time.Now(),
       }
   }

   // AllowRequest returns true if a request should be attempted
   func (cb *CircuitBreaker) AllowRequest() error {
       cb.mu.RLock()
       defer cb.mu.RUnlock()

       switch cb.state {
       case StateClosed:
           return nil // Always allow in closed state

       case StateOpen:
           // Check if enough time has passed to try half-open
           if time.Since(cb.lastStateChange) >= cb.resetTimeout {
               // Transition to half-open
               go cb.transitionToHalfOpen()
               return nil
           }
           return fmt.Errorf("circuit breaker is open (failures: %d)", cb.failures)

       case StateHalfOpen:
           // Allow one request to test
           return nil

       default:
           return fmt.Errorf("unknown circuit breaker state: %s", cb.state)
       }
   }

   // RecordSuccess records a successful operation
   func (cb *CircuitBreaker) RecordSuccess() {
       cb.mu.Lock()
       defer cb.mu.Unlock()

       if cb.state == StateHalfOpen {
           // Success in half-open state, transition to closed
           cb.state = StateClosed
           cb.failures = 0
           cb.lastStateChange = time.Now()
       }

       // Reset failure count in closed state
       if cb.state == StateClosed {
           cb.failures = 0
       }
   }

   // RecordFailure records a failed operation
   func (cb *CircuitBreaker) RecordFailure() {
       cb.mu.Lock()
       defer cb.mu.Unlock()

       cb.failures++
       cb.lastAttempt = time.Now()

       if cb.state == StateHalfOpen {
           // Failure in half-open state, transition back to open
           cb.state = StateOpen
           cb.lastStateChange = time.Now()
           return
       }

       if cb.failures >= cb.maxFailures {
           // Too many failures, open the circuit
           cb.state = StateOpen
           cb.lastStateChange = time.Now()
       }
   }

   // GetState returns the current circuit breaker state
   func (cb *CircuitBreaker) GetState() State {
       cb.mu.RLock()
       defer cb.mu.RUnlock()
       return cb.state
   }

   func (cb *CircuitBreaker) transitionToHalfOpen() {
       cb.mu.Lock()
       defer cb.mu.Unlock()

       if cb.state == StateOpen {
           cb.state = StateHalfOpen
           cb.lastStateChange = time.Now()
       }
   }
   ```

2. **Integrate circuit breaker in state.go:**

   **File:** `tools/auth-providers-common/pkg/state/state.go` (UPDATED)
   ```go
   var (
       // Circuit breakers for token refresh (per-provider)
       refreshCircuitBreakers = make(map[string]*circuitbreaker.CircuitBreaker)
       cbMutex                sync.RWMutex
   )

   func getRefreshCircuitBreaker(providerName string) *circuitbreaker.CircuitBreaker {
       cbMutex.RLock()
       cb, exists := refreshCircuitBreakers[providerName]
       cbMutex.RUnlock()

       if exists {
           return cb
       }

       cbMutex.Lock()
       defer cbMutex.Unlock()

       // Double-check after acquiring write lock
       if cb, exists := refreshCircuitBreakers[providerName]; exists {
           return cb
       }

       // Create new circuit breaker
       // Max 5 failures, 30s reset timeout, 10s half-open timeout
       cb = circuitbreaker.NewCircuitBreaker(5, 30*time.Second, 10*time.Second)
       refreshCircuitBreakers[providerName] = cb

       return cb
   }

   func GetSerializableState(p *oauth2proxy.OAuthProxy, r *http.Request, providerName string) (SerializableState, error) {
       state, err := p.LoadCookiedSession(r)
       if err != nil {
           return SerializableState{}, fmt.Errorf("failed to load cookied session: %v", err)
       }

       if state == nil {
           return SerializableState{}, fmt.Errorf("state is nil")
       }

       var setCookies []string
       if state.IsExpired() || (p.CookieOptions.Refresh != 0 && state.Age() > p.CookieOptions.Refresh) {
           cb := getRefreshCircuitBreaker(providerName)

           // Check circuit breaker
           if err := cb.AllowRequest(); err != nil {
               // Circuit breaker is open, allow stale session temporarily
               log.WithFields(log.Fields{
                   "provider": providerName,
                   "state": cb.GetState(),
                   "error": err.Error(),
               }).Warn("token refresh circuit breaker open, allowing stale session")

               // Return stale session if not too old (< 5 minutes past expiry)
               if !state.IsExpired() || time.Since(*state.ExpiresOn) < 5*time.Minute {
                   return SerializableState{
                       ExpiresOn:         state.ExpiresOn,
                       AccessToken:       state.AccessToken,
                       IDToken:           state.IDToken,
                       PreferredUsername: state.PreferredUsername,
                       User:              state.User,
                       Email:             state.Email,
                       Groups:            state.Groups,
                       GroupInfos:        GroupInfoList{},
                       SetCookies:        []string{},
                   }, nil
               }

               // Session too old, must fail
               return SerializableState{}, fmt.Errorf("session expired and refresh unavailable: %w", err)
           }

           // Attempt token refresh
           start := time.Now()
           setCookies, err = refreshToken(p, r)
           duration := time.Since(start).Seconds()

           metrics.TokenRefreshDuration.WithLabelValues(providerName).Observe(duration)

           if err != nil {
               cb.RecordFailure()
               metrics.TokenRefreshAttempts.WithLabelValues(providerName, "failure").Inc()
               return SerializableState{}, fmt.Errorf("failed to refresh token: %v", err)
           }

           cb.RecordSuccess()
           metrics.TokenRefreshAttempts.WithLabelValues(providerName, "success").Inc()
       }

       return SerializableState{
           ExpiresOn:         state.ExpiresOn,
           AccessToken:       state.AccessToken,
           IDToken:           state.IDToken,
           PreferredUsername: state.PreferredUsername,
           User:              state.User,
           Email:             state.Email,
           Groups:            state.Groups,
           GroupInfos:        GroupInfoList{},
           SetCookies:        setCookies,
       }, nil
   }
   ```

3. **Add circuit breaker state metric:**

   **File:** `tools/auth-providers-common/pkg/metrics/metrics.go` (UPDATED)
   ```go
   var (
       // CircuitBreakerState tracks circuit breaker state
       CircuitBreakerState = promauto.NewGaugeVec(
           prometheus.GaugeOpts{
               Name: "obot_auth_circuit_breaker_state",
               Help: "Circuit breaker state (0=closed, 1=open, 2=half_open)",
           },
           []string{"provider", "operation"}, // operation: token_refresh
       )
   )

   // Helper function to update circuit breaker state metric
   func UpdateCircuitBreakerState(provider, operation string, state circuitbreaker.State) {
       var value float64
       switch state {
       case circuitbreaker.StateClosed:
           value = 0
       case circuitbreaker.StateOpen:
           value = 1
       case circuitbreaker.StateHalfOpen:
           value = 2
       }
       CircuitBreakerState.WithLabelValues(provider, operation).Set(value)
   }
   ```

#### Implementation Priority

**Future (1-2 Months):**
- [ ] Create circuit breaker package
- [ ] Integrate circuit breaker in token refresh flow
- [ ] Add circuit breaker state metric
- [ ] Test with simulated OAuth provider outages
- [ ] Document circuit breaker behavior

---

### HIGH-5: Cookie Domain and Path Configuration

#### Problem Statement

**Current Issues:**
- Cookie domain and path are not explicitly configured
- Cookies may be sent to unintended subdomains
- CSRF vulnerabilities if SameSite not set
- Potential cookie leakage to unrelated paths

#### Recommended Solution

1. **Parse and validate server URL:**

   **File:** `tools/entra-auth-provider/main.go:130-145` (UPDATED)
   ```go
   // Parse server URL for cookie configuration
   parsedURL, err := url.Parse(opts.ObotServerURL)
   if err != nil {
       fmt.Printf("ERROR: entra-auth-provider: invalid OBOT_SERVER_PUBLIC_URL: %v\n", err)
       os.Exit(1)
   }

   // Cookie configuration
   oauthProxyOpts.Cookie.Refresh = refreshDuration
   oauthProxyOpts.Cookie.Name = "obot_access_token"
   oauthProxyOpts.Cookie.Secret = string(cookieSecret)
   oauthProxyOpts.Cookie.Secure = parsedURL.Scheme == "https"
   oauthProxyOpts.Cookie.HTTPOnly = true
   oauthProxyOpts.Cookie.SameSite = "Lax" // Prevents CSRF while allowing OAuth redirects
   oauthProxyOpts.Cookie.CSRFExpire = 30 * time.Minute

   // Set cookie domain and path explicitly
   oauthProxyOpts.Cookie.Domain = parsedURL.Hostname()
   oauthProxyOpts.Cookie.Path = "/"

   // Allow environment variable overrides for advanced configurations
   if cookieDomain := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_DOMAIN"); cookieDomain != "" {
       oauthProxyOpts.Cookie.Domain = cookieDomain
   }

   if cookiePath := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_PATH"); cookiePath != "" {
       oauthProxyOpts.Cookie.Path = cookiePath
   }

   if sameSite := os.Getenv("OBOT_AUTH_PROVIDER_COOKIE_SAMESITE"); sameSite != "" {
       oauthProxyOpts.Cookie.SameSite = sameSite
   }

   fmt.Printf("INFO: entra-auth-provider: cookie configuration:\n")
   fmt.Printf("  - Name: %s\n", oauthProxyOpts.Cookie.Name)
   fmt.Printf("  - Domain: %s\n", oauthProxyOpts.Cookie.Domain)
   fmt.Printf("  - Path: %s\n", oauthProxyOpts.Cookie.Path)
   fmt.Printf("  - Secure: %v\n", oauthProxyOpts.Cookie.Secure)
   fmt.Printf("  - HTTPOnly: %v\n", oauthProxyOpts.Cookie.HTTPOnly)
   fmt.Printf("  - SameSite: %s\n", oauthProxyOpts.Cookie.SameSite)
   ```

   Apply similar changes to Keycloak provider.

2. **Update documentation:**

   **File:** `tools/README.md` (UPDATED)
   ```markdown
   ## Cookie Security Configuration

   ### Required Environment Variables

   - `OBOT_SERVER_PUBLIC_URL` - Full URL with protocol (https://obot.example.com)
     - Used to automatically set cookie domain and Secure flag

   ### Optional Environment Variables

   - `OBOT_AUTH_PROVIDER_COOKIE_DOMAIN` (optional)
     - Override cookie domain (default: hostname from OBOT_SERVER_PUBLIC_URL)
     - Example: `.example.com` (allows cookies on all subdomains)

   - `OBOT_AUTH_PROVIDER_COOKIE_PATH` (optional, default: `/`)
     - Cookie path restriction
     - Example: `/obot` (only send cookies for /obot/* paths)

   - `OBOT_AUTH_PROVIDER_COOKIE_SAMESITE` (optional, default: `Lax`)
     - SameSite cookie attribute
     - Options: `Strict`, `Lax`, `None`
     - `Lax` recommended for OAuth flows (prevents CSRF while allowing redirects)
     - `Strict` for highest security (may break OAuth flows)
   ```

#### Implementation Priority

**Immediate (This Sprint):**
- [ ] Parse server URL and set cookie domain/path explicitly
- [ ] Set HTTPOnly and SameSite flags
- [ ] Add environment variable overrides
- [ ] Add logging for cookie configuration
- [ ] Update documentation

---

## Integration Testing Requirements

### Current Test Coverage Gaps

**Existing Tests:**
- `tools/entra-auth-provider/groups_test.go` - Admin credential selection (unit tests)
- `tools/keycloak-auth-provider/groups_test.go` - Admin credential selection (unit tests)
- `tools/auth-providers-common/pkg/state/state_test.go` - State serialization (unit tests)

**Missing Integration Tests (10 scenarios):**
1. OAuth2 authorization code flow end-to-end
2. Token refresh mechanism with actual OAuth provider
3. Session persistence with PostgreSQL
4. Cookie security (Secure flag, SameSite, HTTPOnly)
5. Error handling for token refresh failures
6. Multi-user concurrent session handling
7. Admin role persistence after re-login (regression test for commit 1e7fb26c)
8. ID token parsing error scenarios
9. Session storage failover (PostgreSQL → cookie fallback)
10. Clock skew handling for token expiry

### Recommended Integration Test Suite

#### Phase 1: Core Authentication Flow (Immediate)

**File:** `tests/integration/auth_flow_test.go` (NEW)

```go
package integration_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Authentication Flow Integration Tests", func() {
    var (
        keycloakClient *KeycloakTestClient
        entraClient    *EntraTestClient
    )

    BeforeEach(func() {
        // Initialize test clients with mock OAuth providers
        keycloakClient = NewKeycloakTestClient()
        entraClient = NewEntraTestClient()
    })

    Context("OAuth2 Authorization Code Flow", func() {
        It("should complete full OAuth2 flow with Keycloak", func() {
            // 1. Initiate OAuth2 flow
            authURL, state := keycloakClient.StartAuthFlow()
            Expect(authURL).To(ContainSubstring("/oauth2/start"))
            Expect(state).NotTo(BeEmpty())

            // 2. Simulate user login and consent
            authCode := keycloakClient.SimulateUserLogin("testuser@example.com", "password")
            Expect(authCode).NotTo(BeEmpty())

            // 3. Exchange auth code for tokens
            callback := keycloakClient.HandleCallback(authCode, state)
            Expect(callback.StatusCode).To(Equal(302)) // Redirect

            // 4. Verify session cookie is set
            cookies := callback.Cookies()
            accessToken := findCookie(cookies, "obot_access_token")
            Expect(accessToken).NotTo(BeNil())
            Expect(accessToken.Secure).To(BeTrue())
            Expect(accessToken.HttpOnly).To(BeTrue())
        })

        It("should complete full OAuth2 flow with Entra ID", func() {
            // Similar test for Entra ID
            authURL, state := entraClient.StartAuthFlow()
            Expect(authURL).To(ContainSubstring("/oauth2/start"))

            authCode := entraClient.SimulateUserLogin("testuser@example.com", "password")
            callback := entraClient.HandleCallback(authCode, state)

            Expect(callback.StatusCode).To(Equal(302))
            cookies := callback.Cookies()
            accessToken := findCookie(cookies, "obot_access_token")
            Expect(accessToken).NotTo(BeNil())
        })
    })

    Context("Token Refresh", func() {
        It("should automatically refresh expired tokens", func() {
            // 1. Establish authenticated session
            session := keycloakClient.CreateAuthenticatedSession("testuser@example.com")

            // 2. Fast-forward time past refresh interval
            session.FastForward(65 * time.Minute) // Past 1 hour default

            // 3. Make authenticated request
            resp, err := session.Get("/api/test")
            Expect(err).NotTo(HaveOccurred())

            // 4. Verify token was refreshed
            Expect(resp.StatusCode).To(Equal(200))
            Expect(session.TokenRefreshCount()).To(Equal(1))

            // 5. Verify new cookies were set
            cookies := resp.Cookies()
            Expect(cookies).NotTo(BeEmpty())
        })

        It("should return ErrInvalidSession on refresh failure", func() {
            // 1. Establish authenticated session
            session := keycloakClient.CreateAuthenticatedSession("testuser@example.com")

            // 2. Invalidate refresh token on OAuth provider side
            keycloakClient.RevokeRefreshToken(session.UserID())

            // 3. Fast-forward time past refresh interval
            session.FastForward(65 * time.Minute)

            // 4. Make authenticated request
            resp, err := session.Get("/api/test")
            Expect(err).NotTo(HaveOccurred())

            // 5. Verify redirect to login (not 500 error)
            Expect(resp.StatusCode).To(Equal(302))
            Expect(resp.Header.Get("Location")).To(ContainSubstring("/oauth2/start"))
        })

        It("should update cookies after successful refresh", func() {
            session := keycloakClient.CreateAuthenticatedSession("testuser@example.com")

            oldCookie := session.GetCookie("obot_access_token")

            session.FastForward(65 * time.Minute)
            resp, _ := session.Get("/api/test")

            newCookie := findCookie(resp.Cookies(), "obot_access_token")
            Expect(newCookie).NotTo(BeNil())
            Expect(newCookie.Value).NotTo(Equal(oldCookie.Value))
        })
    })
})
```

#### Phase 2: Session Persistence (Future)

**File:** `tests/integration/session_storage_test.go` (NEW)

```go
package integration_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Session Persistence Integration Tests", func() {
    Context("PostgreSQL Session Storage", func() {
        It("should persist sessions to PostgreSQL when configured", func() {
            // Test session storage in database
        })

        It("should fall back to cookie-only if PostgreSQL unavailable", func() {
            // Test graceful degradation
        })

        It("should maintain separate sessions for different providers", func() {
            // Test keycloak_ and entra_ table prefixes
        })
    })
})
```

#### Phase 3: Security and Regression Tests (Future)

**File:** `tests/integration/security_test.go` (NEW)

```go
package integration_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Security Integration Tests", func() {
    Context("Admin Role Persistence (Regression)", func() {
        It("should preserve admin role after re-login (Keycloak)", func() {
            // Regression test for commit 1e7fb26c
            user := keycloakClient.CreateUser("admin@example.com")
            keycloakClient.AssignRole(user.ID, "admin")

            // Initial login
            session1 := keycloakClient.Login(user.Email, "password")
            Expect(session1.Roles()).To(ContainElement("admin"))

            // Logout
            session1.Logout()

            // Re-login
            session2 := keycloakClient.Login(user.Email, "password")

            // Verify admin role persists
            Expect(session2.Roles()).To(ContainElement("admin"))
            Expect(session2.UserID()).To(Equal(session1.UserID()))
        })

        It("should preserve admin role after re-login (Entra ID)", func() {
            // Same test for Entra ID
        })
    })

    Context("ID Token Parsing Errors", func() {
        It("should fail authentication if ID token is missing", func() {
            // Test error handling
        })

        It("should fail authentication if ID token parse fails", func() {
            // Test error handling
        })
    })
})
```

### Test Infrastructure Requirements

1. **Mock OAuth Providers:**
   - Mock Keycloak server for testing
   - Mock Azure AD/Entra ID server for testing
   - Support token generation, validation, and refresh

2. **Test Database:**
   - PostgreSQL test instance (use testcontainers)
   - Automatic schema creation/cleanup

3. **Test Fixtures:**
   - Pre-configured users with various roles
   - Test tokens with different expiry times
   - Sample ID tokens with various claims

### Implementation Priority

**Immediate (This Sprint):**
- [ ] Create integration test framework (Phase 1)
- [ ] Implement OAuth2 flow tests (scenarios 1-2)
- [ ] Implement token refresh tests (scenario 5)
- [ ] Add admin role persistence regression test (scenario 7)

**Future (1-2 Months):**
- [ ] Session persistence tests (Phase 2, scenario 3, 9)
- [ ] Cookie security tests (scenario 4)
- [ ] Multi-user concurrency tests (scenario 6)
- [ ] ID token error handling tests (Phase 3, scenario 8)
- [ ] Clock skew tests (scenario 10)

---

## Token Refresh Configuration

### OAuth2-Proxy Settings Validation

Based on the comprehensive analysis, the following OAuth2-Proxy configurations have been reviewed:

#### ✅ Correctly Configured

1. **Offline Access Scope:**
   - Keycloak: `openid email profile offline_access` ✓
   - Entra ID: `openid email profile offline_access` ✓

2. **PKCE (Proof Key for Code Exchange):**
   - Keycloak: `CodeChallengeMethod = "S256"` ✓
   - Entra ID: Default (uses PKCE automatically for public clients) ✓

3. **Multi-Tenant Issuer Verification:**
   - Entra ID: `InsecureSkipIssuerVerification = true` (for multi-tenant) ✓

4. **Cookie Name:**
   - Both: `obot_access_token` ✓

#### ⚠️ Requires Investigation

1. **Token Lifetimes** (Not validated in code):
   - Keycloak: Check realm settings for access/refresh/session token lifetimes
   - Entra ID: Check app registration and conditional access policies

2. **Cookie Encryption Key Stability:**
   - ✅ Should be stable if `OBOT_AUTH_PROVIDER_COOKIE_SECRET` env var is static
   - ⚠️ No validation that secret hasn't changed (would invalidate all sessions)
   - **Recommendation:** Add cookie secret validation on startup

3. **PostgreSQL Session Storage Connection:**
   - ❌ No health check performed on startup
   - ❌ No reconnection logic
   - **Recommendation:** Implement connection validation (see HIGH-2)

4. **Cookie Security Flags:**
   - ❌ Domain not explicitly configured (relies on defaults)
   - ❌ Path not explicitly configured (assumes "/")
   - ❌ SameSite not explicitly configured (relies on defaults)
   - **Recommendation:** Set explicitly (see HIGH-5)

### Keycloak Configuration Checklist

**Realm Settings:**
- [ ] Access Token Lifespan: 5 minutes (recommended)
- [ ] Refresh Token Max Reuse: 0 (one-time use)
- [ ] SSO Session Idle: 30 minutes
- [ ] SSO Session Max: 10 hours
- [ ] Client Session Idle: 30 minutes
- [ ] Client Session Max: 10 hours
- [ ] Offline Session Idle: 30 days
- [ ] Offline Session Max: 60 days

**Client Configuration:**
- [ ] Access Type: `confidential`
- [ ] Standard Flow Enabled: Yes
- [ ] Direct Access Grants Enabled: No (unless needed)
- [ ] Valid Redirect URIs: `https://obot.example.com/oauth2/callback`
- [ ] Web Origins: `https://obot.example.com`

**Client Scopes:**
- [ ] `openid` (default)
- [ ] `profile` (default)
- [ ] `email` (default)
- [ ] `offline_access` (optional, add to default scopes)
- [ ] `groups` (optional, if using group-based access control)

**Mappers:**
- [ ] Client ID mapper (add client ID to token)
- [ ] Audience mapper (add audience claim)
- [ ] Groups mapper (if using group-based access control)

### Azure Entra ID Configuration Checklist

**App Registration:**
- [ ] Application Type: Web
- [ ] Redirect URI: `https://obot.example.com/oauth2/callback`
- [ ] Supported Account Types: Based on requirements
  - Single tenant: Accounts in this organizational directory only
  - Multi-tenant: Accounts in any organizational directory

**API Permissions:**
- [ ] Microsoft Graph: `User.Read` (Delegated)
- [ ] Microsoft Graph: `GroupMember.Read.All` (Delegated, if using groups)
- [ ] Admin consent granted (if required by org policy)

**Token Configuration:**
- [ ] Access tokens enabled
- [ ] ID tokens enabled
- [ ] Optional claims: `email`, `preferred_username`, `groups` (if needed)

**Certificates & Secrets:**
- [ ] Client secret created and not expired
- [ ] Secret securely stored in environment variables

**Authentication:**
- [ ] Token version: v2.0 (recommended)
- [ ] Allow public client flows: No (for web apps)

### Environment Variable Validation

Create validation script for deployment:

**File:** `scripts/validate-auth-config.sh` (NEW)

```bash
#!/bin/bash
# Validation script for authentication configuration

set -e

echo "=== Obot Authentication Configuration Validation ==="
echo ""

# Check required environment variables
required_vars=(
    "OBOT_SERVER_PUBLIC_URL"
    "OBOT_AUTH_PROVIDER_COOKIE_SECRET"
)

missing_vars=()
for var in "${required_vars[@]}"; do
    if [[ -z "${!var}" ]]; then
        missing_vars+=("$var")
    fi
done

if [[ ${#missing_vars[@]} -gt 0 ]]; then
    echo "ERROR: Missing required environment variables:"
    for var in "${missing_vars[@]}"; do
        echo "  - $var"
    done
    exit 1
fi

# Validate OBOT_SERVER_PUBLIC_URL
if [[ ! "$OBOT_SERVER_PUBLIC_URL" =~ ^https?:// ]]; then
    echo "ERROR: OBOT_SERVER_PUBLIC_URL must start with http:// or https://"
    exit 1
fi

# Validate cookie secret length
secret_length=$(echo -n "$OBOT_AUTH_PROVIDER_COOKIE_SECRET" | base64 -d 2>/dev/null | wc -c)
if [[ $secret_length -lt 32 ]]; then
    echo "ERROR: Cookie secret must be at least 32 bytes (256 bits)"
    echo "  Current length: $secret_length bytes"
    echo "  Generate with: openssl rand -base64 32"
    exit 1
fi

# Check HTTPS enforcement
if [[ "$OBOT_SERVER_PUBLIC_URL" =~ ^http:// ]] && [[ "$OBOT_AUTH_INSECURE_COOKIES" != "true" ]]; then
    echo "ERROR: HTTP URL requires OBOT_AUTH_INSECURE_COOKIES=true"
    echo "  This should ONLY be used for local development"
    exit 1
fi

# Validate PostgreSQL connection if configured
if [[ -n "$OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" ]]; then
    echo "Testing PostgreSQL connection..."
    if ! psql "$OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" -c "SELECT 1" >/dev/null 2>&1; then
        echo "ERROR: Cannot connect to PostgreSQL database"
        exit 1
    fi
    echo "PostgreSQL connection: OK"
fi

echo ""
echo "=== Configuration validation passed ==="
echo ""
echo "Summary:"
echo "  - Server URL: $OBOT_SERVER_PUBLIC_URL"
echo "  - Cookie secret: ✓ (${secret_length} bytes)"
echo "  - Session storage: ${OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN:-cookie-only}"
echo "  - Insecure cookies: ${OBOT_AUTH_INSECURE_COOKIES:-false}"
```

### Monitoring Token Refresh Issues

**Prometheus Queries for Token Refresh:**

```promql
# Token refresh failure rate by provider
(
  sum(rate(obot_auth_token_refresh_attempts_total{result="failure"}[5m])) by (provider)
  /
  sum(rate(obot_auth_token_refresh_attempts_total[5m])) by (provider)
)

# Token refresh latency (p95)
histogram_quantile(0.95,
  sum(rate(obot_auth_token_refresh_duration_seconds_bucket[5m])) by (provider, le)
)

# Authentication failures
rate(obot_auth_authentication_attempts_total{result="failure"}[5m])

# Session storage errors
rate(obot_auth_session_storage_errors_total[5m])
```

**Log Queries (Loki/Elasticsearch):**

```logql
# Keycloak token refresh errors
{app="obot",provider="keycloak"} |= "ERROR" |= "token refresh"

# Entra ID token refresh errors
{app="obot",provider="entra"} |= "ERROR" |= "token refresh"

# All authentication errors
{app="obot"} |= "ERROR" |= "authentication"
```

---

## Implementation Roadmap

### Sprint 1 (Current Sprint) - CRITICAL Fixes

**Week 1:**
- [ ] **[CRITICAL-1]** Fix Entra ID ID token parsing error handling
  - Apply Keycloak fix pattern to Entra ID
  - Add unit tests for error scenarios
  - Update integration tests for regression prevention
  - **Estimated effort:** 4 hours

- [ ] **[CRITICAL-2]** Expand token refresh error handling
  - Create `pkg/proxy/errors.go` with `IsSessionError` function
  - Update `authenticateRequest` to use helper
  - Add structured logging
  - Add integration test for token refresh failures
  - **Estimated effort:** 6 hours

**Week 2:**
- [ ] **[CRITICAL-3]** Add TLS validation for cookie Secure flag
  - Update both providers with URL parsing
  - Add `OBOT_AUTH_INSECURE_COOKIES` support
  - Update tool.gpt metadata
  - Update documentation
  - **Estimated effort:** 4 hours

- [ ] **[HIGH-3]** Add basic Prometheus metrics
  - Create metrics package
  - Instrument token refresh in state.go
  - Expose /metrics endpoint
  - Add structured logging
  - **Estimated effort:** 6 hours

- [ ] **[HIGH-5]** Configure cookie domain, path, and SameSite
  - Parse server URL and set explicitly
  - Add environment variable overrides
  - Add logging for cookie configuration
  - Update documentation
  - **Estimated effort:** 3 hours

- [ ] **[Section 10]** Group metadata security validation
  - Add group description validation (XSS, length, unicode)
  - Verify SQL parameterization for description searches
  - Add group metadata security tests (Test Scenario 0)
  - Add group metadata metrics to observability
  - **Estimated effort:** 3 hours

**Total Sprint 1 effort:** ~26 hours

### Sprint 2 (Next 2 Weeks) - HIGH Priority Issues

**Week 3:**
- [ ] **[HIGH-1]** Implement per-provider cookie secrets (Phase 1)
  - Add provider-specific secret support
  - Create secrets validation package
  - Update both providers to use validation
  - **Estimated effort:** 6 hours

- [ ] **[HIGH-2]** Add PostgreSQL connection validation
  - Create database helper package
  - Update both providers with validation on startup
  - Add clear logging for session storage mode
  - **Estimated effort:** 6 hours

**Week 4:**
- [ ] **Integration Tests** Create auth integration test suite
  - Set up test framework with mock OAuth providers
  - Implement OAuth2 flow tests (scenarios 1-2)
  - Implement token refresh tests (scenario 5)
  - Add admin role persistence regression test (scenario 7)
  - **Estimated effort:** 12 hours

**Total Sprint 2 effort:** ~24 hours

### Month 2-3 - Future Enhancements

**Month 2:**
- [ ] **[HIGH-4]** Implement circuit breaker for token refresh
  - Create circuit breaker package
  - Integrate in token refresh flow
  - Add circuit breaker state metric
  - **Estimated effort:** 8 hours

- [ ] **[HIGH-1]** Cookie secret rotation (Phase 2)
  - Implement multi-secret validation
  - Create rotation documentation
  - **Estimated effort:** 6 hours

- [ ] **[HIGH-2]** PostgreSQL connection pooling (Phase 2)
  - Add connection pool configuration
  - Configure oauth2-proxy session options
  - **Estimated effort:** 4 hours

**Month 3:**
- [ ] **[HIGH-3]** Comprehensive observability
  - Create Prometheus alerting rules
  - Set up Grafana dashboards
  - Implement alerting integrations
  - **Estimated effort:** 8 hours

- [ ] **Integration Tests** Complete test suite
  - Session persistence tests (scenarios 3, 9)
  - Cookie security tests (scenario 4)
  - Multi-user concurrency tests (scenario 6)
  - ID token error handling tests (scenario 8)
  - Clock skew tests (scenario 10)
  - **Estimated effort:** 16 hours

**Total Months 2-3 effort:** ~42 hours

---

## Code Examples and Fixes

All critical code fixes are provided inline in the relevant sections above. Key files modified:

### Critical Fixes
1. `tools/entra-auth-provider/main.go:306-328` (CRITICAL-1)
2. `pkg/proxy/proxy.go:283-286` + `pkg/proxy/errors.go` (NEW) (CRITICAL-2)
3. `tools/entra-auth-provider/main.go:130-145` + `tools/keycloak-auth-provider/main.go:150-165` (CRITICAL-3)

### High Priority Fixes
1. `tools/entra-auth-provider/main.go:38,89-93` + `tools/auth-providers-common/pkg/secrets/validation.go` (NEW) (HIGH-1)
2. `tools/auth-providers-common/pkg/database/postgres.go` (NEW) + provider main.go files (HIGH-2)
3. `tools/auth-providers-common/pkg/metrics/metrics.go` (NEW) + state.go instrumentation (HIGH-3)
4. `tools/auth-providers-common/pkg/circuitbreaker/circuitbreaker.go` (NEW) (HIGH-4)
5. Cookie configuration updates in both provider main.go files (HIGH-5)

---

## Monitoring and Observability

### Prometheus Metrics

**Metrics to implement:**
1. `obot_auth_token_refresh_attempts_total` - Token refresh attempts by provider and result
2. `obot_auth_token_refresh_duration_seconds` - Token refresh latency histogram
3. `obot_auth_authentication_attempts_total` - Authentication attempts by provider and result
4. `obot_auth_session_storage_errors_total` - Session storage errors by provider and operation
5. `obot_auth_cookie_decryption_errors_total` - Cookie decryption failures
6. `obot_auth_circuit_breaker_state` - Circuit breaker state (0=closed, 1=open, 2=half_open)

### Grafana Dashboards

**Recommended dashboards:**
1. **Authentication Overview**
   - Authentication success/failure rate
   - Token refresh success/failure rate
   - Active sessions count
   - Top authentication errors

2. **Token Refresh Health**
   - Token refresh latency (p50, p95, p99)
   - Token refresh failure rate by provider
   - Circuit breaker state timeline
   - Token refresh errors breakdown

3. **Session Storage**
   - PostgreSQL connection health
   - Session storage errors
   - Cookie vs PostgreSQL session ratio
   - Session storage latency

### Alerting Rules

**Critical Alerts:**
1. High token refresh failure rate (>5% for 5 minutes)
2. Authentication unavailable (failure rate >90% for 2 minutes)
3. Session storage down (PostgreSQL connection failures)

**Warning Alerts:**
1. Token refresh latency high (p95 >2s for 5 minutes)
2. Circuit breaker open (token refresh disabled)
3. High authentication failure rate (>10% for 5 minutes)
4. Cookie decryption error rate increase

### Log Aggregation

**Structured logging requirements:**
- Use JSON format for all logs
- Include standard fields: `timestamp`, `level`, `provider`, `user_id`, `operation`
- Log token refresh events with success/failure status
- Log authentication errors with detailed context

**Example log entry:**
```json
{
  "timestamp": "2026-01-12T10:30:45Z",
  "level": "error",
  "provider": "keycloak",
  "user_id": "uuid-12345",
  "operation": "token_refresh",
  "result": "failure",
  "error": "refreshing token returned 401: invalid_token",
  "duration_ms": 150
}
```

---

## Testing Strategy

### Unit Tests

**Coverage requirements:**
- All new functions and methods: 100%
- Error handling paths: 100%
- Edge cases: As identified during implementation

**Example test structure:**
```go
func TestValidateCookieSecret(t *testing.T) {
    tests := []struct {
        name        string
        secret      string
        wantErr     bool
        errContains string
    }{
        {
            name:    "valid 32-byte secret",
            secret:  base64.StdEncoding.EncodeToString(make([]byte, 32)),
            wantErr: false,
        },
        {
            name:        "secret too short",
            secret:      base64.StdEncoding.EncodeToString(make([]byte, 16)),
            wantErr:     true,
            errContains: "at least 32 bytes",
        },
        {
            name:        "invalid base64",
            secret:      "not-valid-base64!@#",
            wantErr:     true,
            errContains: "valid base64",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateCookieSecret(tt.secret)

            if tt.wantErr {
                require.Error(t, err)
                require.Contains(t, err.Error(), tt.errContains)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

**Test environment requirements:**
- Docker Compose with Keycloak, PostgreSQL, and mock services
- Test fixtures for users, tokens, and configurations
- Cleanup after each test run

**Example test scenario:**
```go
var _ = Describe("Token Refresh Integration", func() {
    var (
        testServer *httptest.Server
        client     *http.Client
        sessionCookie *http.Cookie
    )

    BeforeEach(func() {
        // Set up test environment
        testServer = setupTestServer()
        client = &http.Client{}

        // Create authenticated session
        sessionCookie = authenticateUser("testuser@example.com")
    })

    AfterEach(func() {
        testServer.Close()
    })

    It("should refresh token when expired", func() {
        // Test implementation
    })
})
```

### Manual Testing Checklist

**Before each release:**
- [ ] Test OAuth2 flow with actual Keycloak instance
- [ ] Test OAuth2 flow with actual Azure AD instance
- [ ] Verify token refresh works correctly
- [ ] Test admin role persistence after re-login
- [ ] Verify PostgreSQL session storage works
- [ ] Test cookie security flags (inspect in browser DevTools)
- [ ] Verify HTTPS enforcement (attempt HTTP access)
- [ ] Test session expiry and redirect to login
- [ ] Verify Prometheus metrics are exposed
- [ ] Check logs for proper structured logging

---

## Future Enhancements and Security Considerations

### Overview

This section addresses security considerations for existing features and planned enhancements based on cross-reference with `.archive/research/ENHANCEMENT_RESEARCH_JAN_2026.md`.

**Key Finding:** Group Description feature (Enhancement 2) is **ALREADY IMPLEMENTED** in the codebase but requires security validation.

### Enhancement 1: Group Metadata (Description) - ALREADY IMPLEMENTED ✅

**Status:** Fully implemented across the codebase
**Files Affected:**
- `pkg/auth/auth.go:50` - Contains `Description *string` field
- `tools/auth-providers-common/pkg/state/state.go:15` - Contains `Description *string` field
- `pkg/gateway/types/group.go:24` - Contains `Description *string` field
- Unit tests exist in `state_test.go` (`TestGroupInfo_WithDescription`)

**Security Validation Required:**

#### 1. XSS Prevention

**Location:** UI components that display group descriptions

```typescript
// ui/user/src/lib/components/groups/GroupCard.svelte
// VALIDATION REQUIRED: Ensure HTML escaping

// GOOD (Svelte auto-escapes by default)
<p>{group.description}</p>

// BAD (dangerous if descriptions contain user input)
<p>{@html group.description}</p>

// BEST (explicit sanitization if HTML is needed)
import DOMPurify from 'isomorphic-dompurify';
<p>{@html DOMPurify.sanitize(group.description)}</p>
```

**Test Cases Required:**
```go
// tests/integration/group_security_test.go (NEW)
func TestGroupDescription_XSSPrevention(t *testing.T) {
    tests := []struct {
        name        string
        description string
        expectSafe  bool
    }{
        {
            name:        "script tag injection",
            description: "<script>alert('XSS')</script>",
            expectSafe:  true, // Should be escaped in UI
        },
        {
            name:        "event handler injection",
            description: "<img src=x onerror='alert(1)'>",
            expectSafe:  true,
        },
        {
            name:        "normal markdown",
            description: "**Bold** and _italic_ text",
            expectSafe:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create group with malicious description
            group := createGroupWithDescription(tt.description)

            // Fetch via API
            resp := httpGet(fmt.Sprintf("/api/groups/%s", group.ID))

            // Verify UI rendering doesn't execute scripts
            assertNoScriptExecution(resp.Body)
        })
    }
}
```

#### 2. SQL Injection Prevention

**Location:** Database queries filtering/searching by description

**Validation Required:**
```go
// pkg/storage/apis/obot.obot.ai/v1/group.go
// VERIFY: All queries use parameterized statements

// GOOD (parameterized)
query := `SELECT * FROM groups WHERE description LIKE $1`
rows, err := db.Query(query, "%"+searchTerm+"%")

// BAD (vulnerable to SQL injection)
query := fmt.Sprintf("SELECT * FROM groups WHERE description LIKE '%%%s%%'", searchTerm)
rows, err := db.Query(query)
```

**Code Review Checklist:**
- [ ] All description searches use parameterized queries
- [ ] No string concatenation in SQL statements
- [ ] ORM/query builder used correctly (pgx parameter binding)

#### 3. Description Length Validation

**Current State:** Unknown if length limits are enforced

**Add Validation:**
```go
// pkg/auth/auth.go (UPDATE)
const MaxGroupDescriptionLength = 1000 // Characters

type GroupInfo struct {
    ID              string   `json:"id,omitempty"`
    Name            string   `json:"name,omitempty"`
    Description     *string  `json:"description,omitempty"`
    IconURL         *string  `json:"iconUrl,omitempty"`
    Type            *string  `json:"type,omitempty"`
}

// Validate validates group info fields
func (g *GroupInfo) Validate() error {
    if g.Description != nil && len(*g.Description) > MaxGroupDescriptionLength {
        return fmt.Errorf("description exceeds maximum length of %d characters", MaxGroupDescriptionLength)
    }

    if g.Name == "" {
        return fmt.Errorf("group name is required")
    }

    return nil
}
```

**Call Validation:**
```go
// tools/auth-providers-common/pkg/state/state.go:70-85 (UPDATE)
groupInfo := GroupInfo{
    ID:          g.ID,
    Name:        g.DisplayName,
    Description: g.Description,
}

// Validate before adding to list
if err := groupInfo.Validate(); err != nil {
    log.WithFields(log.Fields{
        "group_id": g.ID,
        "error": err.Error(),
    }).Warn("invalid group info, skipping")
    continue
}

groupInfos = append(groupInfos, groupInfo)
```

#### 4. Unicode Handling

**Test Cases:**
```go
func TestGroupDescription_UnicodeHandling(t *testing.T) {
    tests := []struct {
        name        string
        description string
    }{
        {
            name:        "emoji",
            description: "Team 🚀 for DevOps",
        },
        {
            name:        "chinese characters",
            description: "开发团队",
        },
        {
            name:        "arabic rtl",
            description: "فريق التطوير",
        },
        {
            name:        "mixed scripts",
            description: "Team 开发 ⚡ مع Developers",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            group := createGroupWithDescription(tt.description)

            // Verify round-trip through database
            retrieved := getGroup(group.ID)
            require.Equal(t, tt.description, *retrieved.Description)

            // Verify API encoding
            resp := httpGet(fmt.Sprintf("/api/groups/%s", group.ID))
            var apiGroup GroupInfo
            json.Unmarshal(resp.Body, &apiGroup)
            require.Equal(t, tt.description, *apiGroup.Description)
        })
    }
}
```

#### 5. Integration with Monitoring (HIGH-3)

**Add Group Metrics:**
```go
// tools/auth-providers-common/pkg/metrics/metrics.go (UPDATE)
var (
    // Existing metrics...

    // GroupMetadata tracks group metadata population
    GroupMetadata = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "obot_auth_group_metadata_total",
            Help: "Total number of groups with metadata populated",
        },
        []string{"provider", "metadata_type"}, // metadata_type: description, icon, type
    )

    // GroupMetadataErrors tracks metadata fetch/parse errors
    GroupMetadataErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "obot_auth_group_metadata_errors_total",
            Help: "Total number of group metadata errors",
        },
        []string{"provider", "metadata_type", "error_type"},
    )
)
```

**Instrument Group Fetching:**
```go
// tools/entra-auth-provider/groups.go (UPDATE)
func fetchGroupsWithMetadata(ctx context.Context, client *msgraph.GraphServiceRequestBuilder) ([]GroupInfo, error) {
    groups, err := client.Me().TransitiveMemberOf().Request().GetN(ctx, 999)
    if err != nil {
        return nil, err
    }

    var groupInfos []GroupInfo
    for _, g := range groups {
        groupInfo := GroupInfo{
            ID:   *g.ID,
            Name: *g.DisplayName,
        }

        // Track description population
        if g.Description != nil && *g.Description != "" {
            groupInfo.Description = g.Description
            metrics.GroupMetadata.WithLabelValues("entra", "description").Inc()
        }

        groupInfos = append(groupInfos, groupInfo)
    }

    return groupInfos, nil
}
```

### Enhancement 2: Group Icon URL Support - Phase 2

**Status:** Planned (from ENHANCEMENT_RESEARCH_JAN_2026.md)
**Priority:** Medium
**Security Considerations:**

#### 1. URL Validation

**Requirement:** Icons must be data URLs (Base64-encoded), not external URLs

```go
// pkg/auth/auth.go (UPDATE when implementing)
const (
    MaxGroupIconSize = 100 * 1024 // 100KB
    AllowedIconMimeTypes = []string{
        "image/png",
        "image/jpeg",
        "image/svg+xml",
    }
)

func ValidateIconURL(iconURL string) error {
    if !strings.HasPrefix(iconURL, "data:") {
        return fmt.Errorf("icon URL must be a data URL (data:image/...;base64,...)")
    }

    // Parse data URL
    parts := strings.SplitN(iconURL, ",", 2)
    if len(parts) != 2 {
        return fmt.Errorf("invalid data URL format")
    }

    // Validate MIME type
    mimeType := strings.TrimPrefix(parts[0], "data:")
    mimeType = strings.TrimSuffix(mimeType, ";base64")

    validMime := false
    for _, allowed := range AllowedIconMimeTypes {
        if mimeType == allowed {
            validMime = true
            break
        }
    }

    if !validMime {
        return fmt.Errorf("unsupported MIME type: %s (allowed: %v)", mimeType, AllowedIconMimeTypes)
    }

    // Decode and validate size
    decoded, err := base64.StdEncoding.DecodeString(parts[1])
    if err != nil {
        return fmt.Errorf("invalid base64 encoding: %w", err)
    }

    if len(decoded) > MaxGroupIconSize {
        return fmt.Errorf("icon exceeds maximum size of %d bytes", MaxGroupIconSize)
    }

    return nil
}
```

#### 2. SSRF Prevention

**Critical:** Never fetch external URLs for group icons

```go
// WRONG - VULNERABLE TO SSRF
func fetchIconFromURL(iconURL string) ([]byte, error) {
    resp, err := http.Get(iconURL) // NEVER DO THIS
    // ...
}

// CORRECT - Require pre-encoded data URLs
func validateIcon(iconURL string) error {
    if !strings.HasPrefix(iconURL, "data:") {
        return fmt.Errorf("external URLs not allowed - use data URLs only")
    }

    return ValidateIconURL(iconURL)
}
```

#### 3. Content Security Policy

**UI Configuration:**
```html
<!-- ui/user/index.html or security headers -->
<meta http-equiv="Content-Security-Policy"
      content="default-src 'self';
               img-src 'self' data:;
               script-src 'self';
               style-src 'self' 'unsafe-inline'">
```

**Explanation:**
- `img-src 'self' data:` - Allow images from same origin and data URLs
- No external image sources allowed
- Prevents SSRF and tracking pixel attacks

### Enhancement 3: Group Type Filtering - Phase 2

**Status:** Planned (from ENHANCEMENT_RESEARCH_JAN_2026.md)
**Priority:** Medium
**Security Considerations:**

#### 1. OData Injection Prevention

**Context:** Microsoft Graph API uses OData filtering syntax

```go
// VULNERABLE - String concatenation
filter := fmt.Sprintf("groupTypes/any(c:c eq '%s')", userInput)

// SECURE - Parameterized with validation
func buildGroupTypeFilter(groupTypes []string) (string, error) {
    // Validate input (whitelist approach)
    validTypes := map[string]bool{
        "Unified":       true, // Microsoft 365 groups
        "DynamicMembership": true,
    }

    var filters []string
    for _, gt := range groupTypes {
        if !validTypes[gt] {
            return "", fmt.Errorf("invalid group type: %s", gt)
        }
        // Use literal strings only, no interpolation
        filters = append(filters, fmt.Sprintf("groupTypes/any(c:c eq '%s')", gt))
    }

    if len(filters) == 0 {
        return "", nil
    }

    return strings.Join(filters, " or "), nil
}
```

#### 2. Access Control for Group Types

**Requirement:** Ensure users can only filter groups they have access to

```go
// pkg/auth/middleware.go (NEW)
func ValidateGroupTypeAccess(userRoles []string, requestedTypes []string) error {
    // Power users can filter all group types
    if containsRole(userRoles, "power_user") || containsRole(userRoles, "admin") {
        return nil
    }

    // Regular users can only access standard groups
    for _, gt := range requestedTypes {
        if gt == "DynamicMembership" {
            return fmt.Errorf("insufficient permissions to access dynamic groups")
        }
    }

    return nil
}
```

### Integration Testing for Group Metadata (Test Scenario 0)

**File:** `tests/integration/group_metadata_security_test.go` (NEW)

```go
package integration_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Group Metadata Security Tests", func() {
    Context("Group Description Security", func() {
        It("should prevent XSS in group descriptions", func() {
            maliciousDesc := "<script>alert('XSS')</script>"
            group := createGroup("test-group", maliciousDesc, nil)

            // Fetch via API
            resp := apiGet(fmt.Sprintf("/api/groups/%s", group.ID))

            // Verify description is escaped in response
            var apiGroup GroupInfo
            json.Unmarshal(resp.Body, &apiGroup)

            // Description should be stored as-is
            Expect(*apiGroup.Description).To(Equal(maliciousDesc))

            // But UI should escape it (verify in browser tests)
            browserResp := browserGet(fmt.Sprintf("/groups/%s", group.ID))
            Expect(browserResp.HTML).NotTo(ContainSubstring("alert('XSS')"))
        })

        It("should reject overly long descriptions", func() {
            longDesc := strings.Repeat("a", 10000) // Exceeds limit

            _, err := createGroup("test-group", longDesc, nil)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("exceeds maximum length"))
        })

        It("should handle unicode correctly", func() {
            unicodeDesc := "Team 🚀 开发 مع"
            group := createGroup("test-group", unicodeDesc, nil)

            // Round-trip through database
            retrieved := getGroup(group.ID)
            Expect(*retrieved.Description).To(Equal(unicodeDesc))
        })
    })

    Context("SQL Injection Prevention", func() {
        It("should safely handle SQL-like characters in descriptions", func() {
            sqlDesc := "'; DROP TABLE groups; --"
            group := createGroup("test-group", sqlDesc, nil)

            // Search for group
            results := searchGroups(sqlDesc)

            // Should find the group without executing SQL
            Expect(results).To(HaveLen(1))
            Expect(results[0].ID).To(Equal(group.ID))

            // Verify groups table still exists
            allGroups := listGroups()
            Expect(allGroups).NotTo(BeEmpty())
        })
    })
})
```

### Implementation Priority for Future Enhancements

**Immediate (Sprint 1 - with other CRITICAL fixes):**
- [ ] Add group description validation (XSS, length, unicode)
- [ ] Verify SQL parameterization for description searches
- [ ] Add group metadata security tests (Test Scenario 0)
- [ ] Add group metadata metrics to observability (integrate with HIGH-3)

**Phase 2 (Months 2-3):**
- [ ] Implement group icon URL support with security validation
- [ ] Implement group type filtering with OData injection prevention
- [ ] Add CSP headers for icon rendering
- [ ] Complete Phase 2 security testing

### Updated Sprint 1 Effort Estimate

**Original Sprint 1:** 23 hours
**Add Group Security Validation:** 3 hours
- Group description validation (1 hour)
- SQL parameterization review (1 hour)
- Group security tests (1 hour)

**Updated Sprint 1 Total:** 26 hours

---

## References

### Related Documentation
- `auth_fix_jan2026` memory - Original Keycloak fix (commit 1e7fb26c)
- `auth_provider_implementation` memory - Auth provider specification
- `project_overview` memory - Repository structure and tech stack
- `CLAUDE.md` - Project conventions and commands
- `tools/README.md` - Auth provider documentation
- `tools/docs/auth-providers.md` (upstream) - Official auth provider spec
- `.archive/research/ENHANCEMENT_RESEARCH_JAN_2026.md` - Enhancement research and planning

### External Resources
- [OAuth2-Proxy Documentation](https://oauth2-proxy.github.io/oauth2-proxy/)
- [Keycloak OAuth2 Configuration](https://www.keycloak.org/docs/latest/server_admin/#_oidc)
- [Microsoft Identity Platform](https://learn.microsoft.com/en-us/azure/active-directory/develop/)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP XSS Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)
- [OWASP SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html)

### Commit References
- `1e7fb26c` - fix(auth): prevent admin role loss due to ID token parsing failures (Keycloak)
- `3bfcc491` - feat: add group description support and admin group listing

---

## Appendix: Fail Fast Philosophy

### Pattern: Authentication Errors Must Never Be Silent

The recent fix (commit 1e7fb26c) established a critical pattern for authentication error handling:

**Before (WRONG):**
```go
if err != nil {
    fmt.Printf("WARNING: failed to parse ID token: %v\n", err)
    // Continue without setting user ID - SILENT FAILURE
}
```

**After (CORRECT):**
```go
if ss.IDToken == "" {
    http.Error(w, "missing ID token - cannot authenticate user", http.StatusUnauthorized)
    return
}

userProfile, err := profile.ParseIDToken(ss.IDToken)
if err != nil {
    fmt.Printf("ERROR: failed to parse ID token: %v\n", err)
    http.Error(w, fmt.Sprintf("failed to parse ID token: %v", err), http.StatusInternalServerError)
    return
}
```

### Apply This Pattern Throughout

**Where to apply:**
1. ✅ Keycloak ID token parsing (FIXED)
2. ❌ Entra ID token parsing (NEEDS FIX - CRITICAL-1)
3. ⚠️ PostgreSQL connection validation (NEEDS IMPROVEMENT - HIGH-2)
4. ⚠️ Cookie secret entropy validation (NEEDS IMPROVEMENT - HIGH-1)
5. ⚠️ Token refresh error handling (NEEDS IMPROVEMENT - CRITICAL-2)

**Checklist for fail-fast authentication:**
- [ ] Identity data (ProviderUserID) must always be set or fail
- [ ] Configuration errors fail at startup, not at runtime
- [ ] Authentication errors return appropriate HTTP status (401/403/500)
- [ ] Errors are logged as ERROR, not WARNING
- [ ] Silent failures are eliminated
- [ ] Diagnostics are clear and actionable

---

## Document Maintenance

**Last Updated:** 2026-01-12
**Next Review:** After Sprint 1 completion (2 weeks)
**Owner:** Development Team
**Approvers:** Security Team, Platform Engineering

**Change Log:**
- 2026-01-12: Initial document creation based on security analysis
