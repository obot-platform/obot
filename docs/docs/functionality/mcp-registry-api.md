---
displayed_sidebar: sidebar
title: MCP Registry API
---

## Overview

Obot implements the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients such as VS Code to discover and connect to vMCPs.

Each discoverable vMCP appears once with its display name, description, icon, and canonical connection URL. vMCPs must contain at least one component to appear in discovery; empty drafts are excluded from both listing and detail requests. Catalog templates, individual user connections, and backing component servers are not registry entries.

## API Endpoint

The registry is exposed at `/v0.1/servers` and supports:

- **List servers**: Get all vMCPs visible to the requester
- **Get server details**: Retrieve a specific vMCP's metadata and connection URL
- **Search**: Filter vMCPs by name, title, or description using the `search` query parameter
- **Pagination**: Use the `limit` and `cursor` query parameters for large result sets

`GET /v0.1/servers/{serverName}/versions` lists the single supported version, `latest`. `GET /v0.1/servers/{serverName}/versions/latest` returns its details. URL-encode the complete registry name, including its slash, when using it as `{serverName}`.

## Discovery access

Admins and owners manage discovery under **Platform → Settings → MCP Registry API**.

- **Authenticated access (default):** Users must authenticate to discover vMCPs they own or can access through matching profiles, including Obot and identity-provider groups. Administrator management privileges alone do not grant discovery access.
- **Authenticated and anonymous access:** Enable **Allow anonymous registry discovery** and click **Save**. Anonymous requests discover only shared vMCPs whose profiles explicitly grant access to **All Obot Users** (`type: selector`, `id: "*"`). Personal vMCPs and vMCPs restricted to specific users or groups remain excluded.

Authenticated requests receive their personalized view in both modes, including accessible private vMCPs. Registry OAuth discovery metadata remains available in both modes. Connecting to a vMCP still requires authentication and the user's current vMCP permissions. Catalog access policies do not determine registry visibility; shared vMCP access comes from [vMCP profiles](../mcp-gateway/access.md).

### Risks of anonymous discovery

:::warning Public metadata exposure
Anyone who can reach the registry endpoints can read eligible vMCPs' names, **full descriptions**, icons, and connection URLs without signing in. The response description is limited to 100 characters, but the full description is included in publisher-provided metadata.

Review descriptions, icons, and URLs for sensitive information before enabling anonymous discovery. This publishes metadata for all eligible shared vMCPs, including new vMCPs that later receive wildcard profile access. Disabling discovery cannot remove copies already collected by clients.
:::

Clients that cannot authenticate registry discovery, including VS Code, need anonymous discovery enabled. Installation and connection still use Obot's authentication and setup flow.

### Upgrading

Deployments that previously relied on anonymous discovery must enable it in **Platform → Settings → MCP Registry API** after upgrading. Neither `OBOT_SERVER_ENABLE_REGISTRY_AUTH` nor `OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS` controls discovery anymore, and the corresponding CLI flags and Helm chart settings have been removed. Existing environment values are not imported; anonymous discovery stays disabled until an admin or owner enables it in the app.

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
