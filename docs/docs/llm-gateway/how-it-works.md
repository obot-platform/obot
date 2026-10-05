---
title: "LLM Gateway overview"
---

## Overview {#overview}

The Obot LLM Gateway lets users call OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, and Azure models through Obot using an **Obot API key** instead of provider credentials. Users point an OpenAI-, Anthropic-, or Bedrock Mantle-compatible client — such as [Claude Code](./connect-clients.md#using-with-claude-code) or [Codex](./connect-clients.md#using-with-codex) — at the gateway, authenticate with an API key that has LLM proxy access, and call models by their provider model names. For Azure, use the deployment name configured on the model.

The gateway proxies client requests to the upstream provider while enforcing per-user access:

- Users do not need upstream provider API keys. An administrator configures the credentials on a [Model Provider](../configuration/model-providers.md), and Obot supplies them when forwarding requests.
- Users can call only the models granted to them through a [Model Access Policy](../functionality/model-access-policies.md).
- The model list returned to a client (`/v1/models`) includes only models that user is allowed to use.


## The Models page {#the-models-page}

The **Models** page lists the OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Azure, and Azure Entra models available to the signed-in user through the gateway. Find it under **Models** in the sidebar (route `/models`).

For each provider the user has access to, the page shows:

- **Base URL** — the gateway endpoint for client requests, with a copy button:
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
- **Example request** — a ready-to-run `curl` command, pre-filled with one of the user's available models and with the API key wired to `obot login --scope llm --print-token`.
- **Available models** — a searchable list of the models the user can call. Each entry has a copy button for the exact model name to send in requests.

If the user has no gateway model access, the page shows **"No gateway models available"**. An administrator must grant access through a [Model Access Policy](../functionality/model-access-policies.md).

:::info Use the name exactly as shown
The model name shown on the Models page is the value to put in your request's `model` field (and to select in your client). For OpenAI, Anthropic, and Generic Responses Compatible providers this is the provider's native model ID, including any `/` characters. For Amazon Bedrock, use the Mantle model ID returned by the Bedrock provider, such as `anthropic.claude-sonnet-5`, `openai.gpt-5.4`, or `google.gemma-4-31b`. For Azure and Azure Entra, use the Azure deployment name exactly as configured in Obot; it does not need to resemble the underlying model name.
:::

### Using model aliases

Clients can send `"model": "llm"` or `"model": "llm-mini"` instead of a provider model ID. These are installation-wide defaults configured by an administrator under **Models > Model Providers > Set Default Models**:

- **`llm`**: Language Model (Chat).
- **`llm-mini`**: Language Model (Chat - Fast).

Obot resolves the alias to its current model, checks the caller's access, and forwards the provider's model ID upstream. The alias must be configured and point to an active model. It does not bypass access policies.

Use the gateway endpoint for the provider that owns the resolved model. For example, if `llm` points to an Anthropic model, send it to the Anthropic endpoint. Obot rejects an alias whose model belongs to a different provider than the requested route. Changing a global alias to a different provider can therefore require a client endpoint change as well.

### Administrator pages

- **Models > Model Providers**: Configure provider credentials, available models, and global default aliases. See [Model Providers](../configuration/model-providers.md).
- **Models > Access Policies**: Grant users and groups access to models or aliases. See [Model Access Policies](../functionality/model-access-policies.md).
- **Operations > Usage > Model**: Review model usage and costs.
- **Operations > Audit Logs > Model**: Review gateway requests and exports. See [Audit data](../security/audit-data.md).

## Before you begin {#before-you-begin}

Gateway access requires:

1. **A configured provider.** An administrator must configure a supported [Model Provider](../configuration/model-providers.md) with valid credentials. This includes OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Amazon Bedrock API key, Azure, and Azure Entra.
2. **Model access.** An administrator must grant the user access to one or more of those models through a [Model Access Policy](../functionality/model-access-policies.md). The [Models page](./how-it-works.md#the-models-page) shows the models available to that user.
3. **The Obot CLI.** Install and set up the `obot` CLI to obtain an API key. See [Obot CLI Setup](../reference/cli-api.md).

For failed requests, model access errors, or provider route mismatches, see [Troubleshooting](./troubleshooting.md).
