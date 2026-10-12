---
title: "Security overview"
---

Obot applies controls at the gateway, identity, workload, and device boundaries. Its protection depends on which components a request actually passes through and how the deployment is configured.

| Boundary | Responsibility |
|---|---|
| Identity | Configure authentication and grant appropriate roles and resource access |
| Gateway | Route clients through Obot to apply MCP/model access checks and record gateway activity |
| Hosted code | Isolate packages, containers, and filters on the execution backend |
| Remote services | Evaluate upstream authentication and service behavior; Obot does not host that code |
| User devices | Install and maintain Sentry hooks for supported auditing/enforcement coverage |
| Stored data | Configure field encryption, storage access, retention, exports, and backups |

Power User and Power User+ can deploy server code. Treat them as privileged roles; the [User Roles security model](./policy-coverage.md#user-roles-security-model) describes why Docker's host socket is appropriate only for trusted evaluation or single-tenant use.

For production, review [Network and workload isolation](./isolation.md), [Credentials and encryption](./credentials.md), and [Policy coverage](./policy-coverage.md). Application encryption is disabled by default and protects selected fields when enabled, not the entire deployment.

Device enforcement is experimental and has explicit [client and failure-mode limitations](../device-management/enforcement.md). Gateway policies do not govern traffic that bypasses the gateway.

The MCP Platform is Obot's unified management interface for deploying, managing, and operating MCP servers. It provides role-based access to server management, registries, audit logs, usage tracking, and platform administration.

For detailed permissions and role definitions, see [User Roles](./policy-coverage.md).

## Roles and Capabilities {#overview-roles-and-capabilities}

The MCP Platform adapts its navigation and available features based on your assigned role.

### Standard User {#overview-standard-user}

Standard Users can deploy and use MCP servers that have been made available to them through an MCP Registry. They can interact with MCP servers via external MCP clients but cannot publish or manage servers.

### Power User {#overview-power-user}

Power Users include all Standard User capabilities and can additionally deploy MCP servers for personal use that are not sourced from an MCP Registry. These servers are only visible to the deploying user. They also have access to audit logs metadata and usage stats for the servers they deploy.

### Power User+ {#overview-power-user-1}

Power Users+ include all Power User capabilities and can additionally publish MCP servers to an MCP Registry for use by other users. They control which users or groups can access the servers they publish.

### Admin / Owner {#overview-admin--owner}

Admins and Owners have full administrative access to the platform, including system-wide configuration, user management, and gateway administration.

Owners can assign the **Owner** and **Auditor** roles; Admins cannot. For more information, see the [Auditor Role](./policy-coverage.md#user-roles-auditor).

## Learn More {#overview-learn-more}

- [Virtual MCPs (vMCPs)](../mcp-gateway/server-types.md) - Combine one or more MCP servers behind a governed connection endpoint
- [MCP Servers](../functionality/mcp-servers.md) - Deploy, configure, and manage MCP servers
- [MCP Tunnels](../functionality/mcp-tunnels.md) - Connect the gateway to remote MCP servers on private networks
- [MCP Access Policies](../mcp-gateway/access.md) - Control which servers are available to which users and groups
- [Audit Logs and Usage](./audit-data.md) - Monitor activity and track consumption
- [Filters](../functionality/filters.md) - Inspect and control MCP traffic
- [Server Scheduling](../operations/capacity.md) - Configure pod scheduling behavior for MCP servers
- [Skills](../registries/publish-skills.md) - Manage skill sources and browse discoverable skills for agents
- [Skill Access Policies](../registries/publish-skills.md) - Control which users and groups can access which skills
- [Device Management](../device-management/how-sentry-works.md) - Inventory local AI clients, MCP servers, skills, and plugins, audit local tool calls, and enforce tool call allowlists
- [User Management](./authentication.md) - Manage users, roles, and authentication
- [Agent Authorization Scopes](../functionality/agent-auth-scopes.md) - Create and manage agent authorization scopes for programmatic Obot access
- [Branding](../functionality/branding.md) - Customize theme colors and branding
- [User Roles](./policy-coverage.md) - Detailed permissions and role definitions
