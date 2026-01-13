---
title: Audit Logs & Usage
---

# Audit Logs & Usage

The MCP Platform provides visibility into MCP activity through audit logs and usage tracking. These features help with monitoring, compliance, and understanding how MCP servers are being used.

:::info Auditor Role
Sensitive data (MCP request/response bodies, chat threads, and task runs) can **only** be viewed by users with the Auditor role. All other roles, including Owner and Admin, see only metadata for these resources. The Auditor role is an add-on permission that can be combined with any other role, granting read-only access to sensitive data across the platform. See [User Roles](/configuration/user-roles/#auditor) for details.
:::

## Audit Logs

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

### Exporting Audit Logs

Audit logs can be exported for external analysis or compliance requirements. See [Audit Log Export](/configuration/audit-log-export/) for configuration options.

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
- Export audit logs
- Metadata only (no request/response content)

**Auditor (add-on)**
- View full request/response payloads and headers
- Export audit logs with full content
- Read-only access to admin views

## Privacy Considerations

Audit logs may contain sensitive information from MCP requests and responses. Consider:

- **Data retention**: Configure how long logs are kept
- **Access control**: Limit who can view detailed logs
- **Export security**: Secure any exported log data
- **Compliance**: Ensure logging meets regulatory requirements
