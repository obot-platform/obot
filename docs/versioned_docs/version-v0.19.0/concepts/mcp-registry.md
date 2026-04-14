---
title: MCP Registry
---

# MCP Registry

The MCP Registry is a central place to list and discover MCP servers. It provides a curated collection of servers available to users based on their access permissions.

## Registry Concepts

### Server Sources

MCP server definitions can come from:

- **Official Obot repository**: The default set from [obot-platform/mcp-catalog](https://github.com/obot-platform/mcp-catalog)
- **Custom Git repositories**: Your own repositories containing server definitions (see [MCP Server GitOps](/configuration/mcp-server-gitops/))
- **Direct entry**: Servers added manually through the UI

### Server Definitions

Each server in the registry includes:

- **Name and Description**: Human-readable identification
- **Runtime Configuration**: How to run the server (npx, uvx, containerized, or remote)
- **Environment Variables**: Required and optional configuration
- **Tool Preview**: Description of available tools
- **Icon and Metadata**: For display in the UI

### Access Control

Administrators and Power Users+ control which servers are visible to which users by assigning servers to registries and granting users access to those registries.

## MCP Registry API

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers.

## Learn More

- [MCP Registries](/functionality/mcp-registries/) - Managing registries, API details, and contributing servers
