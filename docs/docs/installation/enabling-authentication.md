# Enabling Authentication

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

This guide covers the step-by-step process to enable and configure authentication in Obot. Authentication must be setup to use one of the external providers in order to function properly. The bootstrap user is not implemented to operate as a regular user.

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
