---
title: "Claude Code reference deployment"
---

This guide shows how to deploy Claude Code on managed devices so that it only uses MCP servers you approve. It combines three layers:

1. **Your MDM** installs configuration on the device, keeps it in place, and reports devices that drift.
2. **Claude Code's managed settings** decide which MCP servers Claude Code loads. Claude Code applies them itself, and users can't override them.
3. **Obot Sentry** checks each tool call against your Obot allowlist and records it, alongside local audit logs and device inventory.

Managed settings close the gaps in [Limitations](./enforcement.md#limitations) that come from Claude Code loading MCP configuration that Obot Sentry doesn't read. Other AI clients may have similar controls. Check each client you allow, and treat one without them as a gap.

The setting names and file locations below come from Anthropic's [managed MCP](https://code.claude.com/docs/en/managed-mcp) and [settings](https://code.claude.com/docs/en/settings) documentation. Check those pages against the Claude Code version you deploy.

## Choose an approach {#choose-an-approach}

| Approach | What it does | Use it when |
|----------|--------------|-------------|
| [Fixed server list](#fixed-server-list) (`managed-mcp.json`) | Claude Code loads only the servers in this file. Users can't add servers, including plugin servers and servers passed with `--mcp-config`. | Every user can get the same servers, such as a set of Obot vMCPs. |
| [Allowlist](#allowlist) (`managed-settings.json`) | Users can add servers, but Claude Code loads only the ones that match your allowlist and don't match your denylist. | Users need to choose their own servers from an approved set, such as any vMCP on your Obot instance. |

Either way, the [**All Obot-hosted MCP servers**](./enforcement.md#allow-rules) rule in Obot Sentry allows tool calls to any server on your Obot instance's URL, so Obot vMCPs need no other allow rule. Add rules for any other servers you approve.

## File locations {#file-locations}

Deploy both files with your MDM to a location that only administrators can write.

| File | macOS | Windows |
|------|-------|---------|
| `managed-mcp.json` | `/Library/Application Support/ClaudeCode/managed-mcp.json` | `C:\Program Files\ClaudeCode\managed-mcp.json` |
| `managed-settings.json` | `/Library/Application Support/ClaudeCode/managed-settings.json` | `C:\Program Files\ClaudeCode\managed-settings.json` |

Claude Code doesn't read the older `C:\ProgramData\ClaudeCode\managed-settings.json` location.

Any user on the device can read these files, so don't put secrets in them.

## Fixed server list {#fixed-server-list}

`managed-mcp.json` uses the same format as a project `.mcp.json` file. List the vMCP connection URLs you copy from Obot:

```json
{
  "mcpServers": {
    "obot-engineering": {
      "type": "http",
      "url": "https://obot.example.com/mcp-connect/<vmcp-id>"
    },
    "obot-support": {
      "type": "http",
      "url": "https://obot.example.com/mcp-connect/<vmcp-id>"
    }
  }
}
```

When this file exists:

- Claude Code loads no other MCP servers, including servers from user and project configuration, plugins, and `--mcp-config`. Starting Claude Code with `--mcp-config` fails with an error.
- claude.ai connectors are turned off unless a managed source sets `"allowAllClaudeAiMcps": true`.
- Obot Sentry reads this file first and blocks any server that isn't in it.

Because every server in this example is a remote Obot vMCP, there is no local package runner for a user to replace. Users sign in to Obot the first time they connect, and Obot's access controls decide which tools each user gets.

## Allowlist {#allowlist}

Set the allowlist in `managed-settings.json`:

```json
{
  "allowManagedMcpServersOnly": true,
  "allowedMcpServers": [
    { "serverUrl": "https://obot.example.com/*" }
  ],
  "disableSideloadFlags": true
}
```

- `allowedMcpServers` lists the servers Claude Code may load. Each entry has one key: `serverUrl` (wildcards allowed), `serverCommand` (an exact command and argument list), or `serverName`. A `serverName` entry matches only the name a user gives the server, so it isn't a security control. Use `serverUrl` or `serverCommand`. An empty list allows no servers.
- `deniedMcpServers`, if you add it, blocks servers from any source. A denylist match always wins over the allowlist.
- `allowManagedMcpServersOnly` makes the managed allowlist the only one Claude Code uses, so users can't add their own entries.
- `disableSideloadFlags` makes Claude Code reject the `--mcp-config`, `--plugin-dir`, `--plugin-url`, and `--agents` flags at startup. It requires Claude Code 2.1.193 or later.

Claude Code applies the allowlist to servers from every source, including plugins, claude.ai connectors, and `--mcp-config`.

## Don't block Obot Sentry's hooks {#dont-block-obot-sentrys-hooks}

Obot Sentry installs Claude Code's hooks in the user's settings file, `~/.claude/settings.json`. Claude Code settings that allow only managed hooks also block Obot Sentry's hooks, which turns off tool call auditing and enforcement. Don't set:

- `allowManagedHooksOnly`
- `strictPluginOnlyCustomization` set to `true`, or set to a list that includes hooks

The MCP settings in this guide don't depend on Obot Sentry's hooks. If a user removes the hooks, Claude Code still loads only the servers you allow, but Obot loses the audit trail and its own allowlist check for that device until Obot Sentry reinstalls the hooks on its next hourly run.

## Deliver and monitor with your MDM {#deliver-and-monitor-with-your-mdm}

1. Deploy the Obot Sentry package by following the `INSTRUCTIONS.md` in your device configuration download. See [Enroll devices](./enroll.md).
2. Deploy `managed-mcp.json`, `managed-settings.json`, or both to the [file locations](#file-locations) above, writable only by administrators.
3. Add a compliance check that reports a device as out of compliance when either managed file or Obot Sentry's Claude Code hook in `~/.claude/settings.json` is missing.
4. Confirm on a test device: run `/status` in Claude Code and check that it lists **Enterprise managed settings**, then make a tool call and confirm it appears in Obot's enforcement decisions.

Claude Code can also read managed settings from MDM policy (the `com.anthropic.claudecode` preference domain on macOS, or `HKLM\SOFTWARE\Policies\ClaudeCode` on Windows) instead of a file. See Anthropic's [settings documentation](https://code.claude.com/docs/en/settings) for the format.

## Server-managed settings {#server-managed-settings}

On a Claude for Teams or Claude for Enterprise plan, an organization Owner can also deliver Claude Code settings from **Organization settings > Claude Code > Managed settings** in Claude. These apply to every user in the organization who signs in with their Claude account, and they can carry `allowedMcpServers` and `deniedMcpServers`. They take effect only for users who sign in through your organization, so pair them with MDM-delivered files on managed devices. See Anthropic's [server-managed settings documentation](https://code.claude.com/docs/en/server-managed-settings).
