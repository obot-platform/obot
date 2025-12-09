# Keycloak Configuration Guide for Obot

This guide provides detailed instructions for configuring Keycloak to work with the Obot Keycloak authentication provider.

## Prerequisites

- Keycloak 19.0.0 or later (this guide uses the new Admin Console)
- Admin access to your Keycloak instance
- Your Obot server URL (e.g., `https://obot.example.com`)

## Step 1: Create a Client

### 1.1 Navigate to Clients

1. Log into the Keycloak Admin Console
2. Select your realm (or create a new one)
3. In the left sidebar, click **Clients**
4. Click **Create client**

### 1.2 General Settings

| Setting | Value |
|---------|-------|
| Client type | OpenID Connect |
| Client ID | `obot` (or your preferred name) |
| Name | `Obot Authentication` (optional, for display) |
| Description | `OAuth client for Obot platform` (optional) |

Click **Next**

### 1.3 Capability Config

| Setting | Value |
|---------|-------|
| Client authentication | **ON** (this makes it a confidential client) |
| Authorization | OFF |
| Authentication flow | Check **Standard flow** |

Click **Next**

### 1.4 Login Settings

| Setting | Value |
|---------|-------|
| Root URL | `https://your-obot-url` |
| Home URL | `https://your-obot-url` |
| Valid redirect URIs | `https://your-obot-url/*` |
| Valid post logout redirect URIs | `https://your-obot-url/*` |
| Web origins | `https://your-obot-url` |

Click **Save**

### 1.5 Retrieve Client Credentials

After saving, navigate to the **Credentials** tab:

1. Copy the **Client secret** - you'll need this for `OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET`
2. The Client ID you set earlier is your `OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID`

## Step 2: Configure Client Scopes (Required)

The default scopes should work, but verify they're attached:

1. Go to your client → **Client scopes** tab
2. Ensure these scopes are in "Default Client Scopes":
   - `openid`
   - `email`
   - `profile`
3. If using refresh tokens, also add:
   - `offline_access`

## Step 3: Configure Groups (Optional)

If you want to use Keycloak groups for access control in Obot:

### 3.1 Create the Groups Client Scope

1. In the left sidebar, click **Client scopes**
2. Click **Create client scope**
3. Configure:

| Setting | Value |
|---------|-------|
| Name | `groups` |
| Description | `User group memberships` |
| Type | Default |
| Display on consent screen | OFF |
| Include in token scope | ON |

Click **Save**

### 3.2 Add Group Membership Mapper

1. In the new `groups` scope, go to the **Mappers** tab
2. Click **Configure a new mapper**
3. Select **Group Membership**
4. Configure:

| Setting | Value |
|---------|-------|
| Name | `groups` |
| Token Claim Name | `groups` |
| Full group path | OFF (recommended, gives cleaner group names) |
| Add to ID token | ON |
| Add to access token | ON |
| Add to userinfo | ON |

Click **Save**

### 3.3 Attach Scope to Client

1. Go back to **Clients** → your Obot client
2. Go to **Client scopes** tab
3. Click **Add client scope**
4. Select `groups` and add as **Default**

### 3.4 Create Groups in Keycloak

1. In the left sidebar, click **Groups**
2. Click **Create group**
3. Create groups that match what you'll configure in `OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS`

Example groups:
- `obot-admins`
- `obot-users`
- `obot-developers`

### 3.5 Assign Users to Groups

1. Go to **Users** → select a user
2. Go to **Groups** tab
3. Click **Join Group**
4. Select the appropriate group(s)

## Step 4: Configure Roles (Optional)

If you prefer role-based access control instead of (or in addition to) groups:

### 4.1 Create Realm Roles

1. In the left sidebar, click **Realm roles**
2. Click **Create role**
3. Create roles like:
   - `obot-admin`
   - `obot-user`

### 4.2 Assign Roles to Users

1. Go to **Users** → select a user
2. Go to **Role mapping** tab
3. Click **Assign role**
4. Select the appropriate realm role(s)

### 4.3 Configure Role in Obot

Set the `OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES` environment variable:

```bash
# Single role
OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES=obot-user

# Multiple roles (comma-separated)
OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES=obot-admin,obot-user
```

## Step 5: Configure Audience Mapper (Recommended)

The audience mapper ensures the token's `aud` claim includes your client ID, which some validations require.

### 5.1 Add Audience Mapper to Client

1. Go to **Clients** → your Obot client
2. Go to **Client scopes** tab
3. Click on `obot-dedicated` (the dedicated scope for your client)
4. Go to **Mappers** tab
5. Click **Configure a new mapper**
6. Select **Audience**
7. Configure:

| Setting | Value |
|---------|-------|
| Name | `obot-audience` |
| Included Client Audience | Select your client (`obot`) |
| Included Custom Audience | (leave empty) |
| Add to ID token | ON |
| Add to access token | ON |

Click **Save**

## Step 6: Obot Environment Configuration

Configure these environment variables for the Obot Keycloak auth provider:

### Required Variables

```bash
# Your Keycloak client ID (from Step 1)
OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID=obot

# Your Keycloak client secret (from Step 1.5)
OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_SECRET=your-client-secret-here

# Your Keycloak base URL (without /realms/...)
OBOT_KEYCLOAK_AUTH_PROVIDER_URL=https://keycloak.example.com

# Your Keycloak realm name
OBOT_KEYCLOAK_AUTH_PROVIDER_REALM=your-realm-name

# Cookie encryption secret (base64-encoded, 16/24/32 bytes when decoded)
# Generate with: openssl rand -base64 32
OBOT_AUTH_PROVIDER_COOKIE_SECRET=your-base64-secret

# Allowed email domains (* for all, or comma-separated list)
OBOT_AUTH_PROVIDER_EMAIL_DOMAINS=*
```

### Optional Variables

```bash
# PostgreSQL session storage (recommended for production)
OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN=postgres://user:pass@host:5432/db

# Token refresh duration (default: 1h)
OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION=1h

# Restrict access to specific groups (comma-separated)
OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_GROUPS=obot-admins,obot-users

# Restrict access to specific roles (comma-separated)
OBOT_KEYCLOAK_AUTH_PROVIDER_ALLOWED_ROLES=obot-admin,obot-user
```

## Troubleshooting

### Token Validation Errors

If you see issuer validation errors:
- Verify your `OBOT_KEYCLOAK_AUTH_PROVIDER_URL` doesn't have a trailing slash
- Ensure the realm name is correct and case-sensitive
- The expected issuer format is: `https://keycloak.example.com/realms/your-realm`

### Groups Not Appearing

If groups aren't being passed to Obot:
1. Verify the `groups` client scope is attached to your client as a **Default** scope
2. Check that the Group Membership mapper has "Add to ID token" enabled
3. Ensure users are actually assigned to groups in Keycloak
4. Check the token contents using Keycloak's built-in token debugger or jwt.io

### "Invalid redirect URI" Error

- Ensure `https://your-obot-url/*` is in the Valid redirect URIs
- Check for protocol mismatches (http vs https)
- Verify there are no trailing slashes causing issues

### Client Authentication Failed

- Verify the client secret matches exactly
- Ensure "Client authentication" is enabled (confidential client)
- Check that the client isn't disabled

## Security Best Practices

1. **Use HTTPS**: Always use HTTPS for both Keycloak and Obot in production

2. **Restrict Redirect URIs**: Use specific paths instead of wildcards where possible:
   ```
   https://obot.example.com/oauth2/callback
   ```

3. **Token Lifetimes**: Configure appropriate token lifetimes in Keycloak:
   - Realm Settings → Tokens
   - Set reasonable values for Access Token Lifespan (e.g., 5 minutes)
   - Set SSO Session Idle/Max appropriately

4. **Use Groups or Roles**: Implement proper access control using groups or roles rather than allowing all authenticated users

5. **Regular Secret Rotation**: Periodically rotate the client secret and cookie secret

## Verifying the Configuration

### Test Token Contents

1. In Keycloak Admin Console, go to **Clients** → your client
2. Go to **Client scopes** tab
3. Click **Evaluate**
4. Select a user and click **Evaluate**
5. Check the **Generated ID token** to verify:
   - `sub` claim is present (user identifier)
   - `email` claim is present
   - `preferred_username` claim is present
   - `groups` claim is present (if configured)
   - `aud` claim includes your client ID

### Test Login Flow

1. Start Obot with the Keycloak provider configured
2. Navigate to Obot and initiate login
3. You should be redirected to Keycloak's login page
4. After authentication, you should be redirected back to Obot
5. Verify your user information appears correctly in Obot

## Reference: Token Claims Used by Obot

| Claim | Purpose | Required |
|-------|---------|----------|
| `sub` | Unique user identifier | Yes |
| `email` | User's email address | Yes |
| `preferred_username` | Display name / username | No (falls back to email) |
| `name` | Full name | No |
| `groups` | Group memberships | No (for group filtering) |
| `realm_access.roles` | Realm role assignments | No (for role filtering) |
