---
title: Audit Logs & Usage
---

# Audit Logs & Usage

The MCP Platform provides visibility into MCP and LLM gateway activity through audit logs and usage tracking. These features help with monitoring, compliance, and understanding how MCP servers and LLM gateway models are being used.

:::info Auditor Role
Sensitive data (MCP request/response bodies, conversations, and workflow runs) can **only** be viewed by users with the Auditor role. All other roles, including Owner and Admin, see only metadata for these resources. The Auditor role is an add-on permission that can be combined with any other role, granting read-only access to sensitive data across the platform. See [User Roles](../configuration/user-roles.md#auditor) for details.
:::

## MCP Audit Logs

Audit logs capture all MCP interactions that flow through the gateway.

### What's Logged

- **MCP Requests**: Tool calls, resource access, and other MCP operations
- **MCP Responses**: Results returned from MCP servers
- **User Information**: Who made the request
- **Timestamps**: When the request occurred
- **Server Information**: Which MCP server handled the request

### Viewing Audit Logs

Navigate to **MCP Management > Audit Logs** in the MCP Platform.

The audit log view shows:
- Timestamp
- User
- MCP Server
- Operation type
- Status (success/failure)

### Detailed View

Click on any log entry to see additional details:
- Request and response metadata
- Error details (if applicable)
- Full request/response payloads and headers (Auditor role required)

### Filtering

Filter logs by:
- Date range
- User
- MCP Server
- Operation type
- Status

### Retention

Audit logs are automatically deleted after **90 days** by default. To preserve logs beyond this period, use the export functionality before they are deleted. See [Server Configuration](../configuration/server-configuration.md) for retention settings.

### Exporting Audit Logs

MCP audit logs can be exported for external analysis, compliance requirements, or long-term retention. See [Audit Log Export](../configuration/audit-log-export.md) for configuration options.

## LLM Gateway Audit Logs

LLM gateway audit logs capture requests that flow through Obot's OpenAI and Anthropic-compatible gateway routes.

### What's Logged

- **Model information**: Provider, requested model, and target model
- **Request information**: Request path, method, response status, and outcome
- **Token usage**: Input and output token counts
- **Client information**: Client name, version, session ID, and IP address
- **User information**: Who made the request
- **Timestamps and duration**: When the request occurred and how long it took

### Viewing LLM Audit Logs

Navigate to **LLM Gateway > Audit Logs**.

The LLM audit log view shows request metadata, token usage, model information, and outcomes. Users with the Auditor role can view sensitive request and response fields when available.

### Filtering LLM Audit Logs

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

### Exporting LLM Audit Logs

LLM audit logs can be exported as one-time or scheduled JSONL exports using the same storage configuration as MCP audit log exports. See [Audit Log Export](../configuration/audit-log-export.md) for configuration options.

## Body storage controls

MCP and LLM audit bodies are stored in full by default. Configure their limits independently:

| Audit logs | Environment variable | CLI flag | Bodies covered |
| --- | --- | --- | --- |
| MCP | `OBOT_SERVER_MCPAUDIT_LOG_MAX_BODY_BYTES` | `--mcpaudit-log-max-body-bytes` | Request, response, mutated-request, and original-response |
| LLM | `OBOT_SERVER_LLMAUDIT_LOG_MAX_BODY_BYTES` | `--llmaudit-log-max-body-bytes` | Request, policy-modified request, and response |

Leave a setting unset for unlimited bodies, set it to `0` to omit bodies, or use a positive byte count to limit the original payload retained per body. For example, `65536` retains at most 64 KiB per body. Negative values are invalid.

The limit applies independently to each covered body. Bodies within the limit remain unchanged; larger bodies become valid JSON containing a text preview:

```json
{"_obotAuditTruncated":true,"originalBytes":123456,"preview":"prefix of the original JSON text"}
```

The preview ends on a UTF-8 character boundary. JSON escaping, wrapper metadata, and encryption add storage overhead beyond the configured payload limit. Detail views and exports contain the stored preview or omitted bodies; they cannot recover the original content.

For streamed LLM responses, the limit applies to the assembled response JSON in the background writer, before encryption and persistence. It does not limit the existing stream-capture buffer.

Omitting bodies preserves audit metadata, outcomes, mutation and policy-trigger indicators, token counts, response IDs, headers, and audit-derived usage statistics.

Body limits affect new entries only; they do not shrink existing records or change retention. Body limits do not cap headers, row counts, or total database size. Plan storage using daily call volume, average stored row size (including indexes and encoding overhead), and retention days. The 90-day default can retain millions of rows on busy installations; choose a shorter retention period where appropriate.

## Disabling audit logs

To stop collecting and persisting new entries entirely, use the corresponding disable setting:

| Audit logs | Environment variable | CLI flag |
| --- | --- | --- |
| MCP | `OBOT_SERVER_DISABLE_MCPAUDIT_LOG=true` | `--disable-mcpaudit-log` |
| LLM | `OBOT_SERVER_DISABLE_LLMAUDIT_LOG=true` | `--disable-llmaudit-log` |

Disabling collection also stops new audit-derived usage data for that log type. Historical logs remain readable and exportable, and retention cleanup continues. MCP, LLM, and local-agent logging are configured independently.

## Usage

Usage tracking provides aggregate statistics about MCP server activity.

### Metrics Available

- **Request counts**: Total requests per server
- **User activity**: Which users are using which servers
- **Tool usage**: Most frequently called tools
- **Error rates**: Success/failure ratios
- **Response times**: Performance metrics

### Viewing Usage

Navigate to **MCP Management > Usage** in the MCP Platform.

### Use Cases

- **Cost management**: Understand which servers are most used
- **Capacity planning**: Identify servers that may need scaling
- **Adoption tracking**: See which tools are popular
- **Troubleshooting**: Identify servers with high error rates

## Access by Role

**Power User / Power User+**
- View audit logs and usage for their own activity
- Metadata only (no request/response content)

**Admin / Owner**
- View audit logs and usage for all users
- Export MCP and LLM audit logs
- Metadata only (no request/response content)

**Auditor (add-on)**
- View full request/response payloads and headers
- Export audit logs with full content
- Read-only access to admin views

## Privacy Considerations

Audit logs may contain sensitive information from MCP requests/responses and LLM gateway requests/responses. Consider:

- **Data retention**: Configure how long logs are kept (see [Retention](#retention))
- **Access control**: Limit who can view detailed logs
- **Export security**: Secure any exported log data
- **Compliance**: Ensure logging meets regulatory requirements
