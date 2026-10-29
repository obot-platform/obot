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
| `X-Obot-IDP-User-Id` | Provider-assigned user ID read from the caller account's stored identity. The request's provider selects the identity when available; otherwise exactly one linked identity must exist. Omitted when missing or ambiguous. For API keys, identifies the key owner. |
| `X-Obot-IDP-Groups` | JSON array of the caller's IdP group IDs stored by Obot, including provider prefixes, sorted and deduplicated. Uses the authenticated principal's `auth_provider_groups`, which are refreshed from stored memberships during token/API-key authentication. Excludes Obot roles and display names; omitted when empty. For API keys, describes the key owner. |

Empty selections disclose nothing. Incoming copies of all seven headers are
always stripped, and configured static or per-user headers cannot override
gateway-derived values. Unknown names, duplicate selections, and selections on
unsupported runtimes are rejected. Header values are derived for each request.

The `pkg/mcp/attribution` package owns stripping, caller-value derivation, and
filtering configured headers. The gateway strips incoming attribution once at
entry, before hooks, auditing, and forwarding. Client setup, readiness probes,
and composite passthrough copy only filtered configuration, preserving any
trusted attribution already derived for the request.

vMCP preserves API-key attribution in signed internal tokens. Each component
decides what to disclose to its own backend, irrespective of the parent vMCP.
Raw key names are kept separately from the masked names used by audit logs.
Before token signing, raw key names exceeding 512 Unicode characters are
omitted, and oversized audit names use the masked key identifier. This applies
to initial tokens and refreshes, including when header injection is disabled,
so API-key names cannot inflate loopback Authorization headers without bound.
Each component MCP message forwards the current request's signed loopback token,
so pooled connections do not retain stale caller attribution. Background responses
to server-initiated requests, SSE reconnects, and session cleanup keep the OAuth
handler's refreshable connection credential. Caller
metadata does not participate in discovery cache keys, and redirects to another
origin never receive an internal gateway credential.
Changing the header selection does not require restarting a hosted backend.

## Rationale

Individual selection allows a backend to receive only the information it needs.
Authenticated internal claims preserve attribution through vMCP without trusting
client-supplied headers or forwarding Obot credentials to a backend.

## Consequences

Backends must restrict access to trusted gateway traffic before relying on these
headers. Existing clients cannot forward their own values under the seven reserved
names, including when injection is disabled. Key names provide attribution, not
verified autonomous-agent identity. No request/session correlation fields,
STDIO environment mapping, or downstream OpenAPI REST forwarding are introduced.

IdP attribution is optional personal and organizational metadata. Group IDs can
reveal the backing provider through their prefixes. Only selected destinations
receive these values. Group membership follows Obot's existing provider/SCIM
synchronization and caching; it is not a live IdP query or an authorization grant.
The IdP user ID is looked up only when selected. Lookup failures fail the request
rather than silently forwarding incomplete attribution. Composite tokens retain
the request's provider reference, while component requests read current stored
identities and memberships; caller values never affect discovery cache keys.

Selected attribution values are omitted when they contain invalid HTTP header
characters or exceed 512 Unicode characters per header value. Omission logs
contain the header name and reason, never its value. Values and group lists are never truncated. Backends must
handle missing metadata explicitly. Identity lookup failures still fail the request.
New header configuration using a reserved name is rejected during validation.
Storage migrations validate configuration structure separately and preserve
legacy reserved headers so existing configuration cannot block startup.

## References

- `apiclient/types/obotheaders.go`
- `pkg/mcp/attribution/headers.go`
- `pkg/jwt/persistent/persistent.go`
