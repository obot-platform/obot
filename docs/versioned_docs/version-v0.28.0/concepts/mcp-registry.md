---
title: "MCP Servers"
---

# MCP Servers {#mcp-registry}

The MCP Registry is a central place to list and discover MCP servers. It provides a curated collection of servers available to users based on their access permissions.

See the [registry and skills overview](../registries/overview.md) for the difference between a catalog entry, a shared vMCP, and a skill.

## Registry Concepts

### Server Sources

MCP server definitions can come from:

- **Official Obot repository**: The default set from [obot-platform/mcp-catalog](https://github.com/obot-platform/mcp-catalog)
- **Custom Git repositories**: Your own repositories containing server definitions (see [MCP Server GitOps](../configuration/mcp-server-gitops.md))
- **Direct entry**: Servers added manually through the UI

### Server Definitions

Each server in the registry includes:

- **Name and Description**: Human-readable identification
- **Runtime Configuration**: How to run the server (npx, uvx, containerized, or remote)
- **Environment Variables**: Required and optional configuration
- **Tool Preview**: Description of available tools
- **Icon and Metadata**: For display in the UI

### Egress settings for hosted servers

Catalog entries for Kubernetes-hosted servers can specify `egressDomains` and `denyAllEgress` in their runtime configuration. These settings require cluster-level egress enforcement; they do not apply to remote server endpoints. See [MCP server egress control](../configuration/mcp-server-egress-control.md) for cluster prerequisites and [per-entry YAML examples](../configuration/mcp-server-egress-control.md#yaml-configuration-examples).

### Access Control

Administrators and Power Users+ control which servers are visible to which users by assigning servers to registries and granting users access to those registries.

## MCP Registry API

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers.

## Learn More

- [MCP Access Policies](../mcp-gateway/access.md) - Controlling which servers are available to which users and groups
- [MCP Registry API](../functionality/mcp-registry-api.md) - Programmatic server discovery and contributing servers
