# 2026-10-27: Opt-in MCP caller attribution headers

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [Implementation #8029](https://github.com/obot-platform/obot/issues/8029)
- [Release note #8402](https://github.com/obot-platform/obot/issues/8402)

## Related ODPs

None.

## Context

Remote MCP servers using shared credentials may need to attribute actions to the
caller authenticated by Obot. Sending caller metadata by default would disclose
account information to existing backends.

## Decision

Remote servers and catalog entries can opt into individual API-key, Obot-user,
and IdP attribution headers through `injectObotHeaders`. Each component controls
what its backend receives. Header definitions are documented in the
[remote server guide](../docs/docs/mcp-gateway/register-remote.md).

Obot strips client-supplied attribution and rejects reserved names in configured
headers. It derives selected values from the authenticated caller and stored
identities. Static and per-user configuration cannot override those values.

For vMCP requests, Obot derives the union of its remote components' selected
headers before the internal loopback. mmmcp forwards them using its existing
passthrough mechanism. Component gateways accept them only from authenticated,
scoped composite callers after authorization, and forward only their own selected
headers. Missing values stay omitted rather than falling back to pooled identity.

Authorization continues to use the existing refreshable connection token.
Attribution follows each frontend request independently, without a custom HTTP
transport or caller values in discovery cache keys.

## Rationale

Individual selection limits disclosure. Generating attribution before mmmcp
avoids stale values when a frontend session changes credentials, while preserving
OAuth refresh for background traffic. Signed, scoped internal tokens establish
which requests may carry trusted attribution headers.

## Consequences

Backends must restrict access to trusted gateway traffic before relying on these
headers. API keys identify their owning account; key names are mutable and are
not proof of an autonomous agent's identity. IdP groups reflect Obot's stored
memberships, not a live IdP query or an authorization grant.

Unavailable, ambiguous, invalid, or oversized values are omitted. Each header is
limited to 512 Unicode characters; values are never truncated. Identity lookup
failures fail the request. Raw API-key names in internal JWTs are also bounded,
separately from masked audit names.

Reserved header configuration can fail validation during deployment or updates.
Storage migrations preserve legacy headers without applying this policy, so they
cannot prevent startup.

## References

- [Header names and validation](../apiclient/types/obotheaders.go)
- [Attribution generation and filtering](../pkg/mcp/attribution/headers.go)
- [Gateway forwarding](../pkg/api/handlers/mcpgateway/handler.go)
