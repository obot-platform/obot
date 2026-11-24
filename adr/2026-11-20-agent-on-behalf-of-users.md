# 2026-11-20: Agents acting on behalf of users

- **Status:** Proposed (becomes Accepted when the implementation merges)
- **Date:** 2026-11-20
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#75](https://github.com/obot-platform/field-issues/issues/75): agent identities. Delegation was left as a follow-up there. This ADR is that follow-up.

## Related ODPs

- [2026-11-09: Agent identities for headless access](https://github.com/obot-platform/obot-design-proposals/pull/25) listed delegation as a non-goal. This work is designed in this ADR instead of a follow-up ODP; the ODP process has since been retired.
- [2026-11-17: Agent identities](2026-11-17-agent-identities.md) is the ADR this builds on.

## Context

An agent identity acts as itself: vMCP profiles that name the agent decide what it
can reach. It can use only configuration and OAuth that an administrator fixed on the
vMCP. A vMCP that needs anything else blocks it (`409 agent_configuration_required`
or `agent_oauth_required`).

Many agents serve people, such as a chat assistant or an internal tool. Each user
should bring their own upstream accounts and values, and the agent should act only for
the users an administrator allows. A user may also reach the same vMCP
directly. Their own setup and the setup they do through an agent must stay separate.

## Decision

### Two modes, decided by the credential

- **Service account.** The agent presents its `oak1-` key. This is unchanged from
  the agent identities ADR: the agent's vMCP profiles, LLM gateway policies and skill
  access rules apply, and missing setup is a 409.
- **On behalf of a user.** The agent presents a delegated token that a user approved
  (below). The agent acts for that user, with its own grants, while one of its
  profiles matches the user.
- **Mode is dynamic in the sense that matters.** For each component, a value or
  grant that an administrator fixed on the vMCP (shared) is used when it exists.
  Otherwise the user's setup through this agent is used.
  - A key-only connection never has a user, so it can't fall back to one. Its 409 is
    the service-account one, unchanged (`agent_configuration_required` or
    `agent_oauth_required`). Only a connection on behalf of a user gets
    `agent_user_approval_required`.
  - The key-only 409 still tells the agent whether a user's approval would help: its
    readiness reasons. `configuration_required` (a value users enter) and
    `oauth_required` (an upstream account, where the vMCP has no preconfigured OAuth
    for the component) are setup a user can supply through the agent, so the agent
    can ask a user to approve it and then act for them. `oauth_invalid` is a broken
    preconfigured account that only an administrator can fix, so the agent doesn't
    ask the user. The docs recommend this pattern: act on your own key until a vMCP
    needs something only the user can supply.

### Agent profiles

The UI calls agent profiles **user access**, while the agent's API field and the code call
them `profiles`. They're stored on the agent object,
as profiles are on a vMCP, and edited through the agent's own create and update
routes.

An agent has profiles, managed by administrators only. A profile says only which users
the agent may act for.

- **Subjects.** Users, groups (auth provider groups and Obot groups) and All Obot
  Users. Agents are never subjects. A user
  that no profile of the agent matches can't approve the agent, and can't be acted
  for.
- **Effective access on behalf of a user** is exactly the agent's own grants: the vMCP
  profiles, LLM gateway policies and skill access rules that name the agent. It's the
  same for every user a profile matches, live, and limited to the scopes of the key
  that minted the token. It doesn't depend on what the user can reach directly: the
  agent may reach things for a user that they can't reach themselves.

### Approval and tokens (no OAuth client)

The agent needs its API key and nothing else: no client ID and no client secret. It
does need a web front end with sessions, because the approval is bound to the user's
browser session with the agent.

The agent doesn't need to know who the user is before it asks. A user starts using
the agent, and signs in to Obot only when the agent first needs to act for them.

1. **The agent asks for an approval link.**
   - Its backend creates a random secret and keeps it in the user's session with the
     agent, as an OAuth client keeps a PKCE verifier.
   - It calls `POST /api/agent-delegations`, authenticated with its `oak1-` key,
     with:
     - `returnURL` (required)
     - `secretHash`: the S256 hash of the secret (required)
     - `loginHint` (optional): a convenience that pre-fills the Obot sign-in. It's
       not a security check.
     - `delegationID` (optional): a delegation of the agent to approve again, such as
       after a `409 agent_user_approval_required`. Only that delegation's user can
       approve the request.
   - Obot returns a delegation request ID, a one-time approval URL and an expiry. An
     approval link expires after 10 minutes.
   - **Return URLs are restricted.** An administrator sets the allowed return URL
     hosts on the agent, and a request with any other host is refused. Without
     this, anyone holding the agent's key could use Obot as an open redirect.
2. **The user opens the link in Obot.**
   - They sign in.
   - Obot checks that a profile of the agent matches them.
   - The consent page shows the agent and everything it will be able to do for them:
     its own grants. **It's shown on every approval.**
   - The user then sets up every vMCP the agent is granted, **all at once**, through
     the setup API (below). It reuses the user configuration form and the user
     component OAuth flow, with the pair as owner, rather than `vmcp_consent.go`.
     Components that the vMCP fully fixes, or that the vMCP's preconfigured OAuth
     covers, are skipped.
   - On a later approval, the page asks for what's missing. It also shows what's
     already set up through the agent, with set or unset status but never the
     values, and lets the user change it: replace or clear a value, or reconnect an
     upstream OAuth account.
3. **The browser returns to the agent with a one-time code.** After approval, Obot
   sends the browser to `returnURL` with the request ID and a short-lived code.
   - The agent's backend takes the secret from the session of the browser that came
     back. It redeems the code with its key, the code and the secret.
   - Approval creates or updates the delegation, so that the user can see and revoke
     it, and the setup they did, even if the agent never redeems the code. Obot checks
     the secret against `secretHash`, and only then returns the delegation ID and the
     first token. A wrong secret uses the code up.
   - **Why this blocks a forwarded link (the attack Arcade calls COAT).** Say someone
     using the agent forwards their approval link, and a victim approves it. The
     victim's browser returns to the agent with the victim's session, which doesn't
     hold the attacker's secret. The code can't be redeemed, so the agent never gets a
     token for the victim in the attacker's session. Obot enforces this, not the
     agent.
   - The user must approve in the same browser where they use the agent. There's no
     polling, because the return delivers the code.
   - Codes are single-use and expire quickly. A redeemed or expired code is refused.
4. **The delegation** records the agent, the user, who approved and when, and the
   key that requested it (for audit only).
   - **Approving again updates the existing delegation** for that agent and user,
     rather than creating a second one. The agent keeps its delegation ID, and
     tokens it already holds keep working.
   - **The delegation belongs to the agent, not to a key.** Any active key of the
     agent can mint tokens from it, within that key's scopes. Rotating keys doesn't
     make users approve again.
5. **The agent gets tokens from the token service.** The first comes from redeeming
   the code. Later ones come from presenting a key and the delegation ID. Each is a
   short-lived JWT with the user as `sub` and the agent in the RFC 8693 actor claim
   (`act.sub: "agent:<id>"`).
   - Tokens last 15 minutes. When one expires, the agent mints a new one with a key
     and the delegation ID. There are no refresh tokens.
   - The token carries the delegation ID and the minting key ID. Revoking the
     delegation ends every token minted from it. Revoking a key ends the tokens that
     key minted.
   - **Fail closed on `act`.** Because `sub` is the user, a check that reads `sub`
     and ignores `act` would treat the token as the user. So the token service
     refuses any token that carries `act`, unless the caller has explicitly opted in
     to on-behalf-of tokens. A missed check then refuses the token rather than
     granting the user's access. Tests cover every authentication and token decode
     path, including composite loopback tokens and the full authenticator chains
     with authentication on and off.
   - The same token works for vMCP connections, the LLM gateway and Skills, within
     the agent's grants and the key's scopes.
6. **Every request re-checks** that the delegation exists, the agent is active, the
   user is active, a profile still matches the user (group changes apply at once),
   and the key is valid. It also applies the key's scopes.
7. **Later changes need a new approval.** Examples: an administrator names the agent
   in a profile of another vMCP, or a vMCP gains a component that needs user setup.
   A connection that needs setup the delegation doesn't have gets
   `409 agent_user_approval_required`.
   - The 409 doesn't carry a link: only the agent can attach its session secret, so
     the agent requests a new link (step 1).
   - The user sees consent again and fills in only what's missing.
   - The same applies during a session. If something breaks then, for example a
     user's upstream token, the tool call returns an MCP tool error with the same
     code, and the agent requests a link the same way.

### Identity and storage

- **The caller stays the agent.** A delegated request authenticates as the agent's
  principal (`UserID = "agent:<id>"`), with a new `OnBehalfOf` holding the user's ID.
  Every existing agent check keeps working unchanged: key scopes, the route allowlist
  and status checks. The JWT's standard `sub`/`act` shape is mapped to this on
  authentication.
- **Audit and usage tables get an `OnBehalfOf` column** next to `UserID`, on MCP
  audit, LLM audit and run token activity. On those rows, `UserID` stays empty, as it
  is for every agent row, so per-user readers, budgets and daily limits never count
  an agent's work against the user. Filters for a user can include rows where they
  are `OnBehalfOf` (`include_on_behalf_of`).
- **Setup through an agent has its own owner.** Instances, credentials and OAuth
  tokens for a user's setup through an agent are owned by the encoded pair
  `agent:<id>/user:<id>`. That keeps them apart from the user's own setup (`<id>`)
  and from the agent's shared setup (`agent:<id>`), with no change to how lookups
  work.
  - It's an owner only. `principal.Parse` refuses it as a caller, as it does
    `vmcp-oauth:` owners.
  - Setup is per agent: a user who uses two agents sets up a vMCP for each.
- **Runtime instances.**
  - The pair's setup is a vMCP instance owned by the pair, with its configuration
    credential and the upstream tokens of its component connections. It's the user
    setup code with the pair as owner. Nobody connects through it: the agent acting
    for the user connects through its own instance, and uses the pair's component
    servers and connections where the user is needed.
  - A component needs the user when it takes a value the user enters, or when it's a
    remote server the vMCP holds no preconfigured OAuth for. A remote server without a
    grant uses the pair's connection even when it isn't known to ask for OAuth, so
    that an account the user connects is used if it asks later.
  - A single-user component that needs user setup gets one instance per agent and
    user pair.
  - A multi-user component whose only user inputs are headers gets the pair's own
    connection to its shared server, carrying the pair's header values, as a user's
    connection carries theirs.
  - A component that needs nothing from the user (the vMCP or the catalog entry fixes
    it, and the vMCP's preconfigured OAuth covers its OAuth) runs on the agent's own
    instance, shared by every user it acts for.
  - The vMCP's preconfigured OAuth, which everyone using the vMCP shares, is used in
    place of the pair's account wherever the component runs.
  - The pair's instance carries every component that may need a user, whatever the
    vMCP profiles and grants are, so that no change to them removes what the user set
    up.
- **Budgets on behalf of users count against the agent's budgets, for all users
  together.** A user's own budgets and daily token limits don't apply.

### Policy and runtime on behalf of a user

- **Guardrails accumulate.** Guardrails that name the agent and guardrails that
  cover the user (by ID, group or All Obot Users) both apply. A user's guardrails
  can't be avoided by going through an agent.
- **No tool narrowing.** The agent gets every tool its own vMCP profiles grant, for
  every user it acts for. The user can't narrow them at approval.
- **Identity passed on.** MCP servers and guardrail filters receive the agent as the
  caller and the user as on-behalf-of.
  - An MCP server that selects the user attribution headers gets the agent in them,
    and the user's ID, name and email in their on-behalf-of counterparts
    (`X-Obot-On-Behalf-Of-User-Id`, `-Name`, `-Email`). Those can't be selected on
    their own, and clients can't send them.
  - Webhook (filter server) payloads carry only the MCP message, with no caller
    identity, so there is nothing to add to them.
- **Composite vMCPs.** The internal tokens between a vMCP and its components carry
  `OnBehalfOf`, so the user is attributed all the way through.
- **Setup that no longer applies stays.** If no profile of a vMCP names the agent any
  more, or no agent profile matches the user any more, the user's setup through the
  agent stays, unused. It's deleted when the user revokes the agent, or when the
  vMCP, agent or user is deleted. This matches the preconfigured OAuth grants of a
  vMCP.
- **A broken upstream token** (for example, one the user revoked at the provider)
  returns `agent_user_approval_required`: a 409 on connect, or an MCP tool error
  during a session. The user approves again and fixes only that component.
  - A tool call whose upstream refuses the pair's account, or whose account the
    provider refuses to refresh (`invalid_grant`), gets the tool error. Any other
    request at that point gets the 409.
  - An account the provider refuses to refresh is removed, so that the user's setup
    shows it as needing authorization again. The composite then starts a new session
    with the component, so the agent's next request gets the 409.
- **Revocation delay.** Delegation checks use the same 15-second cache as key
  checks, so a revocation takes effect across replicas within 15 seconds.
- **Approval link limits.** Pending approval requests expire, and each agent can
  have at most 100 pending at once.

### Revocation and lifecycle

- **Users** see the agents acting for them, under their profile menu ("Agents acting
  for you"). For each agent, they can do the following at any time, without the agent
  asking:
  - see which vMCPs are set up through it, with set or unset status but never the
    values
  - change or clear values
  - reconnect or disconnect upstream OAuth accounts
  - revoke the agent entirely

  This reuses the screens users already have for their own vMCP setup, pointed at the
  setup they own through the agent. Something the user clears is missing again, so
  the agent gets `agent_user_approval_required` the next time it needs it.
- **One setup API serves the approval page and the user's management page.** It's
  under `/api/agent-delegation-requests/{id}/setup…` while the user approves a
  pending request, and under `/api/me/agent-delegations/{id}/setup…` afterwards.
  - The authorizer decides who reaches it: the approver's browser session, from
    Obot's own pages, for a pending request they may approve; or the delegation's
    own user.
  - An upstream account's OAuth callback is accepted only from the user who started
    it, and returns the browser to the page that started it.
  - Setup done during a request that is never approved stays until the agent, the
    user or the vMCP is deleted. There's no delegation to revoke it with, and the user
    sees it if they approve the agent later.
- **Administrators** see each agent's delegations on a tab of the agent's page, with
  the user, the approval time and the last use, and can revoke any of them. They
  can't see or edit a user's values, as with a user's own vMCP setup. Agent
  profiles are edited on another tab of the same page.
  - No route for vMCP instances reaches the pair's instance, for anyone. The
    authorizer refuses it before any rule applies, since administrators' rules
    otherwise grant every instance route.
- **Readiness** on the agent's page stays about the agent's own use. There's no
  readiness per user.
- **Revoking** deletes the delegation and the user's setup through that agent. Tokens
  are refused on their next request.
- **Lifecycle** mirrors the agent identities cleanup rules:
  - Deleting the agent or the user deletes their delegations and setup.
  - Deleting a vMCP deletes the setup of every pair for it, as it does users' own
    setup.
  - Disabling either refuses the tokens without deleting anything.
- **With authentication off**, everything works the same way, for the single owner
  user.

## Rationale

- **API key instead of an OAuth client.** What makes delegation safe is that only
  the agent can redeem the user's approval, and the agent's key already proves that.
  A client registration would add a second credential with its own rotation.
  - The flow keeps the parts of OAuth's authorization code flow that matter:
    - a code returned to the browser, useless without the agent's key and session
      secret
    - PKCE-style binding to the browser session
    - allowed return hosts, instead of exact redirect URIs
  - The cost is that it isn't the standard MCP authorization flow. An agent built on
    an MCP SDK calls Obot's endpoints to get a token, then sends the token as a
    bearer header.
- **Binding to the browser session, not to a named user.** Agents often don't know
  who the user is until the user has signed in to Obot, and users shouldn't sign in
  to Obot more than necessary.
  - Requiring the agent to name the user up front (`loginHint` as a check) would
    rule that out.
  - Binding the approval to the session that asked for it blocks the forwarded-link
    attack without knowing the user. Obot enforces it through the secret check.
  - Arcade solves the same attack with a "user verifier" that the agent's app must
    host, and it still has to know the user. Composio and Pipedream leave it to the
    app.
- **The agent stays the caller (`UserID`), and the user is `OnBehalfOf`.** Every
  restriction built for agents keys on `UserID`. Swapping the two would mean finding
  every one of those places and adding the agent check back. The cost is that a
  "what did user X do" query checks both columns.
- **One token for everything, and all vMCPs at approval.** Approval happens in
  Obot's own page, so it can show the full consent every time and do all setup in
  one visit. This avoids per-resource tokens and repeated round trips.
- **The agent's grants apply, independent of the user's own access.** That's the
  product intent: an administrator decides what the agent may do, and for whom. The
  limit is the agent's own grants and its key.
- **Agent profiles only decide whom the agent acts for.** What the agent can do is
  set where it already is: vMCP profiles, LLM gateway policies and skill access rules
  that name the agent. Letting each agent profile also select from those grants was
  considered. It would add a second place that decides what an agent can do, and
  selections that grant nothing once what they name is deleted. An administrator who
  wants an agent to do less for some users can give those users another agent.

## Consequences

- New gateway storage:
  - agent profiles (`profiles`) and allowed return hosts, as columns on the agent
  - delegation requests and delegations

  Admin change audit covers agent profiles and delegations.
- New and changed APIs:
  - agent profiles and allowed return URL hosts, on the agent's create and update
    routes
  - `POST /api/agent-delegations`, redeeming the returned code, and minting a token
    from a delegation
  - listing and revoking delegations, for users and administrators
  - the setup API, during approval and afterwards
  - the new 409 code
- A new approval and consent page in the UI, which reuses the user configuration form and
  component OAuth steps. Also new: agent profile editors and a "Agents acting for
  you" page.
- Authentication accepts delegated tokens, and checks on every request that an agent
  profile matches the user. The authorizer and the policy lookups for the LLM gateway,
  Skills and vMCPs then use the agent's own grants, as for the agent acting for
  itself.
- Every new per-user or per-agent store must handle the pair owner in cleanup.
- Every path that authenticates or looks up an Obot token must either opt in to
  on-behalf-of tokens or refuse them. New paths inherit the refusal.
- An agent that acts for users needs a web front end with sessions, and the user must
  approve in the same browser where they use the agent.
- Agents with no browser-capable user still use the service-account mode. Device
  flow, CIBA and MCP URL elicitation remain options for later. Without a browser
  session to bind to, such agents would need another defense against forwarded
  links, such as requiring the agent to name the user.
- `obot agent profile …` CLI commands, to match the agent CLI, come after the first
  version.

## References

- [Agent identities ADR](2026-11-17-agent-identities.md)
- [RFC 8693: OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693) (the `act` claim)
- [RFC 8628: OAuth 2.0 Device Authorization Grant](https://www.rfc-editor.org/rfc/rfc8628), which the approval flow resembles
- `pkg/jwt/persistent` (token service), `pkg/api/handlers/agentdelegation.go` and `pkg/api/handlers/agentdelegationsetup.go` (approval, tokens and the setup API), `pkg/api/handlers/mcpgateway/oauth/delegatedsetup.go` (upstream OAuth through an agent), `pkg/principal` (owner encoding)
