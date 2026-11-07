---
title: "User sign-in and identity"
---

Enable authentication before onboarding users or deploying servers for shared use.

1. Enable authentication using the [bootstrap setup procedure](../configuration/auth-providers.md#enabling-authentication-step-1-set-environment-variables), or use the [preconfigured Local owner](../installation/docker-deployment.md#preconfigure-a-local-owner) Docker setup.
2. Configure one [authentication provider](../configuration/auth-providers.md), including allowed email domains and provider-specific group lookup requirements.
3. Assign [roles](./policy-coverage.md) and resource policies for the users who sign in.
4. Use [agent authorization scopes](../functionality/agent-auth-scopes.md) for programmatic clients that need API keys.

Local authentication stores passwords as salted hashes and supports administrator-created accounts and required password changes. External providers authenticate through their configured OAuth/OIDC integration. Provider availability depends on [edition](../enterprise/overview.md).

Obot login and upstream MCP OAuth are separate authorizations. The [authentication flow](../concepts/architecture.md#authentication-flow) explains what the client receives and what Obot retains.

Before switching providers, read the [provider-switch procedure](../configuration/auth-providers.md#switching-between-auth-providers). Identities are provider-scoped; a new provider login is not an automatic transfer of the user's existing resources.

After enabling authentication, open **Identity & Access** to manage users, roles, agent identities, and providers.

## Users {#user-management-users}

Open **Identity & Access > Users**. From this page you can:

- See all registered users and their current roles
- Update individual user roles
- Monitor user activity

For details on updating roles, see [User Roles](./policy-coverage.md#user-roles-managing-user-roles).

## User Roles {#user-management-user-roles}

Open **Identity & Access > Roles** to configure the default role assigned to new users. Choose from:

- **Standard User**: Connect to approved MCP servers
- **Power User**: Standard User features plus publish personal MCP servers
- **Power User Plus**: Power User features plus share MCP servers through registries
- **Admin**: Full platform management

For detailed role descriptions and permissions, see [User Roles](./policy-coverage.md).

## Agent Authorization Scopes {#user-management-agent-authorization-scopes}

Open **Identity & Access > Agents** to view and manage agent identities and their authorization scopes. Administrators can see which users have created agent authorization scopes and delete any if necessary. For details, see [Agent Authorization Scopes](../functionality/agent-auth-scopes.md).

## Auth Providers {#user-management-auth-providers}

Configure identity providers for user authentication. See [Auth Providers](../configuration/auth-providers.md) for setup details.


## Authentication setup

Installation and provider configuration are covered in [Configure authentication providers](../configuration/auth-providers.md). The links below preserve existing setup bookmarks.

## Step 1: Set Environment Variables {#enabling-authentication-step-1-set-environment-variables}

Continue to [Step 1: Set Environment Variables](../configuration/auth-providers.md#enabling-authentication-step-1-set-environment-variables).

## Step 2: Start Obot and Login {#enabling-authentication-step-2-start-obot-and-login}

Continue to [Step 2: Start Obot and Login](../configuration/auth-providers.md#enabling-authentication-step-2-start-obot-and-login).

## Step 3: Configure Authentication Provider {#enabling-authentication-step-3-configure-authentication-provider}

Continue to [Step 3: Configure Authentication Provider](../configuration/auth-providers.md#enabling-authentication-step-3-configure-authentication-provider).

## Post-Setup {#enabling-authentication-post-setup}

Continue to [Post-Setup](../configuration/auth-providers.md#enabling-authentication-post-setup).

## Troubleshooting {#enabling-authentication-troubleshooting}

Continue to [Troubleshooting](../configuration/auth-providers.md#enabling-authentication-troubleshooting).

### Bootstrap Token Not Working {#enabling-authentication-bootstrap-token-not-working}

Continue to [Bootstrap Token Not Working](../configuration/auth-providers.md#enabling-authentication-bootstrap-token-not-working).

### Authentication Provider Issues {#enabling-authentication-authentication-provider-issues}

Continue to [Authentication Provider Issues](../configuration/auth-providers.md#enabling-authentication-authentication-provider-issues).

## Next Steps {#enabling-authentication-next-steps}

Continue to [Next Steps](../configuration/auth-providers.md#enabling-authentication-next-steps).
