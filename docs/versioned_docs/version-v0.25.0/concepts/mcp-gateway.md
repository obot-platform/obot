---
title: MCP Gateway
---

# MCP Gateway

The MCP Gateway runs inside Obot and proxies traffic between MCP clients and hosted or remote MCP servers. It authenticates users, checks authorization, ensures hosted servers are running, records audit data, and invokes filters before forwarding allowed messages. Filters can reject traffic or modify it when mutation is allowed.

## Gateway Architecture

![Gateway Architecture](/img/gateway-architecture.webp)

Obot owns proxying, authentication, authorization, auditing, and filter enforcement. Hosted server code and filter code execute in separate workloads.

Obot handles OAuth token exchange and refresh to access upstream MCP servers on the user's behalf. See the [authentication flow](./architecture.md#authentication-flow).

### Deployment

- **Kubernetes**: Hosted MCP servers and hosted filters run in separate workloads from the Obot pod and from one another.
- **Docker**: Hosted servers and filters run in separate containers alongside Obot.
- **Remote**: Obot connects to an externally hosted MCP endpoint.
- **STDIO runtimes**: Hosted `npx` and `uvx` servers use a transport adapter in their workload to expose HTTP. Proxy filters run through Obot, not through that adapter.

## Filters

Obot selects matching filters, calls them, and enforces their accept, reject, or permitted mutation responses. Filter implementations run outside the Obot process:

- **Hosted MCP filters** use `npx`, `uvx`, or `containerized` runtimes and run as separate workloads managed by Obot.
- **Remote MCP filters** expose a filter tool at an external MCP endpoint; Obot connects to that endpoint instead of deploying the filter implementation.
- **HTTP webhook filters** use a separately hosted MCP-to-HTTP adapter that calls the external webhook endpoint. The adapter is not a sidecar attached to each target MCP server.

See [Filters](../functionality/filters.md) for selectors and contracts.

## Connecting to the Gateway

### With Obot Agent

Obot Agent connects through the gateway automatically. Users select which MCP servers to enable for their agents, conversations, or workflows.

### With External Clients

External MCP clients can connect using the gateway endpoint:

```
https://your-obot-instance/mcp-connect/{server-id}
```

All servers are exposed via `streamable-http` transport, regardless of their underlying runtime.
