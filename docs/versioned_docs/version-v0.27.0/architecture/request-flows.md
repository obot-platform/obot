---
title: Request flows and trust boundaries
---

An Obot deployment has several enforcement points. Identify which traffic passes through each before relying on a policy.

| Flow | Path | Boundary |
|---|---|---|
| MCP request | Client → Obot gateway → hosted or remote MCP server | Obot authenticates the client and applies server/tool grants and matching filters |
| LLM request | Client → Obot LLM Gateway → model provider | Obot checks model access and supplies the upstream provider credential |
| Private MCP request | Gateway → outbound tunnel connection → private MCP server | The tunnel allowlist limits destinations; the tunnel machine makes the final connection |
| Local tool call | AI client hook → Sentry → Obot decision/audit services | Coverage depends on the client and installed hooks, independently of gateway routing |

The [MCP authentication diagrams](../concepts/architecture.md#authentication-flow) distinguish permission to access Obot from permission to use an upstream service. Clients receive Obot tokens; upstream OAuth credentials remain with Obot.

Hosted MCP servers and filters execute outside the Obot process. Calling a remote server does not place that service inside Obot's workload isolation boundary. [Filters](../functionality/filters.md) inspect matching traffic; they do not sandbox the remote service.

A client that talks directly to a provider or server bypasses the corresponding gateway. Device enforcement has its own [client support and limitations](../device-management/enforcement.md). Use [Security model](../security/model.md) to relate these boundaries to deployment responsibilities.
