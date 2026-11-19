---
title: "Set up vMCPs for agents"
description: Grant agent identities access to a vMCP and authorize the upstream OAuth they need, with no user present.
---

[Agent identities](../security/agent-identities.md) connect to vMCPs through the vMCP's profiles, like users. Unlike a user, an agent can't type configuration values or sign in to an upstream provider. Agents use only the configuration that the vMCP or its catalog entry fixes for everyone. An administrator authorizes OAuth on the vMCP ahead of time, once per component, for every agent that uses the component.

## Grant agents access {#grant-access}

Add agents to a profile in the vMCP's **Profiles**, with **Add User/Group/Agent**, and select each agent by name. A profile applies to an agent only if it names that agent. **All Obot Users** and groups match users, never agents, so a vMCP that every user can use grants no agent until a profile names it. There is no option for all agents: granting an agent a vMCP is always deliberate.

Agents are named only by ID. The API, `obot mcp validate-catalog` and catalog sync refuse an agent subject with the ID `*`, with an error that says agents must be named by ID.

Profiles hold only subjects and permissions, for agents as for users. Everything else agents need is set up on the vMCP itself, in its [Agent setup](#agent-setup) section.

## What an agent gets {#precedence}

An agent can be named in several profiles of a vMCP. It gets what all of them grant, as a user does:

- **Tools** are the union of the tools of every profile that names the agent. Profiles for users, such as **All Obot Users**, add nothing for an agent.
- **Components** are the ones any of those profiles grants: every component with **Allow All Access**, otherwise the ones each profile enables.
- **OAuth grants** are the vMCP's, one per component. Every agent granted a component uses the vMCP's grant for it. See [Authorize OAuth](#oauth).
- **Configuration values** are only the ones the vMCP or its catalog entry fixes for everyone, including values from Kubernetes Secrets. See [Configuration](#configuration).

Removing an agent from one profile takes away only what that profile granted. Removing it from every profile of the vMCP revokes its access, and its next connection is refused with `403`.

Every component the agent can use must be fully set up. If any needs a value agents can't get, or lacks a working grant, the whole connection is refused, rather than offering fewer tools.

## Agent setup {#agent-setup}

The vMCP's page has an **Agent setup** section next to its components. It appears once at least one saved profile names an agent, and it lists each component that a profile naming an agent grants, with:

- **Configuration**: ready, or the keys the component needs that the vMCP or its catalog entry must fix. See [Configuration](#configuration).
- **OAuth**: not required, authorized (with who authorized it and when, and **Revoke**), or needs authorization (with **Authorize**). See [Authorize OAuth](#oauth).

Components that no profile naming an agent grants aren't listed: agents can't use them, so they need no setup. Profiles have no agent setup of their own.

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

**Agent setup** shows each such component's configuration as not ready, with the keys it needs.

## Authorize OAuth {#oauth}

For a component that uses OAuth, an administrator authorizes it once on the vMCP. Every agent that a profile of the vMCP grants the component then uses the resulting tokens, and Obot refreshes them as needed. An agent that isn't granted the component never uses its grant.

1. Under **Agent setup**, click **Authorize** next to the component.
2. Sign in to the provider in the window that opens, with the account the agents should use.
3. The component shows as authorized, with who authorized it and when.

You can authorize only a remote component that a profile naming an agent grants. The administrator who starts an authorization must complete it, in the same Obot session. If the component's URL or definition changes before you finish, start again.

If the component's URL is configurable, the vMCP must fix its **Server URL** (or the values its URL template needs) first, as agents need it too. Until then, **Authorize** is refused, and the error names the missing key. A grant is authorized for the URL the vMCP fixes, which every agent reaches.

### How Obot finds the provider {#oauth-discovery}

Some servers ask for OAuth when Obot connects. Others accept the connection and ask only when Obot lists or calls their tools. **Authorize** works for all of them. Obot sends the server requests without the component's configured headers or any stored token, ends the session it opened, and starts the authorization from the first of:

1. The server's OAuth challenge when Obot connects.
2. Its challenge when Obot lists its tools. The authorization asks for the scopes in that challenge.
3. The OAuth protected resource metadata that the server publishes at its well-known location ([RFC 9728](https://datatracker.ietf.org/doc/html/rfc9728)). This covers servers that challenge only when a tool is called.

If the server shows none of these, **Authorize** fails with "does not require OAuth". If the server doesn't answer in time, **Authorize** fails, and you can try again.

### Components with a localhost callback {#localhost-callback}

Some providers redirect only to a localhost callback. For these, **Authorize** shows a command instead of opening a window:

```bash
obot mcp login "https://obot.example.com/oauth/mcp/login/<attempt>"
```

Run it within 10 minutes on a computer with a browser, where you're signed in to Obot as the administrator who started it. See [MCP localhost OAuth sign-in](../reference/cli-api.md#mcp-localhost-oauth-sign-in). Only the authorization needs the CLI. Agents use and refresh the grant without it.

### When a grant stops working {#grant-invalid}

A grant shows as needing authorization, and agents get `oauth_invalid`, when:

- The provider refuses to refresh it, for example because the token was revoked.
- The URL that agents reach changed after it was authorized, for example because the **Server URL** the vMCP fixes changed. A grant works only for the URL it was authorized for.

Click **Authorize** again to fix it. A temporary provider failure doesn't invalidate a grant.

### Revoke a grant {#revoke}

**Revoke** deletes the vMCP's grant for the component. Every agent granted the component then gets `oauth_required` until an administrator authorizes it again. The grant isn't revoked at the provider. Revoke it there too if needed.

Obot also deletes a component's grant when you remove the component from the vMCP, and every grant of a vMCP when you delete the vMCP. Changing profiles and deleting agents delete no grant. If no profile grants a component to an agent any more, its grant is kept and not used, and it applies again once a profile grants the component to an agent.

## Check readiness {#readiness}

**Agent setup** shows, for each component agents are granted, whether it's ready, and what to fix: a value to fix on the vMCP or the catalog entry, or an authorization.

To see whether a specific agent can connect, open the agent under **Identity & Access > Agents**. **vMCP readiness** lists every vMCP that has a profile naming the agent, and what is missing for the components the agent's profiles grant, each linked to the vMCP's **Agent setup**. It runs the same checks as a connection. A component that none of the agent's profiles grants doesn't count. A vMCP with no profile that names the agent isn't listed, because the agent isn't granted it.

When an agent connects to a vMCP that isn't ready, it gets `409 Conflict` with the reasons. It never gets an OAuth challenge or a sign-in link. See [Errors](../reference/agent-identities.md#errors).

### How Obot knows a component needs OAuth {#oauth-detection}

A component needs OAuth, and an agent granted it isn't ready until the vMCP holds a working grant for it, when any of these is true:

- Its catalog entry requires static OAuth.
- The vMCP holds a grant for it, even an invalid one.
- Obot's background check found that it asks for OAuth.

Obot checks each remote component that a profile naming an agent grants in the background, the same way **Authorize** does: it sends requests without the component's configured headers or any stored token, ends the session it opened, and looks for an OAuth challenge when it connects or lists tools, or for the protected resource metadata the server publishes. It checks the URL agents reach: the URL the catalog entry fixes, or the one built from the values the vMCP and the catalog entry fix. A component whose URL needs a value nothing fixes isn't checked: it already blocks agents. Obot records only the result, on the vMCP, never the URL. It checks again when the component, or the values its URL is built from, change, and once a day to notice a server that starts or stops using OAuth.

Readiness and connections never contact a component themselves. They use the result of the last check that succeeded. If no check of the component has succeeded yet, for example because the server didn't answer in time, the component doesn't count as needing OAuth, so a server that can't be reached never blocks agents. Obot retries a failed check after a minute, then waits twice as long after each further failure, up to an hour.

Obot doesn't check a component that takes configured headers, such as an API key, because a server that challenges a request without them may accept one with them. Such a component counts as needing OAuth once the vMCP holds a grant for it. **Agent setup** shows OAuth as not required for a remote component that Obot doesn't know needs it, and you can still authorize it. An agent that reaches a component that asks for OAuth when nothing showed it gets `409 Conflict` with `agent_oauth_required`, never an OAuth challenge.

## Catalog-synced vMCPs {#catalog-vmcps}

**Agent setup** on a vMCP synced from a [Git catalog](../configuration/mcp-server-gitops.md) is editable in Obot, even though the vMCP's definition is read-only there. OAuth grants are stored on the vMCP outside the catalog, so syncs keep them. Removing a component from the catalog's vMCP deletes its grant, and removing the vMCP deletes all its grants.

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

- Administrators can see and change agent setup.
- Read-Only Admins and auditors can see agent setup, including who authorized each grant and when, but not values, tokens, or URLs that come from configuration.
- Agents can't see or change their setup. They can't reach agent setup, readiness or OAuth routes.

A URL that comes from configuration is treated as a value. Agent setup shows only its hostname, and errors that would include it show a placeholder instead. A URL fixed by the catalog entry is shown in full.
