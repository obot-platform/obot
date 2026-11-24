---
title: "Deploy Kiro with Obot"
---

This page describes how to deploy [Kiro](https://kiro.dev), the agentic IDE and CLI from AWS, so that its tool calls go through Obot. It combines Obot Sentry's [tool call enforcement](./enforcement.md) with the controls that Kiro gives administrators.

Obot Sentry's hooks check each Kiro tool call against your Obot allowlist, but Kiro skips those hooks in some windows and sessions. Kiro's own controls cover some of those cases. The sections below set up each layer and then list which layer covers which case.

The behavior on this page was tested with Kiro IDE 1.4.154 and Kiro CLI 3.6.0. Statements about Kiro features that require IAM Identity Center come from Kiro's documentation, which each section links to.

## Recommended deployment {#recommended-deployment}

1. Sign users in through IAM Identity Center, and block every other sign-in method with a managed settings file. See [Sign-in](#sign-in).
2. Host the MCP servers your users need in Obot, and publish a Kiro MCP registry that lists only those servers. See [MCP registry](#mcp-registry).
3. Enroll devices in Obot Sentry and turn on tool call enforcement. See [Obot Sentry enforcement](#obot-sentry-enforcement).
4. Set the Kiro settings that the Obot Sentry hooks depend on. See [Settings for the hooks](#settings-for-the-hooks).
5. Add permission rules to the managed settings file for the cases the hooks miss. See [Permission rules](#permission-rules).
6. Use your MDM to keep these files in place. See [Device management](#device-management).

## Sign-in {#sign-in}

Kiro reads a managed settings file from a fixed path on each device:

| Operating system | Path |
|------------------|------|
| macOS | `/Library/Application Support/Kiro/managed-settings.json` |
| Windows | `C:\ProgramData\Kiro\managed-settings.json` |

Both the Kiro IDE and the Kiro CLI read this file. Kiro reads it when it starts, so a change takes effect the next time the user starts Kiro.

A `signin_method` rule in this file blocks sign-in methods. The following file allows only IAM Identity Center:

```json
{
  "rules": [
    { "capability": "signin_method", "match": ["*"], "exclude": ["idc"], "effect": "deny" }
  ],
  "settings": {
    "idc_start_url": "https://example.awsapps.com/start",
    "idc_region": "us-east-1"
  }
}
```

Replace the start URL and Region with your own. If you connect Okta or Microsoft Entra ID to Kiro directly instead of through IAM Identity Center, exclude `external_idp` as well. The method names are `idc`, `external_idp`, `builder_id`, `google`, `github`, and `social`. Sign-in rules require Kiro IDE 1.2 or later and Kiro CLI 2.25.0 or later. See Kiro's [sign-in governance](https://kiro.dev/docs/enterprise/governance/sign-in/) documentation for the other settings.

Block the other methods even if your users already have Identity Center accounts. According to Kiro's [MCP governance](https://kiro.dev/docs/enterprise/governance/mcp/) documentation, the MCP registry and the console's MCP toggle apply only to users who sign in through IAM Identity Center or with an API key. A user who signs in with an AWS Builder ID, Google, or GitHub account is not subject to either one.

## MCP registry {#mcp-registry}

Kiro's MCP registry is the only Kiro control that identifies an MCP server by its URL or package. Every other Kiro control identifies a server by the name the user gives it in their own configuration.

You set the registry URL in the Kiro console. According to Kiro's [MCP governance](https://kiro.dev/docs/enterprise/governance/mcp/) documentation:

- The registry is a JSON file served over HTTPS, in a subset of the MCP registry v0.1 format. Each entry names one remote endpoint or one npm, PyPI, or OCI package.
- Kiro fetches the registry when it starts and every 24 hours after that.
- Kiro stops a server that is not in the registry, and prevents the user from adding it back.
- Users can change a registry server's environment variables, HTTP headers, and timeout. They cannot change its URL or package.
- The registry applies to the Kiro IDE and to both the v2 and v3 engines of the Kiro CLI.
- Kiro enforces the registry on the device, so a user with administrative access to the device can get around it.

List the servers that your users reach through Obot: [Obot-hosted servers](../concepts/mcp-hosting.md) and remote servers behind an Obot gateway. A registry entry for a remote server then points at Obot, where gateway access controls and audit logs apply on the server side.

## Obot Sentry enforcement {#obot-sentry-enforcement}

Enroll devices as described in [Enroll devices](./enroll.md), then turn on enforcement as described in [Configure enforcement](./enforcement.md#configure-enforcement).

For Kiro, Obot Sentry installs one hook file, `~/.kiro/hooks/obot-sentry.json`, in each user's home directory. It contains a `PreToolUse` hook that checks each tool call against the allowlist, and a `PostToolUse` hook that records each call. Obot Sentry reinstalls the file every hour.

The `PreToolUse` hook runs before Kiro shows its own approval prompt. Kiro blocks a tool call only when the hook exits with code 2. Obot Sentry exits with code 2 whenever it blocks a call, including when it cannot reach Obot.

For an MCP call, Kiro reports the tool as `mcp_<server>_<tool>`, in lowercase. Obot Sentry reads Kiro's MCP configuration files to find the server's URL or package, and checks that against the allowlist. Renaming a server in the configuration does not change which allowlist rule matches it.

## Settings for the hooks {#settings-for-the-hooks}

### Workspace trust {#workspace-trust}

The Kiro IDE runs hooks only in a folder the user has trusted. In an untrusted folder, Kiro runs the agent without the Obot Sentry hooks. If the agent tries to run a shell command there, Kiro asks the user to trust the folder. When the user accepts, the hooks still do not run in that window until the user restarts Kiro.

To make the hooks run in every folder, set `security.workspace.trust.enabled` to `false` in the user's Kiro settings, through your MDM. Kiro then opens every folder without Restricted Mode, and the hooks run. This setting also turns off Kiro's protection against code in untrusted repositories, and the user can change it back.

If you leave workspace trust on, tell users to restart Kiro after they trust a folder.

### Kiro CLI engine {#kiro-cli-engine}

The Kiro CLI runs hooks only on its v3 engine. Its v2 engine runs no hooks. Kiro CLI 3.6.0 uses v3 for interactive sessions by default. It uses v2 for `--no-interactive` sessions and piped input, unless the session also uses `--trust-all-tools`. It also uses v2 when it resumes a v2 session, or when the user chooses it with `--v2` or the `chat.agentEngine` setting.

The MCP registry applies to both engines, so it covers MCP calls in v2 sessions. The [permission rules](#permission-rules) below do not cover v2 sessions. In a v2 session started with `--trust-all-tools`, Kiro CLI 3.6.0 ran a shell command that a deny rule matched. The same rule blocked the command in a v3 session.

## Permission rules {#permission-rules}

The managed settings file from [Sign-in](#sign-in) also holds permission rules. Kiro applies these rules in every Kiro IDE window and in Kiro CLI sessions on the v3 engine, including the cases where the Obot Sentry hooks do not run. An administrator rule can only deny a call or make the user approve it. It cannot allow a call that the hooks or the user would block.

A permission rule names a capability, such as `mcp`, `shell`, `fs_read`, `fs_write`, or `web_fetch`, and optionally `match` and `exclude` patterns. For the `mcp` capability, Kiro compares the patterns against `<server name>/<tool name>`, where the server name is the key in the user's MCP configuration. A user can give any server any name. Use `mcp` rules for broad effects, not as an allowlist. A rule that denies every server except one named `obot` lets through any server that the user names `obot`.

The following file adds one permission rule to the sign-in rule. It makes the user approve every MCP call, including calls in windows where the hooks do not run:

```json
{
  "rules": [
    { "capability": "signin_method", "match": ["*"], "exclude": ["idc"], "effect": "deny" },
    { "capability": "mcp", "effect": "ask" }
  ],
  "settings": {
    "idc_start_url": "https://example.awsapps.com/start",
    "idc_region": "us-east-1"
  }
}
```

The `ask` rule overrides the user's own auto-approve settings for MCP tools. Leave it out if your users rely on auto-approved MCP tools.

A deny rule on `web_fetch` removes the web fetch tool, but the agent can still fetch a page with a shell command. Kiro's documentation pairs it with shell rules that deny `curl *` and `wget *`.

Kiro rejects the whole file when it contains invalid JSON, an `allow` effect, or an unknown field. Kiro then blocks every tool call and shows **Permissions: Failed** in the status bar. Test a new file on one device before you deploy it. See Kiro's [permission governance](https://kiro.dev/docs/enterprise/governance/permissions/) documentation for the full list of capabilities.

## Device management {#device-management}

Use your MDM to:

- Deploy the managed settings file, writable only by administrators. On Windows, save it as UTF-8 without a byte order mark.
- Report devices where `~/.kiro/hooks/obot-sentry.json` is missing as out of compliance.
- Report devices where the managed settings file is missing or has changed.
- Give users standard accounts without administrator rights. Kiro enforces the managed settings file and the MCP registry on the device, and a local administrator can remove both.

A user can turn off an Obot Sentry hook in Kiro's **Agent Hooks** panel. Kiro records this as `"enabled": false` in the hook file. The next hourly hook installation removes it, and Kiro applies the change without a restart.

A user who starts Kiro with the `KIRO_HOME` environment variable set to another directory loads no hooks from `~/.kiro`. Kiro keeps the user signed in, because it stores the sign-in token in `~/.aws/sso/cache`. The managed settings file still applies, because Kiro reads it from its fixed path.

## Coverage {#coverage}

This table shows which layer applies in each case. The MCP registry requires IAM Identity Center and was not tested for this page. Its column comes from Kiro's documentation where the documentation covers the case, and says "Not tested" where it does not.

| Case | Obot Sentry hooks | Permission rules | MCP registry |
|------|-------------------|------------------|--------------|
| Trusted folder in the Kiro IDE | Yes | Yes | Yes |
| Untrusted folder | No | Yes | Not tested |
| Folder trusted after Kiro started, before a restart | No | Yes | Not tested |
| Window with no folder open | No | Yes | Not tested |
| Kiro started with `KIRO_HOME` set | No | Yes | Not tested |
| Kiro CLI on the v3 engine | Yes | Yes | Yes |
| Kiro CLI on the v2 engine | No | No | Yes |
| User signed in with Builder ID, Google, or GitHub | Yes | Yes | No |

In a window with no folder open, Kiro blocks file tools with a built-in rule, and shell commands fail with an error. MCP calls and web fetches run, and only Kiro's own approval prompt and your permission rules apply to them.
