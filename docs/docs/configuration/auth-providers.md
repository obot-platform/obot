# Auth Providers

Authentication providers allow your Obot installation to authenticate users with the identity provider of your choice.
Administrators must configure at least one authentication provider before users can log in.
Multiple providers can be configured and available for login at the same time.

:::note
In order for authentication to be enabled, the Obot server must be run with the environment variable set:

`OBOT_SERVER_ENABLE_AUTHENTICATION=true`.
:::

## Setting up Authentication

### Bootstrap Token

When launching Obot for the first time, the server will print a randomly generated bootstrap token to the console.

:::info
When installing via Helm, this token is saved inside a kubernetes secret `<helm install name>-config`.
:::

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

### Preconfiguring Owner & Admin Users

If you want to preconfigure owner or admin users, you can set the `OBOT_SERVER_AUTH_OWNER_EMAILS` or `OBOT_SERVER_AUTH_ADMIN_EMAILS` environment variable, respectively.
This is a comma-separated list of email addresses that will be granted owner or admin access when they log in,
regardless of which auth provider they used.

Users can be given the administrator role by other owners or admins in the Users section of the UI.
Users whose email addresses are in configured list will automatically have the administrator role,
and the role cannot be revoked from them.

Similarly, users can be given the owner role by other owners in the Users section of the UI.
Users whose email addresses are in configured list will automatically have the owner role,
and the role cannot be revoked from them.

## Access Control

### Restricting Access by Email Domain

All authentication providers support restricting access to specific email domains, using the "Email Domains" field in the configuration UI.

You can:

- Use `*` to allow all email domains
- Specify a comma-separated list of domains to restrict access

**Example:** `example.com,example.org` would only allow users with email addresses ending in `example.com` or `example.org`.

## Available Auth Providers

Obot currently supports the following authentication providers (using OAuth2). Before getting started you will need to follow the instructions in the auth provider for setting up a new app. You can get the callback URL from the Obot Admin -> Auth Providers -> \<Auth Provider> -> Configure page. The configuration form will also have fields for the data required.

### GitHub

You will need to create an OAuth App in GitHub following these [instructions](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app).

You can view the source code for GitHub provider in this [repo](https://github.com/obot-platform/tools).

### Google

Follow the instructions [here](https://developers.google.com/identity/protocols/oauth2/web-server#creatingcred) to create the OAuth app for Obot.

You can view the source code for Google provider in this [repo](https://github.com/obot-platform/tools).

### Entra (Enterprise Only)

Within the [Microsoft Entra admin center](https://entra.microsoft.com), go to App registrations and click New registration.

Under Supported account types, ensure `Accounts in this organizational directory only` is selected. In the Redirect URI section, set the platform to `Web` and enter the redirect URI provided in Obot's Auth Provider configuration dialog.

![screenshot of Entra App registration](/img/entra-app-registration.png)

After completing the form, click Register.

Next, go to the API permissions tab and add the following delegated permissions:

- `User.Read`
- `GroupMember.Read.All` *requires admin approval*
- `ProfilePhoto.Read.All` *requires admin approval*

After all permissions are approved, your App's Configured permissions section should look something like this:

![screenshot of Entra configured permissions](/img/entra-configured-permissions.png)

Head to the Certificates & secrets tab and click New Client secret.
Select a desired expiration date and click `Add`.

Copy the exposed secret from the `Value` column to a safe location. You will not be able to retrieve the secret value after this point.

![screenshot of Entra client secret](/img/entra-client-secret.png)

Finally, navigate to the Overview tab and copy the values of `Application (client) ID` and `Directory (tenant) ID` for reference.

You can now return to Obot and finish configuring Entra. Use the table below to determine the values to use for each field:

| Obot          | Entra                   | Entra App Tab          |
|---------------|-------------------------|------------------------|
| Client ID     | Application (client) ID | Overview               |
| Client Secret | Secret `Value` column   | Certificates & secrets |
| Tenant ID     | Directory (tenant) ID   | Overview               |


### Okta (Enterprise Only)

Create an OAuth app in Okta following these [instructions](https://developer.okta.com/docs/guides/implement-oauth-for-okta/main/#create-an-oauth-2-0-app-in-okta).

Once your app is created, go to the Sign On tab and scroll to the OpenID Connect ID Token section. Click on Edit.
Then, set the Group Claims Type to `Filter`, and the Groups Claim Filter to `groups Matches regex .*`. It should look like this:

![screenshot of Okta groups claim settings](/img/okta-group-claims.png)

Next, click on the Okta API Scopes tab. Scroll down and find `okta.groups.read`, then click the `✓ Grant` button.

When configuring the Okta auth provider in Obot, be sure to set the Issuer URL to the Issuer URL for your org-level authorization server.
Obot does not work with custom authorization servers in Okta, because they are unable to support the `okta.groups.read` scope.
Typically, the Issuer URL for your org-level authorization server is simply the URL for your Okta workspace itself, with no path.
