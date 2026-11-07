---
title: "API compatibility"
---

## Supported endpoints and formats {#supported-endpoints-and-formats}

The gateway serves every model at one base URL, `https://<your-obot-host>/api/llm-proxy/v1`, in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Any format works with any model, so Claude Code can use GPT models and the OpenAI SDK can use Claude.

| Endpoint | Format |
| --- | --- |
| `/v1/chat/completions` | OpenAI Chat Completions |
| `/v1/responses`, `/v1/responses/input_tokens` | OpenAI Responses |
| `/v1/messages`, `/v1/messages/count_tokens` | Anthropic Messages |
| `/v1/embeddings` | Embeddings. Embedding models can't be granted through a profile yet, so it has no usable models. |
| `/v1/models` | The models you can use |

Model names are `provider/model`, for example `openai/gpt-4.1`, `anthropic/claude-sonnet-4-5` or `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`. Bare provider model IDs such as `claude-sonnet-4-5` also work, as do undated Anthropic names and the `llm` and `llm-mini` [aliases](./how-it-works.md#using-model-aliases). See [Model names](./how-it-works.md#unified-gateway) for how a name that more than one provider serves is resolved.

Errors come back in the request's API format: Anthropic's error JSON for `/v1/messages` and for Anthropic clients (which send `anthropic-version`), and OpenAI's otherwise, so SDKs report them as they would the provider's own. This includes Obot's own refusals, such as HTTP 401 for a missing or invalid key, 403 for a model you may not use, and 404 for a model that isn't available.

The earlier `/api/llm-proxy/openai` and `/api/llm-proxy/anthropic` routes still work. Other per-provider routes are removed and return HTTP 410. See [Client routes](./upgrade.md#client-routes).

## Limitations {#limitations}

- **Access is policy-bound.** You can only call models an administrator has granted you through a [Model Access Policy](../functionality/model-access-policies.md), and `/v1/models` returns only those models. A request for a model you don't have access to is rejected.
- **Send the exact model name.** Use the model name shown on the [LLM Gateway page](./how-it-works.md#the-models-page) exactly as displayed.
- **Claude Code model discovery.** Gateway model discovery is off by default and requires Claude Code v2.1.129 or later.
- **Minimum output token limit for OpenAI-format providers.** A `max_tokens`, `max_completion_tokens` or `max_output_tokens` under 16 is raised to 16 for OpenAI and OpenAI-compatible providers, the smallest limit OpenAI's Responses API accepts. Anthropic models get the limit as sent.
- **Usage is recorded.** Requests count toward Obot [usage and budgets](./audit.md).
- **Audit logs can be exported.** Administrators can create one-time or scheduled exports for LLM gateway audit logs. See [Audit Log Export](../configuration/audit-log-export.md).

## Related topics {#related-topics}

- [Configure providers and models](../configuration/model-providers.md) — add providers and their keys
- [Model Access Policies](../functionality/model-access-policies.md) — grant users access to specific models
- [Guardrails](../functionality/guardrails.md) — block, redact or log content in model traffic
- [Obot CLI Setup](../reference/cli-api.md) — install and configure the `obot` CLI
- [Audit usage and costs](./audit.md) — monitor token usage
- [Audit Log Export](../configuration/audit-log-export.md) — export LLM gateway audit logs
