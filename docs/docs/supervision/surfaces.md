---
title: "Surfaces"
---

Each surface feeds the same session graph and asks the same policy engine. This page lists what each one needs.

## LLM gateway

Nothing to configure. Requests through the LLM gateway record a model turn, ingest the tool results that are new since the previous turn, and decide the tool calls the model proposes before the response reaches the client. A blocked tool call ends the response with an error carrying the decision's reason.

Obot returns `X-Obot-Session-ID` on responses. Clients that send it back on later requests are stitched exactly; otherwise Obot uses the agent's own session identifier (Claude Code's session header, the `session_id` in request metadata) or the API key and agent.

Streams for requests that declare tools are held until the response is complete, so first-token latency rises for tool-using clients.

## MCP gateway

Nothing to configure. Every `tools/call` through `/mcp-connect/...` is decided before it is proxied, and every result is ingested with the tool's capability labels. `tools/list` responses teach Obot the tools' descriptions, which the labelling uses. The session is the MCP client session.

## Devices (Obot Sentry)

The device-side allowlist in [Policy enforcement](../device-management/enforcement.md) still runs first. When it allows a call, the decision endpoint then decides it with session context, provided the hook sends the session identifier and the tool input.

The decision request accepts these optional fields in addition to the existing ones:

| Field | Meaning |
|---|---|
| `sessionID` | The agent's session identifier |
| `toolUseID` | The tool-use identifier, for correlation with the post-tool audit event |
| `input` | The tool input as JSON |
| `cwd` | The working directory |
| `hostname`, `localUsername`, `userEmail` | Device and user hints, since devices are not tied to users |

The response adds `sessionID`, `effect`, `policyName`, `approvalID`, `labels` and `risk`. Post-tool audit events submitted to the local-agent audit log are ingested into the same session.

## Claude Environments

Nothing to configure. Each HTTP request leaving a session is decided at Obot's egress processor before the Substrate policy runs, and responses are ingested as untrusted content unless the host is on the trusted-hosts list. A denied request receives a 403 whose body is the decision's reason.

## Microsoft Copilot Studio

Copilot Studio's [external threat detection](https://learn.microsoft.com/en-us/microsoft-copilot-studio/external-security-provider) (preview) calls an external provider before each tool invocation and expects a block-or-allow answer in under one second; no answer is treated as allow on the Copilot side.

Obot implements the [provider interface](https://learn.microsoft.com/en-us/microsoft-copilot-studio/external-security-webhooks-interface-developers):

| Copilot Studio calls | Obot route |
|---|---|
| `POST {endpoint}/analyze-tool-execution?api-version=2025-05-01` | `POST /api/copilot-studio/runtime-monitor` |
| `POST {endpoint}/validate?api-version=2025-05-01` | `POST /api/copilot-studio/validate` |

Configure the provider in the Power Platform admin center with `https://<your-obot>/api/copilot-studio` as the endpoint. Copilot Studio authenticates with a Microsoft Entra token for the app registration you create for the endpoint; front the route with a proxy that validates that token and injects an Obot API key, or expose the route to the Copilot Studio service only.

For each call Obot records the user's message as a model turn, ingests any previous tool outputs the planner carries, decides the tool invocation, and answers `blockAction: true` with the decision's reason or `blockAction: false`. The diagnostics field carries the session ID and the decision details. The whole exchange is bounded to 900 milliseconds, so `judge` effects degrade to require approval on this surface. Unknown fields and new API versions are tolerated.
