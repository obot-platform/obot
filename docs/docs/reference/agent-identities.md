---
title: "Agent identities reference"
description: API routes, key scopes, error bodies, delegated tokens, audit fields, CEL variables and budget behavior for agent identities, and the preconfigured OAuth routes of vMCP components.
---

Reference for [agent identities](../security/agent-identities.md), [setting up vMCPs for agents](../mcp-gateway/agent-setup.md), [agents acting on behalf of users](../security/agents-on-behalf-of-users.md), and the vMCP's [preconfigured OAuth](#preconfigured-oauth), which agents and users share. All examples use made-up IDs and keys.

## API {#api}

Every route below is for administrators. Read-Only Admins and auditors can call the `GET` routes. Agents can call none of them. The routes with which agents act on behalf of users, and with which users approve and manage them, are under [Acting on behalf of users](#on-behalf-of).

| Method and path | Purpose |
|---|---|
| `GET /api/agents` | List agents. `?includeDeleted=true` includes deleted agents. |
| `POST /api/agents` | Create an agent from its [manifest](#agent-manifest): `{"displayName": "...", "description": "...", "profiles": [...], "allowedReturnHosts": [...]}` |
| `GET`, `PUT`, `DELETE /api/agents/{agent_id}` | Read, replace the manifest of, or delete an agent |
| `POST /api/agents/{agent_id}/disable` | Disable an agent, with an optional `{"reason": "..."}` |
| `POST /api/agents/{agent_id}/enable` | Enable an agent |
| `GET`, `POST /api/agents/{agent_id}/api-keys` | List keys (`?show_revoked=true` includes revoked keys), or create one |
| `GET`, `DELETE /api/agents/{agent_id}/api-keys/{key_id}` | Read or revoke a key |
| `GET /api/agents/{agent_id}/access` | The vMCP profiles, model access policies and skill access rules that name the agent, and `problems` |
| `GET /api/agents/{agent_id}/vmcps` | Readiness of every vMCP the agent is granted |
| `GET /api/agents/{agent_id}/vmcps/{vmcp_id}/readiness` | Readiness of one vMCP, with reasons |
| `GET /api/agents/{agent_id}/delegations` | The users who approved the agent to act on their behalf ([delegations](#delegations)) |
| `DELETE /api/agents/{agent_id}/delegations/{delegation_id}` | Revoke a user's approval, and delete what they set up through the agent |
| `GET`, `POST`, `DELETE /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth` | Read the state of a component's [preconfigured OAuth](#preconfigured-oauth), start its authorization, or revoke it |
| `GET /api/admin-audit-events` | The admin change history |
| `GET /api/admin-api-keys?kind=user` or `?kind=agent` | Every key of one owner kind. The `kind` parameter is required. |

Request bodies reject unknown fields. `PUT /api/agents/{agent_id}` replaces the whole manifest, so send every field: leaving out `profiles` or `allowedReturnHosts` removes them all. Deleting an agent, revoking a key, revoking a delegation and revoking preconfigured OAuth return `204`.

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

## Agent manifest {#agent-manifest}

The agent's create, update and get routes carry its user access (`profiles`) and its allowed return hosts:

```json
{
  "displayName": "Assistant",
  "description": "Answers questions in the help portal",
  "profiles": [
    {
      "id": "ap1example",
      "name": "Support team",
      "subjects": [{"type": "group", "id": "github/1234"}]
    }
  ],
  "allowedReturnHosts": ["agent.example.com", "localhost:3000"]
}
```

| Field | Meaning |
|---|---|
| `profiles[].id` | Assigned by Obot, with the prefix `ap1`, and kept across updates. Send it back to keep an entry. An entry without one is new. |
| `profiles[].name` | Required, and unique within the agent |
| `profiles[].subjects` | The users the agent may act for: `user`, `group` and `obotGroup` subjects, or `{"type": "selector", "id": "*"}` (All Obot Users) alone. Agent subjects and `{"type": "user", "id": "*"}` are refused. |
| `allowedReturnHosts` | Host names or IP addresses, each with an optional port, stored in lower case. No scheme, path, user or wildcard, and no duplicates. |

An entry only says which users the agent may act for. For each of them, the agent has its own grants: the vMCP profiles, model access policies and skill access rules that name it (`GET /api/agents/{agent_id}/access`). Changes are recorded in the admin change history, in the `profiles` detail of `agent.create` and `agent.update` events.

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

An agent that can [act on behalf of users](../security/agents-on-behalf-of-users.md) can also have a user supply `configuration_required` and `oauth_required` through it, by approving the agent. `oauth_invalid` always needs an administrator. See [Act on your own until you need the user](../security/agents-on-behalf-of-users.md#own-key-first).

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

## Acting on behalf of users {#on-behalf-of}

See [Agents acting on behalf of users](../security/agents-on-behalf-of-users.md) for the flow, with a worked example.

### Routes {#on-behalf-of-routes}

| Method and path | Caller | Purpose |
|---|---|---|
| `POST /api/agent-delegations` | The agent's API key | Ask for an approval link. Returns `201`. |
| `POST /api/agent-delegations/token` | The agent's API key | Redeem a code, or mint a token from a delegation |
| `GET /api/agent-delegation-requests/{request_id}` | A signed-in user's browser session | Read a request, with what the agent will be able to do for the user |
| `POST /api/agent-delegation-requests/{request_id}/approve` | A signed-in user's browser session, from Obot's pages | Approve. Returns the URL to send the browser to. |
| `POST /api/agent-delegation-requests/{request_id}/cancel` | A signed-in user's browser session, from Obot's pages | Cancel. Returns the URL to send the browser to. |
| `GET`, `PUT`, `POST`, `DELETE /api/agent-delegation-requests/{request_id}/setup…` | A signed-in user's browser session, from Obot's pages, who may approve the request | The [setup API](#setup-api), while approving |
| `GET /api/me/agent-delegations` | A user | The user's delegations: the agents that act for them |
| `DELETE /api/me/agent-delegations/{delegation_id}` | The delegation's user | Revoke the agent. Returns `204`. |
| `GET`, `PUT`, `POST`, `DELETE /api/me/agent-delegations/{delegation_id}/setup…` | The delegation's user | The [setup API](#setup-api), afterwards |
| `GET /api/agents/{agent_id}/delegations`, `DELETE …/{delegation_id}` | Administrators | List and revoke an agent's delegations. Read-Only Admins and auditors can list them. |

Only the agent's own API key reaches the two agent routes: never a delegated token, and never a user. The approval and setup routes refuse API keys, the bootstrap login, and approval or cancellation from another site's page (`403`). A delegation's routes under `/api/me` answer `403` to anyone but its user. No route reaches the vMCP instance that holds a user's setup through an agent, for anyone, administrators included.

A delegated token is accepted only on `/mcp-connect/{vmcp_id}`, `/api/llm-proxy/…` and the `GET` routes under `/api/skills`. Every other route answers it with `401`.

### Ask for approval {#create-request}

`POST /api/agent-delegations`:

| Field | Meaning |
|---|---|
| `returnURL` | Required. An absolute `https` URL on one of the agent's allowed return hosts, or `http` to `localhost` or a loopback address on the list. At most 2048 characters, with no user information and no fragment. |
| `secretHash` | Required. The unpadded base64url SHA-256 digest (43 characters) of a secret of 43 to 128 characters from `A-Z`, `a-z`, `0-9`, `-`, `.`, `_` and `~`. |
| `loginHint` | Optional, at most 256 characters. Added to `approvalURL` as `login_hint`, to pre-fill sign-in. It never decides who can approve. |
| `delegationID` | Optional. A delegation of the agent to approve again. Only its user can approve the request. |

The response is `{"requestID": "adr1…", "approvalURL": "https://<obot-host>/agents/approve/adr1…", "expiresAt": "…"}`. A request expires after 10 minutes.

### Approve or cancel {#approve}

`GET /api/agent-delegation-requests/{request_id}` returns the request as the signed-in user would approve it:

```json
{
  "id": "adr1example",
  "agent": {"id": "7", "displayName": "Assistant", "status": "active"},
  "expiresAt": "2026-11-24T10:10:00Z",
  "delegationID": "adl1example",
  "access": {
    "vmcps": [{"vmcpID": "vmcp1example", "vmcpDisplayName": "Notes", "profiles": [{"id": "vp1readonly", "name": "read-only"}]}],
    "modelAccessPolicies": [{"id": "map1example", "displayName": "Support models"}],
    "skillAccessRules": []
  }
}
```

`delegationID` is set when the user approved the agent before. `access` is the agent's current grants, which are the same for every user its user access matches.

`POST …/approve` and `POST …/cancel` return `{"redirectURL": "..."}`: the return URL with `requestID` and `code` on approval, or with `requestID` and `error=access_denied` on cancellation. These parameters replace any of the same names in the return URL. Approval creates the user's delegation for the agent, or updates it and keeps its ID, whether or not the agent redeems the code. A code expires after 2 minutes.

### Get tokens {#token}

`POST /api/agent-delegations/token`, with the agent's key, takes one of two grants:

| `grantType` | Fields | Use |
|---|---|---|
| `code` | `requestID`, `code`, `secret` | Redeem the code of an approved request, with the secret from the session of the browser that came back. A wrong or missing secret uses the code up. |
| `delegation` | `delegationID` | Mint a new token from a delegation of the agent |

Both return:

```json
{
  "delegationID": "adl1example",
  "accessToken": "eyJhbGciOiJFZERTQSJ9.EXAMPLE.EXAMPLE",
  "tokenType": "Bearer",
  "expiresAt": "2026-11-24T10:15:00Z",
  "user": {"id": "42", "email": "alice@example.com", "displayName": "Alice", "status": "active"}
}
```

The calling key is the token's minting key. There's no refresh token.

### Delegated token claims {#token-claims}

A delegated token is a JWT signed by Obot. Treat it as opaque. Its claims are:

| Claim | Value |
|---|---|
| `sub` | The user's ID, such as `"42"` |
| `obot_principal_kind` | `"user"` |
| `act` | `{"sub": "agent:7"}`: the agent, as the RFC 8693 actor |
| `obot_delegation_id` | The delegation, such as `"adl1example"` |
| `api_key_id` | The ID of the agent's key that minted it |
| `aud`, `iat`, `exp` | Obot's URL, and at most 15 minutes of validity |

The token carries no name, email or group. Because its subject is the user, Obot refuses any token with `act` or `obot_delegation_id` on every route except the three that accept delegated tokens. On those, every request checks that the delegation exists, the agent and the user are active, a user access entry still matches the user with their current groups, and the minting key is the agent's and neither revoked nor expired. The key's scopes apply.

### Delegations {#delegations}

`GET /api/me/agent-delegations` and `GET /api/agents/{agent_id}/delegations` return:

```json
{
  "items": [
    {
      "id": "adl1example",
      "agent": {"id": "7", "displayName": "Assistant", "status": "active"},
      "user": {"id": "42", "email": "alice@example.com", "displayName": "Alice", "status": "active"},
      "firstApprovedAt": "2026-11-24T10:00:00Z",
      "lastApprovedAt": "2026-11-24T10:00:00Z",
      "lastUsedAt": "2026-11-24T10:05:00Z"
    }
  ]
}
```

`lastUsedAt` is when the agent last got a token from the delegation. Revoking deletes the delegation, its pending requests, and the user's setup through the agent.

### Setup API {#setup-api}

The approval page and **Agents acting for you** use the same routes, under `/api/agent-delegation-requests/{request_id}` while the user approves, and under `/api/me/agent-delegations/{delegation_id}` afterwards:

| Method and path | Purpose |
|---|---|
| `GET …/setup` | The setup of every vMCP that the agent is granted |
| `PUT …/setup/vmcps/{vmcp_id}/configuration` | Set or clear values: `{"components": {"<componentID>": {"<key>": "value", "<other key>": null}}}`. `null` clears a value, and keys left out are kept. Returns the vMCP's setup. |
| `POST …/setup/vmcps/{vmcp_id}/components/{component_id}/oauth` | Start connecting an upstream account. Returns `{"authURL": "..."}`, with `"localhostCallback": true` for a provider that only redirects to localhost. |
| `DELETE …/setup/vmcps/{vmcp_id}/components/{component_id}/oauth` | Disconnect the upstream account. Returns the vMCP's setup. |

```json
{
  "vmcps": [
    {
      "vmcpID": "vmcp1example",
      "displayName": "Notes",
      "ready": false,
      "components": [
        {
          "componentID": "notes",
          "name": "notes",
          "covered": false,
          "ready": false,
          "reasons": [{"componentID": "notes", "code": "configuration_required", "field": "X-TOKEN"}],
          "configuration": [{"key": "X-TOKEN", "name": "Token", "description": "", "sensitive": true, "required": true, "usage": "header", "set": false}],
          "oauth": "notRequired"
        }
      ]
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `covered`, `coveredBy` | The component needs nothing from the user: `fixed` (the vMCP or catalog entry fixes it), `preconfiguredOAuth` (the vMCP's [preconfigured OAuth](#preconfigured-oauth) covers it) or `nothing` |
| `ready`, `reasons` | Whether anything keeps the agent from the component for the user, with reasons as in the [409 bodies](#409) |
| `configuration[].set` | Whether the user set the value through the agent. Values are never returned. |
| `oauth` | `notRequired`, `preconfigured`, `connected` or `needsAuthorization` |

Starting OAuth for a component that the vMCP's preconfigured OAuth covers returns `409`. The provider's callback is accepted only in the browser session of the user who started it, and returns the browser to the page that started it.

### Errors {#on-behalf-of-errors}

| Route | Condition | Response |
|---|---|---|
| `POST /api/agent-delegations` | Invalid `returnURL`, a host that isn't allowed, no allowed return hosts, an invalid `secretHash` or `loginHint`, or a `delegationID` that isn't the agent's | `400` |
| `POST /api/agent-delegations` | The agent has 100 pending requests | `429` |
| Both agent routes | The agent is disabled | `403` |
| `POST /api/agent-delegations/token` | An unknown, unapproved, cancelled, used or expired code, a code that isn't the request's, a wrong or missing secret, or a delegation revoked before the code was redeemed | `400` |
| `POST /api/agent-delegations/token` | A `delegation` grant for a delegation that was revoked or isn't the agent's | `404` |
| `POST /api/agent-delegations/token` | The user is disabled or deleted, or no user access entry matches them any more | `403` |
| Approval routes | The request doesn't exist | `404` |
| Approval routes | The request expired or was used, its return host is no longer allowed, no user access entry matches the user, it approves another user's delegation again, or the agent is disabled or deleted | `403`, with a message for the user |
| Delegated routes | The token is expired or invalid, its delegation was revoked, the agent or the user is disabled or deleted, no entry matches the user any more, or the minting key is revoked or expired | `401` with `agent_delegation_invalid`, as `{"error": "agent_delegation_invalid"}` or, on the LLM gateway, as the plain error text |
| Delegated routes | The agent isn't granted the vMCP or the model, or the minting key's scopes don't cover it | `403`. Skills the agent isn't granted aren't listed. |
| `/mcp-connect/{vmcp_id}` | A component needs a value or an upstream account that the user hasn't set up through the agent | `409` with `agent_user_approval_required` (below) |

```json
{
  "error": "agent_user_approval_required",
  "vmcpID": "vmcp1example",
  "reasons": [
    {"componentID": "notes", "code": "configuration_required", "field": "X-TOKEN"},
    {"componentID": "jira", "code": "oauth_required"}
  ]
}
```

During a session, a tool call whose upstream refuses the user's account, or whose account the provider refuses to refresh, gets an MCP tool error (`isError: true`) whose content and structured content are this body. An account the provider refuses to refresh is removed, so the user's setup shows it as needing authorization. Preconfigured OAuth that no longer works is reported as `409 agent_oauth_required` with `oauth_invalid` reasons, as for the agent acting for itself, since only an administrator can fix it. No `WWW-Authenticate` challenge is sent.

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

When an agent acts on behalf of a user, the agent stays the principal, and the user is in `onBehalfOf`, as `{"kind": "user", "id": "42"}`: on LLM audit logs, token usage, and MCP tool call statistics, and as `actor.onBehalfOf` on MCP audit events. `userID` stays empty, as on every agent record, so the agent's work never counts toward the user's own usage, budgets or daily limits.

A filter by user (`user_id`, and `actor` on MCP audit logs) matches only the user's own records. Add `include_on_behalf_of=true` to also match what agents did on the user's behalf. This works on `GET /api/mcp-audit-logs`, the MCP usage statistics, `GET /api/llm-audit-logs`, `GET /api/token-usage-stats` and `GET /api/users/{user_id}/token-usage`. Audit log exports take `includeOnBehalfOf` in `filters` and `llmFilters`.

To select what agents did for a user, rather than what the user did, filter with `on_behalf_of`: one or more user IDs, repeated or comma-separated. It's a filter of its own that every other filter narrows, so `user_id=agent:7&on_behalf_of=42` returns only what agent 7 did on behalf of user 42. On its own, `on_behalf_of=42` returns what any agent did for user 42, and none of the user's own records. It works on the same endpoints, and in the MCP usage statistics it narrows each server's tool calls too. `GET /api/mcp-audit-logs/filter-options/on_behalf_of` and `GET /api/llm-audit-logs/filter-options/on_behalf_of` list the users agents acted for. Audit log exports take `onBehalfOf` in `filters` and `llmFilters`.

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
| `agentDelegationRequest` | `agent.delegation_request.create` and `agent.delegation_request.redeem`, with the agent (`agent:<id>`) as actor. A refused redemption has `details.refusal`, such as `secret_mismatch`. | The agent |
| `agentDelegation` | `agent.delegation.approve`, with the user as actor, and `agent.delegation.revoke`, with the user or the administrator as actor | The agent |

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
| `principal.on_behalf_of` | `""` | The ID of the user the agent acts for, such as `"42"`, or `""` when it acts for itself |

`principal.id` alone doesn't identify a caller, because a user and an agent can have the same ID. Check `principal.kind` too, for example `principal.kind == "agent" && principal.id == "7"`. Both are empty for callers that aren't users or agents.

## Budgets {#budgets}

Budgets of the model access policies that name an agent apply to it, as they do to users:

- A per-user budget gives each agent its own allowance, counted separately from the user with the same ID.
- A shared budget counts agents and users together.
- **Exempt administrators** never exempts an agent.
- Users' personal daily token limits don't apply to agents. Agents get the *Daily token limit* budget's own limits.

Budget usage reports show agents as `userID: "agent:<id>"`, with a `principal` field.

When an agent acts on behalf of a user, the budgets of the agent's model access policies apply, and count against the agent, for all the users it acts for together. The user's own budgets and daily token limits don't apply, and aren't consumed.
