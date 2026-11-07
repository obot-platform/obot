---
title: "Isolate workloads"
---

Different controls apply to gateway destinations, hosted workloads, and tunnel machines.

| Control | Scope |
|---|---|
| Gateway localhost/private/link-local restrictions | Direct MCP destination validation by Obot |
| Kubernetes MCP NetworkPolicy | Selected MCP pods in the MCP namespace; does not restrict the Obot server itself |
| Pod Security Admission and RuntimeClass | Pod admission and execution isolation on configured Kubernetes nodes |
| Domain egress provider | Per-server hosted MCP domain allowlists; currently Aviatrix, with HTTPS/443 restrictions |
| Tunnel allowed URLs | Destinations reached by the tunnel machine; ordinary direct-gateway destination restrictions do not apply there |

Keep [MCP NetworkPolicy and pod-security settings](../configuration/mcp-deployments-in-kubernetes.md) enabled unless a reviewed workload requires an exception. RuntimeClass requires the corresponding runtime to be installed on eligible nodes.

[Domain-based egress](../configuration/mcp-server-egress-control.md) is separate from the built-in IP-based policy. Its remote endpoints are not hosted workloads and are not covered by this feature.

Obot itself needs connectivity to its database, identity provider, upstream services, storage, and possibly cloud credential metadata. Scope any operator-managed server-egress policy to those dependencies; do not assume a blanket private-IP block is compatible with your topology.

Docker's mounted socket provides host-level container management. Use it for trusted evaluation. For private remote services, review [tunnel security](../functionality/mcp-tunnels.md#security-behavior).
