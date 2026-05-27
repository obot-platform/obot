---
title: Obot CLI Setup
---

# Obot CLI Setup

The `obot setup` command prepares your local workstation to use an Obot server from the command line and from supported local AI clients.

Use it after an Obot server is running and reachable from your machine.

## What it does

`obot setup` performs these steps:

1. Resolves the Obot app URL to use, either from `--url`, from an existing local default, or by prompting you.
2. Authenticates to that Obot server. If `OBOT_TOKEN` is set, the CLI uses that token. Otherwise, it uses the same browser-based token flow as `obot login`.
3. Stores the normalized default Obot URL in the local Obot CLI config.
4. Stores a newly acquired Obot bearer token in the host OS keyring, scoped to that Obot URL.
5. Optionally installs Obot bootstrap skills into supported local AI clients.

The bootstrap skills let local agents use the `obot` CLI to search for Obot-managed skills, install skills, and run local client scans without manually editing client configuration.

:::note
`obot setup` configures the local CLI and local client bootstrap files. It does not deploy the Obot server or configure server-side authentication providers.
:::

## Prerequisites

- The `obot` CLI is installed and available on your `PATH`.
- The Obot server URL is reachable from your workstation.
- If authentication is enabled, Obot has at least one configured authentication provider that your user can use.
- Your local OS keyring is available so the CLI can store a newly acquired authentication token.

If Obot authentication is enabled but no provider is configured yet, finish server-side authentication setup first. See [Enabling Authentication](/installation/enabling-authentication/) and [Auth Providers](/configuration/auth-providers/).

## Basic usage

Run setup with your Obot app URL:

```bash
obot setup --url https://obot.example.com
```

For a local Docker deployment using the default port:

```bash
obot setup --url http://localhost:8080
```

If authentication is required, the CLI opens a browser to complete login. After login succeeds, setup saves the default URL and asks where to install local bootstrap skills.

## Choosing local client targets

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

## Non-interactive setup

For scripts or GUI wrappers, pass both `--url` and `--clients` with `--non-interactive`:

```bash
obot setup \
  --url https://obot.example.com \
  --clients agents \
  --non-interactive
```

Non-interactive mode never reads from stdin. It still uses the normal token flow, so it may open a browser and wait for authentication unless a valid token is already stored.

Use `--yes` to accept defaults and confirmations. If `--clients` is omitted with `--yes`, setup installs the shared `agents` target by default:

```bash
obot setup --url https://obot.example.com --yes
```

If a different default Obot URL is already configured, setup refuses to replace it unless you pass `--yes`:

```bash
obot setup --url https://new-obot.example.com --yes
```

## Check setup status

Use `obot setup status` to verify the local configuration:

```bash
obot setup status
```

The command prints:

- CLI version
- Default Obot URL
- Whether the stored token is valid
- Whether setup is complete

For JSON output:

```bash
obot setup status --json
```

## What setup writes locally

`obot setup` writes:

- The default Obot URL to the Obot CLI config file under the user's XDG config directory.
- A bearer token to the host OS keyring under the `obot` service, scoped by Obot app URL, when setup acquires a new token through the login flow.
- Bootstrap skill files under the selected client skill directories, such as `~/.agents/skills` or `~/.claude/skills`.

## Troubleshooting

### `auth_unavailable`

The Obot server did not report exactly one usable configured authentication provider. Configure an auth provider first, or use an interactive setup flow if multiple providers are configured and you need to choose one.

### `server_unreachable`

Check that the URL points to the Obot app, that the server is running, and that the CLI can reach it from your workstation.

### Missing `--url` in non-interactive mode

Pass `--url`, or run setup interactively and enter the URL when prompted.

### `--clients is required in non-interactive mode`

Pass `--clients agents`, `--clients claude-code`, `--clients agents,claude-code`, or `--clients none`.

### Existing URL mismatch

If setup reports that another Obot URL is already configured, pass `--yes` to replace the stored default URL.
