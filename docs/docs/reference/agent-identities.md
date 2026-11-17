---
title: "Agent identities reference"
description: API routes, key scopes, error bodies, audit fields, CEL variables and budget behavior for agent identities.
---

Reference for [agent identities](../security/agent-identities.md) and [agent setup on vMCP profiles](../mcp-gateway/agent-setup.md). All examples use made-up IDs and keys.

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
| `GET /api/agents/{agent_id}/access` | The vMCP profiles, model access policies and skill access rules that match the agent, and `problems` |
| `GET /api/agents/{agent_id}/vmcps` | Readiness of every vMCP the agent is granted |
| `GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness` | Readiness of one vMCP, with reasons and sources |
| `PUT /api/vmcps/{vmcp_id}/profiles/{profile_id}/configuration` | Set or clear profile configuration |
| `POST /api/vmcps/{vmcp_id}/profiles/{profile_id}/configuration/reset` | Clear all of a profile's configuration |
| `GET /api/vmcps/{vmcp_id}/profiles/{profile_id}/status` | A profile's setup status, without fallback |
| `POST`, `DELETE /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth` | Start an OAuth authorization, or reset the grant |
| `GET /api/admin-audit-events` | The admin change history |
| `GET /api/admin-api-keys?kind=user` or `?kind=agent` | Every key of one owner kind. The `kind` parameter is required. |

Request bodies reject unknown fields. `PUT /api/agents/{agent_id}` replaces both fields, so send the whole manifest. Deleting an agent and revoking a key return `204`.

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

### Configure a profile {#configure-api}

```bash
curl -X PUT https://obot.example.com/api/vmcps/vmcp1example/profiles/agents/configuration \
  -H "Authorization: Bearer $OBOT_ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"components": {"github": {"GITHUB_ORG": "example-org", "LOG_LEVEL": null}}}'
```

A value sets a key, and `null` or a blank value clears it. Keys you leave out keep their values. Only keys with the `userAllowed` policy can be set. Saving incomplete configuration is allowed.

| Status | Meaning |
|---|---|
| `400` | The profile isn't agent-capable, or a key is fixed, set by the catalog entry, bound to a Secret, prohibited, or unknown, or a URL fails the catalog's hostname constraint |
| `404` | Unknown vMCP or profile |
| `409` | Agent selection on the vMCP is ambiguous, or the vMCP is being deleted |

A reset returns `200`. If removing the old values takes longer than 15 seconds, its body has `"pendingCollection": true`, and the profile counts as having configuration until removal finishes.

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
| The agent isn't granted the vMCP, model or route | `403` |
| The agent's vMCP setup is incomplete | `409`, with one of the bodies below |
| A deployment or upstream failure | The existing `502` and `503` responses |

An agent never gets an OAuth challenge, a redirect, or a sign-in link, including from components reached through a composite vMCP.

### 409 bodies {#409}

```json
{
  "error": "agent_configuration_required",
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "GITHUB_ORG", "profileID": "agents"}
  ]
}
```

```json
{
  "error": "agent_oauth_required",
  "reasons": [
    {"componentID": "github", "code": "oauth_required", "profileID": "agents"},
    {"componentID": "jira", "code": "oauth_invalid", "profileID": "vp1example", "urlProfileID": "agents"}
  ]
}
```

```json
{
  "error": "agent_selection_ambiguous",
  "reasons": [
    {"code": "selection_ambiguous"}
  ]
}
```

`error` is chosen by the reasons, in this order:

1. `agent_selection_ambiguous` if any reason is `selection_ambiguous`.
2. `agent_oauth_required` if every reason is `oauth_required` or `oauth_invalid`.
3. `agent_configuration_required` otherwise.

| Reason field | Meaning |
|---|---|
| `componentID` | The vMCP component. It is absent when the reason is about the whole vMCP. |
| `code` | `configuration_required`, `oauth_required`, `oauth_invalid` or `selection_ambiguous` |
| `field` | The missing configuration key. Values are never included. |
| `profileID` | The profile to fix it in: the agent's explicit profile if it has one, otherwise its wildcard profile. For `oauth_invalid`, the profile that holds the grant. |
| `urlProfileID` | For `oauth_invalid` because the grant was authorized for a different URL, the profile whose configuration sets the URL the agent reaches |

A `selection_ambiguous` reason without a `componentID` means the agent has no explicit profile and the vMCP has more than one wildcard profile. With a `componentID`, the agent's explicit profile is missing something only the ambiguous fallback could supply.

When an upstream fails after an MCP response has started, the agent gets a JSON-RPC error without these codes.

### 400 when authorizing OAuth without a URL {#400-missing-keys}

When a component's URL comes from profile configuration that isn't set, `POST …/components/{component_id}/oauth` returns `400` with the missing keys:

```json
{
  "error": "the upstream URL of component \"github\" needs __url, which profile \"agents\" does not configure; configure the profile first",
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "__url", "profileID": "agents"}
  ]
}
```

Other start errors are plain: `400` if the profile isn't agent-capable, the component isn't a remote server, or the URL fails the hostname constraint; `404` for an unknown vMCP, profile or component; and `409` if agent selection is ambiguous or the vMCP is being deleted.

The start sends the server unauthenticated requests: `initialize`, then, if that succeeds, `notifications/initialized` and `tools/list` in the same session, which it then ends with `DELETE`. It starts from the first `401` challenge, and otherwise from the protected resource metadata at the server's well-known location (RFC 9728). It returns `400` with `does not require OAuth` when there is neither, and an error when `tools/list` fails or a request times out. See [How Obot finds the provider](../mcp-gateway/agent-setup.md#oauth-discovery). The callback returns `403` to a different administrator, `409` if the component or its URL changed, and `400` if the attempt expired or was already used. Errors never contain a URL that comes from configuration. They show `<configured URL>` or `<redacted>` instead.

The start response is `{"authURL": "..."}`. For a component with a localhost callback, it also has `"localhostCallback": true`, and `authURL` is the URL to pass to `obot mcp login`.

## Readiness {#readiness}

`GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness`:

```json
{
  "vmcpID": "vmcp1example",
  "ready": false,
  "disabled": false,
  "explicitProfileID": "vp1example",
  "wildcardProfileID": "agents",
  "reasons": [
    {"componentID": "github", "code": "configuration_required", "field": "GITHUB_ORG", "profileID": "vp1example"}
  ],
  "sources": [
    {"kind": "configuration", "componentID": "github", "field": "GITHUB_TOKEN", "profileID": "agents", "fallback": true}
  ]
}
```

Readiness runs the same checks as a connection, when you call it. `sources` names the profile that supplies each value and grant, never the value. `pendingSync: true` means the agent's next connection waits for Obot to apply the latest configuration. `disabled: true` means the agent is disabled and can't connect, whatever its readiness.

`GET /api/agents/{agent_id}/access` lists `problems`:

- `no_model_access_policy`: a key has the LLM scope, but no model access policy matches the agent.
- `no_skill_access_rule`: a key has the Skills scope, but no skill access rule matches the agent.

`GET /api/vmcps/{vmcp_id}/profiles/{profile_id}/status` reports, for each component, the configuration keys that are `set`, `missingRequired` and `optionalUnset`, and for OAuth components the `state` (`authorized`, `missing` or `invalid`), `authorizedBy` (a user ID), `authorizedAt`, `scopes`, and either `target` (the full URL, only when the catalog fixes it) or `targetHostname` (when the URL comes from configuration).

An OAuth component's `required` is `true` when its catalog entry requires static OAuth, or when any profile of the vMCP holds a grant for it, valid or not. Only then do readiness and connections report `oauth_required` for an agent whose profiles hold no grant. A component that challenges only when its tools are listed or called shows `required: false` until a profile authorizes it, and its agents show as ready until then.

## Audit {#audit}

### Activity {#activity-audit}

LLM gateway audit logs, MCP audit logs, token usage and budget usage attribute requests by principal:

| Field | Users | Agents |
|---|---|---|
| `principal` | `{"kind": "user", "id": "42"}` | `{"kind": "agent", "id": "7"}` |
| `userID` | The user's ID | Empty. Budget usage reports use `agent:7`. |
| `agentDisplayName` | — | The agent's current name, kept after deletion |
| `apiKeyID`, `apiKeyName` | The key used | The key used. An unnamed key shows as `oak1-7-12-*****`. |

MCP audit records also carry the vMCP (`vmcpID`), the component (`vmcpComponentID`), the profiles that matched (`profileIDs`), and the profiles that a value or grant fell back to (`fallbackProfileIDs`).

Both audit log APIs accept `actor_kind=user|agent`. To select one agent, use `user_id=agent:<id>` wherever `user_id` works. Records written before agent identities existed have no principal and are read as the user in `userID`. Skill downloads aren't audited.

### Admin change history {#admin-audit}

`GET /api/admin-audit-events` returns changes newest first, as `{"items": [...], "total": N, "limit": 100, "offset": 0}`. Filter with `resourceKind`, `resourceId`, `actor` (a user ID, or `system`) and `action`, each repeatable or comma-separated, and with `since` and `until` (RFC 3339). Page with `limit` (at most 1000) and `offset`. An invalid filter returns `400`.

| `resourceKind` | `action` | `parentID` |
|---|---|---|
| `agent` | `agent.create`, `agent.update`, `agent.disable`, `agent.enable`, `agent.delete` | — |
| `apiKey` | `agent.key.create`, `agent.key.revoke` | The agent |
| `modelAccessPolicy` | `model_access_policy.subjects.update` | — |
| `skillAccessRule` | `skill_access_rule.subjects.update` | — |
| `vmcpProfile` | `vmcp.profile.subjects.update`, `vmcp.profile.configuration.update`, `vmcp.profile.configuration.reset`, `vmcp.profile.oauth.authorize`, `vmcp.profile.oauth.reset` | The vMCP |
| `vmcpProfile` | `vmcp.profile.oauth.invalidate`, `vmcp.profile.oauth.delete`, with actor `system` | The vMCP |

An event's `details` name what changed: fields, configuration keys, subjects, scopes and hostnames. They never contain configuration values, tokens, API key secrets, configured URLs, or the text of a disable reason. Subject events are recorded only when a change adds or removes an agent, **All Agents**, **All Users** or **Everyone**. Changes to users and groups aren't recorded.

Only changes made through the admin API are recorded, plus the `system` events. Changes that arrive through catalog sync and changes made by controllers are not.

Events are kept for the longer of the MCP and LLM audit log retention periods, or forever if either is `0`. They're kept after the agent, key or profile is deleted. To export them, go to Operations → Audit Logs and click **Admin Change Exports**, then create a one-time export or an export schedule. See [Admin Change Exports](../security/audit-data.md#audit-log-export-admin-change-exports). Through the API, create an [audit log export](../security/audit-data.md#audit-log-export-creating-exports) (`POST /api/audit-log-exports` or `/api/scheduled-audit-log-exports`) with `"type": "admin"` and optional `adminFilters`: `resourceKinds`, `resourceIDs`, `actors` and `actions`.

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

Budgets of the model access policies that match an agent apply to it, as they do to users:

- A per-user budget gives each agent its own allowance, counted separately from the user with the same ID.
- A shared budget counts agents and users together.
- **Exempt administrators** never exempts an agent.
- Users' personal daily token limits don't apply to agents. Agents get the *Daily token limit* budget's own limits.

Budget usage reports show agents as `userID: "agent:<id>"`, with a `principal` field.
