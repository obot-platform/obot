---
title: Overview
---

# Overview

The MCP Platform is Obot's unified management interface for deploying, managing, and operating MCP servers. It provides role-based access to server management, registries, audit logs, usage tracking, and platform administration.

For detailed permissions and role definitions, see [User Roles](/configuration/user-roles/).

## Roles and Capabilities

The MCP Platform adapts its navigation and available features based on your assigned role.

### Basic User

Basic Users can deploy and use MCP servers that have been made available to them through an MCP Registry. They can interact with MCP servers via Obot Chat or external MCP clients but cannot publish or manage servers.

### Power User

Power Users include all Basic User capabilities and can additionally deploy MCP servers for personal use that are not sourced from an MCP Registry. These servers are only visible to the deploying user. They also have access to audit logs metadata and usage stats for the servers they deploy.

### Power User+

Power Users+ include all Power User capabilities and can additionally publish MCP servers to an MCP Registry for use by other users. They control which users or groups can access the servers they publish.

### Admin / Owner

Admins and Owners have full administrative access to the platform, including system-wide configuration, user management, and chat administration.

The only functional difference between Owners and Admins is that Owners can assign the **Auditor** role to users. For more information, see the [Auditor Role](/configuration/user-roles/#auditor).

## Navigation by Role

| Section | Basic | Power | Power+ | Admin/Owner |
|---------|:-----:|:-----:|:------:|:-----------:|
| **MCP Management** | | | | |
| MCP Servers | ✓ | ✓ | ✓ | ✓ |
| MCP Registries | | | ✓ | ✓ |
| Audit Logs | | ✓* | ✓* | ✓ |
| Usage | | ✓* | ✓* | ✓ |
| Filters | | | | ✓ |
| Server Scheduling | | | | ✓ |
| **Chat Management** | | | | ✓ |
| **User Management** | | | | ✓ |
| API Keys (admin) | | | | ✓ |
| **API Keys** | ✓ | ✓ | ✓ | ✓ |
| **Branding** | | | | ✓ |
| **Chat** | ✓ | ✓ | ✓ | ✓ |

\* For servers they deployed only

## Learn More

- [MCP Servers](../mcp-servers/) - Deploy, configure, and manage MCP servers
- [MCP Registries](../mcp-registries/) - Control which servers are available to which users and groups
- [Audit Logs and Usage](../audit-logs-and-usage/) - Monitor activity and track consumption
- [Filters](../filters/) - Inspect and control MCP traffic
- [Server Scheduling](../server-scheduling/) - Define server availability windows
- [Chat Management](../chat-management/) - Configure default chat settings and monitor activity
- [User Management](../user-management/) - Manage users, roles, and authentication
- [API Keys](../api-keys/) - Create and manage API keys for programmatic MCP server access
- [Branding](../branding/) - Customize theme colors and branding
- [Obot Chat](../chat/overview/) - Projects, threads, tasks, and chat features
- [User Roles](/configuration/user-roles/) - Detailed permissions and role definitions
