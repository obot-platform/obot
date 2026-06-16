---
title: LLM Gateway
---

## Overview

The Obot LLM Gateway lets you call OpenAI and Anthropic models through Obot using an **Obot API key** instead of a provider API key. Point any OpenAI- or Anthropic-compatible client — such as [Claude Code](#using-with-claude-code) or [Codex](#using-with-codex) — at the gateway, authenticate with an API key that has LLM proxy access, and call models by their native provider names (for example `claude-opus-4.8` or `gpt-5.5`).

The gateway proxies your requests transparently to the upstream provider while enforcing per-user access:

- You never handle the provider's real API key. Obot holds the key (configured by an administrator on a [Model Provider](/configuration/model-providers/)) and substitutes it on each request.
- You can only call models that an administrator has granted you through a [Model Access Policy](/functionality/model-access-policies/).
- The model list returned to your client (`/v1/models`) is scoped to the models you are allowed to use.

## The Models page

The **Models** page lists the OpenAI and Anthropic models you currently have access to through the gateway. Find it under **Models** in the sidebar (route `/llm-gateway/models`).

For each provider you have access to, the page shows:

- **Base URL** — the gateway endpoint to point your client at, with a copy button:
  - OpenAI: `https://<your-obot-host>/api/llm-proxy/openai`
  - Anthropic: `https://<your-obot-host>/api/llm-proxy/anthropic`
- **Example request** — a ready-to-run `curl` command, pre-filled with one of your available models and with the API key wired to `obot login --scope llm --print-token`.
- **Available models** — a searchable list of the models you can call. Each entry has a copy button for the exact model name to send in your requests.

If you don't have access to any OpenAI or Anthropic models, the page shows a **"No gateway models available"** message — contact an administrator to request access through a [Model Access Policy](/functionality/model-access-policies/).

:::info Use the name exactly as shown
The model name shown on the Models page is the value to put in your request's `model` field (and to select in your client). For OpenAI and Anthropic this is the provider's native model ID, such as `gpt-5.5` or `claude-opus-4.8`.
:::

The **LLM Gateway** sidebar section also groups the administrator pages that power this feature:

- **Token Usage** — usage and cost analytics across users and models (admin only).
- **Model Providers** — configure providers and their available models (admin only). See [Model Providers](/configuration/model-providers/).
- **Model Access Policies** — control which users can use which models (admin only). See [Model Access Policies](/functionality/model-access-policies/).

## Before you begin

To use the gateway you need:

1. **A configured provider.** An administrator must configure the OpenAI and/or Anthropic [Model Provider](/configuration/model-providers/) with a valid API key.
2. **Model access.** An administrator must grant you access to one or more of those models through a [Model Access Policy](/functionality/model-access-policies/). The [Models page](#the-models-page) reflects exactly what you can call.
3. **The Obot CLI.** Install and set up the `obot` CLI to obtain an API key. See [Obot CLI Setup](/installation/cli-setup/).

## Getting your API key

Your API key for the gateway must include LLM proxy access. Print one with the CLI:

```bash
obot login --url https://obot.example.com --scope llm --print-token
```

:::note
Add `--no-expiration` if you want a key that doesn't expire.
:::

## Quick start with curl

### Anthropic

```bash
export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy/anthropic"
export ANTHROPIC_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# Assumes you have access to claude-opus-4.8 via a Model Access Policy
curl $ANTHROPIC_BASE_URL/v1/messages \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-opus-4.8","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}'
```

### OpenAI

The OpenAI passthrough serves the OpenAI **Responses API** (`/v1/responses`).

```bash
export OPENAI_BASE_URL="https://obot.example.com/api/llm-proxy/openai"
export OPENAI_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# Assumes you have access to gpt-5.5 via a Model Access Policy
curl $OPENAI_BASE_URL/v1/responses \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.5","input":[{"role":"user","content":"hi"}]}'
```

## Using with Claude Code

Claude Code can route through the Anthropic passthrough and even discover which models you have access to.

```bash
# 1. Point Claude Code at the gateway and authenticate with your Obot API key.
export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy/anthropic"
export ANTHROPIC_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# 2. (Optional) Discover the models you have access to at startup.
#    Claude Code queries the gateway's /v1/models endpoint and adds the results
#    to the /model picker. Requires Claude Code v2.1.129 or later.
export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1

# 3. Launch Claude Code.
claude
```

Inside Claude Code:

- Run **`/model`** and select an Obot gateway model. Discovered models are labeled **"From gateway"**.
- Run **`/status`** to confirm which authentication method is active.

:::note Logging out of an existing account
If Claude Code is already signed in to an Anthropic subscription, run **`/logout`** inside the session first, then restart with the gateway environment variables set. Use `/login` later to switch back.
:::

:::tip Auth header alternative
`ANTHROPIC_API_KEY` is sent as the `x-api-key` header. If your setup prefers a bearer token, you can set `ANTHROPIC_AUTH_TOKEN` instead (sent as `Authorization: Bearer`). The gateway accepts either header with an Obot API key that has LLM proxy access.
:::

See the Claude Code documentation for details:

- [LLM gateway configuration](https://code.claude.com/docs/en/llm-gateway)
- [Model configuration](https://code.claude.com/docs/en/model-config) (the `/model` picker and gateway discovery)
- [Authentication](https://code.claude.com/docs/en/authentication) (`/login` and `/logout`)
- [Environment variables](https://code.claude.com/docs/en/env-vars)

## Using with Codex

Codex works with the OpenAI passthrough. Because Codex uses the OpenAI **Responses API**, define a custom model provider in your Codex config.

1. Add the following to `~/.codex/config.toml`:

   ```toml
   model_provider = "obot_openai"

   [model_providers.obot_openai]
   name = "OpenAI Obot LLM Gateway"
   base_url = "https://obot.example.com/api/llm-proxy/openai"
   env_key = "OBOT_API_KEY"
   env_key_instructions = "Set OBOT_API_KEY and restart to authenticate with the Obot LLM Gateway"
   supports_websockets = false
   ```

   For a local deployment, use `base_url = "http://localhost:8080/api/llm-proxy/openai"`.

2. Set your API key and start Codex:

   ```bash
   export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"
   codex        # Codex CLI
   # or
   codex app    # Codex App
   ```

Set `model` in your config (or pick one in Codex) to a model name shown on the [Models page](#the-models-page), for example `gpt-5.5`.

:::note
Codex uses the OpenAI Responses API by default, which is what the gateway serves. The provider ID (`obot_openai` above) can be any name except the reserved IDs `openai`, `ollama`, and `lmstudio`.
:::

See the Codex documentation for details:

- [Configuration reference](https://developers.openai.com/codex/config-reference) (the `[model_providers]` keys)
- [Advanced configuration](https://developers.openai.com/codex/config-advanced) (custom providers and `wire_api`)

## Limitations

- **OpenAI and Anthropic only.** Only the OpenAI and Anthropic passthrough routes are available to external clients. Other configured providers (Azure, Amazon Bedrock, Google Vertex, the Generic Responses Compatible provider, etc.) are **not yet** exposed through the gateway.
- **Access is policy-bound.** You can only call models an administrator has granted you through a [Model Access Policy](/functionality/model-access-policies/), and `/v1/models` returns only those models. A request for a model you don't have access to is rejected.
- **Send the exact model name.** Use the model name as shown on the [Models page](#the-models-page). If the requested model doesn't match the provider for the route (for example, an Anthropic model on the OpenAI route), the request is rejected.
- **Claude Code model discovery caveats.** Gateway model discovery is **off by default**, requires **Claude Code v2.1.129+**, and only adds models whose IDs begin with `claude` or `anthropic`. Obot's Anthropic models qualify; if discovery doesn't surface a model, select it manually with `/model`.
- **OpenAI uses the Responses API.** The OpenAI passthrough supports the Responses API (`/v1/responses`); the Chat Completions endpoint is not currently supported. Codex uses the Responses API by default.
- **Usage and policies still apply.** Requests count toward Obot [token usage](/functionality/audit-logs-and-usage/) and, where configured, are subject to [Message Policies](/functionality/message-policies/).

## Related topics

- [Model Providers](/configuration/model-providers/) — configure OpenAI and Anthropic providers and their models
- [Model Access Policies](/functionality/model-access-policies/) — grant users access to specific models
- [Obot CLI Setup](/installation/cli-setup/) — install and configure the `obot` CLI
- [Audit Logs and Usage](/functionality/audit-logs-and-usage/) — monitor token usage
