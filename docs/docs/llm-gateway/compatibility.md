---
title: "API compatibility"
---

## Supported endpoints and formats {#supported-endpoints-and-formats}

The gateway serves every model at one base URL, `https://<your-obot-host>/api/llm-proxy`, in the OpenAI Chat Completions, OpenAI Responses and Anthropic Messages formats. Any format works with any model, so Claude Code can use GPT models and the OpenAI SDK can use Claude.

| Endpoint | Format |
| --- | --- |
| `/v1/chat/completions` | OpenAI Chat Completions |
| `/v1/responses`, `/v1/responses/input_tokens` | OpenAI Responses |
| `/v1/messages`, `/v1/messages/count_tokens` | Anthropic Messages |
| `/v1/models` | The models you can use |

Model names are `provider/model`, for example `openai/gpt-4.1`, `anthropic/claude-sonnet-4-5` or `bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0`. Bare provider model IDs such as `claude-sonnet-4-5` also work, as do undated Anthropic names and the `llm` and `llm-mini` [aliases](./how-it-works.md#using-model-aliases). See [Model names](./how-it-works.md#unified-gateway) for how a name that more than one provider serves is resolved.

Errors come back in the request's API format: Anthropic's error JSON for `/v1/messages` and for Anthropic clients (which send `anthropic-version`), and OpenAI's otherwise, so SDKs report them as they would the provider's own. This includes Obot's own refusals, such as HTTP 401 for a missing or invalid key, 403 for a model you may not use, and 404 for a model that isn't available.

## Deprecated provider endpoints {#deprecated-provider-endpoints}

Use `/api/llm-proxy/v1` for new setups. Migrate existing clients to this endpoint with provider-qualified model names. The following provider base URLs retain supported inference and model-listing operations, but are deprecated and will be removed in a future release.

| Provider base URL | Current gateway provider |
| --- | --- |
| `/api/llm-proxy/openai` | `openai` |
| `/api/llm-proxy/anthropic` | `anthropic` |
| `/api/llm-proxy/generic-responses` | `generic-responses` |
| `/api/llm-proxy/aws-bedrock`, `/api/llm-proxy/aws-bedrock-api-key` | `bedrock` when configured, otherwise `bedrock_mantle` |
| `/api/llm-proxy/azure`, `/api/llm-proxy/azure-entra` | `azure` |

Supported paths are `v1/chat/completions`, `v1/responses`, `v1/responses/input_tokens`, `v1/messages`, `v1/messages/count_tokens`, `v1/embeddings` (subject to the limitation above) and `v1/models`; paths without the `v1/` prefix also work. AWS routes also accept paths such as `anthropic/v1/messages` and `openai/v1/responses`, and Azure routes accept `openai/v1/models`. Unsupported operations return HTTP 410 with guidance to use the unified endpoint; these URLs do not restore every legacy operation.

Each route uses its corresponding current gateway provider and keys, including manually configured or recreated providers, independently of migration records. Azure and Bedrock authentication variants share their family's pool. Bare model names resolve within that provider, and model lists return accessible provider-native IDs that can be used on the same URL. Cross-provider model references are rejected with guidance to use `/api/llm-proxy/v1`. Model access policies, audit logging and budgets still apply; inference requests are not redirected.

AWS provider selection is refreshed on each request and prefers `bedrock` even when both adapters are configured. Model IDs are not converted between adapters, and failed inference does not retry against the other adapter. Missing AWS providers and unresolved AWS models return HTTP 410 with guidance to use `/api/llm-proxy/v1` and an appropriate model. Other unconfigured providers return normal gateway errors; authorization, budget and service failures keep their normal errors.

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
