---
title: Overview
---

# Overview

The MCP Platform is Obot's unified management interface for deploying, managing, and operating MCP servers. It provides role-based access to server management, registries, audit logs, usage tracking, and platform administration.

For detailed permissions and role definitions, see [User Roles](../configuration/user-roles.md).

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

The only functional difference between Owners and Admins is that Owners can assign the **Auditor** role to users. For more information, see the [Auditor Role](../configuration/user-roles.md#auditor).

## Learn More

- [MCP Servers](./mcp-servers.md) - Deploy, configure, and manage MCP servers
- [MCP Tunnels](./mcp-tunnels.md) - Connect the gateway to remote MCP servers on private networks
- [MCP Access Policies](./mcp-access-policies.md) - Control which servers are available to which users and groups
- [Audit Logs and Usage](./audit-logs-and-usage.md) - Monitor activity and track consumption
- [Filters](./filters.md) - Inspect and control MCP traffic
- [Server Scheduling](./server-scheduling.md) - Configure pod scheduling behavior for MCP servers
- [Skills](./skills.md) - Manage skill sources and browse discoverable skills for agents
- [Skill Access Policies](./skill-access-policies.md) - Control which users and groups can access which skills
- [Device Management](./device-management.md) - Inventory local AI clients, MCP servers, skills, and plugins, audit local tool calls, and enforce tool call allowlists
- [Obot Agent Management](./obot-agent-management.md) - Configure default agent, conversation, and workflow settings, and monitor activity
- [Message Policies](./message-policies.md) - Enforce content rules on user prompts and tool calls, and review violations
- [User Management](./user-management.md) - Manage users, roles, and authentication
- [Agent Authorization Scopes](./agent-auth-scopes.md) - Create and manage agent authorization scopes for programmatic Obot access
- [Branding](./branding.md) - Customize theme colors and branding
- [Workflow Sharing](./workflow-sharing.md) - Publish, discover, install, and operate shared workflows
- [User Roles](../configuration/user-roles.md) - Detailed permissions and role definitions
