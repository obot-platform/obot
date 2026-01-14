# Authentication Integration Testing Guide

This document describes the integration testing strategy for authentication providers (Entra ID and Keycloak).

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

### 🔄 Manual Integration Tests (Required)

These tests require running Obot with actual Entra ID or Keycloak providers configured.

#### Test 1: Cookie Secret Entropy Enforcement

**Setup:**
1. Deploy Entra ID or Keycloak auth provider
2. Set `OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET` or `OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET`

**Test Cases:**
```bash
# Test 1a: Valid 32-byte secret (should succeed)
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start provider - should succeed with "PostgreSQL connection validated successfully"

# Test 1b: Too short secret (should fail at startup)
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 16)
# Start provider - should exit with error: "cookie secret must be at least 32 bytes (256 bits), got 16 bytes"

# Test 1c: Invalid base64 (should fail at startup)
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET="not-valid-base64!@#$"
# Start provider - should exit with error: "cookie secret must be valid base64"

# Test 1d: Provider-specific secret takes precedence
export OBOT_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start provider - should use OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET
```

**Expected Behavior:**
- Valid secrets: Provider starts successfully
- Invalid secrets: Provider exits immediately with clear error message
- Helpful error includes generation command: `openssl rand -base64 32`

#### Test 2: PostgreSQL Connection Validation

**Setup:**
1. Deploy provider with PostgreSQL session storage
2. Configure `OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN`

**Test Cases:**
```bash
# Test 2a: Valid PostgreSQL connection (should succeed)
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@localhost:5432/obot?sslmode=disable"
# Start provider - should log:
# "INFO: entra-auth-provider: validating PostgreSQL connection..."
# "INFO: entra-auth-provider: PostgreSQL connection validated successfully"
# "INFO: entra-auth-provider: using PostgreSQL session storage (table prefix: entra_)"

# Test 2b: Invalid PostgreSQL connection (should fail at startup)
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:wrongpass@localhost:5432/obot"
# Start provider - should exit with:
# "ERROR: entra-auth-provider: PostgreSQL connection failed: ..."
# "ERROR: Set session storage to PostgreSQL but cannot connect"
# "ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"

# Test 2c: Unreachable PostgreSQL host (should fail at startup with 5s timeout)
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@192.0.2.1:5432/obot?connect_timeout=5"
# Start provider - should exit within 5 seconds with connection error

# Test 2d: No PostgreSQL configured (should use cookies)
unset OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
# Start provider - should log:
# "INFO: entra-auth-provider: using cookie-only session storage"
# "WARNING: Cookie-only sessions do not persist across pod restarts"
```

**Expected Behavior:**
- Valid connection: Provider starts, validates connection before accepting traffic
- Invalid connection: Provider exits immediately, prevents runtime failures
- No PostgreSQL: Provider uses cookies, warns about limitations

#### Test 3: Per-Provider Cookie Secret Isolation

**Setup:**
1. Deploy both Entra ID and Keycloak providers
2. Configure provider-specific secrets

**Test Cases:**
```bash
# Test 3a: Different secrets per provider
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
export OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
# Start both providers - each should use its own secret
# Verify: Session cookie from Entra cannot be decrypted by Keycloak (and vice versa)

# Test 3b: Fallback to shared secret
export OBOT_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
unset OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET
unset OBOT_KEYCLOAK_AUTH_PROVIDER_COOKIE_SECRET
# Start providers - both should use OBOT_AUTH_PROVIDER_COOKIE_SECRET
# Verify: Sessions are interoperable (for backward compatibility testing)
```

**Expected Behavior:**
- Provider-specific secrets: Cookie isolation between providers
- Shared secret: Backward compatibility maintained
- Clear logging shows which secret is being used

### 🔮 Future Automated Integration Tests

These require full test infrastructure (mock OAuth providers, test containers) as described in
`docs/AUTHENTICATION_SECURITY_IMPLEMENTATION_GUIDE.md`.

#### Phase 1: Core Authentication Flow (12 hours)

**Infrastructure Needed:**
- Mock Keycloak server (testcontainers)
- Mock Azure AD/Entra ID server (testcontainers)
- Test PostgreSQL database (testcontainers)
- Ginkgo/Gomega test framework (already in use)

**Test Scenarios:**
1. **OAuth2 Authorization Code Flow** - Complete OAuth flow from start to callback
2. **Token Refresh** - Automatic token refresh when expired
3. **Token Refresh Failure Handling** - ErrInvalidSession on refresh failure (not HTTP 500)
4. **Cookie Updates** - Verify new cookies after successful refresh
5. **Admin Role Persistence (Regression)** - Admin role survives logout/re-login (commit 1e7fb26c)

**Test File:** `tests/integration/auth_flow_test.go` (to be created)

#### Phase 2: Session Persistence (8 hours)

**Test Scenarios:**
1. **PostgreSQL Session Storage** - Sessions persist to database
2. **Cookie-Only Fallback** - Graceful degradation if PostgreSQL unavailable
3. **Table Prefix Isolation** - Separate keycloak_and entra_ sessions
4. **Session Expiry** - Sessions expire after configured duration

**Test File:** `tests/integration/session_storage_test.go` (to be created)

#### Phase 3: Security and Regression (8 hours)

**Test Scenarios:**
1. **Cookie Security Flags** - HttpOnly, Secure, SameSite validation
2. **ID Token Parsing Errors** - Fail authentication if ID token missing/invalid
3. **Clock Skew Tolerance** - Handle time differences between servers
4. **Multi-User Concurrency** - Multiple users authenticating simultaneously

**Test File:** `tests/integration/security_test.go` (to be created)

## Running Unit Tests

```bash
# Run cookie secret validation tests
cd tools/auth-providers-common
go test -v ./pkg/secrets/

# Run PostgreSQL validation tests
go test -v ./pkg/database/

# Run all auth-providers-common tests
go test -v ./...
```

## Running Manual Integration Tests

### Prerequisites

1. **Entra ID Testing:**
   - Azure AD tenant with app registration
   - Client ID, Client Secret, Tenant ID
   - Test users with various roles

2. **Keycloak Testing:**
   - Keycloak instance (local or hosted)
   - Realm configured with client
   - Test users with various roles

3. **PostgreSQL Testing:**
   - PostgreSQL 12+ instance
   - Database with appropriate permissions
   - Connection string (DSN)

### Test Execution

```bash
# Set up environment variables (adjust for your setup)
export OBOT_SERVER_PUBLIC_URL="http://localhost:8080"
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID="your-client-id"
export OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET="your-client-secret"
export OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID="your-tenant-id"
export OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET=$(openssl rand -base64 32)
export OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN="postgres://user:pass@localhost:5432/obot?sslmode=disable"
export OBOT_AUTH_PROVIDER_EMAIL_DOMAINS="*"

# Run the provider
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
Or set OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET for provider-specific secret
exit status 1
```

**Failed Startup (PostgreSQL Connection Failed):**
```
INFO: entra-auth-provider: validating PostgreSQL connection...
ERROR: entra-auth-provider: PostgreSQL connection failed: dial tcp 127.0.0.1:5432: connect: connection refused
ERROR: Set session storage to PostgreSQL but cannot connect
ERROR: Check OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN
exit status 1
```

## Prometheus Metrics Testing

### Available Metrics

Metrics are exposed at `/debug/metrics` on the Obot server (not on the auth providers themselves).

**Authentication Metrics:**
- `obot_auth_authentication_attempts_total{provider, result}` - Counter
  - `result`: success, failure, error
- `obot_auth_token_refresh_attempts_total{provider, result}` - Counter
  - `result`: success, failure
- `obot_auth_token_refresh_duration_seconds{provider}` - Histogram
- `obot_auth_session_storage_errors_total{provider, operation}` - Counter
- `obot_auth_cookie_decryption_errors_total{provider}` - Counter
- `obot_auth_active_sessions{provider}` - Gauge

### Manual Metrics Testing

```bash
# Start Obot server with auth provider configured
make dev

# Perform authentication flow
# - Navigate to http://localhost:8080/oauth2/start
# - Complete login
# - Access protected resources

# Check metrics endpoint
curl http://localhost:8080/debug/metrics | grep obot_auth

# Expected output (sample):
# obot_auth_authentication_attempts_total{provider="entra-auth-provider",result="success"} 5
# obot_auth_token_refresh_attempts_total{provider="entra-auth-provider",result="success"} 2
# obot_auth_token_refresh_duration_seconds_bucket{provider="entra-auth-provider",le="0.1"} 2
# obot_auth_active_sessions{provider="entra-auth-provider"} 3
```

## Test Results Checklist

### Sprint 2 Implementation Validation

- [x] Cookie secret validation unit tests pass
- [x] PostgreSQL validation unit tests pass
- [x] All existing tests still pass (no regressions)
- [x] Linting passes with 0 issues
- [ ] Manual Test 1: Cookie secret entropy enforcement
- [ ] Manual Test 2: PostgreSQL connection validation
- [ ] Manual Test 3: Per-provider cookie secret isolation
- [ ] Manual Metrics Testing: Prometheus metrics validation

### Future Work (Post-Sprint 2)

- [ ] Implement Phase 1 automated tests (auth flow)
- [ ] Implement Phase 2 automated tests (session persistence)
- [ ] Implement Phase 3 automated tests (security and regression)
- [ ] Set up CI/CD integration for automated tests
- [ ] Create Grafana dashboards for auth metrics
- [ ] Document runbooks for common auth issues

## Related Documentation

- Implementation Guide: `docs/AUTHENTICATION_SECURITY_IMPLEMENTATION_GUIDE.md`
- Sprint 1 Completion Commit: `1e7fb26c` (admin role persistence fix)
- Sprint 2 HIGH-1 Commit: `65b87516` (per-provider cookie secrets)
- Sprint 2 HIGH-2 Commit: `1647c308` (PostgreSQL validation)
- Sprint 2 HIGH-3 Commit: (Prometheus metrics - part of Sprint 1)
