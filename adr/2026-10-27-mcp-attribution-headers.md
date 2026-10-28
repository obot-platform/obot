# 2026-10-27: Opt-in MCP caller attribution headers

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [Obot #8029](https://github.com/obot-platform/obot/issues/8029)

## Related ODPs

None.

## Context

An MCP backend using shared service credentials may need to attribute writes to
the caller authenticated by Obot. Forwarding caller metadata by default would
disclose internal account information and API-key names to existing backends.

## Decision

Remote and containerized MCP Server and Catalog Entry manifests expose an
optional `injectObotHeaders` list. Each destination independently selects any of:

| Header | Value |
| --- | --- |
| `X-Obot-Api-Key-Name` | Raw name of the validated API key; omitted for unnamed keys and non-key callers. Names are mutable, user-controlled, and not necessarily unique. |
| `X-Obot-Api-Key-Id` | Decimal ID of the validated API key within this Obot installation; omitted for non-key callers. |
| `X-Obot-User-Id` | Caller account ID; for API keys, the owning account, not an autonomous agent. |
| `X-Obot-User-Name` | Authenticated account username, not display name; mutable. |
| `X-Obot-User-Email` | Email carried by the authenticated principal, when available; not an assertion that the address is verified. |

Empty selections disclose nothing. Incoming copies of all five headers are
always stripped, and configured static or per-user headers cannot override
gateway-derived values. Unknown names, duplicate selections, and selections on
unsupported runtimes are rejected. Header values are derived for each request.

vMCP preserves API-key attribution in signed internal tokens. Each component
decides what to disclose to its own backend, irrespective of the parent vMCP.
Raw key names are kept separately from the masked names used by audit logs.
Each component MCP message forwards the current request's signed loopback token,
so pooled connections do not retain stale caller attribution. Background SSE
reconnects keep the OAuth handler's refreshable connection credential. Caller
metadata does not participate in discovery cache keys, and redirects to another
origin never receive an internal gateway credential.
Changing the header selection does not require restarting a hosted backend.

## Rationale

Individual selection allows a backend to receive only the information it needs.
Authenticated internal claims preserve attribution through vMCP without trusting
client-supplied headers or forwarding Obot credentials to a backend.

## Consequences

Backends must restrict access to trusted gateway traffic before relying on these
headers. Existing clients cannot forward their own values under the five reserved
names, including when injection is disabled. Key names provide attribution, not
verified autonomous-agent identity. No request/session correlation fields,
STDIO environment mapping, or downstream OpenAPI REST forwarding are introduced.

## References

- `apiclient/types/obotheaders.go`
- `pkg/api/handlers/mcpgateway/obotheaders.go`
- `pkg/jwt/persistent/persistent.go`
