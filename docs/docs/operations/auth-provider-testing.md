---
sidebar_position: 2
title: Authentication Provider Testing
description: Integration testing procedures for Entra ID and Keycloak authentication providers
---

# Authentication Provider Testing

This guide describes the integration testing strategy and procedures for validating Obot authentication providers (Microsoft Entra ID and Keycloak).

## Prerequisites

Before testing authentication providers, ensure you have:

**For Entra ID Testing:**
- Azure AD tenant with app registration
- Client ID, Client Secret, Tenant ID
- Test users with various roles
- See: [Entra ID Authentication Setup](../configuration/entra-id-authentication.md)

**For Keycloak Testing:**
- Keycloak instance (local or hosted)
- Realm configured with OIDC client
- Test users with various roles
- See: [Keycloak Authentication Setup](../configuration/keycloak-authentication.md)

**For PostgreSQL Session Testing:**
- PostgreSQL 12+ instance
- Database with appropriate permissions
- Connection string (DSN)

---

## Test Coverage Status

### ✅ Unit Tests (Implemented)

**Cookie Secret Validation** (`tools/auth-providers-common/pkg/secrets/validation_test.go`):
- Empty secret detection
- Invalid base64 format handling
- Entropy validation (32-byte minimum)
- Secret generation correctness
- Secret uniqueness verification

**PostgreSQL Connection Validation** (`tools/auth-providers-common/pkg/database/postgres_test.go`):
- Empty DSN handling
- Invalid DSN format detection
- Unreachable host error handling
- Invalid port detection
- Session table health check error handling

### 🔄 Manual Integration Tests

These tests require running Obot with actual Entra ID or Keycloak providers configured.

---

## Manual Integration Tests

### Test 1: Cookie Secret Entropy Enforcement

Validates that authentication providers enforce minimum 32-byte (256-bit) entropy for cookie encryption secrets.

**Setup:**
1. Deploy Entra ID or Keycloak auth provider
2. Configure cookie secret environment variable

**Test Cases:**

#### Test 1a: Valid 32-byte Secret (Expected: Success)

```bash
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start provider - should succeed with log:
# "PostgreSQL connection validated successfully"
```

**Expected Result:** Provider starts successfully

#### Test 1b: Too Short Secret (Expected: Failure)

```bash
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 16)
# Start provider - should exit with error
```

**Expected Error:**
```
ERROR: cookie secret must be at least 32 bytes (256 bits), got 16 bytes
Generate a valid secret with: openssl rand -base64 32
```

#### Test 1c: Invalid Base64 (Expected: Failure)

```bash
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET="not-valid-base64!@#$"
# Start provider - should exit with error
```

**Expected Error:**
```
ERROR: cookie secret must be valid base64
```

#### Test 1d: Provider-Specific Secret Precedence

```bash
export OBOT_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start provider - should use OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET
```

**Expected Result:** Provider-specific secret takes precedence over shared secret

**Validation:**
- ✅ Valid secrets: Provider starts successfully
- ✅ Invalid secrets: Provider exits immediately with clear error message
- ✅ Error includes generation command: `openssl rand -base64 32`

---

### Test 2: PostgreSQL Connection Validation

Validates fail-fast behavior when PostgreSQL session storage is misconfigured.

**Setup:**
1. Deploy provider with PostgreSQL session storage enabled
2. Configure `OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN`

**Test Cases:**

#### Test 2a: Valid PostgreSQL Connection (Expected: Success)

```bash
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@localhost:5432/obot?sslmode=disable"
# Start provider
```

**Expected Logs:**
```
INFO: entra-auth-provider: validating PostgreSQL connection...
INFO: entra-auth-provider: PostgreSQL connection validated successfully
INFO: entra-auth-provider: using PostgreSQL session storage (table prefix: entra_)
```

#### Test 2b: Invalid Credentials (Expected: Failure)

```bash
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:wrongpass@localhost:5432/obot"
# Start provider - should exit with error
```

**Expected Error:**
```
ERROR: entra-auth-provider: PostgreSQL connection failed: ...
ERROR: Set session storage to PostgreSQL but cannot connect
ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
```

#### Test 2c: Unreachable Host with Timeout

```bash
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@192.0.2.1:5432/obot?connect_timeout=5"
# Start provider - should exit within 5 seconds
```

**Expected Result:** Provider fails within timeout period

#### Test 2d: No PostgreSQL Configured (Expected: Cookie Fallback)

```bash
unset OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
# Start provider
```

**Expected Logs:**
```
INFO: entra-auth-provider: using cookie-only session storage
WARNING: Cookie-only sessions do not persist across pod restarts
```

**Validation:**
- ✅ Valid connection: Provider starts, validates connection before accepting traffic
- ✅ Invalid connection: Provider exits immediately, prevents runtime failures
- ✅ No PostgreSQL: Provider uses cookies, warns about limitations

---

### Test 3: Per-Provider Cookie Secret Isolation

Validates that different auth providers can use separate cookie secrets for defense-in-depth.

**Setup:**
1. Deploy both Entra ID and Keycloak providers
2. Configure provider-specific secrets

**Test Cases:**

#### Test 3a: Different Secrets Per Provider

```bash
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
export OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start both providers
```

**Expected Result:** Each provider uses its own secret

**Validation:** Session cookie from Entra cannot be decrypted by Keycloak (and vice versa)

#### Test 3b: Fallback to Shared Secret

```bash
export OBOT_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
unset OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET
unset OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET
# Start providers
```

**Expected Result:** Both providers use `OBOT_AUTH_PROVIDER_COOKIE_SECRET`

**Validation:** Sessions are interoperable (for backward compatibility)

**Validation:**
- ✅ Provider-specific secrets: Cookie isolation between providers
- ✅ Shared secret: Backward compatibility maintained
- ✅ Clear logging shows which secret is being used

---

## Running Unit Tests

Execute the existing unit test suites to verify authentication provider functionality:

```bash
# Cookie secret validation tests
cd tools/auth-providers-common
go test -v ./pkg/secrets/

# PostgreSQL validation tests
go test -v ./pkg/database/

# All auth-providers-common tests
go test -v ./...
```

---

## Running Manual Integration Tests

### Environment Setup

Configure environment variables for your test environment:

```bash
# Server configuration
export OBOT_SERVER_PUBLIC_URL="http://localhost:8080"

# Entra ID configuration
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID="your-client-id"
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET="your-client-secret"
export OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID="your-tenant-id"
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)

# Session storage (optional)
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@localhost:5432/obot?sslmode=disable"

# Email domain restriction (optional)
export OBOT_AUTH_PROVIDER_EMAIL_DOMAINS="*"
```

### Starting the Provider

```bash
cd tools/entra-auth-provider
go run main.go
```

### Expected Log Output

**Successful Startup with PostgreSQL:**
```
INFO: entra-auth-provider: validating PostgreSQL connection...
INFO: entra-auth-provider: PostgreSQL connection validated successfully
INFO: entra-auth-provider: using PostgreSQL session storage (table prefix: entra_)
[oauth2-proxy] provider: entra configured
[oauth2-proxy] starting server on :8080
```

**Successful Startup without PostgreSQL:**
```
INFO: entra-auth-provider: using cookie-only session storage
WARNING: Cookie-only sessions do not persist across pod restarts
[oauth2-proxy] provider: entra configured
[oauth2-proxy] starting server on :8080
```

**Failed Startup (Invalid Cookie Secret):**
```
ERROR: entra-auth-provider: cookie secret must be at least 32 bytes (256 bits), got 16 bytes
Generate a valid secret with: openssl rand -base64 32
exit status 1
```

**Failed Startup (PostgreSQL Connection Failed):**
```
INFO: entra-auth-provider: validating PostgreSQL connection...
ERROR: entra-auth-provider: PostgreSQL connection failed: connection refused
ERROR: Set session storage to PostgreSQL but cannot connect
ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
exit status 1
```

---

## Prometheus Metrics Testing

Authentication providers expose metrics through the Obot server for monitoring authentication health.

### Available Metrics

Metrics are available at `/debug/metrics` on the Obot server:

**Authentication Metrics:**
- `obot_auth_authentication_attempts_total{provider, result}` - Counter
  - Results: `success`, `failure`, `error`
- `obot_auth_token_refresh_attempts_total{provider, result}` - Counter
  - Results: `success`, `failure`
- `obot_auth_token_refresh_duration_seconds{provider}` - Histogram
- `obot_auth_session_storage_errors_total{provider, operation}` - Counter
- `obot_auth_cookie_decryption_errors_total{provider}` - Counter
- `obot_auth_active_sessions{provider}` - Gauge

### Manual Metrics Testing

```bash
# Start Obot server with auth provider configured
make dev

# Perform authentication flow
# 1. Navigate to http://localhost:8080/oauth2/start
# 2. Complete login
# 3. Access protected resources

# Check metrics endpoint
curl http://localhost:8080/debug/metrics | grep obot_auth
```

**Expected Output (sample):**
```
obot_auth_authentication_attempts_total{provider="entra-auth-provider",result="success"} 5
obot_auth_token_refresh_attempts_total{provider="entra-auth-provider",result="success"} 2
obot_auth_token_refresh_duration_seconds_bucket{provider="entra-auth-provider",le="0.1"} 2
obot_auth_active_sessions{provider="entra-auth-provider"} 3
```

---

## Test Results Checklist

Use this checklist to track manual testing progress:

### Core Functionality
- [ ] **Test 1a**: Valid 32-byte secret - provider starts successfully
- [ ] **Test 1b**: Too short secret - provider fails with error
- [ ] **Test 1c**: Invalid base64 - provider fails with error
- [ ] **Test 1d**: Provider-specific secret takes precedence

### PostgreSQL Session Storage
- [ ] **Test 2a**: Valid PostgreSQL connection - provider starts
- [ ] **Test 2b**: Invalid credentials - provider fails immediately
- [ ] **Test 2c**: Unreachable host - provider times out appropriately
- [ ] **Test 2d**: No PostgreSQL - provider uses cookie fallback

### Multi-Provider Deployment
- [ ] **Test 3a**: Different secrets per provider - cookie isolation works
- [ ] **Test 3b**: Shared secret fallback - backward compatibility maintained

### Metrics and Monitoring
- [ ] Prometheus metrics endpoint accessible
- [ ] Authentication success metrics incrementing
- [ ] Token refresh metrics incrementing
- [ ] Active sessions gauge accurate

---

## Future Automated Tests

The following automated integration tests are planned for future implementation:

### Phase 1: Core Authentication Flow
- OAuth2 Authorization Code Flow (complete flow from start to callback)
- Token Refresh (automatic token refresh when expired)
- Token Refresh Failure Handling (ErrInvalidSession, not HTTP 500)
- Cookie Updates (verify new cookies after successful refresh)
- Admin Role Persistence (regression test)

### Phase 2: Session Persistence
- PostgreSQL Session Storage (sessions persist to database)
- Cookie-Only Fallback (graceful degradation)
- Table Prefix Isolation (separate `keycloak_` and `entra_` sessions)
- Session Expiry (sessions expire after configured duration)

### Phase 3: Security and Regression
- Cookie Security Flags (HttpOnly, Secure, SameSite validation)
- ID Token Parsing Errors (fail authentication if token missing/invalid)
- Clock Skew Tolerance (handle time differences)
- Multi-User Concurrency (multiple users authenticating simultaneously)

---

## See Also

- [Microsoft Entra ID Authentication Setup](../configuration/entra-id-authentication.md)
- [Keycloak Authentication Setup](../configuration/keycloak-authentication.md)
- [Cookie Secret Rotation](./secret-rotation.md)
- [Local Development Guide](../contributing/local-development.md)

---

*Last Updated: 2026-01-13*
*Test Guide Version: 1.0*