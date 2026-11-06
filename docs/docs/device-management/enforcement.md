---
title: "Policy enforcement"
---

## Tool call enforcement {#tool-call-enforcement}

Tool call enforcement controls which tool calls Claude Code, Codex, Cursor, and Kiro may run on enrolled devices. Before a supported client runs a tool, Obot Sentry checks it against the allowlist for the device configuration. A call runs only when an allow rule matches it.

:::note
Tool call enforcement is experimental.
:::

### What enforcement does {#what-enforcement-does}

Enforcement keeps local AI clients on the MCP servers and tools you approve. It blocks the common ways an unapproved server ends up in use, such as a server copied from a blog post, a repository that ships its own `.mcp.json`, or a mistyped package name. Every allow and block is recorded, so you can see which servers people try to use and approve the ones they need.

Obot Sentry runs on the user's device and works from each client's own configuration, so enforcement has limits that a determined user can work around. Read [Limitations](#limitations) before you roll it out. When you need controls that the user can't change, run agents and MCP servers on infrastructure Obot manages instead; see [Strengthen enforcement](#strengthen-enforcement).

### Fail-closed behavior and client support {#fail-closed-behavior}

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

:::important
Kiro runs hooks only in folders the user has trusted. In an untrusted folder, Kiro still runs the agent and asks the user to approve each tool call, but Obot Sentry neither checks nor audits those calls. The same is true in a window with no folder open. Kiro also reads trust only when it starts, so a folder the user trusts afterward stays unchecked until Kiro is restarted.

Cover this in your MDM:

- Decide how to handle workspace trust. Setting Kiro's `security.workspace.trust.enabled` to `false` treats every folder as trusted, so the hooks always run, but it also turns off Kiro's protection against untrusted repositories, and users can change it back. Obot Sentry does not change this setting.
- Tell users to restart Kiro after they trust a new folder.

A user who turns off an Obot Sentry hook in Kiro's Agent Hooks panel only does so until the next hourly hook installation, which turns it back on.

The Kiro CLI runs these hooks only on its v3 agent engine (`kiro-cli chat --v3`, or the CLI setting `chat.agentEngine` set to `v3`). Its default v2 engine does not run them, so CLI sessions on v2 are neither checked nor audited. Obot Sentry does not change this setting. If you set it through your MDM, users can still choose v2 for a single session with `--v2`.
:::

### How Obot Sentry identifies the MCP server {#how-servers-are-identified}

When a client calls an MCP tool, its hook reports the tool name and the name the server is configured under. It does not report the URL, package, or command behind that name. Obot Sentry works that out by reading the client's MCP configuration files the same way the client does, and then checks the result against the allowlist.

For Claude Code, Obot Sentry reads, from highest precedence to lowest:

1. The managed MCP configuration (`managed-mcp.json`). If this file exists and does not list the server, the call is blocked, because Claude Code does not let users override it.
2. Project configuration for the folder Claude Code was started in and each folder above it: `.mcp.json` files and per-project servers in `~/.claude.json`.
3. The user's servers in `~/.claude.json`.
4. claude.ai connectors recorded in `~/.claude.json`.

If the same server name is defined differently in two of these places and Obot Sentry can't tell which one the client used, it blocks the call and the decision asks you to rename one of them.

Because the server is identified from configuration files, an enforcement decision records the server Obot Sentry found in the configuration. It does not record the network connection or process the client actually started.

### Configure enforcement {#configure-enforcement}

In **Administration > Device Management > Devices**, open the **Configuration** view and find **Tool Call Enforcement**.

1. Review the rules under **Allow**. When enforcement is enabled for the first time with an empty allowlist, Obot starts with Obot-hosted MCP servers, built-in agent tools, and built-in agent MCP servers allowed.
2. Add any other MCP servers and tools your users need.
3. Turn on **Enforce tool calls on enrolled devices**, then save the configuration.
4. Download the updated install package and follow its included `INSTRUCTIONS.md` to apply the change to devices. The included instructions contain the setup steps for the selected operating system and deployment method.

Consider starting with a few devices, then fill in the allowlist from their [enforcement decisions](#review-enforcement-decisions) before rolling out more widely.

Changing the allowlist takes effect on devices that already have enforcement set up without reinstalling Obot Sentry. Turning enforcement off in Obot stops blocking immediately and stops recording new enforcement decisions. Follow the package's included instructions if you also want to remove enforcement from devices.

### Allow rules {#allow-rules}

The broad allow rules are:

| Rule | What it allows |
|------|----------------|
| All Obot-hosted MCP servers | Any MCP server hosted by this Obot instance. |
| All built-in agent tools | The client's own tools, such as reading and writing files, running shell commands, and starting tasks. |
| All built-in agent MCP servers | MCP servers that ship as part of a supported AI client: Claude Code's built-in servers, and the JavaScript REPLs that the Codex app's built-in browser and computer use run on, including in the ChatGPT desktop app. |
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

## Limitations {#limitations}

### What a determined user can get around {#what-a-determined-user-can-get-around}

Enforcement depends on files and settings that the user can change on their own device. A user who means to use an unapproved MCP server can do so in ways that include:

- **Removing the hooks.** Claude Code and Kiro hooks are installed in the user's home directory, so the user can edit or delete them until Obot Sentry's next hourly hook installation puts them back. Codex and Cursor hooks are installed in machine-level files, so they are only as protected as those files' permissions. Use your MDM to check that the hook files are in place and to report devices where they're missing as out of compliance.
- **Shadowing an allowed server.** A client can load MCP configuration from places Obot Sentry doesn't read, such as Claude Code's `--mcp-config` flag. A user can define a server there under the same name as an allowed one. Obot Sentry then checks the allowed server while the client runs the other. For Claude Code, a [managed MCP configuration](https://code.claude.com/docs/en/managed-mcp) closes this gap by stopping Claude Code from loading any server that isn't in it, including servers passed with `--mcp-config`. Other clients may have similar controls. Check each client you allow, and treat one without them as a gap.
- **Replacing the package runner.** Obot Sentry treats a configuration entry that changes `PATH` or other package-resolution settings as unidentifiable and blocks it. It can't see which `npx` or `uvx` the user's own shell finds first, so a different executable earlier on the user's `PATH` can run anything under an allowed package name.
- **Changing configuration around the check.** Anything that runs as the user, including the user's own hooks, can change a client's configuration files between the time Obot Sentry reads them and the time the client uses them.

### What it can block by mistake {#what-it-can-block-by-mistake}

Enforcement fails closed, so anything Obot Sentry can't identify is blocked, including servers you want to allow:

- Servers defined only in configuration Obot Sentry doesn't read. For Claude Code, this includes servers that come from plugins or the `--mcp-config` flag.
- Local servers launched from an executable or script path rather than `npx` or `uvx`.
- A server name that is defined differently in more than one configuration file.

The Enforcement Decisions view shows the reason for each block. Use it to find and fix these cases.

## Strengthen enforcement {#strengthen-enforcement}

The limitations above come from enforcement running on a device the user controls and working from the client's own configuration. The strongest option is to move the work somewhere the user doesn't control:

- **Run Claude sessions in Claude Environments.** [Claude Environments](../claude-environments/overview.md) runs Claude cloud sessions in isolated sandboxes on your own Kubernetes cluster. Obot decides who can start a session and which vMCPs it can use, and an egress gateway outside the sandbox controls which hosts the session can reach, injects credentials, and audits every request. See [Network access, secrets, and auditing](../claude-environments/security.md). These controls sit outside the sandbox, so changing the agent's configuration inside a session doesn't get around them.
- **Host MCP servers in Obot.** Instead of letting each device run its own `npx` or `uvx` servers, [host them in Obot](../concepts/mcp-hosting.md) as isolated containers or Kubernetes workloads, with [egress rules](../configuration/mcp-server-egress-control.md) applied, and give users access through a vMCP.

On devices, use enforcement as one of several layers:

- **Use the client's own managed settings.** Some clients can be locked down directly, and those controls don't depend on reading configuration files. For example, Claude Code can be given a [managed MCP configuration](https://code.claude.com/docs/en/managed-mcp) that users can't override, and Obot Sentry respects it. Deliver these settings through your MDM.
- **Send remote MCP traffic through Obot.** A remote MCP server reached through an Obot gateway is governed by gateway access controls, filters, and audit logs on the server side, where the user can't change them. Network egress controls that block direct connections to remote MCP servers close the path around the gateway.
- **Manage the device with your MDM.** Keep the client settings that enforcement depends on in place, such as Kiro's hook and workspace trust settings, and keep the machine-level hook files for Codex and Cursor writable only by administrators.
- **Review enforcement decisions.** Repeated blocks show which servers people need. A device that keeps submitting scans but no longer reports enforcement decisions may have lost its hooks.
