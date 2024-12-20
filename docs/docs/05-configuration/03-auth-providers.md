# Auth Providers

Authentication providers allow your Obot installation to authenticate users with the identity provider of your choice. All authentication providers are configured using environment variables.

## Common Environment Variables

The following environment variables are required for all authentication providers.
Setting the Client ID and Client Secret will mean that the authentication provider is enabled.
The remaining configuration will be validated on startup.

- `OBOT_SERVER_AUTH_CLIENT_ID`: The client ID of the authentication provider.
- `OBOT_SERVER_AUTH_CLIENT_SECRET`: The client secret of the authentication provider.
- `OBOT_SERVER_AUTH_COOKIE_SECRET`: The secret used to encrypt the authentication cookie. Must be of size 16, 24, or 32 bytes.
- `OBOT_SERVER_AUTH_ADMIN_EMAILS`: A comma-separated list of the email addresses of the admin users.

The following environment variables are optional for all authentication providers:
- `OBOT_SERVER_AUTH_EMAIL_DOMAINS`: A comma-separated list of email domains allowed for authentication. Ignored if not set.
- `OBOT_SERVER_AUTH_CONFIG_TYPE`: The type of the authentication provider. For example, `google` or `github`. Defaults to `google`.

## Google

Google is the default authentication provider. There are currently no additional environment variables required for Google authentication.

## GitHub

GitHub authentication has the following optional configuration:

- `OBOT_SERVER_AUTH_GITHUB_ORG`: The name of the organization allowed for authentication. Ignored if not set.
- `OBOT_SERVER_AUTH_GITHUB_TEAM`: The name of the team allowed for authentication. Ignored if not set.
- `OBOT_SERVER_AUTH_GITHUB_REPO`: Restrict logins to collaborators of this repository formatted as `orgname/repo`. Ignored if not set.
- `OBOT_SERVER_AUTH_GITHUB_TOKEN`: The token to use when verifying repository collaborators (must have push access to the repository).
- `OBOT_SERVER_AUTH_GITHUB_ALLOW_USERS`: A comma-separated list of users allowed to log in even if they don't belong to the organization or team.