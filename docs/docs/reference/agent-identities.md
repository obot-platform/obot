---
title: "Agent identities reference"
description: API routes, key scopes, error bodies, audit fields, CEL variables and budget behavior for agent identities, and the preconfigured OAuth routes of vMCP components.
---

Reference for [agent identities](../security/agent-identities.md), [setting up vMCPs for agents](../mcp-gateway/agent-setup.md), and the vMCP's [preconfigured OAuth](#preconfigured-oauth), which agents and users share. All examples use made-up IDs and keys.

## API {#api}

Every route below is for administrators. Read-Only Admins and auditors can call the `GET` routes. Agents can call none of them.

| Method and path | Purpose |
|---|---|
| `GET /api/agents` | List agents. `?includeDeleted=true` includes deleted agents. |
| `POST /api/agents` | Create an agent: `{"displayName": "...", "description": "..."}` |
| `GET`, `PUT`, `DELETE /api/agents/{agent_id}` | Read, replace the display name and description of, or delete an agent |
| `POST /api/agents/{agent_id}/disable` | Disable an agent, with an optional `{"reason": "..."}` |
| `POST /api/agents/{agent_id}/enable` | Enable an agent |
| `GET`, `POST /api/agents/{agent_id}/api-keys` | List keys (`?show_revoked=true` includes revoked keys), or create one |
| `GET`, `DELETE /api/agents/{agent_id}/api-keys/{key_id}` | Read or revoke a key |
| `GET /api/agents/{agent_id}/access` | The vMCP profiles, model access policies and skill access rules that name the agent, and `problems` |
| `GET /api/agents/{agent_id}/vmcps` | Readiness of every vMCP the agent is granted |
| `GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness` | Readiness of one vMCP, with reasons |
| `GET`, `POST`, `DELETE /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth` | Read the state of a component's [preconfigured OAuth](#preconfigured-oauth), start its authorization, or revoke it |
| `GET /api/admin-audit-events` | The admin change history |
| `GET /api/admin-api-keys?kind=user` or `?kind=agent` | Every key of one owner kind. The `kind` parameter is required. |

Request bodies reject unknown fields. `PUT /api/agents/{agent_id}` replaces both fields, so send the whole manifest. Deleting an agent, revoking a key and revoking preconfigured OAuth return `204`.

Profiles have no agent routes. The profile status route (`GET /api/vmcps/{vmcp_id}/profiles/{profile_id}/status`) and the profile OAuth routes (`POST` and `DELETE /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth`) were removed, and return `404` or `405` as unknown routes do.

These routes work with authentication on or off.

### Create a key {#create-key-api}

```bash
curl -X POST https://obot.example.com/api/agents/7/api-keys \
  -H "Authorization: Bearer $OBOT_ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "ci", "mcpServerIds": ["*"], "canAccessLLMProxy": true, "expiresAt": "2030-01-01T00:00:00Z"}'
```

The response includes `key`, the only time the full key is returned:

```json
{
  "id": 12,
  "agentId": "7",
  "name": "ci",
  "canAccessSkills": false,
  "canAccessLLMProxy": true,
  "mcpServerIds": ["*"],
  "createdAt": "2026-11-17T10:00:00Z",
  "expiresAt": "2030-01-01T00:00:00Z",
  "key": "oak1-7-12-EXAMPLEnotARealSecret"
}
```

## Key scopes {#key-scopes}

Agent key scopes work as user key scopes do. A scope you leave out isn't granted, and a key needs at least one.

| Scope | Field | User keys | Agent keys |
|---|---|---|---|
| MCP | `mcpServerIds`: vMCP IDs, or `["*"]` | Servers the user can reach | vMCPs that a profile grants the agent. An empty list means no MCP access. |
| LLM gateway | `canAccessLLMProxy` | Yes | Yes |
| Skills | `canAccessSkills` | Yes | Yes |
| Obot API | `canAccessAPI` | Yes | Never |
| Device scans | `canAccessDeviceScans` | Yes | Never |

A scope only allows a route. Model access policies, skill access rules and vMCP profiles decide what the agent can use there, and they're read on every request. LLM and Skills access don't need an MCP scope or a vMCP profile.

With its scopes, an agent key reaches only:

- `/api/llm-proxy/…`, with the LLM scope. This includes `/api/llm-proxy/v1/files` and `/api/llm-proxy/v1/code/agent-proxy/…`, which forward the caller's own Anthropic credential and use no Obot model or budget, as they do for users.
- `GET /api/skills…`, with the Skills scope. `?all=true` is ignored for agents.
- `/mcp-connect/{vmcp_id}`, with an MCP scope that covers the vMCP.
- `POST /api/api-keys/auth`, which answers for the presented key.

Agent keys use the format `oak1-<agentID>-<keyID>-<secret>`. User keys use `ok1-`.

## Errors {#errors}

| Condition | Response |
|---|---|
| Missing, invalid, revoked or expired key, or a deleted agent (whose keys are revoked) | `401`. On MCP connections, no `WWW-Authenticate` header is sent for an agent key. The registry API (`/v0.1`) still sends one. |
| The agent is disabled | `403`, as for a disabled user |
| The agent isn't granted the vMCP, model or route, including a vMCP with no profile that names the agent | `403` |
| The agent's vMCP setup is incomplete | `409`, with one of the bodies below |
| A deployment or upstream failure | The existing `502` and `503` responses |

An agent never gets an OAuth challenge, a redirect, or a sign-in link, including from components reached through a composite vMCP.

### 409 bodies {#409}

```json
{
  "error": "agent_configuration_required",
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "GITHUB_ORG"}
  ]
}
```

```json
{
  "error": "agent_oauth_required",
  "reasons": [
    {"componentID": "github", "code": "oauth_required"},
    {"componentID": "jira", "code": "oauth_invalid"}
  ]
}
```

`error` is `agent_oauth_required` if every reason is `oauth_required` or `oauth_invalid`, and `agent_configuration_required` otherwise.

| Reason field | Meaning |
|---|---|
| `componentID` | The vMCP component |
| `code` | `configuration_required`, `oauth_required` or `oauth_invalid` |
| `field` | The missing configuration key. Values are never included. |

Every reason is fixed on the vMCP. Fix `configuration_required` by setting the value as fixed on the vMCP or in the catalog entry, or by no longer granting the component to the agent: agents get only values fixed for everyone. Fix `oauth_required` and `oauth_invalid` by authorizing the component's [preconfigured OAuth](#preconfigured-oauth) in its settings in the vMCP designer. Reasons cover only the components that the profiles naming the agent grant.

When an upstream fails after an MCP response has started, the agent gets a JSON-RPC error without these codes.

## Readiness {#readiness}

`GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness`:

```json
{
  "vmcpID": "vmcp1example",
  "ready": false,
  "disabled": false,
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "GITHUB_ORG"}
  ],
  "preconfiguredComponents": ["calendar"]
}
```

Readiness runs the same checks as a connection, when you call it, over the components that the profiles naming the agent grant. An agent named by several profiles is checked for the union of their components. A component that none of them grants doesn't count. A vMCP with no profile that names the agent isn't in `GET /api/agents/{agent_id}/vmcps`, and returns `404` here. `disabled: true` means the agent is disabled and can't connect, whatever its readiness. The reasons are those of the [409 bodies](#409), and each is fixed on the vMCP.

`preconfiguredComponents` lists the IDs of the components the agent is granted for which the vMCP holds [preconfigured OAuth](#preconfigured-oauth) that works, checked as a connection checks it. The agent uses that account upstream. Preconfigured OAuth that no longer works isn't listed: it's an `oauth_invalid` reason. The list is always present, and empty when there are none.

`GET /api/agents/{agent_id}/access` lists `problems`:

- `no_model_access_policy`: a key has the LLM scope, but no model access policy names the agent.
- `no_skill_access_rule`: a key has the Skills scope, but no skill access rule names the agent.

Every entry in `vmcpProfiles`, `modelAccessPolicies` and `skillAccessRules` names the agent by ID. A policy for **All Obot Users** never matches an agent, so it isn't listed and doesn't count against these problems. `vmcpProfiles` lists every profile that names the agent, including several of one vMCP.

## Preconfigured OAuth {#preconfigured-oauth}

A shared vMCP holds at most one [preconfigured OAuth](../mcp-gateway/server-types.md#preconfigured-oauth) account per remote component. While it works, every user and agent that uses the component through the vMCP uses it, and a user's own token for the component isn't used. The routes are under `/api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth`. Administrators can call all of them, and Read-Only Admins and auditors only `GET`. Each returns `404` for an unknown vMCP or component.

### State {#preconfigured-oauth-state}

`GET /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth`:

```json
{
  "vmcpID": "vmcp1example",
  "componentID": "jira",
  "state": "authorized",
  "authorizedBy": "42",
  "authorizedAt": "2026-11-18T09:30:00Z",
  "target": "https://mcp.jira.example.com/mcp",
  "scopes": ["read"]
}
```

| Field | Meaning |
|---|---|
| `state` | `not_required`, `needs_authorization`, `authorized` or `invalid` |
| `authorizedBy`, `authorizedAt` | The administrator (a user ID) who authorized it, and when |
| `target`, `targetHostname` | The URL it was authorized for: `target`, the full URL, only when the catalog entry fixes it, and otherwise only `targetHostname` |
| `scopes`, `invalidReason` | Its scopes, and why `invalid` OAuth stopped working |

The response never contains a configuration value, a token, or a URL that comes from configuration.

A component with no preconfigured OAuth is `needs_authorization` when its catalog entry requires static OAuth, or when Obot's background check found that the server asks for OAuth at the URL that the vMCP and the catalog entry fix. Otherwise it's `not_required`. The background check runs for each remote component of a shared vMCP, sends the same unauthenticated requests as an authorization start, and records its result on the vMCP's status. A check that fails, and a component not checked yet, count as not required. Components that take configured headers aren't checked. See [Which components need OAuth](../mcp-gateway/server-types.md#preconfigured-oauth-detection).

For an agent granted the component, readiness and connections report `oauth_required` in the `needs_authorization` state, and `oauth_invalid` in the `invalid` state.

### Authorize {#preconfigured-oauth-authorize}

`POST /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth` starts an authorization and returns `{"authURL": "..."}`. For a component with a localhost callback, it also has `"localhostCallback": true`, and `authURL` is the URL to pass to `obot mcp login`. Authorizing a component that is already authorized replaces its account once the new authorization completes.

The start returns `400` if the component isn't a remote server, the vMCP is personal, the URL fails the hostname constraint, the server doesn't require OAuth, or the URL needs a value that isn't fixed (see below). It returns `409` if the vMCP is being deleted.

The start sends the server unauthenticated requests: `initialize`, then, if that succeeds, `notifications/initialized` and `tools/list` in the same session, which it then ends with `DELETE`. It starts from the first `401` challenge, and otherwise from the protected resource metadata at the server's well-known location (RFC 9728). It returns `400` with `does not require OAuth` when there is neither, and an error when `tools/list` fails or a request times out. See [How Obot finds the provider](../mcp-gateway/server-types.md#preconfigured-oauth-discovery).

The callback returns `403` to an administrator other than the one who started it, `409` if the component or its URL changed, and `400` if the attempt expired or was already used. Errors never contain a URL that comes from configuration. They show `<configured URL>` or `<redacted>` instead.

### 400 when the URL isn't fixed {#400-missing-keys}

When a component's URL needs a value that the vMCP and the catalog entry don't fix, the start returns `400` with the missing keys:

```json
{
  "error": "the upstream URL of component \"github\" needs __url, which is not fixed for everyone: set it as fixed on the vMCP or on the catalog entry first",
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "__url"}
  ]
}
```

### Revoke {#preconfigured-oauth-revoke}

`DELETE /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth` deletes the component's preconfigured OAuth and returns `204`. Users are then asked for their own OAuth for the component again, and agents granted it get `oauth_required`.

### Errors for users {#preconfigured-oauth-errors}

When a component's preconfigured OAuth stops working, a user's MCP request to the vMCP gets `409 Conflict`, never an OAuth challenge for that component:

```json
{
  "error": "preconfigured_oauth_invalid",
  "message": "The OAuth account an administrator preconfigured for \"Jira\" on this vMCP no longer works. An administrator must authorize it again.",
  "componentIDs": ["jira"]
}
```

`componentIDs` lists the components whose preconfigured OAuth stopped working, and `message` names them as the vMCP does. Agents get the [409 body](#409) with `oauth_invalid` instead.

## Audit {#audit}

### Activity {#activity-audit}

LLM gateway audit logs, MCP audit logs, token usage and budget usage attribute requests by principal:

| Field | Users | Agents |
|---|---|---|
| `principal` | `{"kind": "user", "id": "42"}` | `{"kind": "agent", "id": "7"}` |
| `userID` | The user's ID | Empty. Budget usage reports use `agent:7`. |
| `agentDisplayName` | — | The agent's current name, kept after deletion |
| `apiKeyID`, `apiKeyName` | The key used | The key used. An unnamed key shows as `oak1-7-12-*****`. |

MCP audit records also carry the vMCP (`vmcpID`), the component (`vmcpComponentID`) and the profiles that matched (`profileIDs`). For an agent, that's every profile that names it.

Both audit log APIs accept `actor_kind=user|agent`. To select one agent, use `user_id=agent:<id>` wherever `user_id` works. Records written before agent identities existed have no principal and are read as the user in `userID`. Skill downloads aren't audited.

### Admin change history {#admin-audit}

`GET /api/admin-audit-events` returns changes newest first, as `{"items": [...], "total": N, "limit": 100, "offset": 0}`. Filter with `resourceKind`, `resourceId`, `actor` (a user ID, or `system`) and `action`, each repeatable or comma-separated, and with `since` and `until` (RFC 3339). Page with `limit` (at most 1000) and `offset`. An invalid filter returns `400`.

| `resourceKind` | `action` | `parentID` |
|---|---|---|
| `agent` | `agent.create`, `agent.update`, `agent.disable`, `agent.enable`, `agent.delete` | — |
| `apiKey` | `agent.key.create`, `agent.key.revoke` | The agent |
| `modelAccessPolicy` | `model_access_policy.subjects.update` | — |
| `skillAccessRule` | `skill_access_rule.subjects.update` | — |
| `vmcpProfile` | `vmcp.profile.subjects.update` | The vMCP |
| `vmcp` | `vmcp.preconfigured_oauth.authorize`, `vmcp.preconfigured_oauth.revoke` | — |
| `vmcp` | `vmcp.preconfigured_oauth.invalidate`, when the provider refuses to refresh preconfigured OAuth, and `vmcp.preconfigured_oauth.delete`, when a component or vMCP is removed, with actor `system` | — |

Preconfigured OAuth events are recorded against the vMCP: `resourceID` is the vMCP's ID, and `details.componentID` names the component.

An event's `details` name what changed: fields, subjects, scopes and hostnames. They never contain configuration values, tokens, API key secrets, configured URLs, or the text of a disable reason. Subject events are recorded only when a change adds or removes an agent. Changes to users, groups and **All Obot Users** aren't recorded.

Only changes made through the admin API are recorded, plus the `system` events. Changes that arrive through catalog sync and changes made by controllers are not.

Events are kept for the longer of the MCP and LLM audit log retention periods, or forever if either is `0`. They're kept after the agent, key, profile or vMCP is deleted. To export them, go to Operations → Audit Logs and click **Admin Change Exports**, then create a one-time export or an export schedule. See [Admin Change Exports](../security/audit-data.md#audit-log-export-admin-change-exports). Through the API, create an [audit log export](../security/audit-data.md#audit-log-export-creating-exports) (`POST /api/audit-log-exports` or `/api/scheduled-audit-log-exports`) with `"type": "admin"` and optional `adminFilters`: `resourceKinds`, `resourceIDs`, `actors` and `actions`.

## CEL variables {#cel}

Guardrail conditions, for both LLM guardrails and MCP guardrail filters, can use:

| Variable | Users | Agents |
|---|---|---|
| `user.id` | The user's ID, such as `"42"` | `"agent:7"` |
| `user.groups` | The user's auth provider group IDs | Empty |
| `principal.kind` | `"user"` | `"agent"` |
| `principal.id` | `"42"` | `"7"` |

`principal.id` alone doesn't identify a caller, because a user and an agent can have the same ID. Check `principal.kind` too, for example `principal.kind == "agent" && principal.id == "7"`. Both are empty for callers that aren't users or agents.

## Budgets {#budgets}

Budgets of the model access policies that name an agent apply to it, as they do to users:

- A per-user budget gives each agent its own allowance, counted separately from the user with the same ID.
- A shared budget counts agents and users together.
- **Exempt administrators** never exempts an agent.
- Users' personal daily token limits don't apply to agents. Agents get the *Daily token limit* budget's own limits.

Budget usage reports show agents as `userID: "agent:<id>"`, with a `principal` field.
