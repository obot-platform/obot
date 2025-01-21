# Auth Providers

Authentication providers allow your Obot installation to authenticate users with the identity provider of your choice.
Administrators must configure at least one authentication provider before users can log in.
Multiple providers can be configured and available for login at the same time.

:::note
In order for authentication to be enabled, the Obot server must be run with `--enable-authentication` or
`OBOT_SERVER_ENABLE_AUTHENTICATION=true`.
:::

## Setting up

When launching Obot for the first time, the server will print a randomly generated bootstrap token to the console.
This token can be used to authenticate as an admin user in the UI.
You will then be able to configure authentication providers.

:::tip
You can use the `OBOT_BOOTSTRAP_TOKEN` environment variable to provide a specific value for the token,
rather than having the server generate one for you. If you do this, the value will **not** be printed to the console.
:::

### Preconfiguring admin users

If you want to preconfigure admin users, you can set the `OBOT_SERVER_AUTH_ADMIN_EMAILS` environment variable.
This is a comma-separated list of email addresses that will be granted admin access when they log in,
regardless of which auth provider they used.

Users can be given the administrator role by other admins in the Users section of the UI.
Users whose email addresses are in the `OBOT_SERVER_AUTH_ADMIN_EMAILS` list will automatically have the administrator role,
and the role cannot be revoked from them.

## Restricting access to specific email domains

All authentication providers support restricting access to specific email domains, using the "Email Domains" field in the configuration UI.
You can use the value `*` to allow all email domains, or a comma-separated list of domains to restrict access to those domains.
For example, `example.com,example.org` would only allow users with email addresses ending in `example.com` or `example.org`.

## Available Auth Providers

Obot currently supports the following authentication providers (using OAuth2):
- GitHub
- Google

The code for these providers is available in the [Obot tools repo](https://github.com/obot-platform/tools).

## Disabling the Bootstrap User

If you do not want to be able to log in as the bootstrap user, you can set the `OBOT_SERVER_ENABLE_BOOTSTRAP_USER` environment variable to `false`.
This will prevent you from logging in as the bootstrap user.
When you first run Obot, do not set this environment variable, because you need to be able to use the bootstrap user to set up the first auth provider.
Once that is done, you can restart the server and set this environment variable if you would like.
