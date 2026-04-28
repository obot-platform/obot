---
title: Overview
---

# Overview

The MCP Platform is Obot's unified management interface for deploying, managing, and operating MCP servers. It provides role-based access to server management, registries, audit logs, usage tracking, and platform administration.

For detailed permissions and role definitions, see [User Roles](/configuration/user-roles/).

## Roles and Capabilities

The MCP Platform adapts its navigation and available features based on your assigned role.

### Basic User

Basic Users can deploy and use MCP servers that have been made available to them through an MCP Registry. They can interact with MCP servers via Obot Agent or external MCP clients but cannot publish or manage servers.

### Power User

Power Users include all Basic User capabilities and can additionally deploy MCP servers for personal use that are not sourced from an MCP Registry. These servers are only visible to the deploying user. They also have access to audit logs metadata and usage stats for the servers they deploy.

### Power User+

Power Users+ include all Power User capabilities and can additionally publish MCP servers to an MCP Registry for use by other users. They control which users or groups can access the servers they publish.

### Admin / Owner

Admins and Owners have full administrative access to the platform, including system-wide configuration, user management, and Obot Agent administration.

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
| **Obot Agent Management** | | | | ✓ |
| Message Policies | | | | ✓ |
| Skills | | | | ✓ |
| Skill Access Policies | | | | ✓ |
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
- [Skills](../skills/) - Manage skill sources and browse discoverable skills for agents
- [Skill Access Policies](../skill-access-policies/) - Control which users and groups can access which skills
- [Obot Agent Management](../obot-agent-management/) - Configure default agent, conversation, and workflow settings, and monitor activity
- [Message Policies](../message-policies/) - Enforce content rules on user prompts and tool calls, and review violations
- [User Management](../user-management/) - Manage users, roles, and authentication
- [API Keys](../api-keys/) - Create and manage API keys for programmatic MCP server access
- [Branding](../branding/) - Customize theme colors and branding
- [Obot Agent](../agent/overview/) - Agents, conversations, workflows, and chat features
- [Workflow Sharing](../workflow-sharing/) - Publish, discover, install, and operate shared workflows
- [User Roles](/configuration/user-roles/) - Detailed permissions and role definitions
