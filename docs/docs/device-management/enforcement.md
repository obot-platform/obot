---
title: "Policy enforcement"
---

## Tool call enforcement {#tool-call-enforcement}

Tool call enforcement controls which tool calls Claude Code, Codex, and Cursor may run on enrolled devices. Before a supported client runs a tool, Obot Sentry checks it against the allowlist for the device configuration. A call runs only when an allow rule matches it.

:::warning Experimental feature
Tool call enforcement is experimental and is not recommended for production use. Test the policy on non-production devices first. An incomplete allowlist or an unavailable Obot server can block users' work.
:::

Enforcement fails closed. A call is blocked when:

- No allow rule matches it.
- Obot Sentry cannot identify the MCP server targeted by the call.
- Obot cannot be reached or cannot return a decision.
- The device is not enrolled.

Local tool call auditing for Visual Studio Code continues to work, but Visual Studio Code does not currently support enforcement.

:::important
Cursor users with enforcement enabled need to go to `Cursor Settings → Rules, Skills, Subagents → Include third-party Plugins, Skills, and other configs` and turn it off.
Doing this will prevent Cursor from loading Claude's hooks and allow enforcement to work as expected.
:::

### Configure enforcement {#configure-enforcement}

In **Administration > Device Management > Devices**, open the **Configuration** view and find **Tool Call Enforcement**.

1. Review the rules under **Allow**. When enforcement is enabled for the first time with an empty allowlist, Obot starts with Obot-hosted MCP servers, built-in agent tools, and built-in agent MCP servers allowed.
2. Add any other MCP servers and tools your users need.
3. Turn on **Enforce tool calls on enrolled devices**, then save the configuration.
4. Download the updated install package and follow its included `INSTRUCTIONS.md` to apply the change to devices. The included instructions contain the setup steps for the selected operating system and deployment method.

Changing the allowlist takes effect on devices that already have enforcement set up without reinstalling Obot Sentry. Turning enforcement off in Obot stops blocking immediately and stops recording new enforcement decisions. Follow the package's included instructions if you also want to remove enforcement from devices.

### Allow rules {#allow-rules}

The broad allow rules are:

| Rule | What it allows |
|------|----------------|
| All Obot-hosted MCP servers | Any MCP server hosted by this Obot instance. |
| All built-in agent tools | The client's own tools, such as reading and writing files, running shell commands, and starting tasks. |
| All built-in agent MCP servers | MCP servers that ship as part of a supported AI client (currently supported only for Claude Code). |
| Everything | Every call that Obot Sentry can identify. All other allow rules are ignored while this is selected. |

You can also allow an individual MCP server by:

| Identity | Matching behavior |
|----------|-------------------|
| URL | Matches the scheme, hostname, port, and URL path. A path also covers paths beneath it. |
| Hostname | Matches every MCP server on that hostname, regardless of path or port. |
| Package | Matches an npm package launched with `npx` or a Python package launched with `uvx`. You can allow any version or require a specific version. |
| Connector | Matches a connector by display name, such as a claude.ai Connector. |

For each server, leave **Tools** empty to allow all of its tools, or list the specific tool names to allow. Enforcement matches tool names, not the arguments passed to a tool.

A local MCP server launched directly from an executable or script path does not provide a supported identity for the allowlist. Obot Sentry blocks calls to a server it cannot identify rather than treating its local command as trusted.

### Review enforcement decisions {#review-enforcement-decisions}

Open **Administration > Device Management > Enforcement Decisions** to review calls checked while enforcement was enabled. The view shows allowed and blocked totals for the selected date range and supports searching and filtering by result, device, agent, tool type, MCP server, and tool.

Open a decision to see:

- Why the call was allowed or blocked.
- The client, tool, and MCP server involved.
- The server identity Obot Sentry found, when available.
- The device that made the call.

For an identified, blocked MCP call, an administrator can add an allow rule directly from the decision. Depending on the identity available, the rule can allow the hostname, all tools on that server, or only the tool in that decision. The new rule applies to matching calls from every device using the configuration; it does not change the historical decision.
