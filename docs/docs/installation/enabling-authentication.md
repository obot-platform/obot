# Enabling Authentication

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

This guide covers the step-by-step process to enable and configure authentication in Obot. You can use the built-in Local provider or configure an external identity provider. The bootstrap user is only for initial configuration and is not intended to operate as a regular user.

:::note
If any MCP servers were created with authentication disabled, they will be deleted when authentication is enabled.
:::

## Step 1: Set Environment Variables

Enabling authentication begins with launching Obot with additional configuration options in the form of environment variables. See the [Docker](./docker-deployment.md) or [Kubernetes](./kubernetes-deployment.md) deployment guides for full setup details.

<Tabs>
  <TabItem value="docker" label="Docker" default>

```bash
docker run \
  ... # other flags
  -e OBOT_SERVER_ENABLE_AUTHENTICATION=true \
  -e OBOT_BOOTSTRAP_TOKEN=your-secret-token \
  -e OBOT_SERVER_AUTH_OWNER_EMAILS=owner@company.com \
  ghcr.io/obot-platform/obot:latest
```

  </TabItem>
  <TabItem value="kubernetes" label="Kubernetes">

```yaml
config:
  # Required: Enable authentication
  OBOT_SERVER_ENABLE_AUTHENTICATION: "true"

  # Required: Set a bootstrap token for initial login
  OBOT_BOOTSTRAP_TOKEN: "your-secret-token"

  # Required: Set the owner email (can also be configured in the UI later)
  OBOT_SERVER_AUTH_OWNER_EMAILS: "owner@company.com"

  # Optional: Set additional admin emails
  OBOT_SERVER_AUTH_ADMIN_EMAILS: "admin1@company.com,admin2@company.com"
```

  </TabItem>
</Tabs>

### Provision an initial local owner with a secure setup link

Automated provisioning systems can bypass the bootstrap-token and provider-configuration flow. Generate a different high-entropy setup secret for every Obot environment, then launch Obot with the owner's email and that secret:

```bash
OWNER_SETUP_TOKEN="$(openssl rand -hex 32)"

docker run -d \
  --name obot \
  -p 8080:8080 \
  -v obot-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e OBOT_SERVER_ENABLE_AUTHENTICATION=true \
  -e OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL=owner@example.com \
  -e OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN="$OWNER_SETUP_TOKEN" \
  ghcr.io/obot-platform/obot:latest
```

Send the owner a link built by the provisioning system:

```text
https://your-obot-host/activate#token=<OWNER_SETUP_TOKEN>
```

The URL fragment is not sent in HTTP requests, so the secret does not appear in proxy or server access logs. The UI immediately removes it from browser history and exchanges it for a restricted, HTTP-only session. The session can only read its own profile, set its password, or sign out; the backend rejects access to every other Obot API and page until setup is complete.

The setup link remains usable for 168 hours by default and can be reopened if setup is interrupted. The password page also offers **Finish later**, which signs out without consuming the link. The first successful password set revokes the link and all other setup sessions. Obot stores only a SHA-256 hash of the high-entropy setup secret.

The expiration is absolute: restarting with the same secret does not extend or revive the link. If a pending link expires or may have been disclosed, configure a newly generated secret, restart all replicas with that same value, and send the replacement link. Rotation never resets a completed owner's password. In a multi-replica deployment, use a coordinated rollout so a replica with a stale Secret value cannot rotate the database back to an older link.

For Helm, put the email under `config` and the setup secret under `secret`:

```yaml
config:
  OBOT_SERVER_ENABLE_AUTHENTICATION: "true"
  OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL: "owner@example.com"
  OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN_EXPIRATION_HOURS: "168"
secret:
  OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN: "<random-setup-secret>"
```

When these values are present, Obot configures the Local provider for the owner's email domain, creates the owner account exactly once, and disables bootstrap-token generation and login.

| Variable | Required | Description |
|----------|----------|-------------|
| `OBOT_SERVER_ENABLE_AUTHENTICATION` | Yes | Enables authentication |
| `OBOT_BOOTSTRAP_TOKEN` | No | Token used for bootstrap login while no auth provider is configured or no non-bootstrap owner user exists. If not set, a token will be generated and printed to the logs. |
| `OBOT_SERVER_AUTH_OWNER_EMAILS` | No | Email address that will have owner access after logging in via the auth provider. If not set, the bootstrap user will be prompted to log in via the auth provider and set themselves as the owner. |
| `OBOT_SERVER_AUTH_ADMIN_EMAILS` | No | Additional email addresses that will have admin access |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL` | No | Initial local-auth owner's email. Must be set with the setup token. |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN` | No | At least 32 characters of high-entropy, randomly generated secret material used to activate the initial owner. Store as a secret; `openssl rand -hex 32` is the recommended generator. |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN_EXPIRATION_HOURS` | No | Setup-link validity in hours. Defaults to `168`. |

## Step 2: Start Obot and Login

Start (or restart) your Obot deployment with the new environment variables. Navigate to your Obot installation and use the bootstrap token to login. You'll now see User Management options enabled in the left navigation.

## Step 3: Configure Authentication Provider

1. Go to **Auth Providers** under the **User Management** section in the left navigation
2. Click **Configure** on your desired provider (GitHub, Google, Entra, Okta)
3. Follow the provider-specific configuration steps

For detailed provider configuration, see the [Auth Providers](../configuration/auth-providers.md) documentation.

## Post-Setup

Once you have configured an authentication provider:

1. Users can login using the configured authentication provider
2. Users with emails matching `OBOT_SERVER_AUTH_OWNER_EMAILS` will have owner access
3. Users with emails matching `OBOT_SERVER_AUTH_ADMIN_EMAILS` will have admin access

Note that you can always assign the owner or admin role to additional users through the User pages.

## Troubleshooting

### Bootstrap Token Not Working

- Ensure `OBOT_SERVER_ENABLE_AUTHENTICATION=true` is set
- Check that you're using the correct token
- If an auth provider has already been configured and a non-bootstrap owner user exists, set `OBOT_SERVER_FORCE_ENABLE_BOOTSTRAP=true` to re-enable bootstrap login

### Authentication Provider Issues

- Verify callback URLs match between Obot and your OAuth provider
- Check that client ID and secret are correct
- Ensure proper scopes and permissions are configured

## Next Steps

- Review [Auth Providers configuration](../configuration/auth-providers.md) for detailed provider setup
