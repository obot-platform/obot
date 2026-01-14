# Microsoft Entra ID Authentication

This guide covers the implementation and configuration of Microsoft Entra ID (formerly Azure Active Directory) as an authentication provider for obot-entraid.

## Overview

The Entra ID authentication provider enables enterprise single sign-on using Microsoft's identity platform. This implementation includes:

- OAuth 2.0/OIDC authentication with PKCE
- Azure AD group membership sync for RBAC (including 200+ groups)
- Multi-tenant support with tenant validation
- Workload Identity support for Kubernetes deployments
- Production-grade security and observability

## Architecture

### Authentication Flow

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
  |                       | 8. Fetch user profile   |                     |
  |                       |------------------------------------------------>|
  |                       |                         |                     |
  |                       | 9. User info + groups   |                     |
  |                       |<------------------------------------------------|
  |                       |                         |                     |
  | 10. Session cookie    |                         |                     |
  |<----------------------|                         |                     |
```

### Component Architecture

The auth provider wraps oauth2-proxy v7.13.0 with Microsoft Entra ID configuration:

- **OAuth2-proxy**: Handles OAuth2/OIDC protocol, session management, and token refresh
- **Custom handlers**: Implement obot-specific endpoints for state, user info, and groups
- **Microsoft Graph client**: Fetches user profiles and group memberships with pagination
- **Expirable LRU cache**: Caches user state with configurable TTL

## Azure AD App Registration Setup

### Step 1: Create App Registration

1. Navigate to [Microsoft Entra admin center](https://entra.microsoft.com)
2. Go to **App registrations** > **New registration**
3. Configure:
   - **Name**: Obot Authentication
   - **Supported account types**:
     - "Accounts in this organizational directory only" (single tenant - recommended)
     - OR "Accounts in any organizational directory" (multi-tenant)
   - **Redirect URI**: Web - `https://<your-obot-domain>/oauth2/callback`

### Step 2: Configure API Permissions

**Delegated permissions** (requested via OAuth scope):

| Permission | Description |
|------------|-------------|
| `openid` | Sign in and read user profile |
| `email` | View user's email address |
| `profile` | View user's basic profile |
| `User.Read` | Sign in and read user profile |

**Application permissions** (configured in Azure portal, require admin consent):

| Permission | Description |
|------------|-------------|
| `GroupMember.Read.All` | Read all group memberships |
| `User.Read.All` | Read all users' profiles |

### Step 3: Choose Authentication Method

#### Option A: Workload Identity Federation (Recommended for Kubernetes)

For AKS or Kubernetes deployments, use Azure Workload Identity to eliminate secrets:

**Prerequisites:**
- Cluster has public OIDC provider URL enabled
- Workload Identity admission webhook deployed
- Federated credential configured in App Registration

**Configuration:**
```yaml
# Service account annotation
azure.workload.identity/client-id: <client-id>

# Pod label
azure.workload.identity/use: "true"
```

Set `use_workload_identity: true` in tool configuration - no `client_secret` required.

#### Option B: Certificate Credentials (Recommended for non-Kubernetes)

1. Generate or obtain a certificate from Azure Key Vault
2. Upload public key to **App Registration > Certificates & secrets > Certificates**
3. Store private key securely (not in source control)
4. Configure paths via `client_cert_path` and `client_key_path`

**Best Practices:**
- Use certificates from a trusted CA (Azure Key Vault recommended)
- Maximum lifetime: 180 days
- Configure automatic rotation via Key Vault

#### Option C: Client Secret (Development Only)

**Not recommended for production.**

1. Go to **Certificates & secrets > Client secrets**
2. Click **New client secret**
3. Set expiration (maximum: 24 months)
4. **Save the Value immediately** - cannot be retrieved later

### Step 4: Note Required Values

| Obot Parameter | Azure Portal Location |
|----------------|----------------------|
| `client_id` | Overview > Application (client) ID |
| `tenant_id` | Overview > Directory (tenant) ID |
| `client_secret` | Certificates & secrets > Value |
| `allowed_tenants` | Your allowed tenant IDs (for multi-tenant) |

## Configuration Parameters

### Required Parameters

| Parameter | Description |
|-----------|-------------|
| `client_id` | The Application (client) ID from Azure App Registration |
| `tenant_id` | The Directory (tenant) ID, or `common`/`organizations` for multi-tenant |
| `cookie_secret` | Base64-encoded secret (must decode to 16, 24, or 32 bytes for AES) |
| `allowed_email_domains` | Comma-separated list of allowed email domains, or `*` to allow all |

### Authentication Method (Choose One)

| Parameter | Description |
|-----------|-------------|
| `client_secret` | The client secret value from Certificates & secrets |
| `use_workload_identity` | Enable Azure Workload Identity authentication (set to `true`) |
| `client_cert_path` | Path to client certificate PEM file |
| `client_key_path` | Path to client private key PEM file |

### Optional Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `postgres_dsn` | - | PostgreSQL DSN for session storage |
| `token_refresh_duration` | `1h` | How often to refresh the access token |
| `allowed_groups` | - | Comma-separated Azure AD group IDs allowed to authenticate |
| `allowed_tenants` | - | Comma-separated tenant IDs for multi-tenant apps |
| `group_cache_ttl` | `1h` | How long to cache user group memberships |
| `icon_cache_ttl` | `24h` | How long to cache profile pictures |
| `log_level` | `info` | Logging level (debug, info, warn, error) |
| `metrics_enabled` | `true` | Enable Prometheus metrics endpoint |

## Helm Chart Configuration

```yaml
# chart/values.yaml
config:
  OBOT_SERVER_AUTH_PROVIDER: "entra-id"
  OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID: "<client-id>"
  OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID: "<tenant-id>"
  OBOT_AUTH_PROVIDER_COOKIE_SECRET: "<base64-encoded-secret>"
  OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: "*"

# For client secret authentication
secrets:
  OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET: "<client-secret>"

# For workload identity (AKS)
serviceAccount:
  annotations:
    azure.workload.identity/client-id: "<client-id>"
```

## Multi-Tenant Configuration

When using `tenant_id` as `common`, `organizations`, or `consumers`:

1. **Required**: Set `allowed_tenants` to an explicit list of permitted tenant IDs
2. Token validation ensures the `tid` claim matches the allowed list
3. Issuer validation is adjusted for multi-tenant scenarios

```yaml
config:
  OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID: "common"
  OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_TENANTS: "tenant-id-1,tenant-id-2"
```

## Group Membership & RBAC

### Handling 200+ Groups

Azure AD has a token claim limit for groups. When a user belongs to 200+ groups, the provider:

1. Uses the `/me/transitiveMemberOf` Graph API endpoint
2. Includes pagination support via `@odata.nextLink`
3. Requires `ConsistencyLevel: eventual` header for advanced queries
4. Fetches complete group hierarchy including nested groups

### Group Filtering

Optionally restrict authentication to specific Azure AD groups:

```yaml
config:
  OBOT_ENTRA_AUTH_PROVIDER_ALLOWED_GROUPS: "group-id-1,group-id-2"
```

## Observability

### Health Endpoints

| Endpoint | Purpose | Kubernetes Probe |
|----------|---------|------------------|
| `/health` | Liveness check | `livenessProbe` |
| `/ready` | Readiness check (verifies Entra ID connectivity) | `readinessProbe` |
| `/metrics` | Prometheus metrics | N/A |

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `entra_auth_requests_total` | Counter | Total authentication requests |
| `entra_auth_failures_total` | Counter | Failed authentications by reason |
| `entra_graph_api_duration_seconds` | Histogram | Graph API latency |
| `entra_cache_hits_total` | Counter | Cache hits |
| `entra_cache_misses_total` | Counter | Cache misses |

### Structured Logging

All logs use JSON format for observability platform integration:

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

## Sprint 1 Security Enhancements

The following security enhancements were implemented to improve authentication reliability and security:

### Fail-Fast Authentication

**ID token parsing is mandatory** for reliable user identification. Authentication will fail immediately if:
- ID token is missing from the OAuth callback
- ID token cannot be parsed successfully
- Tenant validation fails (for multi-tenant configurations)

This prevents silent authentication failures and ensures consistent user identity across sessions, eliminating issues like admin/owner permission loss after re-login.

### Cookie Security Configuration

Production-grade cookie security with explicit configuration:

| Setting | Production | Development |
|---------|------------|-------------|
| `Secure` flag | Required (HTTPS) | Optional with `OBOT_AUTH_INSECURE_COOKIES=true` |
| `HTTPOnly` | Always enabled | Always enabled |
| `SameSite` | `Lax` (default) | Configurable via `OBOT_AUTH_PROVIDER_COOKIE_SAMESITE` |
| Domain | Auto-detected from server URL | Override with `OBOT_AUTH_PROVIDER_COOKIE_DOMAIN` |
| Path | `/` (default) | Override with `OBOT_AUTH_PROVIDER_COOKIE_PATH` |

**Environment Variables:**

```yaml
# REQUIRED in production: Server must use HTTPS
# For local development with HTTP only:
OBOT_AUTH_INSECURE_COOKIES: "true"  # NOT for production

# Optional: Override cookie configuration
OBOT_AUTH_PROVIDER_COOKIE_DOMAIN: "example.com"
OBOT_AUTH_PROVIDER_COOKIE_PATH: "/"
OBOT_AUTH_PROVIDER_COOKIE_SAMESITE: "Lax"  # Options: Strict, Lax, None
```

**HTTPS Enforcement:**
- Server URL must use `https://` scheme in production
- Application exits on startup if HTTP is detected without `OBOT_AUTH_INSECURE_COOKIES=true`
- Development environments can opt-out with explicit environment variable

### Token Refresh Error Handling

Automatic session error detection and graceful handling:

**Detected Error Patterns:**
- `record not found` - Session not in storage
- `session ticket cookie failed validation` - Cookie decryption failure
- `refreshing token returned` - Token refresh HTTP errors
- `REFRESH_TOKEN_ERROR` - OAuth2-proxy refresh error
- `RESTART_AUTHENTICATION_ERROR` - Auth restart required
- `invalid_token` - OAuth2 RFC 6749 standard error
- `failed to refresh token` - Token refresh failure

**Behavior:**
- Users automatically redirected to login page on session errors
- No confusing HTTP 500 error pages shown to users
- Diagnostic logging for troubleshooting

### Group Metadata Validation

Security validation for Azure AD group information:

| Validation | Limit | Purpose |
|------------|-------|---------|
| Group name | Required | Ensures valid group identification |
| Description length | 1000 characters | Prevents storage abuse and display issues |

Invalid group metadata is rejected with clear error messages.

## Security Considerations

### Security Requirements

| Requirement | Implementation |
|-------------|----------------|
| oauth2-proxy **v7.13.0** | Required for CVE fixes |
| PKCE (S256) | SHA-256 code challenge required by Microsoft |
| Header smuggling protection | v7.13.0 normalizes underscore headers |
| Issuer validation | Multi-tenant uses allowed tenants list |
| Token validation | Checks expiry, issuer, and tenant |
| Cookie encryption | AES-128/192/256 based on secret length |

### Cookie Security

| Setting | Value | Purpose |
|---------|-------|---------|
| `HttpOnly` | true | Prevent XSS access to cookies |
| `Secure` | true | HTTPS only transmission |
| `SameSite` | lax | CSRF protection |

### Credential Preference Hierarchy

Microsoft recommends credentials in this order (most to least secure):

1. **Managed Identity** - No credentials to manage (Azure-hosted only)
2. **Workload Identity Federation** - For Kubernetes/GitHub Actions
3. **Certificate Credentials** - Recommended for production
4. **Client Secrets** - Not recommended for production

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| 401 on callback | Invalid client secret | Regenerate and update secret |
| No groups returned | Missing Graph API permissions | Add `GroupMember.Read.All` with admin consent |
| Slow group fetching | 200+ groups | Normal - pagination takes time |
| Token expired | Refresh token invalid | User needs to re-authenticate |

### Debug Logging

Enable debug logging for troubleshooting:

```yaml
config:
  OBOT_ENTRA_AUTH_PROVIDER_LOG_LEVEL: "debug"
```

## See Also

- [Authentication Provider Testing](../operations/auth-provider-testing.md) - Testing and validation procedures
- [Local Development Guide](../contributing/local-development.md) - Running auth providers locally
- [Cookie Secret Rotation](../operations/secret-rotation.md) - Rotating authentication secrets
- [Keycloak Authentication](./keycloak-authentication.md) - Alternative OIDC provider

## References

- [Microsoft Entra ID OAuth 2.0 Authorization Code Flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow)
- [OAuth2-Proxy Microsoft Entra ID Provider](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/ms_entra_id/)
- [Microsoft Graph API - User](https://learn.microsoft.com/en-us/graph/api/user-get)
- [Azure Workload Identity](https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation)
- [Entra ID Provider Source Code](https://github.com/jrmatherly/obot-entraid/tree/main/tools/entra-auth-provider) - Package-level documentation
