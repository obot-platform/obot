# 2026-08-27: Route each composite MCP server through one Obot replica

- **Status:** Accepted
- **Date:** 2026-08-27
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7625](https://github.com/obot-platform/obot/issues/7625) — MCP tool call latency regression after the proxy rework.

## Related ODPs

- [2026-08-14: Support MCP Specification Version 2026-07-28](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-14-migrate-to-go-mcp-sdk/README.md)

## Context

Each Obot replica hosts its own in-process MMMCP instance for composite MCP servers. MMMCP retains client and component-server connection state in that process. In a multi-replica deployment, successive `initialize`, `tools/list`, and `tools/call` exchanges for one composite can reach different replicas. Each replica then creates independent composite state and repeats component initialization, increasing latency and allowing requests to observe a different state from the one that handled initialization.

Obot cannot assume load-balancer affinity, and the same resolved composite server can be addressed through different URL identifiers. The routing boundary must also preserve streaming responses, cancellation, the authenticated user identity, and the existing outer gateway's hook and audit behavior.

## Decision

Obot will assign one active replica as the owner of each resolved composite MCP server and route every internal composite HTTP exchange to that replica. Ownership is keyed by an opaque, namespaced digest of `MCPServerName`; URL aliases, users, and MCP session identifiers do not create separate owners.

The tunnel manager's existing remotedialer peer mesh will advertise owner registrations and carry forwarded exchanges. When no owner is advertised, the first replica to claim the key opens a reconnecting, capability-authenticated loopback WebSocket registration. Concurrent claims on that replica are coalesced. Other replicas discover the advertised key through the peer mesh and forward the complete request body and response through a dedicated remotedialer network and address. The forwarding path preserves HTTP status, headers, full-duplex request handling, SSE flushing, and cancellation.

The owner reinjects a forwarded request into its local gateway through an owner-only marker derived from the tunnel bridge capability. Registration is limited to the tunnel bridge principal and valid composite keys. All remotedialer and composite control headers are removed before requests reach MMMCP and before responses reach callers; an invalid externally supplied owner marker is rejected.

Owner registrations have the gateway handler's lifetime and are stopped before its MMMCP instance closes. Obot will not replay an exchange on another replica if its owner disappears. Routing failures return `503 Service Unavailable`; a later request may establish a new owner, but clients must reinitialize any state lost with the former owner.

## Rationale

Routing to one owner preserves MMMCP's process-local state model while removing repeated initialization and cross-replica state divergence. Reusing the tunnel peer mesh avoids introducing another coordination service and works without load-balancer configuration. Keying by the resolved server name ensures every alias reaches the same state domain.

Persisting or distributing MMMCP's live client and component connections would require a substantially larger change and would not make active network connections transferable between replicas. Load-balancer stickiness was rejected because it is deployment-specific, can group requests by the wrong identity, and cannot guarantee that aliases of one composite share an owner. Retrying a failed in-flight exchange on a new owner was rejected because it could duplicate side effects and cannot restore the lost MCP session.

## Consequences

- All requests for one resolved composite server share a coherent in-process MMMCP state domain across an Obot cluster.
- Different composite servers can have different owners, but one busy composite is handled by one replica and does not gain request-processing capacity from additional replicas.
- The remotedialer peer mesh and owner registration path become availability dependencies for composite MCP traffic in multi-replica deployments.
- An owner restart or network partition discards that owner's live composite state. Requests can fail with `503`, and clients may need to initialize a new MCP session.
- Ownership is intentionally shared across URL aliases and users of the same resolved composite server, so future isolation changes must revisit the affinity key.
- The internal registration and forwarding protocol must continue to authenticate owner traffic and strip its control headers at both trust boundaries.

## References

- [ADR: Transparently proxy MCP traffic through the gateway](2026-08-17-transparently-proxy-mcp-traffic.md)
- [Rancher remotedialer](https://github.com/rancher/remotedialer)
