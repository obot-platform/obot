# Enabling Authentication

This guide covers the step-by-step process to enable and configure authentication in Obot. Authentication must be setup to use one of the external providers in order to function properly. The bootstrap user is not implemented to operate as a regular user.

## Overview

By default, Obot runs without authentication in development mode. For production deployments, you'll need to:

1. Set the authentication environment variable
1. Login using the bootstrap token
1. Configure your authentication provider
1. Configure admins/owners
1. Restart the system

## Step 1: Enable Authentication

### Docker/Compose Deployment

Set the environment variable in your deployment:

```bash
OBOT_SERVER_ENABLE_AUTHENTICATION=true
```

### Kubernetes Deployment

Add the environment variable to your Helm values:

```yaml
config:
  OBOT_SERVER_ENABLE_AUTHENTICATION: "true"
```

## Step 2: Login with Bootstrap Token

When Obot starts with authentication enabled for the first time, it generates a bootstrap token that's printed to the console logs. 

### Finding the Bootstrap Token

**Docker/Compose:**

```bash
# Check the container logs
docker logs <container-name> 
```

**Kubernetes:**

```bash
# Check pod logs
kubectl logs <pod-name> 
```

### Using the Bootstrap Token

1. Navigate to your Obot installation
2. Use the bootstrap token to login as an admin user
3. You can now access the Admin interface to configure authentication

:::tip Custom Bootstrap Token
You can set a custom bootstrap token using the `OBOT_BOOTSTRAP_TOKEN` environment variable instead of using the auto-generated one.
:::

## Step 3: Configure Authentication Provider

Once logged in with the bootstrap token:

1. Go to **Admin** → **Auth Providers**
2. Click **Add Provider**
3. Select your desired provider (GitHub, Google, Entra, Okta)
4. Follow the provider-specific configuration steps

For detailed provider configuration, see the [Auth Providers](/configuration/auth-providers) documentation.

## Step 4: Set Admin/Owner Users and Restart

Logout of Obot and configure the following.

### Set Admin/Owner Environment Variables

**Docker/Compose:**

```bash
# Set admin users (comma-separated email addresses)
OBOT_SERVER_AUTH_ADMIN_EMAILS=admin1@company.com,admin2@company.com

# Set owner users (comma-separated email addresses)  
OBOT_SERVER_AUTH_OWNER_EMAILS=owner@company.com
```

**Kubernetes:**

```yaml
config:
  OBOT_SERVER_AUTH_ADMIN_EMAILS: "admin1@company.com,admin2@company.com"
  OBOT_SERVER_AUTH_OWNER_EMAILS: "owner@company.com"
```

### Restart Obot

After setting the environment variables, restart your Obot deployment:

**Docker/Compose:**

```bash
docker restart <container>
```

**Kubernetes:**

```bash
helm upgrade <release-name> <chart-name> -f values.yaml
```

## Post-Setup

After restart:

1. The bootstrap token will no longer be valid
2. Users can now login using the configured authentication provider
3. Users with emails matching `OBOT_SERVER_AUTH_ADMIN_EMAILS` will automatically have admin access
4. Users with emails matching `OBOT_SERVER_AUTH_OWNER_EMAILS` will automatically have owner access

## Troubleshooting

### Bootstrap Token Not Working

- Ensure `OBOT_SERVER_ENABLE_AUTHENTICATION=true` is set
- Check that you're using the correct token from the logs
- If Auth Provider has been configured, you need to set `OBOT_SERVER_FORCE_ENABLE_BOOTSTRAP=true`

### Authentication Provider Issues

- Verify callback URLs match between Obot and your OAuth provider
- Check that client ID and secret are correct
- Ensure proper scopes and permissions are configured

## Next Steps

- Review [Auth Providers configuration](/configuration/auth-providers) for detailed provider setup
- Configure [OAuth settings](../../configuration/oauth-configuration/) for additional customization
- Set up proper [access control](/configuration/auth-providers#access-control) with email domain restrictions
