# Auth Providers

Authentication providers allow your Obot installation to authenticate users with the identity provider of your choice.
Administrators must configure at least one authentication provider before users can log in.
Multiple providers can be configured and available for login at the same time.

:::note
In order for authentication to be enabled, the Obot server must be run with `--enable-authentication` or
`OBOT_SERVER_ENABLE_AUTHENTICATION=true`.
:::

## Setting up Authentication

### Bootstrap Token

When launching Obot for the first time, the server will print a randomly generated bootstrap token to the console.

:::info
When installing via Helm, this token is saved inside a kubernetes secret.

This token can be used to authenticate as an admin user in the UI.
You will then be able to configure authentication providers.
Once you have configured at least one authentication provider, and have granted admin access to at least one user,
the bootstrap token will no longer be valid.

:::tip Custom Bootstrap Token
You can use the `OBOT_BOOTSTRAP_TOKEN` environment variable to provide a specific value for the token,
rather than having one generated for you. If you do this, the value will **not** be printed to the console.

Obot will persist the value of the bootstrap token on its first launch (whether randomly generated or
supplied by `OBOT_BOOTSTRAP_TOKEN`), and all future server launches will use that same value.
`OBOT_BOOTSTRAP_TOKEN` can always be used to override the stored value.
:::

### Preconfiguring Admin Users

If you want to preconfigure admin users, you can set the `OBOT_SERVER_AUTH_ADMIN_EMAILS` environment variable.
This is a comma-separated list of email addresses that will be granted admin access when they log in,
regardless of which auth provider they used.

Users can be given the administrator role by other admins in the Users section of the UI.
Users whose email addresses are in the `OBOT_SERVER_AUTH_ADMIN_EMAILS` list will automatically have the administrator role,
and the role cannot be revoked from them.

## Access Control

### Restricting Access by Email Domain

All authentication providers support restricting access to specific email domains, using the "Email Domains" field in the configuration UI.

You can:

- Use `*` to allow all email domains
- Specify a comma-separated list of domains to restrict access

**Example:** `example.com,example.org` would only allow users with email addresses ending in `example.com` or `example.org`.

## Available Auth Providers

Obot currently supports the following authentication providers (using OAuth2):

- **GitHub**
- **Google**

The code for these providers is available in the [Obot tools repo](https://github.com/obot-platform/tools).
