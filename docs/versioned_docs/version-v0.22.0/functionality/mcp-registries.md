---
title: MCP Registries
---

## Overview

MCP Registries control which MCP servers are available to which users. Administrators use registries to map server entries from the MCP Servers page to specific users and groups, ensuring each team has access to the tools they need.

To manage registries, go to **MCP Management > MCP Registries** in the MCP Platform.

## Default Access

By default, there's an "everyone" group that's assigned to all users. This means anyone that logs into Obot will have access to all MCP servers that are added to a registry with the "everyone" group.

If this default behavior is not what you want, you can restrict access to specific users or groups, or remove the "everyone" group entirely. However, it's recommended that administrators at least should have access to all servers.

## Creating a Registry

To create a new registry:

1. Click the **Add New Registry** button in the MCP Registries section
2. Give your registry a name
3. Assign users and groups to the registry
4. Add the MCP servers that this registry should include

## Example: Marketing Team Registry

For instance, if you were creating a registry for a marketing team:

1. Create a new registry named "Marketing Team"
2. Assign your marketing team members, either individually or through an existing group
3. Add relevant MCP servers such as:
   - Email tools
   - Google Calendar
   - Google Sheets
   - CRM systems
   - Other tools your marketing team needs for their day-to-day work

This approach ensures that each team only has access to the tools they need while maintaining security and organization.

## MCP Registry API

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers.

### API Endpoint

The registry is exposed at `/v0.1/servers` and supports:

- **List servers**: Get all servers visible to the authenticated user
- **Get server details**: Retrieve configuration for a specific server
- **Search**: Filter servers by name, title, or description
- **Pagination**: Cursor-based pagination for large result sets

### Authentication Modes

**No-Auth Mode (Default)**: Returns servers that have been granted access to all users via MCP Registries. Ideal for public instances.

**Auth Mode**: Returns all servers the authenticated user has access to. Enable with `OBOT_SERVER_ENABLE_REGISTRY_AUTH=true`.

### Server Naming

Obot uses a reverse DNS naming scheme for global uniqueness:

```
{reverse-dns}/{server-id}
```

Examples:
- `com.example.obot/github-server` for `https://obot.example.com`
- `local.localhost/my-server` for `http://localhost:8080`

## Contributing to the Default Server Set

To add your MCP server to Obot's default server set, submit a PR to the [mcp-catalog](https://github.com/obot-platform/mcp-catalog) repository.

### Submission Requirements

1. **Remote HTTP servers**: Submit only a server entry YAML file
2. **Containerized/STDIO servers**: First submit to [mcp-images](https://github.com/obot-platform/mcp-images) for repackaging, then submit the server entry

### Server Entry Format

```yaml
name: Your Server Name
description: |
  One-line summary of what this server does.

  ## Features
  - Key capability 1
  - Key capability 2

  ## What you'll need to connect
  - API key from https://example.com/api-keys

metadata:
  categories: category-a, category-b

icon: https://example.com/icon.png
repoURL: https://github.com/your-org/your-mcp-repo

env:
  - key: API_KEY
    name: API Key
    required: true
    sensitive: true
    description: Your API key from the developer dashboard

runtime: remote  # or containerized
remoteConfig:
  fixedURL: https://api.example.com/v1/mcp
```

See the [mcp-catalog repository](https://github.com/obot-platform/mcp-catalog) for complete examples and documentation.
