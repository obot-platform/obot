---
title: Audit usage and costs
---

Use audit records to investigate individual requests and usage views to understand consumption over time.

1. Open **Audit Logs** and select **Model**.
2. Filter by time, user, provider, model, client, or session.
3. Inspect the request outcome, duration, and input/output token counts. Sensitive request and response fields require the Auditor role.
4. Use the token-usage view to compare users and models over the same period.
5. Configure a one-time or scheduled [audit export](../configuration/audit-log-export.md) when records need to outlive the configured retention period.

Estimated spend is not a provider invoice. In particular, Azure API-key deployments with custom names may report tokens without estimated spend because their deployment names cannot be matched to the pricing catalog. See [provider configuration](../configuration/model-providers.md#azure).

MCP and LLM audit retention are configured independently. Review [Audit data, privacy, and retention](../security/audit-data.md) before exporting sensitive fields.

## LLM Gateway Audit Logs {#audit-logs-and-usage-llm-gateway-audit-logs}

LLM gateway audit logs capture requests that flow through Obot's OpenAI and Anthropic-compatible gateway routes.

### What's Logged {#audit-logs-and-usage-whats-logged-1}

- **Model information**: Provider, requested model, and target model
- **Request information**: Request path, method, response status, and outcome
- **Token usage**: Input and output token counts
- **Client information**: Client name, version, session ID, and IP address
- **User information**: Who made the request
- **Timestamps and duration**: When the request occurred and how long it took

### Viewing LLM Audit Logs {#audit-logs-and-usage-viewing-llm-audit-logs}

Navigate to **Operations > Audit Logs**, then select **Model**.

The LLM audit log view shows request metadata, token usage, model information, and outcomes. Users with the Auditor role can view sensitive request and response fields when available.

### Filtering LLM Audit Logs {#audit-logs-and-usage-filtering-llm-audit-logs}

Filter LLM logs by:

- Date range
- User
- Model provider
- Target model
- Request path
- Response status
- Outcome
- Client
- Client session
- Search query

### Exporting LLM Audit Logs {#audit-logs-and-usage-exporting-llm-audit-logs}

LLM audit logs can be exported as one-time or scheduled JSONL exports using the same storage configuration as MCP audit log exports. See [Audit Log Export](../configuration/audit-log-export.md) for configuration options.
