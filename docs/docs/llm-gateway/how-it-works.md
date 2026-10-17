---
title: "LLM Gateway overview"
---

## Overview {#overview}

Obot reaches models through its built-in LLM gateway. The gateway serves every configured model behind one base URL, and clients call it with an **Obot API key** instead of provider credentials. Point an OpenAI- or Anthropic-compatible client, such as [Claude Code](./connect-clients.md#using-with-claude-code) or [Codex](./connect-clients.md#using-with-codex), at the gateway and call models by name.

The gateway forwards requests to the upstream provider and enforces per-user access:

- You never handle the provider's real API key. An administrator adds providers and their keys under **LLM Gateway** → **Providers**, and Obot substitutes the key on each request. See [Configure providers and models](../configuration/model-providers.md).
- You can only call models that an administrator has granted you through a [Model Access Policy](../functionality/model-access-policies.md).
- The model list returned to your client (`/v1/models`) is scoped to the models you are allowed to use.

Obot's own features, such as the MCP Inspector's chat and LLM judge guardrails, use the same gateway.

## Unified gateway {#unified-gateway}

The gateway serves models from OpenAI, Anthropic, Amazon Bedrock, Google Vertex AI and Gemini, Azure, Mistral, Groq, xAI, DeepSeek, OpenRouter, Cohere, vLLM, Ollama and more behind a single base URL:

```
https://<your-obot-host>/api/llm-proxy/v1
```

- **Any model, any API format.** Every model is available in the OpenAI Chat Completions (`/v1/chat/completions`), OpenAI Responses (`/v1/responses`) and Anthropic Messages (`/v1/messages`) formats, so Claude Code can use GPT models and the OpenAI SDK can use Claude. See [API compatibility](./compatibility.md#supported-endpoints-and-formats) for every endpoint.
- **Model names** are `provider/model`, for example `openai/gpt-4.1`, `anthropic/claude-sonnet-4-5` or `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`. Bare provider model IDs such as `claude-sonnet-4-5` also work, which is what Claude Code sends by default. So do undated Anthropic names such as `claude-sonnet-4-5` or `anthropic/claude-sonnet-4-5`, which Anthropic lists only by their dated IDs (`claude-sonnet-4-5-20250929`): they are served by the latest dated version. When several providers serve a bare ID, such as `gpt-4o-mini` from both OpenAI and Azure, the gateway picks one you have access to, preferring the provider that makes the model (OpenAI, Anthropic, Gemini, Mistral, Cohere, xAI, DeepSeek) over one that hosts it. Use `provider/model` to choose the provider yourself.
- **Fallbacks.** Add `"fallbacks": ["anthropic/claude-sonnet-4-5"]` to a request to fail over to other models when the primary one errors. Each fallback must also be a model you have access to. See [Routing rules and fallbacks](#routing-rules-and-fallbacks).
- **Authentication and access.** Use an Obot API key (`obot login --scope llm --print-token`). You can call only the models granted to you by your [profiles](../functionality/model-access-policies.md). [Guardrails](../functionality/guardrails.md), the LLM audit log and usage reporting all apply.
- **Budgets.** Usage of a model is limited by the budgets of your profiles that grant it. A request that would exceed a budget gets `429` with the budget's name and when more becomes available.

## The LLM Gateway page {#the-models-page}

The **Connect** tab of the **LLM Gateway** page shows the gateway's base URLs, how many models you can use (select it to list them by provider), and a card for each client (Claude Code, Codex CLI, the OpenAI and Anthropic SDKs and `curl`). Select a card to open ready-to-copy configuration for that client, and pick the model it uses. Find the page under **AI Resources** → **LLM Gateway** in the sidebar (route `/models`). If you don't have access to any models, contact an administrator to request access through a [profile](../functionality/model-access-policies.md).

Models from gateway providers are listed on the **LLM Gateway** page under the *LLM Gateway* provider and are granted to users by profiles.

### Administering the gateway {#administering-the-gateway}

Administrators manage the gateway from these pages:

| Where | What it does |
| --- | --- |
| **Usage** → **Model** | Requests, success rate, latency, tokens and spend over time, spend by provider and the top models, and usage by user, API key and model. |
| **LLM Gateway** → **Providers** | Add upstream providers and their keys, and set the [default models](#default-models). See [Configure providers and models](../configuration/model-providers.md). |
| **LLM Gateway** → **Profiles** | Who gets what: each [profile](../functionality/model-access-policies.md) names users and groups, the models they may use, their budgets and their applied guardrails, all edited and saved together in the profile's form. Profiles add up: a user in several profiles may use the models of each, and the applied guardrails of each apply to them. A budget covers only its own profile's models. |
| **LLM Gateway** → **Profiles** → a profile → **Budgets** | Caps on spend (USD), total/input/output tokens or requests over a rolling window of up to a year (e.g. the last 6 hours) or a calendar window (this hour, day, week or month, UTC). A budget either gives each of the profile's users their own allowance or is one pool they share, covers all of its profile's models or only some of them, and can exempt administrators. A budget applies to a request only when its profile grants the requested model, and every budget that applies must have room, so when several profiles' budgets cover the same model, the tightest one limits it. A profile without models has budgets that never apply. Obot enforces budgets from the usage it records in its own database, so usage is counted exactly across Obot replicas. A budget isn't a hard ceiling, though: a request is admitted while the budget has room and its usage is recorded when it finishes, so requests running at the same time can together go past the limit. |
| **LLM Gateway** → **Profiles** → a profile → **Applied Guardrails** | Guardrails applied to the profile (guardrail policies in the API) that block, redact or log sensitive or unwanted content in the profile's model requests and responses (including streams), using guardrails. See [Guardrails](../functionality/guardrails.md). |
| **LLM Gateway** → **Guardrails** | **Guardrails** configure the checks to run, for profiles' applied guardrails and for [guardrail filters](../functionality/filters.md#guardrail-filters) on MCP traffic, with built-in secrets, PII and LLM-judge checks and 3rd party integrations with Presidio, Azure AI Language and Content Safety, AWS Bedrock Guardrails, Google Model Armor, Lakera Guard (Check Point), Patronus and CrowdStrike AIDR. Its **Used by** column shows what uses each guardrail, including MCP guardrail filters. See [Guardrails](../functionality/guardrails.md). |
| **LLM Gateway** → **Routing** | Rules that match requests with a [CEL](https://github.com/google/cel-spec) expression (over `model`, `provider`, `headers`, ...) and send them to other providers or models, split traffic by weight, or define fallback chains. See [Routing rules and fallbacks](#routing-rules-and-fallbacks). |
| **Audit Logs** → **Model** | Every request with its user, the model it asked for and the provider, model and key that served it, whether a fallback did, its guardrail findings, tokens, duration and content. See [Audit usage and costs](./audit.md). |
| **LLM Gateway** → **Settings** | Queueing behavior and request size limits. |

Prometheus metrics for the gateway (requests, latency, tokens and cost by provider, model, key and budget) are served at `GET /api/llm-gateway/metrics` (metric names start with `obot_llm_gateway_`) to administrators, auditors and credentials allowed to read `/debug/metrics`.

## Default models {#default-models}

Obot features such as LLM judge guardrails use default model aliases instead of specific models. Set them with **Set Default Models** on the **Providers** tab:

| Alias | Used for |
| --- | --- |
| **Language Model (Chat)** (`llm`) | The primary conversational model |
| **Language Model (Chat - Fast)** (`llm-mini`) | Quick responses and screening passes |
| **Vision** (`vision`) | Image understanding |
| **Image Generation** (`image-generation`) | Image generation tools |
| **Text Embedding** (`text-embedding`) | Embeddings |

Obot fills an unset alias with a preferred model as soon as a provider serves one:

- `llm` and `vision`: `openai/gpt-5.4`, then `anthropic/claude-sonnet-4-6`
- `llm-mini`: `openai/gpt-5-mini`, then `anthropic/claude-haiku-4-5`
- `image-generation`: `openai/dall-e-3`
- `text-embedding`: `openai/text-embedding-3-large`

Setting a default model does not grant anyone access to it. A [Model Access Policy](../functionality/model-access-policies.md#default-model-aliases) that grants the alias gives access to whichever model the alias points to.

### Using model aliases {#using-model-aliases}

Clients may also send `llm` or `llm-mini` as the model name in gateway requests. These aliases use the same global targets, and the caller still needs access to the model the alias resolves to.

## Routing rules and fallbacks {#routing-rules-and-fallbacks}

A request can be served by a different model from the one the client asked for:

- A **routing rule** (**LLM Gateway** → **Routing**) whose condition matches sends the request to its targets, splitting traffic between them by weight. Rules are checked in priority order, lowest number first, and the first match applies. A rule can also list fallbacks.
- A **fallback** is tried when the model before it fails. Fallbacks come from the request's `fallbacks` field or from the routing rule that matched. They aren't tried when the provider rejects the request itself as invalid (such as HTTP 400 for a malformed image or an unknown parameter), since the fallbacks would reject it too: the caller gets the provider's error, and no other provider receives the request.
- A **time to first token timeout** on a routing rule target (in seconds) cuts off a streaming request that gets no first token in time and moves it to the next fallback without retrying. When that target is chosen, the timeout applies to it and to each fallback except the last, which always runs to an answer, so it has no effect without a fallback. Requests that don't stream aren't affected.

Whichever model ends up serving a request:

- **The caller must be allowed to use it.** [Model access policies](../functionality/model-access-policies.md) are checked for the requested model, for each fallback in the request before it is sent, and for every model the gateway then tries, including routing rule targets and fallbacks. An attempt on a model the caller may not use is refused with HTTP 403 and isn't sent to the provider. If the request has a fallback the caller may use, it is tried next. Otherwise the caller gets the 403, for example `user does not have permission to use model "anthropic/claude-opus-4-8"`.

  A routing rule therefore can't give anyone a model their access policy doesn't include. To route requests for one model to another, grant the target model to the same users and groups. This also covers rules that match a request header: a client can set any header, but the header can only route it to models it may already use.

- **Its budgets must have room.** Every budget of the caller's profiles that grant the model being tried, and that covers that model, is checked for each attempt (except attempts a user's [Claude subscription](./connect-clients.md#claude-code-with-a-claude-subscription) pays for), so a budget limited to a model applies whenever that model serves a request, however the request reached it. An attempt whose budget is exhausted is refused, and the gateway tries the next fallback, if any. If none can serve the request, the caller gets the budget's 429.

- **Usage is recorded against the model that served it.** Usage reports and budgets record that model, price the request at its rates, and charge the budgets that apply to it. A request that was refused is recorded against the model that was requested.

The LLM audit log (**Audit Logs** → **Model**) records both the model each request asked for and the provider, model and provider key that served it, and whether a fallback did.

## Before you begin {#before-you-begin}

To use the gateway you need:

1. **A configured provider.** An administrator must add at least one provider with valid credentials under **LLM Gateway** → **Providers**. See [Configure providers and models](../configuration/model-providers.md).
2. **Model access.** An administrator must grant you access to one or more models through a [Model Access Policy](../functionality/model-access-policies.md). The [LLM Gateway page](#the-models-page) shows exactly what you can call.
3. **The Obot CLI.** Install and set up the `obot` CLI to obtain an API key. See [Obot CLI Setup](../reference/cli-api.md).

Then [connect your clients](./connect-clients.md). For failed requests or model access errors, see [Troubleshooting](./troubleshooting.md).
