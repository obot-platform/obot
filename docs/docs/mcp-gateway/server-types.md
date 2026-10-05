---
title: "Create vMCP"
---

## How vMCPs work {#virtual-mcps-how-vmcps-work}

```mermaid
flowchart LR
    C[Catalog entries] -->|snapshot| V[vMCP]
    P[Profiles] -->|users, groups, and tools| V
    V --> I[Per-user vMCP instance]
    I --> R1[Shared component deployment]
    I --> R2[Per-user component deployment]
```

The main resources are:

- **Catalog entry**: Defines how an MCP server runs or where a remote server is located.
- **vMCP component**: A snapshot of a catalog entry, plus its configuration policy and exposed tools.
- **vMCP**: One stable connection endpoint containing one or more components.
- **Profile**: An administrator-defined grant that makes a shared vMCP and selected tools available to users or groups.
- **vMCP instance**: A user's connection state, credentials, and optional tool selection for a vMCP.

A vMCP with one component replaces a standalone MCP server connection. A vMCP with several components replaces a composite server and aggregates their tools behind the same endpoint.


## Shared and personal vMCPs {#virtual-mcps-shared-and-personal-vmcps}

The creator's role determines the scope of a new vMCP.

| | Administrator-created shared vMCP | User-created personal vMCP |
|---|---|---|
| Who can use it | Users and groups granted access by profiles | Its author only |
| Source catalog access for consumers | Not required | The owner must have access to every selected catalog entry |
| Access management | Administrators manage profiles and tool grants | Cannot be shared; profiles do not change its owner-only scope |
| Component access changes | Consumer catalog access does not remove components | Losing access to a catalog entry removes that component |
| Typical use | Publish a governed tool endpoint for a team or organization | Assemble a private endpoint from servers available to the user |

Only administrators can create a vMCP that other users can consume. Any user who can access catalog entries can use them in a personal vMCP, but non-administrators cannot publish that vMCP to other users or groups.


### Create a shared vMCP as an administrator {#virtual-mcps-create-a-shared-vmcp-as-an-administrator}

1. Open **vMCPs** and select **Create vMCP**.
2. Drag an MCP server from the **MCP Servers** panel onto the vMCP. Repeat to add more components.
3. Enter a name and description, then select **Create**.
4. For each component, choose how every configuration field is supplied. See [Configuration policies](../functionality/virtual-mcps.md#configuration-policies).
5. Configure the component's exposed tools. You can disable tools, rename them, change their descriptions, or add a prefix to avoid name collisions.
6. Open **Profiles** and replace or refine the default access grant. Assign users, groups, or **All Obot Users**, then choose the tools that profile grants.
7. Connect to or test the vMCP after its components are ready.

:::note

A new administrator-created shared vMCP includes a default profile that grants all administrators access to every component. Administrators can replace or refine this profile to grant access to other users or groups. A personal vMCP remains accessible only to its owner.

:::

Profiles are grant-only and additive. If a user matches several profiles, Obot combines their tool grants. A narrower profile cannot deny a tool granted by another matching profile, so review broad profiles when troubleshooting unexpected access.


### Create a personal vMCP as a user {#virtual-mcps-create-a-personal-vmcp-as-a-user}

1. Open **vMCPs** and select **Create vMCP**.
2. Drag MCP servers available to you from the **MCP Servers** panel onto the vMCP.
3. Enter a name and description, then select **Create**.
4. Choose the configuration policy and exposed tools for each component.
5. Select **Connect** to configure and launch your connection.

The personal vMCP is accessible only by its owner, and the owner has no **Profiles** view. Selecting **Provided at connection** prompts the owner for that value when connecting.

If the owner later loses access to a selected MCP server, Obot removes that component and its component-specific configuration from the personal vMCP. If no components remain, Obot removes the personal vMCP and its instance. Deleting a source catalog entry is different: its existing snapshot can continue to run as described in [Snapshots and updates](../functionality/virtual-mcps.md#snapshots-and-updates).


### Configuration policies {#virtual-mcps-configuration-policies}

When a catalog entry declares configuration, the vMCP creator assigns one policy to each field.

| UI option | Behavior |
|---|---|
| **Preconfigured** | The creator supplies one fixed value used by every connection. |
| **Provided at connection** | Each connecting user supplies a value stored with that user's vMCP instance. |
| **Ignore** | The vMCP does not accept or supply a value for the field. |

Required fields must be either **Preconfigured** or **Provided at connection**. Optional fields default to **Ignore** in the UI.

Configuration also determines whether a component can share a deployment:

- A component with only fixed or ignored configuration is normally eligible to use a shared deployment.
- User-provided headers remain isolated per connection but do not require separate deployments.
- Any other user-provided value causes Obot to run a separate component deployment for each user.
- **Force single-user** always gives each user a separate deployment when that option is available.

Obot makes this decision independently for every component. One vMCP can therefore use shared and per-user component deployments at the same time.

