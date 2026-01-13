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
regardless of which auth provider they used. Email matching is case-insensitive.

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

Next, go to the API permissions tab and add the following **delegated** permissions:

- `User.Read`
- `ProfilePhoto.Read.All` *requires admin approval*

Then add the following **application** permissions:

- `GroupMember.Read.All` *requires admin approval*
- `User.Read.All` *requires admin approval*

After all permissions are approved, your App's Configured permissions section should look something like this:

![screenshot of Entra configured permissions](/img/entra-new-configured-permissions.png)

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

#### Restricting Login to Specific Users and Groups (Optional)

You can restrict login access to specific Entra users and groups by taking the following steps:

1. Navigate to the [Microsoft Entra admin center](https://entra.microsoft.com)
2. Select `Enterprise apps`, then select `All applications`
3. Select the application corresponding to your Obot instance
4. On the application's `Overview` page, under the `Manage` section, select `Properties`
5. Toggle the `Assignment required?` setting to `Yes`
6. Select `Save` in the top bar
7. Under `Manage`, select `Users and Groups` and then select `Add user/group`
8. Search for and add the desired Entra users and groups to the application

For more details, [see Entra's docs](https://learn.microsoft.com/en-us/entra/identity-platform/howto-restrict-your-app-to-a-set-of-users).

:::tip Security Features
The Entra ID auth provider includes production-grade security features including fail-fast authentication, HTTPS enforcement with cookie security configuration, automatic token refresh error handling, and group metadata validation. For comprehensive configuration details and advanced security options, see [Microsoft Entra ID Authentication](/next/configuration/entra-id-authentication).
:::

### Keycloak (Enterprise Only)

Keycloak is an open-source identity and access management solution that provides enterprise-grade authentication and authorization.

To configure Keycloak authentication:

1. Create a new OpenID Connect client in your Keycloak realm
2. Set **Client authentication** to ON (confidential client)
3. Configure the **Valid redirect URIs** with the callback URL from Obot's Auth Provider configuration dialog
4. Note the **Client ID** and **Client secret** from the Credentials tab
5. (Optional) Enable **Service accounts roles** and assign `view-users` and `view-realm` roles for comprehensive user metadata

You can configure role-based and group-based access control:

| Obot Field | Description |
|------------|-------------|
| Client ID | The Client ID from Keycloak |
| Client Secret | The Client secret from the Credentials tab |
| Keycloak URL | Base URL of your Keycloak server (e.g., `https://keycloak.example.com`) |
| Realm | Keycloak realm name |
| Allowed Roles | (Optional) Comma-separated list of role names allowed to authenticate |
| Allowed Groups | (Optional) Comma-separated list of group names allowed to authenticate |

:::tip Security Features
The Keycloak auth provider includes production-grade security features including fail-fast authentication, HTTPS enforcement with cookie security configuration, automatic token refresh error handling, role and group-based access control, and Admin API integration for comprehensive user metadata. For comprehensive configuration details and advanced security options, see [Keycloak Authentication](/next/configuration/keycloak-authentication).
:::

### Okta (Enterprise Only)

:::note
Only the org-level authorization server is supported (no custom authorization servers).
:::

Create an OAuth app of type `OIDC` in Okta following these [instructions](https://developer.okta.com/docs/guides/implement-oauth-for-okta/main/#create-an-oauth-2-0-app-in-okta).
Make sure to add the redirect URL displayed in Obot to the list of Sign-in redirect URIs in your Okta application settings.

Take note of the Client ID and Client Secret. You will need to provide these to Obot.

Next, create another OAuth app, this time of type `API Services`. Grant it the API scopes `okta.users.read` and `okta.groups.read`.
Assign an administrator role to this app that has, at a minimum, `okta.users.read` and `okta.groups.read` permissions.
You can use an existing administrator role (such as Read-Only Administrator), or create a custom one.

Next, change the client authentication method on your API Services app from `Client secret` to `Public key / Private key`.
Then, add a new key to your app. Ensure that your app has only one key in total. You can generate the key on your own,
or have Okta do it for you. Make sure you save the private key in PEM format, as you will need to provide that to Obot,
in addition to the Client ID for this app. When you are done with that, the `Client Credentials` section should look similar to this:

![Okta API Services App Client Credentials configuration](/img/okta-client-credentials.png)

#### Restricting Login to Specific Users and Groups (Optional)

You can restrict login access to specific Okta users and groups by taking the following steps:

1. While creating the `OIDC` App for Obot, select the `Skip group assignment for now` option in the `Assignments` configuration section
2. After the App is created, go to the App's configuration page
3. In the `Assignments` tab, click on the `Assign` button and select `Assign to Groups` in the dropdown menu
4. Select `Assign` on groups you want to allow Obot access to
5. Once you've made your selections, click `Done`
