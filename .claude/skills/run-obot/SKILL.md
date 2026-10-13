---
name: run-obot
description: Launch Obot locally and drive a change end to end — a UI flow in a real browser, or an API/MCP request against the running server. Use when asked to verify, run, or screenshot a change in the running app rather than only running tests.
---

# Verify a change in a running Obot

## Launch

```bash
make dev   # Go server on :8080 (dev mode, auth off) + SvelteKit UI on :5174, proxied through :8080
```

- Ready in seconds once Go is cached. If it isn't up within ~1 minute, read the log; don't keep waiting.
- `pnpm` requires Node `^24 || >=26`. If the default Node is outside that, run with a 24 on PATH, e.g. `PATH=$(mise where node@24)/bin:$PATH make dev`.
- The UI **must** be on 5174. If an old Vite still holds 5174, the new one moves to 5175 and :8080 can't reach it. Kill leftovers by PID (`ss -ltnp | grep -E ':(8080|8443|5174) '`), not with `pkill -f` patterns that can match your own shell.
- Readiness: `curl -sf localhost:8080/api/healthz && curl -sf -o /dev/null localhost:5174`.
- The first page load shows a "Welcome to Obot" EULA dialog; click **Continue**.

## Optional: connecting a local MCP server through a vMCP

Only needed when the change involves connecting to MCP servers, vMCPs, or the MCP Tester. Skip this section otherwise.

Remote MCP servers on localhost are refused by default, so start Obot with `OBOT_SERVER_DISALLOW_LOCALHOST_MCP=false`. Then:

```bash
# catalog entry for a local streamable-HTTP MCP server
curl -s -X POST localhost:8080/api/mcp-catalogs/default/entries -H 'content-type: application/json' \
  -d '{"name":"Test","runtime":"remote","remoteConfig":{"fixedURL":"http://localhost:9123/mcp/"}}'
# wrap it in a vMCP to connect to it (there is no POST /api/mcp-servers)
curl -s -X POST localhost:8080/api/vmcps -H 'content-type: application/json' \
  -d '{"displayName":"Test","components":[{"name":"t","mcpCatalogID":"default","mcpServerCatalogEntryID":"<entry id>"}]}'
```

- MCP Tester (inspector) UI: `http://localhost:8080/mcp-servers/test/<vmcp id>` → Tools tab.
- Protocol-level check: `POST localhost:8080/mcp-connect/<vmcp or vmcp instance id>` with a JSON-RPC `initialize`.
- After adding a tool to a test server, an existing vMCP kept showing the old tool list, even after Refresh in the tester (cause not investigated). Creating a new catalog entry and vMCP (e.g. for the server on another port) showed the new tool.

## Driving the UI

Playwright is in `ui/user/node_modules` (`.pnpm/playwright@*/node_modules/playwright/index.mjs`). Import it by absolute path from a script kept outside the repo. Don't wait on `networkidle`, which never settles against the Vite dev server; wait on visible elements instead. Use `exact: true` for button names like "Raw JSON", which also appear in tool descriptions.

## Sandbox-only gotchas (discobox and similar)

- With no default route, the server exits with `failed to detect current IP: dial udp 1.1.1.1:80`. Add a dummy default route: `ip link add e2edummy type dummy; ip addr add 10.254.254.1/24 dev e2edummy; ip link set e2edummy up; ip route add default dev e2edummy metric 1000`.
- If `HTTP_PROXY` is set, add that dummy subnet to `NO_PROXY`/`no_proxy`. Otherwise `/mcp-connect` hangs, because the gateway's request to its own detected IP goes through the egress proxy.
- With no CJK fonts installed, ja/ko/zh-CN pages render as boxes in screenshots; assert on DOM text instead.
