# Registry API

The Registry API provides a standardized, MCP-compliant interface for discovering and accessing MCP servers configured in your Obot instance. This API follows the [MCP Registry specification](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/generic-registry-api.md), enabling MCP clients to programmatically discover available servers based on user permissions.

## Overview

The Registry API exposes all MCP servers that a user has access to through a unified `/v0/servers` endpoint. This includes:

- **Personal Servers**: Single-user servers deployed specifically for you
- **Catalog Servers**: Multi-user servers shared across your organization
- **Workspace Servers**: Servers available within your Power User workspace

All servers are returned in a standardized format, regardless of their underlying runtime (npx, uvx, containerized, or remote).

## API Endpoints

### List Servers

```http
GET /v0/servers
```

Returns a paginated list of all MCP servers accessible to the authenticated user.

#### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cursor` | string | - | Pagination cursor (server name from previous response) |
| `limit` | integer | 50 | Maximum results per page (1-100) |
| `search` | string | - | Filter servers by name, title, or description |

#### Example Request

```bash
curl -H "Authorization: Bearer <token>" \
  "https://obot.example.com/v0/servers?limit=1&search=github"
```

#### Response Format

```json
{
  "servers": [
    {
      "server": {
        "name": "com.example.obot/github-server",
        "title": "GitHub MCP Server",
        "description": "Access GitHub repositories, issues, and pull requests",
        "version": "latest",
        "icons": [
          {
            "src": "https://example.com/github-icon.png",
            "mimeType": "image/png"
          }
        ],
        "remotes": [
          {
            "type": "streamable-http",
            "url": "https://obot.example.com/mcp-connect/ms1-abc123"
          }
        ],
        "repository": {
          "url": "https://github.com/example/mcp-github",
          "source": "github"
        }
      }
    }
  ],
  "metadata": {
    "nextCursor": "com.example.obot/other-github-server",
    "count": 1
  }
}
```

### List Server Versions

```http
GET /v0/servers/{serverName}/versions
```

Returns available versions for a specific server. Currently, Obot only supports the `latest` version.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `serverName` | string | Full server name in format: `reverseDNS/server-id` |

#### Example Request

```bash
curl -H "Authorization: Bearer <token>" \
  "https://obot.example.com/v0/servers/com.example.obot%2Fgithub-server/versions"
```

### Get Server Version

```http
GET /v0/servers/{serverName}/versions/{version}
```

Returns details for a specific server version.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `serverName` | string | Full server name in format: `reverseDNS/server-id` |
| `version` | string | Version identifier (currently only `latest` is supported) |

#### Example Request

```bash
curl -H "Authorization: Bearer <token>" \
  "https://obot.example.com/v0/servers/com.example.obot%2Fgithub-server/versions/latest"
```

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

The Registry API respects all Obot access control rules:

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
curl "https://obot.example.com/v0/servers?limit=50"

# Next page (using cursor from previous response)
curl "https://obot.example.com/v0/servers?limit=50&cursor=com.example.obot%2Flast-server"
```

## Search Filtering

The `search` parameter performs case-insensitive substring matching across:

- Server name
- Server title
- Server description

**Example:**

```bash
# Find all servers related to GitHub
curl "https://obot.example.com/v0/servers?search=github"

# Find database-related servers
curl "https://obot.example.com/v0/servers?search=database"
```

## Authentication

All authenticated users can access the Registry API. Registry clients can follow the [registry authorization spec](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/api/registry-authorization.md) to authenticate.

Obot only supports the `mcp-registry:read` scope. Obot's implementation of the Registry API is read-only and does not
include any routes related to publishing, editing, or deleting entries in the registry.

## Error Responses

### 404 Not Found

```json
{
  "title": "Not Found",
  "status": 404,
  "detail": "Server not found"
}
```

### 401 Unauthorized

Returned when authentication is missing or invalid.

### 403 Forbidden

Returned when the authenticated user doesn't have access to the requested server.
