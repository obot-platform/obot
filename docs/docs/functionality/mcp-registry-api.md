---
displayed_sidebar: sidebar
title: MCP Registry API
---

## Overview

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients such as VS Code to discover and connect to vMCPs.

Each vMCP appears once with its display name, description, icon, and canonical connection URL. Catalog templates, individual user connections, and backing component servers are not registry entries.

## API Endpoint

The registry is exposed at `/v0.1/servers` and supports:

- **List servers**: Get all vMCPs visible to the requester
- **Get server details**: Retrieve a specific vMCP's metadata and connection URL
- **Search**: Filter vMCPs by name, title, or description using the `search` query parameter
- **Pagination**: Use the `limit` and `cursor` query parameters for large result sets

`GET /v0.1/servers/{serverName}/versions` lists the single supported version, `latest`. `GET /v0.1/servers/{serverName}/versions/latest` returns its details. URL-encode the complete registry name, including its slash, when using it as `{serverName}`.

## Authentication Modes

**Authenticated access (Default)**: Registry discovery requires authentication (`OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS=false`). Requests return vMCPs the user owns or can access through matching profiles, including Obot and identity-provider groups. Administrator management privileges alone do not grant discovery access.

**Authenticated and anonymous access (Opt-In)**: Set `OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS=true` to also allow anonymous discovery. Anonymous requests return only shared vMCPs whose profiles explicitly grant access to **All Obot Users** (`type: selector`, `id: "*"`). Personal vMCPs and vMCPs restricted to specific users or groups are excluded. Catalog access policies do not determine registry visibility; shared vMCP access comes from [vMCP profiles](../mcp-gateway/access.md).

Anyone who can reach the discovery endpoints with anonymous access enabled can read the names, descriptions, icons, and connection URLs of these vMCPs. Opt in only when you intend to publish this metadata. Connecting still requires authentication and the user's current vMCP permissions. Authenticated registry requests receive their personalized view, including accessible private vMCPs, in both modes. Registry OAuth discovery metadata remains available in both modes.

Clients that cannot authenticate registry discovery, including VS Code, require the no-auth opt-in. Authentication and setup still happen when connecting to an installed vMCP.

Deployments that previously relied on anonymous discovery must set `OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS=true` when upgrading. This replaces the old `OBOT_SERVER_ENABLE_REGISTRY_AUTH` setting, which is no longer supported. The Helm chart also disables anonymous registry access by default.

## Server Naming

Obot uses a reverse DNS naming scheme for global uniqueness:

```
{reverse-dns}/{vmcp-id}
```

Examples:
- `com.example.obot/vmcp1example` for `https://obot.example.com`
- `local.localhost/vmcp1example` for `http://localhost:8080`

Names identify vMCP definitions rather than a user's connection. Migrated aliases that identify a vMCP definition remain usable for detail requests and return the canonical vMCP name and URL. Instance IDs and aliases that identify individual user connections are not registry entries. Inaccessible or missing entries return 404.

## Contributing to the Default Server Set

To add your MCP server to Obot's default server set, submit a PR to the [mcp-catalog](https://github.com/obot-platform/mcp-catalog) repository. This adds a catalog template; expose it through a vMCP with the appropriate profiles to make it discoverable through the MCP Registry API.

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
