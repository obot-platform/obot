---
title: Audit usage and costs
---

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

Navigate to **LLM Gateway > Audit Logs**.

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

