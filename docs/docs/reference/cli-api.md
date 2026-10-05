---
description: CLI setup, commands, and API references.
title: CLI and APIs
---

Set up CLI authentication and local bootstrap skills with the procedure below. Use the linked references for catalog validation, tunnel operations, and gateway APIs. Internal Obot APIs are not all stable public integration surfaces.

| Task | Reference |
|---|---|
| Configure CLI authentication and local bootstrap skills | [CLI setup](#cli-setup-basic-usage) |
| Obtain and scope API keys with `obot login` | [Agent authorization scopes](../functionality/agent-auth-scopes.md#creating-an-agent-authorization-scope-with-the-cli) |
| Generate vMCP definitions for migrated Git composites | [Catalog migration](../configuration/mcp-server-gitops.md#migrating-git-synced-composites-to-vmcps) |
| Validate catalog YAML | [Catalog validation](../configuration/mcp-server-gitops.md#validating-catalog-entries) |
| Run an outbound MCP tunnel | [Tunnel CLI and API](../functionality/mcp-tunnels.md) |
| Discover MCP catalog entries | [MCP Registry API](../functionality/mcp-registry-api.md) |
| Call models through Obot | [LLM client examples](../llm-gateway/connect-clients.md) and [compatibility](../llm-gateway/compatibility.md) |

Check `obot --help` and each subcommand's `--help` for the installed CLI version. Use the server's displayed connection URLs and model identifiers rather than assuming catalog IDs and provider display names are connection identifiers.

The CLI's default login capabilities do not include general API access. Request `--scope api` explicitly when necessary and keep keys limited to the integration's needs. All credentials remain subject to the owning user's permissions.

The `obot setup` command prepares your local workstation to use an Obot server from the command line and from supported local AI clients.

Use it after an Obot server is running and reachable from your machine.

## What it does {#cli-setup-what-it-does}

`obot setup` performs these steps:

1. Resolves the Obot app URL to use, either from `--url`, from an existing local default, or by prompting you.
2. Authenticates to that Obot server. If `OBOT_TOKEN` is set, the CLI uses that token. Otherwise, it uses the same browser-based API key flow as `obot login`.
3. Stores the normalized default Obot URL in the local Obot CLI config.
4. Stores a newly acquired Obot API key in the host OS keyring, scoped to that Obot URL.
5. Optionally installs Obot bootstrap skills into supported local AI clients.

The bootstrap skills let local agents use the `obot` CLI to search for Obot-managed skills, install skills, and run local client scans without manually editing client configuration.

:::note
`obot setup` configures the local CLI and local client bootstrap files. It does not deploy the Obot server or configure server-side authentication providers.
:::

## Prerequisites {#cli-setup-prerequisites}

- The `obot` CLI is installed and available on your `PATH`.
- The Obot server URL is reachable from your workstation.
- If authentication is enabled, Obot has at least one configured authentication provider that your user can use.
- Your local OS keyring is available so the CLI can store a newly acquired API key.

If Obot authentication is enabled but no provider is configured yet, finish server-side authentication setup first. See [Enabling Authentication](../security/authentication.md) and [Auth Providers](../configuration/auth-providers.md).

## Basic usage {#cli-setup-basic-usage}

Run setup with your Obot app URL:

```bash
obot setup --url https://obot.example.com
```

For a local Docker deployment using the default port:

```bash
obot setup --url http://localhost:8080
```

If authentication is required, the CLI opens a browser to complete login. After login succeeds, setup saves the default URL and asks where to install local bootstrap skills.

## Choosing local client targets {#cli-setup-choosing-local-client-targets}

Use `--clients` to choose where bootstrap skills are installed:

| Value | Description | Install location |
|-------|-------------|------------------|
| `agents` | Install into the shared Agent Skills directory used by clients that support `~/.agents`. | `~/.agents/skills` |
| `claude-code` | Install into Claude Code's skills directory. | `~/.claude/skills` |
| `none` | Skip local client bootstrap installation. | Not applicable |

You can install into more than one target:

```bash
obot setup --url https://obot.example.com --clients agents,claude-code
```

To configure only CLI authentication and the default URL:

```bash
obot setup --url https://obot.example.com --clients none
```

When `--clients` is omitted in an interactive terminal, setup prompts you. The prompt always offers `agents`. It offers `claude-code` when Claude Code is detected locally. You can still install Claude Code support explicitly with `--clients claude-code`.

## Non-interactive setup {#cli-setup-non-interactive-setup}

For scripts or GUI wrappers, pass both `--url` and `--clients` with `--non-interactive`:

```bash
obot setup \
  --url https://obot.example.com \
  --clients agents \
  --non-interactive
```

Non-interactive mode never reads from stdin. It still uses the normal API key flow, so it may open a browser and wait for authentication unless a valid key is already stored.

Use `--yes` to accept defaults and confirmations. If `--clients` is omitted with `--yes`, setup installs the shared `agents` target by default:

```bash
obot setup --url https://obot.example.com --yes
```

If a different default Obot URL is already configured, setup refuses to replace it unless you pass `--yes`:

```bash
obot setup --url https://new-obot.example.com --yes
```

## Check setup status {#cli-setup-check-setup-status}

Use `obot setup status` to verify the local configuration:

```bash
obot setup status
```

The command prints:

- CLI version
- Default Obot URL
- Whether the stored API key is valid
- Whether setup is complete

For JSON output:

```bash
obot setup status --json
```

## What setup writes locally {#cli-setup-what-setup-writes-locally}

`obot setup` writes:

- The default Obot URL to the Obot CLI config file under the user's XDG config directory.
- An API key to the host OS keyring under the `obot` service, scoped by Obot app URL, when setup acquires a new key through the login flow.
- Bootstrap skill files under the selected client skill directories, such as `~/.agents/skills` or `~/.claude/skills`.

## Troubleshooting {#cli-setup-troubleshooting}

### `auth_unavailable` {#cli-setup-auth_unavailable}

The Obot server did not report exactly one usable configured authentication provider. Configure an auth provider first.

### `server_unreachable` {#cli-setup-server_unreachable}

Check that the URL points to the Obot app, that the server is running, and that the CLI can reach it from your workstation.

### Missing `--url` in non-interactive mode {#cli-setup-missing---url-in-non-interactive-mode}

Pass `--url`, or run setup interactively and enter the URL when prompted.

### `--clients is required in non-interactive mode` {#cli-setup---clients-is-required-in-non-interactive-mode}

Pass `--clients agents`, `--clients claude-code`, `--clients agents,claude-code`, or `--clients none`.

### Existing URL mismatch {#cli-setup-existing-url-mismatch}

If setup reports that another Obot URL is already configured, pass `--yes` to replace the stored default URL.
