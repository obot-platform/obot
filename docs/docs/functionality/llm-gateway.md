---
title: LLM Gateway
---

## Overview

The Obot LLM Gateway lets you call OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, and Azure models through Obot using an **Obot API key** instead of provider credentials. Point an OpenAI-, Anthropic-, or Bedrock Mantle-compatible client — such as [Claude Code](#using-with-claude-code) or [Codex](#using-with-codex) — at the gateway, authenticate with an API key that has LLM proxy access, and call models by their provider model names. For Azure, use the deployment name configured on the model.

The gateway proxies your requests transparently to the upstream provider while enforcing per-user access:

- You never handle the provider's real API key. Obot holds the key (configured by an administrator on a [Model Provider](/configuration/model-providers/)) and substitutes it on each request.
- You can only call models that an administrator has granted you through a [Model Access Policy](/functionality/model-access-policies/).
- The model list returned to your client (`/v1/models`) is scoped to the models you are allowed to use.

## The Models page

The **Models** page lists the OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Azure, and Azure Entra models you currently have access to through the gateway. Find it under **Models** in the sidebar (route `/llm-gateway/models`).

For each provider you have access to, the page shows:

- **Base URL** — the gateway endpoint to point your client at, with a copy button:
  - OpenAI: `https://<your-obot-host>/api/llm-proxy/openai`
  - Anthropic: `https://<your-obot-host>/api/llm-proxy/anthropic`
  - Generic Responses Compatible: `https://<your-obot-host>/api/llm-proxy/generic-responses`
  - Amazon Bedrock:
    - Static credentials auth: `/api/llm-proxy/aws-bedrock`
    - API key auth: `/api/llm-proxy/aws-bedrock-api-key`
    - The gateway detects the API request format from the requested `/messages` or `/responses` endpoint. Bedrock-aware clients may also include an `anthropic/` or `openai/` path prefix.
  - Azure:
    - API key auth: `/api/llm-proxy/azure`
    - Entra auth: `/api/llm-proxy/azure-entra`
    - The gateway detects the API request format from the requested `/messages` or `/responses` endpoint. The deployment name does not determine the format.
- **Example request** — a ready-to-run `curl` command, pre-filled with one of your available models and with the API key wired to `obot login --scope llm --print-token`.
- **Available models** — a searchable list of the models you can call. Each entry has a copy button for the exact model name to send in your requests.

If you don't have access to any gateway models, the page shows a **"No gateway models available"** message — contact an administrator to request access through a [Model Access Policy](/functionality/model-access-policies/).

:::info Use the name exactly as shown
The model name shown on the Models page is the value to put in your request's `model` field (and to select in your client). For OpenAI, Anthropic, and Generic Responses Compatible providers this is the provider's native model ID, including any `/` characters. For Amazon Bedrock, use the Mantle model ID returned by the Bedrock provider, such as `anthropic.claude-haiku-4-5`, `openai.gpt-5.4`, or `google.gemma-4-31b`. For Azure and Azure Entra, use the Azure deployment name exactly as configured in Obot; it does not need to resemble the underlying model name.
:::

The **LLM Gateway** sidebar section also groups the administrator pages that power this feature:

- **Token Usage** — usage and cost analytics across users and models (admin only).
- **Audit Logs** — request history, token usage, outcomes, and exports for LLM gateway traffic (admin only). See [Audit Logs and Usage](/functionality/audit-logs-and-usage/).
- **Model Providers** — configure providers and their available models (admin only). See [Model Providers](/configuration/model-providers/).
- **Model Access Policies** — control which users can use which models (admin only). See [Model Access Policies](/functionality/model-access-policies/).

## Before you begin

To use the gateway you need:

1. **A configured provider.** An administrator must configure a supported [Model Provider](/configuration/model-providers/) with valid credentials. This includes OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Amazon Bedrock API key, Azure, and Azure Entra.
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

### Generic Responses Compatible

The Generic Responses route serves the **Responses API** using the base URL configured by your administrator. The upstream API key is optional, which supports local services such as Ollama as well as authenticated Responses API-compatible services such as LiteLLM.

```bash
export OPENAI_BASE_URL="https://obot.example.com/api/llm-proxy/generic-responses"
export OPENAI_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# Use a model name shown in the Generic Responses Compatible section of the Models page
curl $OPENAI_BASE_URL/v1/responses \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"open-model","input":[{"role":"user","content":"hi"}]}'
```

To list the Generic Responses models you can access:

```bash
curl $OPENAI_BASE_URL/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

### Amazon Bedrock

Amazon Bedrock gateway routes proxy directly to Bedrock Mantle. You must use the `/messages` API with `anthropic.*` models and the `/responses` API with `openai.*` and `google.*` models. The gateway uses the requested endpoint to select the corresponding Bedrock API. Vendor-specific tools such as Claude Code and Codex choose the correct request path automatically.

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# Anthropic-compatible Bedrock model
curl https://obot.example.com/api/llm-proxy/aws-bedrock/v1/messages \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"anthropic.claude-haiku-4-5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}'

# OpenAI-compatible Bedrock model
curl https://obot.example.com/api/llm-proxy/aws-bedrock/v1/responses \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "content-type: application/json" \
  -d '{"model":"openai.gpt-5.4","input":[{"role":"user","content":"hi"}]}'
```

Use `/api/llm-proxy/aws-bedrock-api-key/...` instead of `/api/llm-proxy/aws-bedrock/...` when your administrator configured the **Amazon Bedrock (API Key)** provider.

To list the Bedrock models Obot knows you can access:

```bash
curl https://obot.example.com/api/llm-proxy/aws-bedrock/v1/models \
  -H "Authorization: Bearer $OBOT_API_KEY"
```

For Anthropic models on Bedrock, model availability depends on AWS region and account access. See the [AWS Bedrock Anthropic model cards](https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards-anthropic.html) for region availability.

### Azure

Azure has separate gateway routes for API key and Entra authentication. The gateway uses the request endpoint to select the model dialect:

| Provider    | Base URL                            |
| ----------- | ----------------------------------- |
| Azure       | `/api/llm-proxy/azure`              |
| Azure Entra | `/api/llm-proxy/azure-entra`        |

Use `/v1/messages` for an `AnthropicMessages` deployment and `/v1/responses` for an `OpenAIResponses` deployment. Use the deployment name shown on the Models page as `model`; it is not used to select the request format.

```bash
export OBOT_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)"

# AnthropicMessages deployment
curl https://obot.example.com/api/llm-proxy/azure/v1/messages \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"my-claude-deployment","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}'

# OpenAIResponses deployment
curl https://obot.example.com/api/llm-proxy/azure/v1/responses \
  -H "Authorization: Bearer $OBOT_API_KEY" \
  -H "content-type: application/json" \
  -d '{"model":"my-gpt-deployment","input":[{"role":"user","content":"hi"}]}'
```

Replace `/azure/` with `/azure-entra/` when the administrator configured the **Azure Entra** provider. Client requests always authenticate to Obot with the Obot API key; Obot supplies the Azure API key or obtains the Entra token upstream.

#### Provider setup

An administrator configures one of these providers under **Model Providers**:

- **Azure** requires the Azure resource endpoint (for example, `https://my-resource.services.ai.azure.com`) and an API key.
- **Azure Entra** requires the Azure resource endpoint plus tenant ID, client ID, and client secret for a service principal. Obot requests tokens for the `https://ai.azure.com/.default` scope. Assign the service principal **Cognitive Services User** or **Foundry User** (formerly Azure AI User) on the specific Foundry resource. **Cognitive Services OpenAI User** may allow OpenAI requests but does not provide all permissions needed for Anthropic requests. See [Azure provider configuration](/configuration/model-providers/#azure-enterprise-only) for portal instructions and official references.

Configure each model's target model as its Azure deployment name. The provider's model metadata must expose either `AnthropicMessages` or `OpenAIResponses` as the dialect. The Models page groups models by that dialect, even if deployment names are arbitrary or misleading.

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

### Claude Code with Amazon Bedrock Mantle

Claude Code can also route Bedrock Mantle traffic through Obot. Use this mode for Bedrock `anthropic.*` models.

```bash
export ANTHROPIC_BEDROCK_MANTLE_BASE_URL="https://obot.example.com/api/llm-proxy/aws-bedrock"
export ANTHROPIC_AUTH_TOKEN="$(obot login --url https://obot.example.com --scope llm --print-token)"
export CLAUDE_CODE_SKIP_MANTLE_AUTH=1
export CLAUDE_CODE_USE_MANTLE=1

claude --model anthropic.claude-haiku-4-5
```

For a local Obot server, use `ANTHROPIC_BEDROCK_MANTLE_BASE_URL="http://localhost:8080/api/llm-proxy/aws-bedrock"`.

`--model` is optional, but it is useful since Claude Code does not support model discovery with AWS Bedrock. The default models presented by Claude Code's model selector should be compatible with our Bedrock provider as long as the corresponding Bedrock model ID is enabled in Obot.

For more details, see Claude Code's [Route Mantle through a gateway](https://code.claude.com/docs/en/amazon-bedrock#route-mantle-through-a-gateway) documentation.

### Claude Code with Azure

Claude Code can use an Azure deployment whose model dialect is `AnthropicMessages`:

```bash
ANTHROPIC_FOUNDRY_BASE_URL='https://obot.example.com/api/llm-proxy/azure' \
ANTHROPIC_FOUNDRY_API_KEY="$(obot login --url https://obot.example.com --scope llm --print-token)" \
CLAUDE_CODE_USE_FOUNDRY=1 \
claude --model my-claude-deployment
```

For Azure Entra through a local Obot server:

```bash
ANTHROPIC_FOUNDRY_BASE_URL='http://localhost:8080/api/llm-proxy/azure-entra' \
ANTHROPIC_FOUNDRY_API_KEY="$(obot login --url http://localhost:8080 --scope llm --print-token)" \
CLAUDE_CODE_USE_FOUNDRY=1 \
claude --model claude-haiku-4-5
```

Use `/api/llm-proxy/azure-entra` for the Azure Entra provider. `ANTHROPIC_FOUNDRY_API_KEY` contains an Obot gateway token in this setup, not an Azure API key; Obot replaces it with the configured Azure credential before forwarding the request. The value passed to `--model` must be the exact Azure deployment name listed in the Anthropic-compatible Azure section on the Models page.

:::note Model discovery
Microsoft Foundry does not implement the Anthropic Models API. Do not enable `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY`; pass the Azure deployment name with `--model` or configure Claude Code's default Foundry model variables.
:::

See the Claude Code documentation for details:

- [LLM gateway configuration](https://code.claude.com/docs/en/llm-gateway)
- [Claude Code on Microsoft Foundry](https://code.claude.com/docs/en/microsoft-foundry)
- [Route Mantle through a gateway](https://code.claude.com/docs/en/amazon-bedrock#route-mantle-through-a-gateway)
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

### Codex with Amazon Bedrock

Codex can use Bedrock `openai.*` and `google.*` models through Obot's OpenAI-compatible Bedrock route.

To adapt the OpenAI configuration above for Bedrock, change the model and provider base URL:

```toml
model = "openai.gpt-5.4"
# Bedrock's google.* models are also compatible
# model = "google.gemma-4-31b"

[model_providers.obot_openai]
base_url = "https://obot.example.com/api/llm-proxy/aws-bedrock"
```

For a local Obot server, use `base_url = "http://localhost:8080/api/llm-proxy/aws-bedrock"`.

Use `/api/llm-proxy/aws-bedrock-api-key` when your administrator configured the **Amazon Bedrock (API Key)** provider.

### Codex with Azure

Codex can use an Azure deployment whose model dialect is `OpenAIResponses`. Adapt the OpenAI configuration with the Azure deployment name and route:

```toml
model = "my-gpt-deployment"

[model_providers.obot_openai]
name = "Azure via Obot LLM Gateway"
base_url = "https://obot.example.com/api/llm-proxy/azure"
env_key = "OBOT_API_KEY"
supports_websockets = false
```

Use `/api/llm-proxy/azure-entra` for the Azure Entra provider.

See the Codex documentation for details:

- [Configuration reference](https://developers.openai.com/codex/config-reference) (the `[model_providers]` keys)
- [Advanced configuration](https://developers.openai.com/codex/config-advanced) (custom providers and `wire_api`)

## Limitations

- **Supported gateway providers.** External LLM Gateway clients can use OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Amazon Bedrock API key, Azure, and Azure Entra providers. Other configured providers such as Google Vertex are not exposed through provider-specific gateway routes yet.
- **Access is policy-bound.** You can only call models an administrator has granted you through a [Model Access Policy](/functionality/model-access-policies/), and `/v1/models` returns only those models. A request for a model you don't have access to is rejected.
- **Send the exact model name.** Use the model name shown on the [Models page](#the-models-page) exactly as displayed.
- **Claude Code model discovery caveats.** Gateway model discovery is off by default and requires Claude Code v2.1.129 or later for the standard Anthropic gateway path. Claude Code's Bedrock Mantle mode may not populate the `/model` picker from Obot, so pass `--model` or select an enabled Mantle model manually.
- **OpenAI-compatible routes use the Responses API.** The OpenAI, Generic Responses Compatible, Bedrock, and Azure OpenAI-compatible routes support the Responses API (`/v1/responses`); the Chat Completions endpoint is not currently supported. Codex uses the Responses API by default.
- **Usage and policies still apply.** Requests count toward Obot [token usage](/functionality/audit-logs-and-usage/) and, where configured, are subject to [Message Policies](/functionality/message-policies/).
- **Audit logs can be exported.** Administrators can create one-time or scheduled exports for LLM gateway audit logs. See [Audit Log Export](/configuration/audit-log-export/).

## Related topics

- [Model Providers](/configuration/model-providers/) — configure supported providers and their models
- [Model Access Policies](/functionality/model-access-policies/) — grant users access to specific models
- [Obot CLI Setup](/installation/cli-setup/) — install and configure the `obot` CLI
- [Audit Logs and Usage](/functionality/audit-logs-and-usage/) — monitor token usage
- [Claude Code: Route Mantle through a gateway](https://code.claude.com/docs/en/amazon-bedrock#route-mantle-through-a-gateway)
- [AWS Bedrock Anthropic model cards](https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards-anthropic.html) — check Anthropic model region availability
- [Audit Log Export](/configuration/audit-log-export/) — export LLM gateway audit logs
