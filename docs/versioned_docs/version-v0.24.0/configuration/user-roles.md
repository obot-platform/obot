---
title: User Roles
---

# User Roles

Obot uses role-based access control to manage what users can do in the MCP Platform. Each role has different permissions and sees different parts of the interface.

## Available Roles

### Owner

Full platform management plus the ability to assign the Owner and Auditor roles to users.

### Admin

Full platform management: MCP Management, Obot Agent Management, User Management, and App Preferences. Cannot assign the Owner or Auditor roles.

### Power User+

All Power User permissions plus the ability to create MCP Registries and share MCP servers with other users.

### Power User

All Basic User permissions plus publishing custom MCP servers (personal use only) and viewing Audit Logs and Usage statistics for their activity.

### Basic User

Connect to MCP servers, use Obot Agent, and create conversations and workflows.

### Auditor

Add-on permission that grants read-only access to sensitive data across the platform. Sensitive data (MCP request/response bodies, conversations, and workflow runs) can only be viewed by users with this role. All other roles, including Owner, see only metadata for these resources. Can be combined with any other role.

## Role Comparison

| Capability | Basic | Power | Power+ | Admin | Owner |
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

## Security Model

Obot's MCP hosting platform runs the MCP servers that users add. The **Power User** and **Power User+** roles can publish and deploy MCP servers — including `npx` (npm) and `uvx` (PyPI) packages, and **containerized** servers that run an arbitrary OCI image with a user-supplied command, arguments, and environment. By design, these roles can therefore cause code to execute on Obot's MCP hosting backend.

Treat Power User and Power User+ as **privileged** roles. Grant them only to users you trust to run code on your infrastructure, and harden the hosting backend so that the code those users deploy stays contained:

- **Docker deployments** bind-mount the host Docker socket, so a containerized MCP server runs on the host Docker daemon — effectively host-level access on that machine. Use Docker deployments only for development or single-machine, single-tenant use. See [Docker Deployment](/installation/docker-deployment/).
- **Kubernetes deployments** isolate each MCP server in its own pod. For multi-tenant or untrusted users, keep the restricted Pod Security Admission policy and the MCP NetworkPolicy enabled (both are on by default), and consider a sandboxed container runtime such as gVisor or Kata Containers. See [MCP Deployments in Kubernetes](/configuration/mcp-deployments-in-kubernetes/).

The [default role for new users](#default-role-for-new-users) is configurable. Do not default new users to Power User or Power User+ unless every user who can sign up is trusted to run code on your infrastructure.

## Managing User Roles

### Updating a User's Role

1. Navigate to **User Management > Users**
2. Click the three vertical dots on the user's current role
3. Click **Update Role**
4. Select the new role

### Default Role for New Users

Configure the default role for new users on the **User Management > User Roles** page.

### Pre-Assigning Roles

To grant admin or owner access to users before they log in, set these environment variables during deployment. See [Enabling Authentication](/installation/enabling-authentication/) for details.

```bash
OBOT_SERVER_AUTH_ADMIN_EMAILS=admin@example.com,admin2@example.com
OBOT_SERVER_AUTH_OWNER_EMAILS=owner@example.com
```
