---
title: "Set up vMCPs for agents"
description: Grant agent identities access to a vMCP and supply the configuration and upstream OAuth they need, with no user present.
---

[Agent identities](../security/agent-identities.md) connect to vMCPs through the vMCP's profiles, like users. Unlike a user, an agent can't type configuration values or sign in to an upstream provider. An administrator supplies both on the profile ahead of time, once for every agent the profile applies to.

## Grant agents access {#grant-access}

Add agents to a profile in the vMCP's **Profiles**, with **Add User/Group/Agent**:

- A specific agent
- **All Agents**
- **Everyone (All Obot Users and Agents)**

**All Users** matches users only. Permissions add up across every profile that matches an agent, as they do for users. If a wildcard profile grants component A and the agent's own profile grants component B, the agent can use both.

A profile that names an agent, **All Agents**, or **Everyone** is *agent-capable*. Only agent-capable profiles have **Agent setup**, and it appears in the profile once the profile is saved that way. A new profile, or a saved profile you just added an agent to, asks you to save it first. A profile that no agent can use has no **Agent setup**.

## Subject rules {#subject-rules}

So that every agent gets its configuration and grants from a known profile, a vMCP's profiles must follow these rules:

- One profile names **Everyone**, or at most one profile names **All Users** and at most one names **All Agents**. A vMCP can't use both forms.
- An agent can be named explicitly in only one profile.

The UI and the API refuse changes that break these rules. A vMCP saved before these rules may have several **Everyone** profiles. It keeps working for users, but agents that would fall back to a wildcard profile can't connect, and the vMCP shows **More than one profile names every agent**. Remove **Everyone** or **All Agents** from all but one profile to fix it. Until you do, agent setup on that vMCP is read-only.

## How an agent's setup is chosen {#precedence}

For each vMCP, an agent has at most one *explicit* profile, the one that names it, and at most one *wildcard* profile, the one that names **All Agents** or **Everyone**.

A profile supplies setup only for the components it grants: all of them with **Allow All Access**, otherwise the ones you enable on it. The rules below apply to each component separately, between the agent's profiles that grant it. If the agent's explicit profile doesn't grant a component that its wildcard profile grants, the agent takes all of that component's setup from the wildcard profile, optional values and its OAuth grant included, as an agent without an explicit profile does.

- **Required values** come from the explicit profile. A required value the explicit profile doesn't set comes from the wildcard profile.
- **Optional values** come from the explicit profile if the agent has one, and from the wildcard profile otherwise. They never mix: an optional value the explicit profile leaves unset stays unset.
- **OAuth grants** come from the explicit profile if it has authorized the component, and from the wildcard profile otherwise. If the explicit profile's grant stops working, the agent fails rather than falling back to a different upstream account.
- **Values fixed by the vMCP or its catalog entry**, including values from Kubernetes Secrets, apply to agents as to everyone. A profile can't change them.

Removing an agent from its explicit profile doesn't revoke its access. It falls back to the wildcard profile's access and setup, if there is one.

Every component the agent can use must be fully set up. If any is missing a required value or a working grant, the whole connection is refused, rather than offering fewer tools.

## Configure values {#configuration}

1. Open the vMCP, then **Profiles**, and open an agent-capable profile. Save a new profile first: setup is kept by the profile's ID, which it gets when saved.
2. Under **Agent setup**, enter values for each component. The list includes only the components this profile grants. To set up another component on this profile, grant it on the profile first.
3. Click **Save configuration**.

Only values a user could enter on their own connection can be set. Values fixed by the vMCP or the catalog entry are listed under **Applies to everyone**, read-only and without their values.

Values are write-only. After saving, the page shows only whether each key is set. Use **Replace** to change a value and **Clear** to remove it. You can save incomplete configuration. The profile shows what is still missing.

For a remote component whose URL is configurable, set **Server URL**. It must match the hostname the catalog entry allows.

Saved values reach agents' connections automatically. An agent's next connection waits until its connection has them.

**Reset** clears every value on the profile. If the old values take longer than 15 seconds to remove, the reset still succeeds and the profile shows that cleanup is still finishing. Agents stop getting the values at once.

If you stop granting a component on a profile, the values and OAuth grant the profile holds for it are kept, but they aren't shown or used. They apply again if you grant the component again. Deleting the profile deletes them. Removing the component from the vMCP deletes its grants, and its values go with the profile's next save. **Reset** clears kept values too.

## Authorize OAuth {#oauth}

For a component that uses OAuth, an administrator authorizes it once on the profile. Every agent that gets its grant from this profile then uses the resulting tokens, and Obot refreshes them as needed.

1. Under **Agent setup**, click **Authorize** next to the component.
2. Sign in to the provider in the window that opens, with the account the agents should use.
3. The profile shows the component as authorized, with who authorized it and when.

The administrator who starts an authorization must complete it, in the same Obot session. If the component's URL or definition changes before you finish, start again.

If the component's URL comes from the profile's configuration, set **Server URL** first. Until it is set, **Authorize** is refused, and the error names the missing key.

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

A grant shows as invalid, and agents get `oauth_invalid`, when:

- The provider refuses to refresh it, for example because the token was revoked.
- The URL that agents reach changed after it was authorized, for example because the profile's **Server URL** changed. A grant works only for the URL it was authorized for.

Click **Authorize** again to fix it. A temporary provider failure doesn't invalidate a grant.

**Reset** deletes the grant. It isn't revoked at the provider. Revoke it there too if needed.

## Check readiness {#readiness}

Each agent-capable profile shows whether it is ready, and lists what is missing, each linked to the field or button that fixes it. This describes the profile alone. An agent can still get missing values from its wildcard profile.

To see whether a specific agent can connect, open the agent under **Identity & Access > Agents**. **vMCP readiness** lists every vMCP the agent is granted, what is missing, and which profile to fix it in. It also shows where each value comes from, including fallbacks. It runs the same checks as a connection.

When an agent connects to a vMCP that isn't ready, it gets `409 Conflict` with the reasons. It never gets an OAuth challenge or a sign-in link. See [Errors](../reference/agent-identities.md#errors).

### How Obot knows a component needs OAuth {#oauth-detection}

A component needs OAuth, and an agent whose profiles hold no grant for it isn't ready, when any of these is true:

- Its catalog entry requires static OAuth.
- Any profile of the vMCP holds a grant for it, even an invalid one.
- Obot's background check found that it asks for OAuth.

Obot checks each remote component that an agent-capable profile grants in the background, the same way **Authorize** does: it sends requests without the component's configured headers or any stored token, ends the session it opened, and looks for an OAuth challenge when it connects or lists tools, or for the protected resource metadata the server publishes. It checks each URL agents reach: the URL the catalog entry fixes, or each URL that the profiles' **Server URL** configuration sets. Obot keeps only a digest of a configured URL, never the URL. It checks again when the component or a profile's URL changes, and once a day to notice a server that starts or stops using OAuth.

Readiness and connections never contact a component themselves. They use the result of the last check that succeeded. If no check of a URL has succeeded yet, for example because the server didn't answer in time, the component doesn't count as needing OAuth there, so a server that can't be reached never blocks agents. Obot retries a failed check after a minute, then waits twice as long after each further failure, up to an hour.

Obot doesn't check a component that takes configured headers, such as an API key, because a server that challenges a request without them may accept one with them. Such a component counts as needing OAuth once a profile authorizes it. An agent that reaches a component that asks for OAuth when nothing showed it gets `409 Conflict` with `agent_oauth_required`, never an OAuth challenge.

## Catalog-synced vMCPs {#catalog-vmcps}

Agent setup on a vMCP synced from a [Git catalog](../configuration/mcp-server-gitops.md) is editable in Obot, even though the vMCP's definition is read-only there. Configuration and grants are stored by profile ID, outside the catalog.

Catalog YAML can name `agent` with `*` and `selector` with `*`, but not a specific agent, because agent IDs belong to one Obot deployment. `obot mcp validate-catalog` checks the subject rules.

Give each profile an explicit `id`, so that renaming it keeps its configuration and grants:

```yaml
profiles:
  - id: agents
    name: All agents
    subjects:
      - type: agent
        id: "*"
    vmcpPermissions:
      allowAllComponents: true
```

An `id` is 1–63 lowercase letters, digits and hyphens, and starts and ends with a letter or digit. Without one, Obot derives the ID from the vMCP and profile names. Renaming the profile then makes it a new profile, and the old profile's configuration and grants are deleted.

## Who can do what {#permissions}

- Administrators can see and change agent setup.
- Read-Only Admins and auditors can see profile status, including who authorized each grant and when, but not values, tokens, or URLs that come from configuration.
- Agents can't see or change their setup. They can't reach configuration, readiness or OAuth routes.

A URL that comes from configuration is treated as a value. Status shows only its hostname, and errors that would include it show a placeholder instead. A URL fixed by the catalog entry is shown in full.
