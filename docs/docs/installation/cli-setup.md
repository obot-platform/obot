---
displayed_sidebar: sidebar
title: Obot CLI Setup
---

For the current task-based instructions, see [CLI and APIs](../reference/cli-api.md). This page preserves existing bookmarks.

## What it does {#what-it-does}

Continue to [What it does](../reference/cli-api.md#cli-setup-what-it-does).

## Prerequisites {#prerequisites}

Continue to [Prerequisites](../reference/cli-api.md#cli-setup-prerequisites).

## Basic usage {#basic-usage}

Continue to [Basic usage](../reference/cli-api.md#cli-setup-basic-usage).

## Choosing local client targets {#choosing-local-client-targets}

Continue to [Choosing local client targets](../reference/cli-api.md#cli-setup-choosing-local-client-targets).

## Non-interactive setup {#non-interactive-setup}

Continue to [Non-interactive setup](../reference/cli-api.md#cli-setup-non-interactive-setup).

## Check setup status {#check-setup-status}

Continue to [Check setup status](../reference/cli-api.md#cli-setup-check-setup-status).

## What setup writes locally {#what-setup-writes-locally}

Continue to [What setup writes locally](../reference/cli-api.md#cli-setup-what-setup-writes-locally).

## Troubleshooting {#troubleshooting}

Continue to [Troubleshooting](../reference/cli-api.md#cli-setup-troubleshooting).

### `auth_unavailable` {#auth_unavailable}

Continue to [`auth_unavailable`](../reference/cli-api.md#cli-setup-auth_unavailable).

### `server_unreachable` {#server_unreachable}

Continue to [`server_unreachable`](../reference/cli-api.md#cli-setup-server_unreachable).

### Missing `--url` in non-interactive mode {#missing---url-in-non-interactive-mode}

Continue to [Missing `--url` in non-interactive mode](../reference/cli-api.md#cli-setup-missing---url-in-non-interactive-mode).

### `--clients is required in non-interactive mode` {#--clients-is-required-in-non-interactive-mode}

Continue to [`--clients is required in non-interactive mode`](../reference/cli-api.md#cli-setup---clients-is-required-in-non-interactive-mode).

### Existing URL mismatch {#existing-url-mismatch}

Continue to [Existing URL mismatch](../reference/cli-api.md#cli-setup-existing-url-mismatch).

## Connect an MCP client through the local CLI

Use `obot mcp connect <URL>` as a STDIO server command in your MCP client's configuration:

```json
{
  "mcpServers": {
    "obot": {
      "command": "obot",
      "args": ["mcp", "connect", "https://obot.example.com/mcp-connect/<connection-id>"]
    }
  }
}
```

Copy the connection URL from Obot. Install a CLI version that includes `mcp connect` and ensure your MCP client can find `obot` on its `PATH`, or supply the full executable path.

The CLI opens your browser to authenticate and relays MCP traffic through Obot. This connection uses OAuth independently of the API key stored by `obot setup`. Its Obot OAuth credentials are stored with restricted file permissions under the platform's user data directory in `obot/mcp-connect`; upstream provider credentials remain on the Obot server.

For a provider that requires localhost OAuth redirects, enable **Localhost OAuth callback** in the remote server or catalog entry configuration. Leave the callback path blank for `/oauth/callback`, or enter the provider's required path. The CLI registers its own callback with Obot, receives the provider callback on the same local port, and redirects the browser back to Obot to finish authentication.

By default, the CLI accepts provider callbacks only at `/oauth/callback`. For a custom catalog callback path, pass `--callback-path`:

```bash
obot mcp connect "https://obot.example.com/mcp-connect/<connection-id>" \
  --callback-path /custom/provider/callback
```

Repeat the flag for a vMCP with multiple provider callback paths. Explicit flags replace the default, so include `--callback-path /oauth/callback` if some components use the default and others use custom paths. Obot's generated client configurations include these arguments automatically. If a catalog callback path changes, reinstall or update the client configuration.

Paths must be absolute, without query strings or fragments. `/oauth/obot/callback` is reserved for the CLI's own login. The CLI forwards provider callbacks only on the allowed paths while an authorization flow is active.

Run the CLI on the same computer as your browser. It binds an available loopback port; providers that require a fixed port are not supported. Hosted callbacks remain the default when the setting is disabled. A browser-only connection cannot supply the local relay needed by a localhost-only provider.

The MCP client manages the CLI process. Diagnostics use stderr, while stdout is reserved for MCP messages. If the browser cannot open automatically, the CLI prints an authorization URL to stderr. Authentication times out after five minutes; retry the connection to start again.
