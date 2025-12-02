# Registry API

The Registry API provides a standardized, MCP-compliant interface for discovering and accessing MCP servers configured in your Obot instance. This API follows the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers based on user permissions.

## Overview

The Registry API exposes MCP servers through a unified `/v0.1/servers` endpoint. The servers returned depend on the authentication mode:

**No-Auth Mode (Default)**: Returns servers from the default catalog, accessible without authentication. This is ideal for public Obot instances sharing a curated set of MCP servers. The only servers that will be visible are ones that are given access to All Obot Users with an Access Control Rule.

**Auth Mode**: Returns all MCP servers that an authenticated user has access to, including:
- **Personal Servers**: Single-user servers deployed specifically for you
- **Catalog Servers**: Multi-user servers shared across your organization
- **Workspace Servers**: Servers available within your Power User workspace

All servers are returned in a standardized format, regardless of their underlying runtime (npx, uvx, containerized, or remote).

To enable Auth Mode, set the environment variable: `OBOT_SERVER_ENABLE_REGISTRY_AUTH=true`

## API Endpoints

Refer to the [MCP Generic Registry API Specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md) for reference on the available endpoints. Obot supports all endpoints except for `/publish`.

## Server Naming Convention

Obot uses a reverse DNS naming scheme for servers to ensure global uniqueness:

```
{reverse-dns}/{server-id}
```

**Examples:**
- `com.example.obot/github-server` for `https://obot.example.com`
- `local.localhost/my-server` for `http://localhost:8080`
- `ai.obot.chat/slack-server` for `https://chat.obot.ai`

The reverse DNS portion is automatically generated from your Obot instance URL.

## Server Response Schema

### ServerDetail Object

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique server identifier in reverse DNS format |
| `title` | string | Human-readable display name |
| `description` | string | Detailed server description (supports Markdown) |
| `version` | string | Version identifier (always `latest` for Obot) |
| `icons` | array | List of icon objects for UI display |
| `remotes` | array | Connection endpoints (when server is configured) |
| `repository` | object | Source code repository information |

### Remote Object

All Obot servers expose a `streamable-http` remote endpoint, regardless of their underlying runtime.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Always `streamable-http` for Obot servers |
| `url` | string | MCP connection URL via `/mcp-connect/{server-id}` |

### Metadata Object

The `_meta` field contains Obot-specific metadata about the server.

| Field | Type | Description |
|-------|------|-------------|
| `ai.obot/server.configurationRequired` | boolean | Whether the server needs configuration before use |
| `ai.obot/server.configurationMessage` | string | Instructions for configuring the server |

## Server States

### Configured Servers

Servers that are ready to use will include a `remotes` array with connection details:

```json
{
  "server": {
    "name": "com.example.obot/weather-api",
    "remotes": [
      {
        "type": "streamable-http",
        "url": "https://obot.example.com/mcp-connect/ms1-xyz789"
      }
    ]
  },
}
```

### Unconfigured Servers

Servers requiring configuration will have the `configurationRequired` flag set and no `remotes` array:

```json
{
  "server": {
    "name": "com.example.obot/database-server"
  },
  "_meta": {
    "ai.obot/server": {
      "configurationRequired": true,
      "configurationMessage": "This server requires configuration. Please visit the Obot UI to configure it."
    }
  }
}
```

To configure these servers, users must visit the Obot web interface and provide required credentials or settings.

## Access Control

Access control behavior varies based on the authentication mode:

### No-Auth Mode (Default)

In No-Auth Mode, the registry returns only a curated subset of servers:

- **Default Catalog Only**: Only servers from the default catalog are returned
- **Wildcard ACRs Required**: Only servers with wildcard (`*`) Access Control Rules are visible
- **No Personal Servers**: User-specific servers are never returned
- **No Workspace Servers**: Workspace-scoped servers are not included

This ensures that unauthenticated users only see publicly approved servers that administrators have explicitly made available to everyone.

### Auth Mode

In Auth Mode, the Registry API respects all Obot access control rules:

- **Personal Servers**: Only visible to the owning user
- **Catalog Servers**: Visible based on Access Control Rules (ACRs)
- **Workspace Servers**: Visible based on Access Control Rules (ACRs)

If you don't have access to a server, it will not appear in API responses.

## Pagination

The API uses cursor-based pagination to handle large result sets efficiently:

1. Make an initial request with optional `limit` parameter
2. Check the `metadata.nextCursor` field in the response
3. If present, make another request with `cursor` set to the `nextCursor` value
4. Repeat until `nextCursor` is absent

**Example Pagination:**

```bash
# First page
curl "https://obot.example.com/v0.1/servers?limit=50"

# Next page (using cursor from previous response)
curl "https://obot.example.com/v0.1/servers?limit=50&cursor=com.example.obot%2Flast-server"
```

## Search Filtering

The `search` parameter performs case-insensitive substring matching across:

- Server name
- Server title
- Server description

**Example:**

```bash
# Find all servers related to GitHub
curl "https://obot.example.com/v0.1/servers?search=github"

# Find database-related servers
curl "https://obot.example.com/v0.1/servers?search=database"
```

## Authentication

When `OBOT_SERVER_ENABLE_REGISTRY_AUTH=true`:

All authenticated users can access the Registry API. Registry clients can follow the [registry authorization spec](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/registry-authorization.md) to authenticate.

Obot's implementation of the Registry API is read-only and does not include any routes related to publishing, editing, or deleting entries in the registry.

## Error Responses

### 404 Not Found

```json
{
  "title": "Not Found",
  "status": 404,
  "detail": "Server not found"
}
```

Returned when the requested server does not exist or is not visible to the user.

### 401 Unauthorized

**Auth Mode Only:** Returned when authentication is missing or invalid.

**No-Auth Mode:** This error is not returned; unauthenticated requests are allowed.

### 403 Forbidden

**Auth Mode Only:** Returned when the authenticated user doesn't have access to the requested server.
