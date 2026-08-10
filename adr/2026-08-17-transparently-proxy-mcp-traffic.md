# 2026-08-17: Transparently proxy MCP traffic through the gateway

- **Status:** Accepted
- **Date:** 2026-08-17
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7309](https://github.com/obot-platform/obot/issues/7309) — support the 2026-07-28 MCP specification.

## Related ODPs

- [2026-08-14: Support MCP Specification Version 2026-07-28](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-14-migrate-to-go-mcp-sdk/README.md)

## Context

The MCP gateway previously terminated client connections in Nanobot and opened separate connections to upstream MCP servers. This coupled forwarded traffic to Nanobot's protocol implementation and prevented the gateway from carrying the complete 2026-07-28 specification, including stateless requests, while retaining 2025-11-25 compatibility.

Obot must still enforce authentication and network policy, supply upstream OAuth credentials, route through tunnels, run request and response hooks, and produce the same audit information. Legacy MCP transports also allow a response to arrive on a different HTTP exchange and, in a multi-replica deployment, at a different Obot replica.

## Decision

The gateway will relay public MCP HTTP traffic with a reverse proxy instead of terminating non-composite MCP connections. The client and upstream server therefore negotiate and exchange MCP directly. Obot will use the official Go MCP SDK for MCP calls that Obot originates, including calls to hook servers, but not as an intermediary for forwarded protocol messages.

The reverse proxy will use an Obot-owned transport that applies configured upstream headers, refreshable OAuth tokens, tunnels, and the configured blocking of loopback, private, and link-local addresses. Authorization credentials are retained only for redirects to the same upstream origin, including when that origin is represented by an Obot tunnel bridge.

Hooks and audit logging remain gateway responsibilities. The gateway will inspect JSON-RPC messages and SSE events at the proxy boundary, run matching request and response hooks, apply allowed mutations or rejections, and record request and response audit data. Request and response audit entries from one HTTP exchange are correlated with an internal proxy exchange identifier. Legacy responses that arrive on another exchange continue to use MCP session and request identifiers as a fallback.

The gateway will persist two internal resources for legacy cross-request behavior in horizontally scaled deployments:

- `MCPClientSession` associates a legacy MCP session with its client name and version for audit attribution and expires after seven days without use.
- `MCPHookCorrelation` stores the method, name, and request-hook mutation needed to process a later response. Its key is derived from the server, user, session, request identifier, and request origin; it is consumed when the response arrives and expires after 24 hours.

Composite MCP protocol handling remains on the existing Nanobot path until the composite migration is complete. A public composite request is reverse-proxied to that internal endpoint with a short-lived, user-scoped token, and the internal hop is excluded from duplicate audit logging.

## Rationale

Transparent forwarding removes the gateway's protocol-version boundary: support for compatible MCP messages and transports does not require the gateway to negotiate or translate them. It also avoids creating a second logical MCP session between the gateway and the upstream server.

Using the Go SDK remains appropriate where Obot is the MCP client, because those calls need a protocol implementation. Keeping governance at the proxy boundary preserves Obot-specific behavior without retaining Nanobot as the implementation for ordinary forwarded traffic. Persisting only the legacy correlation data preserves stateful compatibility without imposing session affinity or in-memory coordination on stateless traffic.

This narrows the ODP's proposed gateway boundary: rather than terminating non-composite traffic with the Go SDK, the implementation confines the SDK to MCP calls originated by Obot and transparently relays forwarded traffic. This achieves the proposal's compatibility and protocol-maintenance goals without redundant client-to-gateway and gateway-to-server negotiation.

Updating Nanobot or replacing it with another terminating proxy was rejected because either approach would keep protocol compatibility coupled to an intermediary. A completely opaque HTTP proxy was also rejected because it would remove hooks and protocol-level audit records.

## Consequences

- Clients can use 2025-11-25 or 2026-07-28 behavior supported by the upstream server, including stateless requests, without a gateway-side MCP handshake.
- Obot keeps its existing authorization, OAuth, tunnel, network-policy, hook, and audit responsibilities.
- Gateway replicas can process legacy responses without session affinity, at the cost of two internal persisted resource types and their cleanup controllers.
- Hook and audit processing still depends on recognizing JSON-RPC and SSE envelopes. Hook-inspected messages are buffered with a 10 MiB limit, and modified encoded responses must have stale encoding, integrity, and length headers removed.
- The gateway must prevent upstream credentials from following cross-origin redirects and must continue treating the tunnel's encoded target as the credential origin.
- Composite traffic continues to depend on Nanobot until the separate composite-server migration is completed.

## References

- [Official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- [MCP 2026-07-28 specification](https://modelcontextprotocol.io/specification/2026-07-28)
