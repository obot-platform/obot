---
title: "LLM Gateway overview"
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

## Overview {#overview}

The Obot LLM Gateway lets you call OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, and Azure models through Obot using an **Obot API key** instead of provider credentials. Point an OpenAI-, Anthropic-, or Bedrock Mantle-compatible client — such as [Claude Code](../functionality/llm-gateway.md#using-with-claude-code) or [Codex](../functionality/llm-gateway.md#using-with-codex) — at the gateway, authenticate with an API key that has LLM proxy access, and call models by their provider model names. For Azure, use the deployment name configured on the model.

The gateway proxies your requests transparently to the upstream provider while enforcing per-user access:

- You never handle the provider's real API key. Obot holds the key (configured by an administrator on a [Model Provider](../configuration/model-providers.md)) and substitutes it on each request.
- You can only call models that an administrator has granted you through a [Model Access Policy](../functionality/model-access-policies.md).
- The model list returned to your client (`/v1/models`) is scoped to the models you are allowed to use.


## The Models page {#the-models-page}

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

If you don't have access to any gateway models, the page shows a **"No gateway models available"** message — contact an administrator to request access through a [Model Access Policy](../functionality/model-access-policies.md).

:::info Use the name exactly as shown
The model name shown on the Models page is the value to put in your request's `model` field (and to select in your client). For OpenAI, Anthropic, and Generic Responses Compatible providers this is the provider's native model ID, including any `/` characters. For Amazon Bedrock, use the Mantle model ID returned by the Bedrock provider, such as `anthropic.claude-sonnet-5`, `openai.gpt-5.4`, or `google.gemma-4-31b`. For Azure and Azure Entra, use the Azure deployment name exactly as configured in Obot; it does not need to resemble the underlying model name.
:::

The **LLM Gateway** sidebar section also groups the administrator pages that power this feature:

- **Token Usage** — usage and cost analytics across users and models (admin only).
- **Audit Logs** — request history, token usage, outcomes, and exports for LLM gateway traffic (admin only). See [Audit Logs and Usage](../functionality/audit-logs-and-usage.md).
- **Model Providers** — configure providers and their available models (admin only). See [Model Providers](../configuration/model-providers.md).
- **Model Access Policies** — control which users can use which models (admin only). See [Model Access Policies](../functionality/model-access-policies.md).


## Before you begin {#before-you-begin}

To use the gateway you need:

1. **A configured provider.** An administrator must configure a supported [Model Provider](../configuration/model-providers.md) with valid credentials. This includes OpenAI, Anthropic, Generic Responses Compatible, Amazon Bedrock, Amazon Bedrock API key, Azure, and Azure Entra.
2. **Model access.** An administrator must grant you access to one or more of those models through a [Model Access Policy](../functionality/model-access-policies.md). The [Models page](../functionality/llm-gateway.md#the-models-page) reflects exactly what you can call.
3. **The Obot CLI.** Install and set up the `obot` CLI to obtain an API key. See [Obot CLI Setup](../installation/cli-setup.md).

