---
title: "Add remote MCP servers"
---

Register an existing HTTP MCP endpoint when the server runs outside Obot. Obot proxies the connection; it does not deploy or sandbox that remote service.

1. Open **MCP Servers**, select **Add MCP Server**, and choose **Remote Server**.
2. Enter a name, short description, and the server's MCP URL.
3. Configure required headers or user-supplied values. A remote service must be reachable from Obot, or through a configured [MCP tunnel](./filters-tunnels.md#mcp-tunnels-create-an-mcp-tunnel).
4. If the provider requires a pre-registered OAuth application, enable **Static OAuth** and configure its client credentials using [MCP Server OAuth Configuration](../configuration/mcp-server-oauth-configuration.md).
5. Save the entry and review catalog access policies.
6. Add the entry to a [vMCP](./server-types.md), configure its tools and profiles, and test the resulting connection.

For a complete provider-specific setup, see the [Slack MCP tutorial](../configuration/tutorials/slack-mcp-server.md).

Obot blocks localhost, private-IP, and link-local remote destinations by default. Use a tunnel for private services when appropriate. Do not disable these protections globally just to make an arbitrary endpoint connect. See [Network isolation](../security/isolation.md) for the separate gateway and workload boundaries. A remote server's URL is refused when it resolves to such an address; set `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP=false` (or the `LOCALHOST` and `LINK_LOCAL` equivalents) only when that is intended. Guardrail services are different: they may be on a private network, as described in [Guardrails](../functionality/guardrails.md).

## Remote server {#mcp-servers-remote-server}

MCP Servers that are HTTP Streaming compatible should be configured this way. These servers can be provided by trusted 3rd party vendors. Remote servers also work for MCP servers deployed through existing CI/CD pipeline within the organization.

Choose this type when:

- You have MCP services deployed through traditional application deployment mechanisms
- External partners provide MCP endpoints and you just want to integrate
- You are building MCP servers through existing CI/CD workflows or SaaS services

Remote MCP servers that conform to the MCP spec authentication schema will work out of the box. Servers that do not conform to the spec may not work within the gateway. Please open a GitHub issue if you run into issues with remote servers.

**Configuration**: Specify the remote URL endpoint. Additional options include connection restrictions for unconventional configurations, custom HTTP headers, and configuration values to send to the remote server.

If Obot cannot directly reach a remote server, use an [MCP tunnel](./filters-tunnels.md#mcp-tunnels-create-an-mcp-tunnel) to route requests through a machine on the server's network. Keep the remote server's real HTTP or HTTPS URL and select the tunnel separately in **Advanced Configuration**.

## Caller attribution headers

Remote MCP servers can receive selected information about the caller authenticated by Obot. This is useful when a backend uses shared credentials but needs to record who performed an action.

For entries managed in Obot, edit the catalog entry or server, expand **Inject Obot headers**, select the values to share, and save. For Git-managed entries, configure [`injectObotHeaders`](../configuration/mcp-server-gitops.md#caller-attribution-headers) in the catalog file. All selections are optional and disabled by default.

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

After changing a catalog entry's selections, apply the catalog update to existing vMCP components that use it. They keep their saved catalog definition until an update is applied; see [Snapshots and updates](./publish.md#virtual-mcps-snapshots-and-updates).

Backends should accept attribution only through trusted Obot gateway traffic. API-key names are mutable and not necessarily unique; use IDs when you need stable attribution.

## Localhost OAuth callbacks

Some MCP servers require their OAuth clients to use a localhost callback. For these servers, enable **Localhost OAuth callback** in **Advanced Configuration** when adding or editing the remote server.

Leave **Callback path** blank to use `/oauth/callback`, or enter the path required by the server. The path must start with `/` and contain no query string or fragment. `/oauth/obot/callback` is reserved for Obot to notify the CLI that sign-in has finished; it cannot be used for the provider callback. The server must accept an available callback port; fixed ports are not supported.

During authentication, Obot displays an [`obot mcp login` command](../reference/cli-api.md#mcp-localhost-oauth-sign-in) to run on a computer with a web browser. MCP clients continue using their normal Obot HTTP connection URL.
