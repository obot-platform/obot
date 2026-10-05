---
title: "Roles and access policies"
---

Roles determine platform capabilities. Resource policies and profiles determine which resources a caller can use. Review both.

| Control | What it governs | Important limit |
|---|---|---|
| [User roles](./policy-coverage.md#user-roles-available-roles) | Administration, publishing, and audit visibility | Owner/Admin does not automatically include sensitive audit payload access |
| [MCP access policies](../mcp-gateway/access.md) | Catalog server access | Does not replace shared-vMCP profiles |
| [vMCP profiles](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles) | Shared endpoint and tool grants | Matching grants are additive |
| [Model policies](../functionality/model-access-policies.md) | Accessible models | A configured provider alone does not grant model access |
| [Skill policies](../registries/publish-skills.md) | Skill discovery and installation | Administrators bypass these grants; downloaded copies remain |
| [Authorization scopes](../functionality/agent-auth-scopes.md) | Programmatic credential capabilities | Cannot exceed the owning user's access |
| [AI Judge policies](../functionality/ai-judge-policies.md) | Supported user-message and tool-call content checks | Experimental; requires configured review models and compatible enforcement flow |
| [Sentry allowlists](../device-management/enforcement.md) | Supported local client tool calls | Experimental; client coverage and fail-closed behavior apply |

For content policies, consult the AI Judge Policies guide's exact enforcement flow. Do not assume that model-output signaling prevents every external client from executing a tool; device hooks and MCP tool authorization are separate controls.

Use a regular test account with the intended group membership to verify grants. Review broadly matching profiles and policies before concluding a narrow rule is ineffective. [User management](./authentication.md) describes administrative account controls.

Obot uses role-based access control to manage what users can do in the MCP Platform. Each role has different permissions and sees different parts of the interface.

## Available Roles {#user-roles-available-roles}

### Owner {#user-roles-owner}

Full platform management plus the ability to assign the Owner and Auditor roles to users.

### Admin {#user-roles-admin}

Full platform management, including server and model configuration, users, and platform settings. Cannot assign the Owner or Auditor roles.

### Power User+ {#user-roles-power-user}

All Power User permissions plus the ability to create MCP Registries and share MCP servers with other users.

### Power User {#user-roles-power-user-1}

All Standard User permissions plus publishing custom MCP servers (personal use only) and viewing Audit Logs and Usage statistics for their activity.

### Standard User {#user-roles-standard-user}

Connect to MCP servers, use Obot Agent, and create conversations and workflows.

### Auditor {#user-roles-auditor}

Add-on permission that grants read-only access to sensitive data across the platform. Sensitive data (MCP request/response bodies, conversations, and workflow runs) can only be viewed by users with this role. All other roles, including Owner, see only metadata for these resources. Can be combined with any other role.

## Role Comparison {#user-roles-role-comparison}

| Capability | Standard | Power | Power+ | Admin | Owner |
|------------|-------|-------|--------|-------|-------|
| Connect to MCP servers | Yes | Yes | Yes | Yes | Yes   |
| Use Obot Agent | Yes | Yes | Yes | Yes | Yes   |
| View Audit Logs | | Yes* | Yes* | Yes** | Yes** |
| View Usage | | Yes* | Yes* | Yes | Yes   |
| Publish personal MCP servers | | Yes | Yes | Yes | Yes   |
| Share MCP servers through registries | | | Yes | Yes | Yes   |
| Manage Filters | | | | Yes | Yes   |
| Server Scheduling | | | | Yes | Yes   |
| Obot Agent Management | | | | Yes | Yes   |
| User Management | | | | Yes | Yes   |
| App Preferences | | | | Yes | Yes   |
| Assign Owner/Auditor roles | | | | | Yes   |

\* Only for servers they deployed

\*\* Metadata only. Full request/response bodies require the Auditor role. Owners can assign Auditor to themselves, but this is an explicit action to prevent accidental exposure to sensitive data.

## Security Model {#user-roles-security-model}

Obot's MCP hosting platform runs the MCP servers that users add. The **Power User** and **Power User+** roles can publish and deploy MCP servers — including `npx` (npm) and `uvx` (PyPI) packages, and **containerized** servers that run an arbitrary OCI image with a user-supplied command, arguments, and environment. By design, these roles can therefore cause code to execute on Obot's MCP hosting backend.

Treat Power User and Power User+ as **privileged** roles. Grant them only to users you trust to run code on your infrastructure, and harden the hosting backend so that the code those users deploy stays contained:

- **Docker deployments** bind-mount the host Docker socket, so a containerized MCP server runs on the host Docker daemon — effectively host-level access on that machine. Use Docker deployments only for development or single-machine, single-tenant use. See [Docker Deployment](../installation/docker-deployment.md).
- **Kubernetes deployments** isolate each MCP server in its own pod. For multi-tenant or untrusted users, keep the restricted Pod Security Admission policy and the MCP NetworkPolicy enabled (both are on by default), and consider a sandboxed container runtime such as gVisor or Kata Containers. See [MCP Deployments in Kubernetes](../configuration/mcp-deployments-in-kubernetes.md).

The [default role for new users](./policy-coverage.md#user-roles-default-role-for-new-users) is configurable. Do not default new users to Power User or Power User+ unless every user who can sign up is trusted to run code on your infrastructure.

## Managing User Roles {#user-roles-managing-user-roles}

### Updating a User's Role {#user-roles-updating-a-users-role}

1. Navigate to **Identity & Access > Users**
2. Click the three vertical dots on the user's current role
3. Click **Update Role**
4. Select the new role

### Default Role for New Users {#user-roles-default-role-for-new-users}

Configure the default role for new users on the **Identity & Access > Roles** page.

### Pre-Assigning Roles {#user-roles-pre-assigning-roles}

To grant admin or owner access to users before they log in, set these environment variables during deployment. See [Enabling Authentication](./authentication.md) for details.

```bash
OBOT_SERVER_AUTH_ADMIN_EMAILS=admin@example.com,admin2@example.com
OBOT_SERVER_AUTH_OWNER_EMAILS=owner@example.com
```
