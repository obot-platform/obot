---
title: "Security overview"
---

## Roles and Capabilities {#overview-roles-and-capabilities}

The MCP Platform adapts its navigation and available features based on your assigned role.


### Standard User {#overview-standard-user}

Standard Users can deploy and use MCP servers that have been made available to them through an MCP Registry. They can interact with MCP servers via Obot Agent or external MCP clients but cannot publish or manage servers.


### Power User {#overview-power-user}

Power Users include all Standard User capabilities and can additionally deploy MCP servers for personal use that are not sourced from an MCP Registry. These servers are only visible to the deploying user. They also have access to audit logs metadata and usage stats for the servers they deploy.


### Power User+ {#overview-power-user-1}

Power Users+ include all Power User capabilities and can additionally publish MCP servers to an MCP Registry for use by other users. They control which users or groups can access the servers they publish.


### Admin / Owner {#overview-admin--owner}

Admins and Owners have full administrative access to the platform, including system-wide configuration, user management, and Obot Agent administration.

The only functional difference between Owners and Admins is that Owners can assign the **Auditor** role to users. For more information, see the [Auditor Role](../configuration/user-roles.md#auditor).


## Learn More {#overview-learn-more}

- [Virtual MCPs (vMCPs)](../functionality/virtual-mcps.md) - Combine one or more MCP servers behind a governed connection endpoint
- [MCP Servers](../functionality/mcp-servers.md) - Deploy, configure, and manage MCP servers
- [MCP Tunnels](../functionality/mcp-tunnels.md) - Connect the gateway to remote MCP servers on private networks
- [MCP Access Policies](../functionality/mcp-access-policies.md) - Control which servers are available to which users and groups
- [Audit Logs and Usage](../functionality/audit-logs-and-usage.md) - Monitor activity and track consumption
- [Filters](../functionality/filters.md) - Inspect and control MCP traffic
- [Server Scheduling](../functionality/server-scheduling.md) - Configure pod scheduling behavior for MCP servers
- [Skills](../functionality/skills.md) - Manage skill sources and browse discoverable skills for agents
- [Skill Access Policies](../functionality/skill-access-policies.md) - Control which users and groups can access which skills
- [Device Management](../functionality/device-management.md) - Inventory local AI clients, MCP servers, skills, and plugins, audit local tool calls, and enforce tool call allowlists
- [Obot Agent Management](../functionality/obot-agent-management.md) - Configure default agent, conversation, and workflow settings, and monitor activity
- [AI Judge Policies](../functionality/ai-judge-policies.md) - Enforce content rules on user prompts and tool calls, and review violations
- [User Management](../functionality/user-management.md) - Manage users, roles, and authentication
- [Agent Authorization Scopes](../functionality/agent-auth-scopes.md) - Create and manage agent authorization scopes for programmatic Obot access
- [Branding](../functionality/branding.md) - Customize theme colors and branding
- [Workflow Sharing](../functionality/workflow-sharing.md) - Publish, discover, install, and operate shared workflows
- [User Roles](../configuration/user-roles.md) - Detailed permissions and role definitions
