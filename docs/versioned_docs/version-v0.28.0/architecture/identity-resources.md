---
title: "Resource ownership"
---

Obot uses identity and resource ownership together with explicit grants. A successful sign-in identifies the caller; it does not make every resource available.

| Resource | Purpose and scope |
|---|---|
| User and authentication-provider identity | The signed-in account and its external group membership |
| Catalog entry | A definition of a hosted or remote MCP server |
| vMCP component | A snapshot of a catalog entry plus configuration and tool selection |
| Shared vMCP | An administrator-published endpoint governed by profiles |
| Personal vMCP | An endpoint usable only by its author |
| Profile | An additive grant of shared-vMCP components/tools to users or groups |
| vMCP instance | A user's connection state and user-supplied credentials |
| Agent authorization scope | Programmatic capabilities limited by the owning user's permissions |

The [vMCP guide](../mcp-gateway/server-types.md) explains snapshot updates, profile composition, and loss of catalog access. [Agent authorization scopes](../functionality/agent-auth-scopes.md) explain key expiration and revocation.

Logical isolation and runtime isolation are different. A shared component can serve users with separate headers; user-provided non-header configuration can require separate deployments. Kubernetes pods, network policies, and runtime classes provide workload boundaries. See [Network and workload isolation](../security/isolation.md).

Authentication-provider changes can create different Obot user identities. Read [Switching auth providers](../configuration/auth-providers.md#switching-between-auth-providers) before changing providers; do not assume ownership and stored work automatically transfer.
