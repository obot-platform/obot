---
title: MCP Gateway
---

# MCP Gateway

The MCP Gateway is a reverse-proxy passthrough that sits between MCP clients and MCP servers. It authenticates users, ensures servers are deployed, and forwards requests without modifying the MCP protocol.

## Gateway Architecture

The gateway is intentionally simple. It handles three things:

1. **Authentication**: Validates users against the configured identity provider
2. **Server Deployment**: Ensures the target MCP server is running (via Docker or Kubernetes)
3. **Proxy**: Forwards requests to the MCP server and returns responses

![Gateway Architecture](/img/gateway-architecture.webp)

Authorization, audit logging, and webhook filters are applied while the gateway proxies MCP traffic.

### Deployment

- **Kubernetes**: Each MCP server runs in its own pod
- **Docker**: Containers communicate via `host.docker.internal` or local IP

## Filters

Filters allow inspection and modification of MCP traffic. They can be implemented as MCP filter servers or as HTTP webhook filters.

MCP filter servers are deployed like other MCP servers in Obot, with one additional setting: the filter configuration must identify the tool name that Obot calls for filtering. Existing HTTP webhooks are automatically converted to MCP servers that run as their own deployments.

## Connecting to the Gateway

### With Obot Agent

Obot Agent connects through the gateway automatically. Users select which MCP servers to enable for their agents, conversations, or workflows.

### With External Clients

External MCP clients (Claude Desktop, Cursor, VS Code) can connect using the gateway endpoint:

```
https://your-obot-instance/mcp-connect/{server-id}
```

All servers are exposed via `streamable-http` transport, regardless of their underlying runtime.
