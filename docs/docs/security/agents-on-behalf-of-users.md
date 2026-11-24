---
title: "Agents acting on behalf of users"
description: Let an agent identity act for the people who use it, with each user's approval and setup, for the users that administrators allow.
---

An [agent identity](./agent-identities.md) can act on behalf of the people who use it, such as a chat assistant or an internal tool. Each user approves the agent in Obot, in their own browser, and sets up the upstream values and accounts the agent uses for them. An administrator decides which users the agent can act for. For each of them, the agent has its own grants. The agent stays the caller in every check and audit record, and the user is recorded as the user it acts for.

## Two modes {#modes}

An agent acts in one of two modes, decided by the credential it presents:

| | Service account | On behalf of a user |
|---|---|---|
| Credential | The agent's `oak1-` API key | A token that the agent mints from a user's approval |
| What it can use | Everything that policies naming the agent grant, within the key's scopes | The same, for a user whom the agent's [user access](#user-access) matches, within the scopes of the key that minted the token |
| Configuration and upstream OAuth | Only values fixed on the vMCP, and the vMCP's preconfigured OAuth | Values fixed on the vMCP and the vMCP's preconfigured OAuth when they exist, and otherwise what the user set up through the agent |
| When setup is missing | `409 agent_configuration_required` or `agent_oauth_required` | `409 agent_user_approval_required`, so the agent asks the user to approve it again |
| Audit | The agent | The agent, on behalf of the user |

The service-account mode is unchanged. A key never acts for a user, so its `409` is the one described in [Set up vMCPs for agents](../mcp-gateway/agent-setup.md#configuration), even when a user could supply the missing value through the agent. Its reasons tell the agent when a user could, so the agent can [ask for approval only then](#own-key-first).

## Prepare the agent {#prepare}

An administrator does this once for each agent:

1. [Create the agent and a key](./agent-identities.md#create-an-agent). Give the key the scopes the agent needs for users: the vMCPs, the LLM gateway, Skills, or a combination.
2. [Grant the agent](./agent-identities.md#grant-access) what it may use, for itself and for every user it acts for: name it in vMCP profiles, model access policies and skill access rules.
3. On the agent's page, open the **User access** tab and add [user access](#user-access) entries for the users the agent may act for.
4. On the same tab, add the agent's [allowed return hosts](#return-hosts).

The **Delegations** tab lists the users who approved the agent. See [Revocation and lifecycle](#revocation). Read-Only Admins and auditors can see both tabs, but can't change them.

## User access {#user-access}

User access says which users an agent can act for. Each entry has a name and its users and groups: users, auth provider groups, Obot groups, or **All Obot Users**, which must then be the only subject. An agent is never a subject.

- A user that no entry matches can't approve the agent, and a token for them stops working once no entry matches them. Group changes apply at once.
- For a user that an entry matches, the agent has exactly its own grants: the vMCP profiles, model access policies and skill access rules that name it. Their tools, models, budgets and guardrails apply as they do when the agent acts for itself, and change as those grants change.

What the agent can do for a user is limited, on every request, to the agent's own grants and the scopes of the key that minted the token. It doesn't depend on what the user can reach directly. An administrator can let an agent use a vMCP for users who can't use that vMCP themselves. To let an agent do less for some users than for others, use a separate agent for them, with its own grants.

In the API, user access is the `profiles` field of the agent. See the [reference](../reference/agent-identities.md#agent-manifest).

## Allowed return hosts {#return-hosts}

After a user approves or cancels, Obot sends their browser back to the agent's return URL. An administrator lists the hosts the agent may use, under **Allowed return hosts** on the **User access** tab.

- Each entry is a host name or IP address, with an optional port, such as `agent.example.com` or `localhost:3000`. Wildcards, schemes and paths aren't allowed.
- A return URL must use `https`. Plain `http` is allowed only to `localhost` or a loopback address on the list, for development.
- An entry without a port allows only the default port of the URL's scheme.
- An agent with no allowed return hosts can't ask users for approval.
- The return URL is checked when the agent asks, and again when the user approves. Removing a host stops pending requests that use it.

Without this list, anyone holding the agent's key could use Obot to send a signed-in user's browser anywhere.

## The approval flow {#approval-flow}

The agent needs its API key and nothing else: no OAuth client ID or secret. It does need a web front end with sessions, because the approval is bound to the browser session in which the user uses the agent. The agent doesn't need to know who the user is before it asks. The user signs in to Obot only when the agent first needs to act for them.

1. The agent's backend creates a random secret and keeps it in the user's session. It asks Obot for an approval link with the secret's hash.
2. The agent sends the user's browser to the link. The user signs in to Obot, reads what the agent will be able to do for them, sets up what it needs, and approves.
3. Obot sends the browser back to the agent's return URL with a one-time code.
4. The agent's backend redeems the code with its key and the secret from the session of the browser that came back. It gets a delegation ID and a token.
5. The agent uses the token to act for the user. When the token expires, the agent mints a new one with its key and the delegation ID.

### Act on your own until you need the user {#own-key-first}

An agent doesn't need anyone's approval for what its own key can already do. We recommend that it act on its own, and ask a user to approve it only when a vMCP needs something that the user can supply:

1. Connect to the vMCP with the agent's own key.
2. If the connection is refused with `409 agent_configuration_required` or `409 agent_oauth_required`, read the `reasons` in the body.
3. If a reason is `oauth_invalid`, don't ask the user. The component's [preconfigured OAuth](../mcp-gateway/server-types.md#preconfigured-oauth) no longer works, and it's used in place of any account a user connects, so only an administrator can fix it by authorizing it again.
4. If the reasons are `configuration_required` (a value that users enter) or `oauth_required` (an upstream account, for a component with no preconfigured OAuth), a user can supply them through the agent. [Ask the user to approve the agent](#example), then connect on their behalf with the token.

The body of the `409` to the agent's key is:

```json
{
  "error": "agent_configuration_required",
  "reasons": [
    {"componentID": "notes", "code": "configuration_required", "field": "X-TOKEN"},
    {"componentID": "jira", "code": "oauth_required"}
  ]
}
```

`error` is `agent_oauth_required` when every reason is `oauth_required` or `oauth_invalid`, and `agent_configuration_required` otherwise. `field` names the missing configuration key, never its value. See [409 bodies](../reference/agent-identities.md#409).

Acting on behalf of a user, the agent gets `409 agent_user_approval_required` instead, with the same reasons, when the user hasn't set up what's missing. It then asks the same user to [approve it again](#example) with its delegation ID. An `oauth_invalid` reason still comes as `409 agent_oauth_required`, because approval can't fix it.

### Example for agent developers {#example}

All examples use made-up IDs and keys. `$OBOT_AGENT_KEY` is the agent's key. Keep it on the agent's backend: the browser never needs it.

**1. Create the secret and keep it in the session.** The secret is 43 to 128 of the characters `A-Z`, `a-z`, `0-9`, `-`, `.`, `_` and `~`, like a PKCE code verifier (RFC 7636). Its hash is the unpadded base64url SHA-256 digest. Keep the secret in a server-side session store, never in the URL.

```python
import base64, hashlib, secrets

secret = secrets.token_urlsafe(48)  # 64 characters
secret_hash = base64.urlsafe_b64encode(hashlib.sha256(secret.encode()).digest()).rstrip(b"=").decode()
session["obot_secret"] = secret
```

**2. Ask for an approval link.** Call `POST /api/agent-delegations` with the agent's key:

```bash
curl -X POST https://obot.example.com/api/agent-delegations \
  -H "Authorization: Bearer $OBOT_AGENT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"returnURL": "https://agent.example.com/obot/callback", "secretHash": "Xd0kRh5r0B6l0dKq2w2t8m0mVb9y0EXAMPLEhash012", "loginHint": "alice@example.com"}'
```

```json
{
  "requestID": "adr1example",
  "approvalURL": "https://obot.example.com/agents/approve/adr1example?login_hint=alice%40example.com",
  "expiresAt": "2026-11-24T10:10:00Z"
}
```

- `returnURL` must be on the agent's [allowed return hosts](#return-hosts). It may carry query parameters of its own, and no fragment.
- `loginHint` is optional. It pre-fills the Obot sign-in, as a convenience only: it never decides who can approve.
- To have a user approve the agent again, for example after a [`409 agent_user_approval_required`](../reference/agent-identities.md#on-behalf-of-errors), add `"delegationID"` with the delegation the agent holds. Then only that delegation's user can approve the request.

Store `requestID` in the session if you want to check that the browser comes back with the same one.

**3. Send the user to `approvalURL`.** Redirect the browser to it. The link expires after 10 minutes, and works for one approval.

**4. Handle the return, and redeem the code with the secret.** After approval, Obot sends the browser to:

```text
https://agent.example.com/obot/callback?requestID=adr1example&code=EXAMPLEcode
```

If the user cancels, the browser comes back with `requestID` and `error=access_denied` instead. On approval, take the secret from the session of the browser that came back, and redeem the code within 2 minutes:

```bash
curl -X POST https://obot.example.com/api/agent-delegations/token \
  -H "Authorization: Bearer $OBOT_AGENT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"grantType": "code", "requestID": "adr1example", "code": "EXAMPLEcode", "secret": "<the secret from this session>"}'
```

```json
{
  "delegationID": "adl1example",
  "accessToken": "eyJhbGciOiJFZERTQSJ9.EXAMPLE.EXAMPLE",
  "tokenType": "Bearer",
  "expiresAt": "2026-11-24T10:15:00Z",
  "user": {"id": "42", "email": "alice@example.com", "displayName": "Alice", "status": "active"}
}
```

A code works once. A wrong or missing secret uses it up, and the user must approve again. Store `delegationID` with the user's account in your app. Remove the secret from the session.

**5. Act for the user, and mint new tokens.** Send the token as a bearer token, as you would the key:

- **MCP**: `https://obot.example.com/mcp-connect/<vmcp-id>`, with `Authorization: Bearer <accessToken>`.
- **LLM gateway**: the base URL `https://obot.example.com/api/llm-proxy/v1`.
- **Skills**: the `GET` routes under `/api/skills`.

The token works on nothing else. A token lasts 15 minutes, and there are no refresh tokens. Before it expires, mint a new one from the delegation, with any active key of the agent:

```bash
curl -X POST https://obot.example.com/api/agent-delegations/token \
  -H "Authorization: Bearer $OBOT_AGENT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"grantType": "delegation", "delegationID": "adl1example"}'
```

The response has the same shape as in step 4. The key you mint with is the token's minting key: the token gets that key's scopes, and stops working if the key is revoked.

**6. Handle the errors.** See the [reference](../reference/agent-identities.md#on-behalf-of-errors) for every status.

| Error | Meaning | What to do |
|---|---|---|
| `401` with `agent_delegation_invalid` | The token no longer acts for the user: it expired, its delegation was revoked, the agent or the user was disabled or deleted, no user access entry matches the user any more, or the minting key was revoked | Mint a new token from the delegation. If that's refused too, ask the user to approve the agent again. |
| `409` with `agent_user_approval_required` | A vMCP needs something only the user can set up through the agent: a value, or an upstream account. The body names the vMCP. | Ask for a new approval link with the `delegationID` (step 2), and send the user to it. They see the consent again and fill in only what's missing. |
| A tool error whose content has `"error": "agent_user_approval_required"` | The same, found during a tool call, for example because the user revoked their upstream account at the provider | The same. The MCP session stays open. |
| `404` from minting a token | The delegation was revoked, or isn't this agent's | Ask the user to approve the agent again, without `delegationID`. |

The `409` doesn't carry a link, because only your backend can attach the session's secret.

### Why the approval is tied to the browser session {#forwarded-links}

Say someone who uses the agent forwards their approval link to a victim, and the victim approves it. The victim's browser returns to the agent with the victim's own session, which doesn't hold the forwarder's secret. The agent can't redeem the code, and the wrong secret uses the code up, so the agent never gets a token for the victim in the forwarder's session. Obot enforces this through the secret check, not the agent.

This is why the user must approve in the same browser where they use the agent, and why the agent must take the secret from the session of the browser that came back, never from the request ID. Binding the approval to the session also means the agent doesn't need to know who the user is before it asks.

## What users see {#users}

**The approval page.** A user who follows the link signs in to Obot, if they aren't signed in, and comes back to the page. Every approval shows the consent again:

- the agent, and the account the user is signed in as;
- every vMCP, with its profiles that name the agent, every LLM gateway policy, and every skill access rule that the agent will be able to use for them: the agent's own grants;
- a **Set up** section for each vMCP: the values to enter and the upstream accounts to connect. Values only show as set or not set, never their contents. Components that need nothing from the user are listed as ready.

The user can approve with setup missing, after a confirmation. The agent then gets `agent_user_approval_required` when it needs the missing setup. A user who approved the agent before sees that approving again updates their approval. **Cancel** sends the browser back to the agent with `error=access_denied`.

**Agents acting for you.** Users open **Agents acting for you** from their profile menu. It lists the agents that can act for them, with when they approved each and when it was last used. Opening an agent shows what the user set up through it, and lets them change or clear values, connect or disconnect upstream accounts, and **Revoke** the agent. A value they clear is missing again, so the agent gets `agent_user_approval_required` the next time it needs it.

**Setup through an agent is separate.** What a user enters or connects through an agent is used only by that agent acting for them. It's kept apart from the user's own connection to the same vMCP, and from the agent's own setup. A user who uses two agents sets up a vMCP for each.

For each component of a vMCP, a value fixed on the vMCP, and the vMCP's [preconfigured OAuth](../mcp-gateway/server-types.md#preconfigured-oauth), are used when they exist. Otherwise the user's setup through the agent is used. A component needs the user when it takes a value users enter, or when it's a remote server for which the vMCP holds no preconfigured OAuth. Administrators can't see or edit what users set up through an agent.

Preconfigured OAuth that stops working is reported as `409 agent_oauth_required`, as it is to the agent acting for itself, because only an administrator can authorize it again. The user isn't asked to connect their own account for that component either.

## Revocation and lifecycle {#revocation}

- **A user revokes** the agent under **Agents acting for you**.
- **An administrator revokes** a user's approval on the agent's **Delegations** tab. The tab shows each user, when they first and last approved, and when the agent last got a token.

Revoking deletes the approval and everything the user set up through that agent. The agent's tokens are refused on their next request: at once on the replica that handled the revocation, and within 15 seconds on the others, which cache delegation checks as they cache key checks.

| Event | Effect on the agent acting for users |
|---|---|
| A key is revoked or expires | Tokens minted with it stop working. Other active keys of the agent can still mint tokens. Users don't approve again. |
| The agent or the user is disabled | Tokens are refused. Nothing is deleted, and enabling restores them. |
| The agent or the user is deleted | Their approvals and the setup done through the agent are deleted. |
| A vMCP is deleted | The setup every user did for it through agents is deleted. |
| No user access entry matches the user any more | Tokens are refused, and minting is refused. The approval and setup stay, unused, until revoked. |
| A vMCP no longer names the agent, or an administrator grants the agent a vMCP or adds a component that needs user setup | Setup that no longer applies stays, unused. Missing setup returns `409 agent_user_approval_required`. |

Setup that a user does while approving a request they never approve stays until the agent, the user or the vMCP is deleted. The user sees it if they approve the agent later.

**Readiness** on the agent's page is about the agent acting for itself. It marks what a user supplies through the agent, values that users enter and upstream accounts for components without preconfigured OAuth, as **OBO**, and a vMCP that lacks only those as **OBO** instead of **Not ready**. There's no readiness per user.

## Guardrails, budgets and identity {#policy}

**Guardrails add up.** When an agent acts for a user, the guardrails for the agent and those for the user both apply, so going through an agent never avoids a user's guardrails:

- An [MCP guardrail filter](../functionality/filters.md#guardrail-filters) applies when it names the agent, or covers the user by ID, group, Obot group or everyone.
- LLM guardrails apply from the agent's model access policies, and also from the model access policies that cover the user. The user's policies add only guardrails: their models and budgets don't apply.
- Guardrail conditions can read the user as `principal.on_behalf_of`. See [CEL variables](../reference/agent-identities.md#cel).

**Budgets count against the agent.** The budgets of the agent's model access policies apply, and count against the agent, for all the users it acts for together. A user's own budgets and daily token limits don't apply, and the agent's work never counts against them.

**Identity passed to MCP servers.** An MCP server that selects the user [attribution headers](../mcp-gateway/server-configuration.md#caller-attribution-headers) gets the agent in `X-Obot-User-Id`, `X-Obot-User-Name` and `X-Obot-User-Email`, and the user's ID, display name and email in their on-behalf-of counterparts: `X-Obot-On-Behalf-Of-User-Id`, `X-Obot-On-Behalf-Of-User-Name` and `X-Obot-On-Behalf-Of-User-Email`. Each counterpart is sent with the header it pairs with, and can't be selected on its own. Clients can't send these headers: Obot removes them from requests and sets them from authentication. Through a composite vMCP, the user is passed on to every component.

**Webhooks.** Filter webhook payloads carry only the MCP message, with no caller identity, so nothing about the user is added to them.

**Audit.** MCP and LLM audit records, and token usage, keep the agent as the actor, and record the user as `onBehalfOf`. The audit and usage views show such a record as the agent for the user, such as **Assistant for Alice**. When you filter by a user, the agent's actions for that user are left out unless you turn on **Include actions on behalf of this user**, or pass `include_on_behalf_of=true` in the API. To see only what agents did for a user, use the **On behalf of** filter next to the user filter, or pass `on_behalf_of` in the API. With an agent selected as well, it shows what that agent did for that user, and nothing else. Requests for approval, redemptions, approvals and revocations are recorded in the [admin change history](../reference/agent-identities.md#admin-audit).

## With authentication off {#authentication-off}

Everything works the same way when authentication is disabled, for the single owner user.

## Limitations {#limitations}

- **Same browser.** The user must approve in the same browser where they use the agent, and the agent needs a web front end with sessions.
- **No headless agents yet.** An agent with no browser-capable user can only act as itself. There's no device flow or other approval without a browser.
- **Not the MCP authorization flow.** An agent built on an MCP SDK gets its token from Obot's endpoints, then sends it as a bearer header. It can't use the SDK's OAuth support for this.
- **Short lifetimes.** Approval links expire after 10 minutes and work once. Codes expire after 2 minutes and work once. Tokens last 15 minutes, with no refresh tokens.
- **At most 100 pending requests per agent.** Further requests get `429` until some are approved, cancelled or expire.
- **Skills on behalf of users** cover discovering and downloading skills only.
- **Revocation across replicas** takes up to 15 seconds.
- **Setup is per agent.** A user sets up a vMCP separately for each agent, and apart from their own connection.
- **No CLI.** User access and approvals are managed in the UI or through the API.
