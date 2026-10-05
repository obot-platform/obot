---
title: "Audit data and retention"
---

## MCP Audit Logs {#audit-logs-and-usage-mcp-audit-logs}

Audit logs capture all MCP interactions that flow through the gateway.


### What's Logged {#audit-logs-and-usage-whats-logged}

- **MCP Requests**: Tool calls, resource access, and other MCP operations
- **MCP Responses**: Results returned from MCP servers
- **User Information**: Who made the request
- **Timestamps**: When the request occurred
- **Server Information**: Which MCP server handled the request


### Viewing Audit Logs {#audit-logs-and-usage-viewing-audit-logs}

Navigate to **MCP Management > Audit Logs** in the MCP Platform.

The audit log view shows:
- Timestamp
- User
- MCP Server
- Operation type
- Status (success/failure)


### Detailed View {#audit-logs-and-usage-detailed-view}

Click on any log entry to see additional details:
- Request and response metadata
- Error details (if applicable)
- Full request/response payloads and headers (Auditor role required)


### Filtering {#audit-logs-and-usage-filtering}

Filter logs by:
- Date range
- User
- MCP Server
- Operation type
- Status


### Retention {#audit-logs-and-usage-retention}

Audit logs are automatically deleted after **90 days** by default. To preserve logs beyond this period, use the export functionality before they are deleted. See [Server Configuration](../configuration/server-configuration.md) for retention settings.


### Exporting Audit Logs {#audit-logs-and-usage-exporting-audit-logs}

MCP audit logs can be exported for external analysis, compliance requirements, or long-term retention. See [Audit Log Export](../configuration/audit-log-export.md) for configuration options.


## Usage {#audit-logs-and-usage-usage}

Usage tracking provides aggregate statistics about MCP server activity.


### Metrics Available {#audit-logs-and-usage-metrics-available}

- **Request counts**: Total requests per server
- **User activity**: Which users are using which servers
- **Tool usage**: Most frequently called tools
- **Error rates**: Success/failure ratios
- **Response times**: Performance metrics


### Viewing Usage {#audit-logs-and-usage-viewing-usage}

Navigate to **MCP Management > Usage** in the MCP Platform.


### Use Cases {#audit-logs-and-usage-use-cases}

- **Cost management**: Understand which servers are most used
- **Capacity planning**: Identify servers that may need scaling
- **Adoption tracking**: See which tools are popular
- **Troubleshooting**: Identify servers with high error rates


## Access by Role {#audit-logs-and-usage-access-by-role}

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


## Privacy Considerations {#audit-logs-and-usage-privacy-considerations}

Audit logs may contain sensitive information from MCP requests/responses and LLM gateway requests/responses. Consider:

- **Data retention**: Configure how long logs are kept (see [Retention](../functionality/audit-logs-and-usage.md#retention))
- **Access control**: Limit who can view detailed logs
- **Export security**: Secure any exported log data
- **Compliance**: Ensure logging meets regulatory requirements

## Creating Exports {#audit-log-export-creating-exports}


### One-Time Exports {#audit-log-export-one-time-exports}

One-time exports allow you to export MCP or LLM audit logs for a specific time range with optional filters.

1. **Navigate to Audit Logs**:

   - For MCP audit logs, go to MCP Management → Audit Logs
   - For LLM audit logs, go to LLM Gateway → Audit Logs
   - Apply any desired filters

2. **Create Export**:

   - Click "Create Export" → "Create One-time Export"
   - If filters are applied, you'll be asked whether to include them

3. **Configure Export**:

   - **Name**: Descriptive name for the export
   - **Bucket**: Storage bucket name where exports will be saved
   - **Key Prefix**: Path prefix within the bucket. If empty, defaults to `mcp-audit-logs/YYYY/MM/DD/` for MCP exports and `llm-audit-logs/YYYY/MM/DD/` for LLM exports, based on the current date.
   - **Time Range**: Start and end dates/times
   - **Filters**: Additional filters to apply

4. **Submit Export**:
   - Click "Create Export" to start the process
   - Monitor progress in the exports list


### Scheduled Exports {#audit-log-export-scheduled-exports}

Scheduled exports run automatically at specified intervals.

1. **Create Schedule**:

   - Click "Create Export" → "Create Export Schedule"
   - Configure the same options as one-time exports

2. **Schedule Configuration**:

   - **Frequency**: Hourly, Daily, Weekly, or Monthly
   - **Time**: Specific time to run (for daily/weekly/monthly)
   - **Day**: Day of week (weekly) or month (monthly)
   - **Bucket**: Storage bucket name where exports will be saved
   - **Key Prefix**: Path prefix within the bucket. If empty, defaults to `mcp-audit-logs/YYYY/MM/DD/` for MCP exports and `llm-audit-logs/YYYY/MM/DD/` for LLM exports, based on the current date.

3. **Manage Schedules**:
   - View and manage schedules in the "Export Schedules" tab
   - Enable/disable schedules as needed
   - Edit schedule configuration

