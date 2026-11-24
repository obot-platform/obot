---
title: "Set up vMCPs for agents"
description: Grant agent identities access to a vMCP, and set up ahead of time the configuration and upstream OAuth they need, with no user present.
---

[Agent identities](../security/agent-identities.md) connect to vMCPs through the vMCP's profiles, like users. Unlike a user, an agent can't type configuration values or sign in to an upstream provider. Agents use only the configuration that the vMCP or its catalog entry fixes for everyone, and the upstream accounts that an administrator [preconfigures](./server-types.md#preconfigured-oauth) on the vMCP's components for everyone.

## Grant agents access {#grant-access}

Add agents to a profile in the vMCP's **Profiles**, with **Add User/Group/Agent**, and select each agent by name. A profile applies to an agent only if it names that agent. **All Obot Users** and groups match users, never agents, so a vMCP that every user can use grants no agent until a profile names it. There is no option for all agents: granting an agent a vMCP is always deliberate.

Agents are named only by ID. The API, `obot mcp validate-catalog` and catalog sync refuse an agent subject with the ID `*`, with an error that says agents must be named by ID.

Profiles hold only subjects and permissions, for agents as for users. Everything else agents need is set on the vMCP's components, for everyone who uses the vMCP: [configuration](#configuration) that the vMCP or its catalog entry fixes, and [preconfigured OAuth](#oauth).

## What an agent gets {#precedence}

An agent can be named in several profiles of a vMCP. It gets what all of them grant, as a user does:

- **Tools** are the union of the tools of every profile that names the agent. Profiles for users, such as **All Obot Users**, add nothing for an agent.
- **Components** are the ones any of those profiles grants: every component with **Allow All Access**, otherwise the ones each profile enables.
- **Upstream accounts** are the vMCP's preconfigured OAuth, one per component, which every user and agent of the vMCP uses. See [Upstream OAuth](#oauth).
- **Configuration values** are only the ones the vMCP or its catalog entry fixes for everyone, including values from Kubernetes Secrets. See [Configuration](#configuration).

Removing an agent from one profile takes away only what that profile granted. Removing it from every profile of the vMCP revokes its access, and its next connection is refused with `403`.

Every component the agent can use must be fully set up. If any needs a value agents can't get, or OAuth that isn't preconfigured and working, the whole connection is refused, rather than offering fewer tools.

## Configuration {#configuration}

Agents never enter configuration, and nothing enters it for them. A component's server gets, for an agent, only the values fixed for everyone:

- values the vMCP fixes (the **Fixed** policy), including values from Kubernetes Secrets;
- values the catalog entry sets, including its Secret bindings.

A value a user would enter on their own connection is never given to an agent:

- An **optional** one is left unset. It doesn't block the agent.
- A **required** one, or a configurable **Server URL** that the vMCP doesn't fix, blocks every agent granted the component. The connection is refused with `409 agent_configuration_required` naming the component and key, and readiness reports `configuration_required`.

To fix a component that blocks agents, do one of these:

- Set the value as **Fixed** on the vMCP, or set it in the catalog entry. It then applies to everyone who uses the vMCP, users included.
- Stop granting the component in the profiles that name agents, if the agents don't need it.

## Upstream OAuth {#oauth}

An agent can use a component that needs OAuth only when an administrator has [preconfigured OAuth](./server-types.md#preconfigured-oauth) for it on the vMCP. To do that, open the component's settings in the vMCP designer and select **Authorize** in its **OAuth** section. Any remote component of a shared vMCP can be authorized, whether or not a profile grants it to an agent.

Preconfigured OAuth isn't only for agents. Every user and agent that uses the component through the vMCP acts upstream as the authorized account, and users aren't asked to authorize the component themselves. Authorize only an account that is meant to be shared by all of them.

For an agent granted a component that needs OAuth:

- With no preconfigured OAuth, the vMCP isn't ready for the agent: readiness reports `oauth_required`, and connections get `409 agent_oauth_required`.
- When the preconfigured OAuth [stops working](./server-types.md#preconfigured-oauth-invalid), the agent gets `409 agent_oauth_required` with `oauth_invalid`, and readiness shows `oauth_invalid` when the provider refused a refresh or the URL changed. Authorize it again to fix it.
- After an administrator selects **Revoke**, the agent gets `oauth_required` until the component is authorized again.

An agent that isn't granted the component never reaches it, so it never uses the component's preconfigured OAuth.

## Check readiness {#readiness}

To see whether an agent can connect, open the agent under **Identity & Access > Agents**. **vMCP readiness** lists every vMCP that has a profile naming the agent, and what is missing for the components the agent's profiles grant: a value to fix on the vMCP or the catalog entry, or OAuth to authorize. Each one links to the component's settings in the vMCP designer. Components whose preconfigured OAuth works are listed as **Preconfigured**. Readiness runs the same checks as a connection. A component that none of the agent's profiles grants doesn't count. A vMCP with no profile that names the agent isn't listed, because the agent isn't granted it.

When an agent connects to a vMCP that isn't ready, it gets `409 Conflict` with the reasons. It never gets an OAuth challenge or a sign-in link. See [Errors](../reference/agent-identities.md#errors).

### How Obot knows a component needs OAuth {#oauth-detection}

A component needs OAuth when its catalog entry requires static OAuth, when the vMCP holds preconfigured OAuth for it, even OAuth that no longer works, or when Obot's background check found that it asks for OAuth. See [Which components need OAuth](./server-types.md#preconfigured-oauth-detection).

Readiness and connections never run this check themselves. They use the result of the last check that succeeded. If no check of the component has succeeded yet, for example because the server didn't answer in time, the component doesn't count as needing OAuth, so a server that can't be reached never blocks agents.

Obot doesn't check a component that takes configured headers, such as an API key. Such a component counts as needing OAuth once the vMCP holds preconfigured OAuth for it. An agent that reaches a component that asks for OAuth when nothing showed it gets `409 Conflict` with `agent_oauth_required`, never an OAuth challenge.

## Catalog-synced vMCPs {#catalog-vmcps}

A vMCP synced from a [Git catalog](../configuration/mcp-server-gitops.md) is read-only in Obot, but you can still [preconfigure OAuth](./server-types.md#preconfigured-oauth) for its remote components there. Syncs keep it. Removing a component from the catalog's vMCP deletes its preconfigured OAuth, and removing the vMCP deletes all of it.

Catalog YAML can name specific agents, so agents can use catalog-synced vMCPs:

```yaml
profiles:
  - id: nightly-report
    name: Nightly report agent
    subjects:
      - type: agent
        id: "7"
    vmcpPermissions:
      allowAllComponents: true
```

An agent ID belongs to one Obot deployment: in another deployment that syncs the same catalog, it names a different agent, or none. `obot mcp validate-catalog` checks only that each agent ID is a decimal ID, such as `"7"` rather than `"agent:7"`, because it can't check that the agent exists. On sync, an ID that names no agent, or a deleted one, matches nobody, as a deleted agent's subjects do elsewhere. The sync doesn't fail. An agent can be named in several profiles of a catalog vMCP, and gets their union, as in any vMCP.

Give each profile an explicit `id`, as above, so that renaming it keeps its ID, which audit records name it by. An `id` is 1–63 lowercase letters, digits and hyphens, and starts and ends with a letter or digit. Without one, Obot derives the ID from the vMCP and profile names, and renaming the profile makes it a new profile.

## Who can do what {#permissions}

- Administrators can see agents' readiness, and authorize and revoke preconfigured OAuth.
- Read-Only Admins and auditors can see readiness and the state of preconfigured OAuth, including who authorized it and when, but not values, tokens, or URLs that come from configuration.
- Agents can't see or change any of this. They can't reach readiness or preconfigured OAuth routes.

A URL that comes from configuration is treated as a value. Readiness and the **OAuth** section show only its hostname, and errors that would include it show a placeholder instead. A URL fixed by the catalog entry is shown in full.
