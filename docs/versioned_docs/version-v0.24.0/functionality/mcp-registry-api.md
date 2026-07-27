---
title: MCP Registry API
---

## Overview

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers.

## API Endpoint

The registry is exposed at `/v0.1/servers` and supports:

- **List servers**: Get all servers visible to the authenticated user
- **Get server details**: Retrieve configuration for a specific server
- **Search**: Filter servers by name, title, or description
- **Pagination**: Cursor-based pagination for large result sets

## Authentication Modes

**No-Auth Mode (Default)**: Returns servers that have been granted access to all users via [MCP Access Policies](./mcp-access-policies.md). Ideal for public instances.

**Auth Mode**: Returns all servers the authenticated user has access to. Enable with `OBOT_SERVER_ENABLE_REGISTRY_AUTH=true`.

## Server Naming

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
