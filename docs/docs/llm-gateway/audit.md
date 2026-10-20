---
title: Audit usage and costs
---

Use audit records to investigate individual requests and usage views to understand consumption over time.

1. Open **Audit Logs** and select **LLM**.
2. Filter by time, user, API key, provider, model, user agent, or session.
3. Inspect the request outcome, duration, token counts and spend, and how the gateway routed it: the provider, model and provider key that served it, and whether a fallback did. Sensitive request and response fields require the Auditor role.
4. Use **Usage** > **Model** for requests, success rate, latency, tokens and spend over time, spend by provider and the top models, and to compare users, API keys and models over the same period.
5. Configure a one-time or scheduled [audit export](../configuration/audit-log-export.md) when records need to outlive the configured retention period.

Estimated spend is not a provider invoice; see [model pricing](#model-pricing). To cap spend rather than report it, use [budgets](./how-it-works.md#administering-the-gateway).

Usage reports come from Obot's token usage records, which are kept whether or not the LLM audit log is enabled. Every model request Obot sends to the gateway has one, failed ones included. Requests Obot refuses itself, such as for a model the caller may not use, and model listing and token counting requests, have none.

## Model pricing {#model-pricing}

The gateway prices each request for the model that served it, from the request's token usage and the model's published rates, including cache reads and writes and long-context rates. Spend in audit records, usage reports and USD budgets all use this price.

- **Where the rates come from:** Obot's published images and release binaries include model prices and capabilities derived from [LiteLLM's model list](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) (MIT license), and refreshes them every 6 hours from `https://upgrade-server.obot.ai/v1/model-data`. If the refresh can't reach that URL, the gateway keeps the prices it has. To change where they come from, or to use only the included copy, set [`OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL`](../configuration/model-providers.md#server-configuration).
- **Models without prices:** a model the list doesn't include, such as a custom deployment name or a self-hosted model, reports tokens without spend, and doesn't count toward USD budgets.
- **Cost overrides:** set `overrideCost` on a model with `PUT /api/models/{id}` to price it at your own rates instead. There's no UI control for it. An override replaces the gateway's price for that model.

MCP and LLM audit retention are configured independently. Review [Audit data, privacy, and retention](../security/audit-data.md) before exporting sensitive fields.

## LLM Gateway Audit Logs {#audit-logs-and-usage-llm-gateway-audit-logs}

LLM gateway audit logs capture requests that flow through Obot's LLM gateway, in any of the API formats it serves.

### What's Logged {#audit-logs-and-usage-whats-logged-1}

- **Model information**: Provider and requested model
- **Routing**: The provider and model that served the request, which [routing rules and fallbacks](./how-it-works.md#routing-rules-and-fallbacks) can make differ from the requested model, the provider key it was served with, or that the user's Claude subscription paid for it, and whether a fallback served it
- **Request information**: Request path, method, response status, and outcome
- **Token usage and spend**: Input and output token counts, the input tokens read from and written to the provider's prompt cache, the output tokens spent reasoning, and what the request cost (for a model with a price)
- **Client information**: Client name, version, session ID, and IP address
- **User information**: Who made the request
- **Timestamps and duration**: When the request occurred and how long it took
- **Guardrail findings**: For each [applied guardrail](../functionality/guardrails.md#applied-guardrails) that found something in the request or its response, or failed to check it: the policy, the guardrail, whether it checked the request or the response, the categories it found, whether it blocked, redacted, logged or failed, and, if it failed, why. Findings never include the content that was found.

### Viewing LLM Audit Logs {#audit-logs-and-usage-viewing-llm-audit-logs}

Navigate to **Operations > Audit Logs**, then select **LLM**.

The LLM audit log view shows request metadata, token usage, model information, and outcomes. Users with the Auditor role can view sensitive request and response fields when available.

A fork icon next to a request's model marks a request that another model served; the model that served it is shown under the requested one, and its details show the full route under **Routing**. A request a user's [Claude subscription](./connect-clients.md#claude-code-with-a-claude-subscription) paid for has no provider key; **Routing** shows **Paid by**: *The user's Claude subscription* instead. Its tokens are recorded, but no spend.

A shield next to a request's timestamp marks a request a guardrail acted on, and its details list each finding under **Guardrails**. Blocked requests are logged too, with the block in their response. Guardrail findings are not sensitive fields, so every user who can view the audit log sees them.

### Filtering LLM Audit Logs {#audit-logs-and-usage-filtering-llm-audit-logs}

Filter LLM logs by:

- Date range
- User
- API key
- Model provider
- Target model
- Served provider
- Path
- Response status
- Outcome
- User agent
- Client session
- Model discovery requests, which shows or hides model list requests. They're hidden by default.
- Search query

### Exporting LLM Audit Logs {#audit-logs-and-usage-exporting-llm-audit-logs}

LLM audit logs can be exported as one-time or scheduled JSONL exports using the same storage configuration as MCP audit log exports. See [Audit Log Export](../configuration/audit-log-export.md) for configuration options.
