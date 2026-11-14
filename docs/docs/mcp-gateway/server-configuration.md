---
title: "MCP server configuration"
---

Use this page for configuration shared by multiple MCP server runtimes. Start with [Add remote MCP servers](./register-remote.md) or [Add hosted MCP servers](../concepts/mcp-hosting.md) to create a catalog entry. Each setting below describes which runtimes support it.

## Static OAuth

Some remote MCP servers require a pre-registered OAuth application instead of dynamic client registration. Configure the application's client credentials on the remote server's catalog entry. Users connect through a vMCP and authenticate individually with the upstream provider.

1. Register an OAuth application with the provider. Set its callback URL to `https://<your-obot-host>/oauth/mcp/callback` and record its client ID and, for a confidential client, its client secret.
2. Create or edit the remote catalog entry under **MCP Servers**, enter the remote URL, enable **Static OAuth**, and save.
3. Open **Configure OAuth Credentials**, enter the client ID and any required client secret, and save.
4. Add the configured server to a vMCP and connect your AI client to that vMCP.

### Credential status and rotation

**Configure OAuth Credentials** shows whether credentials are configured and displays the client ID. The client secret is not returned after it is saved. Public OAuth clients do not require a client secret; confidential clients do.

To rotate the client credentials:

1. Open **Configure OAuth Credentials** on the catalog entry.
2. Click **Clear Credentials** and confirm.
3. Reopen the dialog, enter the replacement client ID and any required client secret, and save.

Users cannot connect to the upstream server until replacement credentials are configured. Remote catalog entries requiring static OAuth are hidden from non-admin users until their credentials are configured. Afterward, the usual access policies still determine who can use them through a vMCP.

## Caller attribution headers

Remote and containerized MCP servers can receive selected information about the caller authenticated by Obot. This is useful when a backend uses shared credentials but needs to record who performed an action.

For containerized servers, injection works on both Docker and Kubernetes. NPX, UVX, and OpenAPI runtimes do not support this setting.

For entries managed in Obot, edit the catalog entry or server, open **Advanced Configuration**, enable **Inject Obot headers**, turn on the individual headers to share, and save. For Git-managed entries, configure [`injectObotHeaders`](../configuration/mcp-server-gitops.md#caller-attribution-headers) in the catalog file. All selections are optional and disabled by default.

| Header | Value |
| --- | --- |
| `X-Obot-Api-Key-Name` | Name of the Obot API key used for the request. Omitted for unnamed keys and callers without an API key. |
| `X-Obot-Api-Key-Id` | API-key ID within this Obot installation. Omitted for callers without an API key. |
| `X-Obot-User-Id` | Authenticated caller's Obot account ID. For API keys, identifies the owning account. |
| `X-Obot-User-Name` | Account username, which can change over time. This is not the display name. |
| `X-Obot-User-Email` | Email carried by the authenticated account, when available. This does not guarantee that the address is verified. |
| `X-Obot-IDP-User-Id` | User ID assigned by the authentication provider. Omitted when unavailable or ambiguous across linked identities. |
| `X-Obot-IDP-Groups` | Comma-separated list of stored IdP group IDs, including provider prefixes, sorted and deduplicated. Excludes Obot roles and group display names. Omitted when empty. |

For API-key requests, the user and IdP fields describe the key owner, not an autonomous agent. Group memberships reflect Obot's stored provider or SCIM data. They are not a live query to the identity provider. Names, email addresses, and group memberships can disclose internal information, so select only what the destination needs.

For example, the groups header might contain:

```http
X-Obot-IDP-Groups: okta:admins,okta:engineering
```

Group IDs containing commas or quotes use CSV quoting. For example, the two IDs `okta:team,west` and `okta:users` are sent as `"okta:team,west",okta:users`. Backends should use a CSV parser if group IDs can contain these characters.

Obot removes incoming copies of all seven headers and supplies only the selected values. These names are reserved and cannot be configured as static or per-user headers. Unavailable values, values containing invalid HTTP header characters, and values exceeding 512 Unicode characters are omitted. An oversized groups list is omitted in full rather than truncated.

Backends should accept attribution only through trusted Obot gateway traffic. API-key names are mutable and not necessarily unique; use IDs when you need stable attribution.
