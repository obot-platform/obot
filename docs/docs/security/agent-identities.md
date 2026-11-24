---
title: "Agent identities"
description: Give automation its own identity, API keys, policy grants, and audit trail, independent of any user.
---

An agent identity is a non-human identity for automation, such as a CI job, a scheduled task, or a service that calls MCP servers and models with no user present. An administrator creates the agent, grants it access through policies, and issues API keys that authenticate as the agent. The agent's access and audit history belong to the agent, not to the person who created it, and the upstream OAuth accounts it uses are preconfigured on the vMCP.

An agent can also act on behalf of the people who use it, once each of them approves it. See [Agents acting on behalf of users](./agents-on-behalf-of-users.md).

## How agents differ from other identities {#concepts}

| | Agent identity | User | User API key |
|---|---|---|---|
| What it is | A service account that administrators manage | A person who signs in | A credential that acts as its user |
| Where it's managed | **Identity & Access > Agents** | **Identity & Access > Users** | **Identity & Access > API Keys** |
| How it authenticates | `oak1-` API keys | The auth provider | An `ok1-` API key |
| Where its access comes from | Policies that name the agent | Roles, groups and policies | Its user's access, narrowed by its scopes |

Agents differ from users in these ways:

- **No roles, groups, or sign-in.** An agent is never an administrator or an auditor, can't use the Obot UI, and never matches a group subject or **All Obot Users**. Its access comes only from the policies that name it by ID.
- **Independent of people.** Each agent records the administrator who created it, but that person grants it nothing. Changing or deleting that user doesn't change the agent.
- **Headless.** Agents use only the configuration that a vMCP or its catalog entry fixes for everyone. When a vMCP component needs upstream OAuth, an administrator [preconfigures](../mcp-gateway/server-types.md#preconfigured-oauth) it ahead of time, with one account that every user and agent of the vMCP uses. An agent is never asked to sign in. When setup is missing, the agent gets a `409` that names what is missing. Acting on behalf of a user, it can also use the values and accounts that the user [set up through it](./agents-on-behalf-of-users.md#users).
- **Limited routes.** An agent key reaches only the LLM gateway, skill discovery and download, vMCP connections, and the two routes with which it [asks users for approval and gets tokens](./agents-on-behalf-of-users.md#example) to act for them. It never reaches the Obot admin API, user routes, or device APIs, whatever its scopes.

Agent identities aren't the same thing as user API keys. A user API key acts as its user, with that user's roles and grants. Before agent identities existed, user API keys were called *agent identities* and *agent authorization scopes* in the UI. They're now under **Identity & Access > API Keys**. See [Agent authorization scopes](../functionality/agent-auth-scopes.md).

Agent identities also aren't the agents that Obot runs for users in [Claude Environments](../claude-environments/overview.md), which replaced hosted agents, or the AI clients on managed devices that [Device Management](../device-management/how-sentry-works.md) inventories. Those run as, or on behalf of, a user. An agent identity is its own principal.

## Create an agent {#create-an-agent}

Only administrators can create agents. [Read-Only Admins](./policy-coverage.md#user-roles-admin-read-only) and auditors can view agents, their keys, their access and their readiness, but can't change them.

1. Open **Identity & Access > Agents**.
2. Click **Create Agent**.
3. Enter a **Name** and, optionally, a **Description**. Names don't need to be unique: an agent is identified by its ID.
4. Click **Create**.

Open the agent to see its ID, its keys, its access, its readiness for each vMCP, and the history of changes made to it.

## Create an API key for an agent {#create-a-key}

1. Open the agent and click **Create API Key**.
2. Enter a **Name**, and optionally a description and an expiration date.
3. Click **Create**, then copy the key. Obot shows it only once.

A key created in the UI can use everything the agent has access to: every vMCP the agent is granted now or later, the LLM Gateway, and Skills. To create a key with narrower scopes, use the [CLI](#cli) or the API. Scopes only narrow what the agent is granted. They grant nothing by themselves: a key with LLM access can't use any model until a model access policy grants the agent one. Agent keys never have the API access or device scan access scopes that user keys can have. See [Key scopes](../reference/agent-identities.md#key-scopes).

An agent can have several keys. To rotate a key, create a new one, move the automation to it, and then revoke the old key. Revoking a key takes effect on its next request.

You can create a key for a disabled agent, to prepare a rotation. It doesn't authenticate until the agent is enabled.

## Use the CLI {#cli}

The `obot agents` commands manage agents and their keys. Every command needs an administrator's credential with API access, and the CLI may ask you to sign in again to get it. Read-Only Admins and auditors can run only the read commands (`list`, `get`, `keys list`, and `keys get`).

```bash
obot agents create --name "Nightly report" --description "Builds the nightly sales report"
obot agents list
obot agents keys create 7 --name ci --mcp '*' --llm --expires-in 720h
obot agents keys list 7
obot agents keys revoke 7 12
obot agents disable 7 --reason "Rotating credentials"
obot agents enable 7
obot agents delete 7 --yes
```

`keys create` prints the key alone on standard output and its details on standard error, so a script can capture just the key:

```bash
OBOT_AGENT_KEY=$(obot agents keys create 7 --name ci --llm --skills)
```

- `--mcp` takes a vMCP ID or `'*'` for every vMCP the agent is granted, and can be repeated. `--llm` and `--skills` turn on those scopes. A key with none of the three is refused.
- `--expires-in` (a duration such as `720h`) and `--expires-at` (an RFC 3339 time) are mutually exclusive. With neither, the key doesn't expire.
- `--json` prints the API object instead. On `keys create`, that includes the key.
- `update` changes only the flags you pass. For example, `--description ""` clears the description.
- `delete` asks for confirmation on a terminal. When standard input isn't a terminal, it requires `--yes`.
- Flags can also be set through environment variables named for the command and flag, such as `AGENTS_KEYS_CREATE_NAME`.

Run `obot agents --help` for every command and flag.

## Grant access {#grant-access}

An agent can use only what policies grant it. Four kinds of policy can name agents:

| Policy | Grants | Where |
|---|---|---|
| [vMCP profiles](../mcp-gateway/agent-setup.md) | vMCPs, their components and their tools | A vMCP's **Profiles** |
| [Model access policies](../functionality/model-access-policies.md#subjects) (LLM gateway profiles) | Models, with their budgets and applied guardrails | **LLM Gateway > Profiles** |
| [Skill access rules](../registries/publish-skills.md#skill-access-policies-users-and-groups) | Skills and skill sources | **Skills > Access Policies** |
| [MCP guardrail filters](../functionality/filters.md#guardrail-filters) | Nothing: a filter restricts MCP traffic | **MCP Servers > Filters** |

In each of these editors, select agents by name. A policy applies to an agent only if it names that agent:

| Option | Matches | API subject |
|---|---|---|
| A specific agent | That agent | `{"type": "agent", "id": "7"}` |
| **All Obot Users** | Every user, and no agents | `{"type": "selector", "id": "*"}`, or `everyone` in guardrail filters |

There is no option for all agents, and the API refuses `{"type": "agent", "id": "*"}`: agents must be named by ID. Agents never match **All Obot Users**, users or groups. So a policy for everyone grants an agent nothing, and a guardrail filter for everyone, or with no subjects, doesn't apply to agents. To filter an agent's MCP traffic, name the agent in the filter. Granting or restricting an agent is always deliberate.

Grants add up, as they do for users. An agent named by several model access policies can use the models of each, and their budgets and applied guardrails all apply. An agent named by several profiles of a vMCP gets the components and tools of each. Grant changes apply to the agent's existing keys at once. Keys don't need to be reissued.

The upstream OAuth that agents use belongs to the vMCP, not to a profile: its [preconfigured OAuth](../mcp-gateway/server-types.md#preconfigured-oauth), one account per component, which every user and agent of the vMCP uses. See [Upstream OAuth](../mcp-gateway/agent-setup.md#oauth).

MCP access policies (catalog access control rules) and message policies don't apply to agents and can't name them. An agent reaches MCP servers only through vMCP profiles.

## Check what an agent can do {#readiness}

The agent's page shows:

- **Access**: every vMCP profile, LLM gateway policy and skill access rule that names the agent.
- **Problems**: a key with LLM access but no LLM gateway policy that names the agent, so every LLM request is refused; or a key with skill access but no skill access rule, so the agent sees no skills.
- **vMCP readiness**: for each vMCP with a profile that names the agent, whether it can connect now, and if not, what is missing, with a link to the component's settings in the vMCP designer. Components whose [preconfigured OAuth](../mcp-gateway/server-types.md#preconfigured-oauth) works are marked **Preconfigured**, and what a user supplies when the agent [acts on their behalf](./agents-on-behalf-of-users.md) is marked **OBO**. Readiness covers the components that the agent's profiles grant, and runs the same checks as a connection, at the moment you look.

The same information is in the API (`GET /api/agents/{agent_id}/access`, `GET /api/agents/{agent_id}/vmcps`, and `GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness`). See the [reference](../reference/agent-identities.md#api).

## Connect an agent {#connect}

Pass the key as a bearer token:

```bash
curl https://obot.example.com/api/llm-proxy/v1/models \
  -H "Authorization: Bearer $OBOT_AGENT_KEY"
```

- **MCP**: configure the client with the vMCP's connection URL, `https://<obot-host>/mcp-connect/<vmcp-id>`, and the `Authorization` header. Every key of an agent shares the agent's one connection to each vMCP.
- **LLM gateway**: use the base URL `https://<obot-host>/api/llm-proxy/v1`, as in [Connect clients and applications](../llm-gateway/connect-clients.md).
- **Skills**: use the skill routes under `/api/skills`.

Agent keys can't call `/api/me` or other Obot API routes. Test a key with the route it's meant for.

## Disable or delete an agent {#disable-delete}

**Disable** stops the agent at once: every later request with its keys or its internal tokens is refused with `403`, including requests on MCP sessions that are already open. Disabling keeps its keys, grants and connections, so **Enable** restores it without other changes.

**Delete** can't be undone. It:

- Revokes all the agent's keys.
- Removes its vMCP connections and the servers dedicated to it, with their credentials and tokens.
- Keeps its audit and usage history, which still shows the agent's name.
- Leaves policies alone. Subjects that name a deleted agent stay in place, match nobody, and show as **Deleted agent (ID)**, as a deleted user's subjects do. Agent IDs are never reused. Remove the subjects when you tidy up the policies.
- Leaves the vMCP's preconfigured OAuth alone. It belongs to the vMCP, and the vMCP's users and other agents keep using it.

From the CLI, use `obot agents disable`, `obot agents enable`, and `obot agents delete`.

## Audit {#audit}

LLM gateway and MCP audit logs attribute an agent's requests to the agent and the key it used, with an **Agent** badge, and you can filter by actor kind or by agent. MCP audit records also name the vMCP, the component, and every profile that names the agent. Usage reports and budgets count each agent separately.

Changes to agents, their keys, the policies and profiles that name agents, and vMCPs' preconfigured OAuth are recorded in the admin change history. The agent's page shows its history. These records never contain configuration values, tokens, API key secrets, or configured URLs. See [Audit fields](../reference/agent-identities.md#audit).

## With authentication off {#authentication-off}

Agents work when authentication is disabled. The single owner manages agents in the UI, the API, and the CLI. A request with an agent key authenticates as that agent, with the agent's access and the key's scopes, not as the owner. A request with a revoked, expired or otherwise invalid agent key is refused, never treated as the owner.

## Roll back agent identities {#rollback}

Versions of Obot without agent identities refuse to save policies that name agents. Changing the image alone isn't enough. Before downgrading to such a version:

1. Disable every agent, so none of their keys authenticate.
2. Remove agents from model access policies, skill access rules, vMCP profiles and MCP guardrail filters.
3. Revoke the preconfigured OAuth of every vMCP component, in the component's settings in the vMCP designer. Do this before downgrading: older versions ignore these tokens, which would stay stored. After the downgrade, users authorize those components with their own accounts.
4. Delete the agents.
5. Downgrade, following [Upgrades and rollback](../operations/upgrades.md#rollback).

Leave the database as it is. The tables and columns that agent identities added are safe for older versions, which ignore them.

## Limitations {#limitations}

- **No agent groups or agent wildcard.** Name agents one by one, in every policy.
- **No personal vMCPs.** Agents can use only shared vMCPs that administrators publish, through vMCP profiles.
- **No user daily token limits.** Users' personal daily token limits, including the per-user limit of the *Daily token limit* budget, don't apply to agents. Agents get the budget's own limits. Budgets that exempt administrators never exempt an agent.
- **Agent IDs in catalog YAML are per deployment.** Catalog-synced vMCPs can name agents by ID, but an ID names a different agent, or none, in another deployment, and `obot mcp validate-catalog` can't check that it exists. See [Catalog-synced vMCPs](../mcp-gateway/agent-setup.md#catalog-vmcps).
- **No configuration for agents.** Agents get only values that the vMCP or its catalog entry fixes for everyone. A component that needs a value a user would enter, including a configurable server URL the vMCP doesn't fix, keeps agents off the vMCP until the value is fixed or the component is no longer granted. See [Configuration](../mcp-gateway/agent-setup.md#configuration).
- **One upstream account per vMCP component.** Every user and agent of a vMCP uses a component's preconfigured OAuth, so they can't use different upstream accounts for the same component. Use separate vMCPs for that.
- **Preconfigured OAuth is bound to a URL.** If the component's URL changes after it was authorized, it shows as invalid until an administrator authorizes it again.
- **Mid-stream errors carry no reason code.** When an upstream fails after an MCP response has started, the agent gets a JSON-RPC error without the `agent_oauth_required` code.
- **Skill downloads aren't audited**, for agents or users.
- **Some changes have no history.** Only changes made through the admin API are in the change history, plus invalidations and removals of preconfigured OAuth by Obot itself. Subject changes that arrive through catalog sync are not.
- **Guardrail conditions in the UI.** The guardrail condition editor doesn't yet suggest the `principal.kind` and `principal.id` variables, but conditions can use them. See [CEL variables](../reference/agent-identities.md#cel).
- **No CLI for preconfigured OAuth.** It's managed in the UI or through the API. The CLI manages only agents and keys.
