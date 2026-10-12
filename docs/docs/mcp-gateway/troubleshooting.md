---
title: Troubleshooting
---

Start with the failing boundary rather than changing several settings at once.

| Symptom | Check |
|---|---|
| Cannot sign in | [Authentication setup](../security/authentication.md#enabling-authentication-troubleshooting), configured provider, and callback URL |
| OAuth reports a missing resource parameter | The client's OAuth resource must identify the same MCP endpoint; see the [FAQ](../faq.md) |
| vMCP is missing | The user's profiles, or ownership for a personal vMCP |
| A tool is missing or unexpectedly allowed | Component exposure, all matching profile grants, and instance tool selection |
| Remote connection fails | Server URL, upstream credentials, and reachability from Obot or its tunnel machine |
| Hosted server fails to start | Runtime/package settings, required configuration, workload events, and [image pull secrets](../configuration/image-pull-secrets.md#troubleshooting) |
| Private endpoint is unreachable | [Tunnel troubleshooting](../functionality/mcp-tunnels.md#troubleshooting) and its allowlist |
| Source changes do not appear in a vMCP | For an Obot-managed vMCP, apply its [snapshot update](./publish.md#virtual-mcps-snapshots-and-updates). For a Git-managed vMCP, check source sync errors and component references |

Use **Test vMCP** to isolate the gateway connection from your external client. Inspect audit metadata for the failing request. Request and response bodies require Auditor access.

When reporting an issue, include the Obot version, runtime, client version, timestamp, and sanitized error. Do not include API keys, upstream tokens, or unredacted audit payloads.
