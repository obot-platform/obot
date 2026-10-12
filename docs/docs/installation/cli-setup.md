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

## Authenticate providers that require localhost callbacks

Add the MCP connection URL from Obot to your client using its normal HTTP connection configuration. This also applies to vMCPs with providers that require localhost OAuth callbacks.

When your client connects, its browser authorization flow will show a command if a provider needs a localhost callback:

```bash
obot mcp login --url 'https://obot.example.com/oauth/mcp/login/<attempt>'
```

Install a CLI version that includes `mcp login` and ensure `obot` is on your terminal's `PATH`. Run the command on the same computer as your browser. It starts temporary listeners on IPv4 and IPv6 loopback addresses where available, opens the provider's authorization page, and relays the callback to Obot. Obot exchanges the code and stores the provider credentials; the login command does not create an MCP session or store tokens locally.

After the command completes, return to the Obot authorization page. Obot automatically verifies authentication and resumes your client's connection once every required component is authenticated. If a vMCP also requires ordinary browser authentication, complete those providers' sign-in steps as well. MCP traffic continues over the configured HTTP URL after the command exits.

The inspector and tool selection use the same login command. Each command belongs to one authentication attempt and expires after ten minutes. Obot automatically checks completed or expired attempts and obtains a new command if authentication is still needed. If the callback port is occupied or you need to restart sign-in, select **Get a new sign-in command**. The login helper retries failed checks automatically; screens that display a **Retry** action can also be retried manually. If the browser cannot open automatically, the command prints the provider authorization URL.

Administrators can enable **Localhost OAuth callback** in a remote server or catalog entry configuration. Leave the callback path blank for `/oauth/callback`, or enter the provider's required path. Paths must be absolute, without query strings or fragments. `/oauth/obot/callback` is reserved for the login helper's completion endpoint. Callback paths are carried in the authentication attempt and require no client configuration flags. Providers must accept an available port; fixed callback ports are not supported. Hosted callbacks remain the default when this setting is disabled.
