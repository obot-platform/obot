# Keycloak Authentication

This guide covers the implementation and configuration of Keycloak as an authentication provider for obot-entraid.

## Overview

The Keycloak authentication provider enables enterprise single sign-on using Keycloak's open-source identity and access management platform. This implementation includes:

- OAuth 2.0/OIDC authentication with PKCE
- Keycloak role and group membership sync for RBAC
- Multi-realm support
- Admin API integration for comprehensive user metadata
- Production-grade security and observability

## Architecture

### Authentication Flow

```
User                    Obot                    Keycloak           Admin API
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
  |                       |------------------------>|                     |
  |                       |                         |                     |
  | 4. User authenticates |                         |                     |
  |<------------------------------------------------|                     |
  |                       |                         |                     |
  | 5. Auth code callback |                         |                     |
  |---------------------->|                         |                     |
  |                       |                         |                     |
  |                       | 6. Exchange code +      |                     |
  |                       |    code_verifier        |                     |
  |                       |------------------------>|                     |
  |                       |                         |                     |
  |                       | 7. Validate PKCE        |                     |
  |                       |    Return tokens        |                     |
  |                       |<------------------------|                     |
  |                       |                         |                     |
  |                       | 8. Fetch user details   |                     |
  |                       |------------------------------------------------>|
  |                       |                         |                     |
  |                       | 9. User info + groups   |                     |
  |                       |<------------------------------------------------|
  |                       |                         |                     |
  | 10. Session cookie    |                         |                     |
  |<----------------------|                         |                     |
```

### Component Architecture

The auth provider wraps oauth2-proxy v7.13.0 with Keycloak configuration:

- **OAuth2-proxy**: Handles OAuth2/OIDC protocol, session management, and token refresh
- **Custom handlers**: Implement obot-specific endpoints for state, user info, and groups
- **Keycloak Admin API client**: Fetches user profiles and group memberships
- **Expirable LRU cache**: Caches admin tokens and user state with configurable TTL

## Keycloak Client Setup

### Step 1: Create Realm (if needed)

1. Log in to Keycloak Administration Console
2. Hover over the realm dropdown in the top-left
3. Click **Add realm** or **Create Realm**
4. Enter a name (e.g., `obot`) and click **Create**

### Step 2: Create OpenID Connect Client

1. In your realm, navigate to **Clients** > **Create client**
2. Configure the client:
   - **Client type**: OpenID Connect
   - **Client ID**: `obot` (or your preferred identifier)
   - Click **Next**
3. Configure capability:
   - **Client authentication**: ON (Confidential client)
   - **Authorization**: OFF (not needed)
   - **Authentication flow**: Check all standard flows
   - Click **Next**
4. Configure login settings:
   - **Root URL**: `https://<your-obot-domain>`
   - **Valid redirect URIs**: `https://<your-obot-domain>/oauth2/callback`
   - **Web origins**: `https://<your-obot-domain>`
   - Click **Save**

### Step 3: Get Client Credentials

1. Navigate to your client's **Credentials** tab
2. Note the **Client secret** value
3. Save the **Client ID** and **Client secret** - you'll provide these to Obot

### Step 4: Configure Client Scopes (Optional - for Groups)

If you want to retrieve user group memberships:

1. Navigate to **Client Scopes** > **Create client scope**
2. Configure:
   - **Name**: `groups`
   - **Type**: Optional
   - **Protocol**: OpenID Connect
   - Click **Save**
3. Go to the **Mappers** tab > **Configure a new mapper**
4. Select **Group Membership**:
   - **Name**: `groups`
   - **Token Claim Name**: `groups`
   - **Full group path**: OFF (recommended)
   - **Add to ID token**: ON
   - **Add to access token**: ON
   - **Add to userinfo**: ON
   - Click **Save**
5. Go back to your client configuration
6. Navigate to **Client scopes** tab
7. Click **Add client scope**
8. Select the `groups` scope and add as **Optional**

### Step 5: Configure Service Account (for Admin API Access)

For comprehensive user metadata and group information:

#### Option A: Use Main Client with Service Account

1. In your client configuration, go to **Settings** tab
2. Enable **Service accounts roles**
3. Click **Save**
4. Navigate to **Service account roles** tab
5. Click **Assign role** > **Filter by realm roles**
6. Assign these roles:
   - `view-users`
   - `view-realm`
7. Click **Assign**

#### Option B: Create Dedicated Admin Client

1. Create a new client with **Client type**: OpenID Connect
2. Set **Client ID**: `obot-admin` (or preferred name)
3. Enable **Client authentication** and **Service accounts roles**
4. In **Service account roles**, assign:
   - `view-users`
   - `view-realm`
5. Note the Client ID and Client secret

### Step 6: Note Required Values

| Obot Parameter | Keycloak Location |
|----------------|-------------------|
| `client_id` | Client > Settings > Client ID |
| `client_secret` | Client > Credentials > Client secret |
| `url` | Your Keycloak base URL (e.g., `https://keycloak.example.com`) |
| `realm` | Realm name (e.g., `obot`) |
| `admin_client_id` | (Optional) Admin client's Client ID |
| `admin_client_secret` | (Optional) Admin client's Client secret |

## Configuration Parameters

### Required Parameters

| Parameter | Description |
|-----------|-------------|
| `client_id` | The Client ID from Keycloak client configuration |
| `client_secret` | The Client secret from Keycloak credentials |
| `url` | The base URL of your Keycloak server (e.g., `https://keycloak.example.com`) |
| `realm` | The Keycloak realm name (e.g., `obot`) |
| `cookie_secret` | Base64-encoded secret (must decode to 16, 24, or 32 bytes for AES) |
| `allowed_email_domains` | Comma-separated list of allowed email domains, or `*` to allow all |

### Admin API Access (Optional but Recommended)

| Parameter | Description |
|-----------|-------------|
| `admin_client_id` | Service account client ID for Admin API (uses main client if not specified) |
| `admin_client_secret` | Service account client secret (uses main client secret if not specified) |

### Optional Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `postgres_dsn` | - | PostgreSQL DSN for session storage |
| `token_refresh_duration` | `1h` | How often to refresh the access token |
| `allowed_groups` | - | Comma-separated Keycloak group names allowed to authenticate |
| `allowed_roles` | - | Comma-separated Keycloak role names allowed to authenticate |
| `group_cache_ttl` | `1h` | How long to cache user group memberships |
| `log_level` | `info` | Logging level (debug, info, warn, error) |
| `metrics_enabled` | `true` | Enable Prometheus metrics endpoint |

## Helm Chart Configuration

```yaml
# chart/values.yaml
config:
  OBOT_SERVER_AUTH_PROVIDER: "keycloak"
  OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID: "<client-id>"
  OBOT_KEYCLOAK_AUTH_PROVIDER_URL: "https://keycloak.example.com"
  OBOT_KEYCLOAK_AUTH_PROVIDER_REALM: "obot"
  OBOT_AUTH_PROVIDER_COOKIE_SECRET: "<base64-encoded-secret>"
  OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: "*"

# Client secret (required)
secrets:
  OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET: "<client-secret>"

# Optional: Admin API access with dedicated service account
secrets:
  OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_ID: "<admin-client-id>"
  OBOT_KEYCLOAK_AUTH_PROVIDER_ADMIN_CLIENT_SECRET: "<admin-client-secret>"
```

## Role and Group-Based Access Control

### Role-Based Restrictions

Restrict authentication to users with specific Keycloak roles:

```yaml
config:
  OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES: "obot-user,obot-admin"
```

When configured, only users with at least one of the specified roles can authenticate. Roles can be:
- **Realm roles**: Assigned directly in the realm
- **Client roles**: Assigned specific to the Obot client

### Group-Based Restrictions

Restrict authentication to members of specific Keycloak groups:

```yaml
config:
  OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS: "obot-users,engineering"
```

When configured, only users who are members of at least one of the specified groups can authenticate.

### Combining Roles and Groups

You can configure both `allowed_roles` and `allowed_groups`. In this case:
- User must satisfy **either** the role requirement **or** the group requirement
- This provides flexible access control based on your organization's structure

## Admin API Integration

The provider can use Keycloak's Admin REST API to fetch comprehensive user metadata:

### What Admin API Provides

- Full user profiles (name, email, attributes)
- Complete group memberships
- User attributes and custom fields
- Comprehensive role assignments

### Authentication Methods

The provider supports two patterns for Admin API access:

#### 1. Single Client (Recommended for Simple Setups)

Use the main OAuth client with service account enabled:
- Enable **Service accounts roles** on the main client
- Assign `view-users` and `view-realm` roles
- Provider automatically uses the main client credentials

#### 2. Dedicated Admin Client (Recommended for Production)

Use a separate service account client for Admin API:
- Create dedicated client with service account enabled
- Configure via `admin_client_id` and `admin_client_secret`
- Provides separation of concerns and easier auditing

### Token Caching

Admin access tokens are cached with a 4-minute TTL (Keycloak tokens typically valid for 5 minutes):
- Reduces API calls to Keycloak
- Improves authentication performance
- Automatic token refresh on expiry

## Observability

### Health Endpoints

| Endpoint | Purpose | Kubernetes Probe |
|----------|---------|------------------|
| `/health` | Liveness check | `livenessProbe` |
| `/ready` | Readiness check (verifies Keycloak connectivity) | `readinessProbe` |
| `/metrics` | Prometheus metrics | N/A |

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `keycloak_auth_requests_total` | Counter | Total authentication requests |
| `keycloak_auth_failures_total` | Counter | Failed authentications by reason |
| `keycloak_admin_api_duration_seconds` | Histogram | Admin API latency |
| `keycloak_cache_hits_total` | Counter | Cache hits |
| `keycloak_cache_misses_total` | Counter | Cache misses |

### Structured Logging

All logs use JSON format for observability platform integration:

```json
{
  "time": "2025-12-07T10:00:00Z",
  "level": "INFO",
  "msg": "User authenticated",
  "user_id": "abc123",
  "email": "user@example.com",
  "realm": "obot",
  "roles": ["obot-user"],
  "group_count": 5
}
```

## Sprint 1 Security Enhancements

The following security enhancements were implemented to improve authentication reliability and security:

### Fail-Fast Authentication

**ID token parsing is mandatory** for reliable user identification. Authentication will fail immediately if:
- ID token is missing from the OAuth callback
- ID token cannot be parsed successfully
- Required claims are missing from the token

This prevents silent authentication failures and ensures consistent user identity across sessions.

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

## Security Considerations

### Security Requirements

| Requirement | Implementation |
|-------------|----------------|
| oauth2-proxy **v7.13.0** | Required for CVE fixes |
| PKCE (S256) | SHA-256 code challenge required |
| Header smuggling protection | v7.13.0 normalizes underscore headers |
| Token validation | Checks expiry, issuer, and signature |
| Cookie encryption | AES-128/192/256 based on secret length |

### Cookie Security

| Setting | Value | Purpose |
|---------|-------|---------|
| `HttpOnly` | true | Prevent XSS access to cookies |
| `Secure` | true | HTTPS only transmission |
| `SameSite` | lax | CSRF protection |

### Client Secret Management

**Best Practices:**
- Store secrets in Kubernetes secrets or secure vault
- Never commit secrets to source control
- Rotate secrets regularly (recommended: every 90 days)
- Use dedicated admin client for separation of concerns
- Monitor Admin API access logs

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| 401 on callback | Invalid client secret | Verify client secret in Keycloak |
| No groups returned | Missing groups scope | Add groups client scope and mapper |
| No roles returned | Roles not in token | Check token mappers in client configuration |
| Admin API fails | Missing service account roles | Assign `view-users` and `view-realm` roles |
| Token expired | Refresh token invalid | User needs to re-authenticate |

### Debug Logging

Enable debug logging for troubleshooting:

```yaml
config:
  OBOT_KEYCLOAK_AUTH_PROVIDER_LOG_LEVEL: "debug"
```

### Testing Admin API Access

You can test Admin API connectivity manually:

```bash
# Get admin token (client credentials flow)
curl -X POST "https://keycloak.example.com/realms/obot/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=<client-id>" \
  -d "client_secret=<client-secret>"

# Test user endpoint
curl "https://keycloak.example.com/admin/realms/obot/users/<user-id>" \
  -H "Authorization: Bearer <access-token>"

# Test groups endpoint
curl "https://keycloak.example.com/admin/realms/obot/users/<user-id>/groups" \
  -H "Authorization: Bearer <access-token>"
```

## References

- [Keycloak Documentation](https://www.keycloak.org/documentation)
- [Keycloak OpenID Connect Provider](https://www.keycloak.org/docs/latest/server_admin/#_oidc)
- [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/index.html)
- [OAuth2-Proxy Keycloak-OIDC Provider](https://oauth2-proxy.github.io/oauth2-proxy/configuration/providers/keycloak_oidc/)
- [Service Accounts in Keycloak](https://www.keycloak.org/docs/latest/server_admin/#_service_accounts)
