# 2026-11-17: Agent identities for headless access

- **Status:** Accepted
- **Date:** 2026-11-17
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#75](https://github.com/obot-platform/field-issues/issues/75): independent agent identity, credentials, auditing, and access to MCP, the LLM Gateway and Skills. Delegation remains a follow-up.

## Related ODPs

- [2026-11-09: Agent identities for headless access](https://github.com/obot-platform/obot-design-proposals/pull/25) (`proposals/2026-11-09-agent-identities/README.md`, reviewed at `7f89902`).
- [2026-09-01: Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md), which this extends with agent subjects and profile-owned configuration and OAuth grants.

The implementation departs materially from the first ODP in the ways listed under [Departures from the ODP](#departures-from-the-odp). The ODP process has since been retired, so this ADR records the departures instead of a follow-up ODP (see the [ADR guidance](README.md)).

## Context

Automation in Obot ran as users, with user-owned API keys. An agent's access, upstream credentials and audit trail were therefore tied to whoever created its key. Policies changed when that person changed roles or left, and vMCP connections that needed configuration or upstream OAuth prompted an interactive user who was never there.

Several things in the codebase already used the word "agent": hosted agent sessions (`v1.AgentInstance`), AI agents on managed devices (`pkg/localagents`, `LocalAgentToolCall*` audit fields), and the Identity & Access "Agents" tab, which listed a user's own API keys. Every owner field in storage held a decimal user ID, and about 125 call sites parsed it as one.

## Decision

### Identity and storage

- **Typed principals with a hybrid owner encoding.** `pkg/principal` gives every caller and owner a kind (`user` or `agent`) and an ID. Owner strings in storage (`spec.userID` on stored objects and `UserID` keys in gateway tables) hold `"<uint>"` for users, unchanged, and `"agent:<id>"` for agents. All reads and writes go through `principal.Parse` and `Principal.String()`. API responses and audit tables expose a typed `{kind, id}` (`types.PrincipalRef`). Authenticators record the kind in `Extra["obot_principal_kind"]` with `principal.RecordKind`, which always overwrites what a provider supplies. `principal.FromUserInfo` requires the marker and rejects a marker that disagrees with the UID.
- **Call-site classification.** Each `GetUID()`, `ParseUint(userID)` and `api.Context.UserID()` site was classified as: (1) needs a user, which rejects agents explicitly, with `api.Context.UserID()` still 0 for non-users; (2) any owner, which stores and compares `Principal.String()`; or (3) policy matching, which moved to `pkg/subjects`. Stored owners are reloaded through `gateway/client.IdentityInfo`. `pkg/principal/guard_test.go` fails on new user-ID parses of owner strings outside the allowlist.
- **Agents are gateway identities.** `gatewaytypes.Agent` (table `agents`) has a display name, a description, an immutable `CreatorUserID`, and active, disabled or deleted states. Rows are only soft-deleted, so agent IDs are never reused and deleted agents still resolve for audit display. Lifecycle changes write an `AgentLifecycleEvent` in the same transaction. The user outbox delivery loop is now generic and delivers both event types as idempotent `v1.AgentIdentityChange` (prefix `aic1`) and `v1.AgentIdentityDelete` (prefix `aid1`) objects. Cluster and API types use the `AgentIdentity` name to avoid clashing with `AgentInstance`.

### Keys and authentication

- **One key table, two owner kinds.** `api_keys.user_id` became a nullable `*uint` in Go (the column was already nullable), with a new nullable `agent_id` and a `CHECK ((user_id IS NULL) <> (agent_id IS NULL))`. The migration runs once through `migrateIfEntryNotFoundInMigrationsTable`. Agent keys use the prefix `oak1-<agentID>-<keyID>-<secret>`. User keys keep `ok1-`. Parsing is driven by the prefix, and the agent validation path filters on `agent_id`, so `oak1-5-7-…` never matches user 5's key 7. The 15-second validation cache still re-reads revocation, expiry and the owner's status on every hit.
- **Key scopes match user key scopes (decision 6).** An omitted scope isn't granted, at least one of MCP, LLM and Skills is required, and an empty `mcpServerIds` means no MCP. Agent keys never carry `CanAccessAPI` or `CanAccessDeviceScans`. Effective access is computed live as the intersection of the key's scopes and the agent's current grants, so grant changes apply to existing keys.
- **Agent route allowlist.** An agent principal is authorized only by `pkg/api/authz/agent.go` and never by the user rules. It reaches `/api/llm-proxy/` with the LLM scope, `GET /api/skills…` with the Skills scope, MCP connections with an MCP scope (and internal composite connections), and the API key webhook. System MCP servers are excluded. A principal that claims to be an agent but carries any other group, such as `api` or a role group, reaches nothing. Any new route that agents need must be added there.
- **Loopback tokens and webhooks.** Internal loopback JWTs carry the encoded owner and a `PrincipalKind` claim. Old tokens without the claim are users. The agent branch checks the agent's current status on every request and refuses to mint for a non-active agent. The API key webhook returns `sub: "agent:<id>"`. MCP OAuth token minting rejects agents.
- **No feature flag (decision 8).** Agent identities are always on. The hidden `EnableAgentIdentities` option existed during development and was removed before release.
- **Agents work with authentication off (decision 9).** With authentication disabled, the single owner manages agents, and an `oak1-` key or agent loopback token authenticates as its agent, with the agent's allowlist and scopes, ahead of the catch-all owner. A presented agent credential either authenticates or is refused. It never falls through to the owner.

### Policy subjects

- **Wildcards on the existing subject shape (decision 5).** `subjects:*` is the existing `{selector,"*"}`, `users:*` is `{user,"*"}`, `agents:*` is `{agent,"*"}`, and an explicit agent is `{agent,"<id>"}` with the bare ID. `pkg/subjects.Evaluate` returns the match strength. Agents match only `{agent,<id>}`, `{agent,*}` and `{selector,*}`, and never groups.
- **Where agents can be named.** Agent subjects are accepted by vMCP profiles, model access policies, skill access rules and MCP guardrail filters. MCP guardrail filters use their own subject type, where everyone is `everyone`. Access control rules and message policies reject agent subjects and `{user,"*"}`, and access control rules never match an agent, even through `{selector,*}`. Existing `{selector,*}` subjects in the three grant systems now also match agents. Wildcard migration is deliberately absent, as the ODP specifies.
- **`{user,"*"}` data check.** `{user,"*"}` validated before this change but matched nobody. It now means all users. `pkg/controller/userwildcard.go` logs a startup warning for every stored `{user,"*"}` subject, and the message-policy migration is unchanged.
- **CEL.** Guardrail conditions (LLM and MCP) keep `user.id` as the storage encoding (`agent:<id>` for agents) and `user.groups` (empty for agents), and add `principal.kind` and `principal.id`.

### vMCP profiles

- **Profile IDs and the profile-data hook.** Profiles have immutable IDs. API-managed vMCPs get server-assigned `vp1<random>` IDs, carried by ID on update. A profile without an ID is new, even with an existing name, and invented IDs are rejected. Catalog YAML may set `id`. Otherwise the ID is `vp1` plus a hash of the vMCP and profile names, so renaming a profile in YAML without an `id` creates a new profile. Profile data stores implement `vmcp.ProfileDataStore` (`ProfilesWithData`, `DeleteProfileData`) and register in `svcs.VMCPProfileData`. The vMCP controller calls `DeleteProfileData` for IDs that left the spec (`status.profileIDs`) and for every ID on vMCP deletion. A profile's ID is recorded in status, at a resource version that still has the profile, before data is written for it. `vmcp.CheckReplacedProfiles` refuses replacing a profile that has data by name.
- **Selection rules (catalog vMCP handling, decision 3).** A vMCP has at most one `{selector,*}` profile, or at most one `{user,*}` and one `{agent,*}` profile, but not both forms. An explicit agent appears in at most one profile. Catalog YAML may use `{agent,*}` and `{selector,*}` but not explicit agent IDs. Stored vMCPs that break the wildcard rule keep working for users until their subjects change or they gain profile data. For agents they fail closed (`selection_ambiguous`), with an `AgentSelectionAmbiguous` condition. `obot mcp validate-catalog` reports the rule.
- **Configuration storage and garbage collection.** Profile configuration lives in credential context `vmcp-profile/<vmcpID>/<profileID>` with keys `vmcp.v1.<componentID>.<key>`. Each save writes a revision named `configuration-<random>`. The name is random rather than a digest, so it reveals nothing about secrets. The current revision and its key names are published on vMCP status (`profileConfigurationRevisions`). Superseded revisions are collected once no agent instance references them, which keeps a permanent instance watch on that vMCP. A reset waits up to 15 seconds for collection, then returns 200 with `pendingCollection: true`. The profile counts as having data until collection finishes. Manifests carry no values, which is why catalog-synced vMCPs can hold agent setup too.
- **Profiles never override fixed values.** A profile can set only `userAllowed` keys. Fixed keys, catalog-static values, Secret bindings and prohibited keys are rejected and apply to agents as to everyone. So the existing per-component sharing decision (`vmcp.IsMultiUser`) still holds for agents.
- **Resolution.** The agent's explicit profile is checked first. For each missing required key, the single wildcard profile that grants agent access is checked next. Optional keys never fall back. With no explicit profile, the wildcard's values apply directly. `SyncAgentProfileConfiguration` writes the resolved values to the agent instance's credential and records `agentConfigurationRevision`. Connect waits for that revision. Admins can't configure, deconfigure or reveal agent-owned instances through `/api/vmcp-instances` (409).
- **Profile OAuth grants.** A grant is a `gatewaytypes.MCPOAuthToken` row with owner `vmcp-profile:<vmcpID>/<profileID>` (`principal.ProfileGrant`, which `principal.Parse` never accepts, so it can never authenticate) and `MCPID = <vmcpID>/<componentID>`. Catalog static OAuth clients remain authoritative for client identity. Pending state binds the initiating admin, the vMCP, the profile, the component, the target URL and the component definition hash. The callback requires the same admin and checks all of them again. A grant is bound to the URL an agent reaches through the grant's profile, resolved with the configuration rules (fixed URL, `__url`, or URL template), and is checked again at readiness and at every component hop. A mismatch is `oauth_invalid`. Localhost-callback components hold grants too: only the one-time authorization goes through the `obot mcp login` relay, within 10 minutes, as the initiating admin. Grant metadata is reported as `status.profileOAuthGrants`. Selection uses the explicit profile's grant if a row exists, otherwise the wildcard's. A broken explicit grant never falls back. Starting an authorization finds the challenge with unauthenticated probes that carry no configured header or token and end their session: `initialize`, then `tools/list` in the same session, then the server's RFC 9728 protected resource metadata, so servers that challenge late can be authorized. Readiness never probes. A component counts as requiring OAuth when it requires static OAuth or any profile of the vMCP holds a grant for it, valid or not, so a late-challenge component is visible to readiness only after its first grant.
- **Refresh lock.** Shared grants are refreshed under a per-token in-process lock. On Postgres, a transaction-scoped advisory lock keyed on owner and MCPID follows. At most two refreshes per process hold a database connection at once (`mcpOAuthRefreshHolders`). A waiter re-reads the row and uses the token the holder saved. `invalid_grant` marks the grant invalid for every agent, and transient failures don't.
- **Readiness shared with the connect path.** The connect path and the readiness API both call `SessionManager.AgentReadiness`, and nothing is cached. Every component enabled for the agent is checked, and any reason blocks the whole connection. Agents get 409 with `agent_selection_ambiguous`, `agent_configuration_required` or `agent_oauth_required` and `reasons`, chosen in that order. They never get `WWW-Authenticate`, a redirect or an MCP OAuth challenge row, including through composite loopbacks. A refused agent key gets a plain 401. Transient deployment and upstream failures keep their 502/503 paths.

### Lifecycle, audit and redaction

- **No owners, only a creator (decision 7).** The ODP's owner list granted nothing and was removed. `CreatorUserID` is set automatically and is immutable. It is 0 for a caller with no user row, such as the bootstrap admin.
- **Cleanup mirrors user cleanup (decision 7a).** Disable removes nothing. Authentication refuses the agent on its next request, including on cache hits and with loopback tokens. Delete revokes keys in the delete transaction. The cleanup controller then removes the agent's vMCP instances, standalone servers and connections, with their credentials and tokens. Subjects stay in policies and match nobody, as a deleted user's do. Profile configuration and grants are untouched, and audit and usage rows are kept.
- **Activity attribution.** LLM audit, MCP audit, token usage and budget usage rows gain `principal_kind` (default `user`) and `principal_id`. Agent rows leave `user_id` empty, except budget usage, which keys counters on `agent:<id>`. There is no backfill: a row without `principal_id` reads as the user in `user_id`. MCP audit rows also record the vMCP, the component, and the contributing and fallback profile IDs. Agents record no daily API activity.
- **Budgets.** Budgets apply per principal, so an agent and the user with the same numeric ID count separately. `ExemptAdmins` never exempts an agent. Users' personal daily token limits, including the migrated *Daily token limit* budget's per-user override, don't apply to agents, which get the budget's own limits.
- **Secret-free admin change audit (decision 4).** `gatewaytypes.AdminAuditEvent` records agent, key, policy-subject, profile-subject, profile configuration and profile OAuth changes made through the admin API, with the acting admin, plus `system` events for grant invalidation and grant removal. Details name keys, subjects, scopes and hostnames, never configuration values, tokens, key secrets, configured URLs or disable-reason text. Retention reuses the API-key rule: the longer of the MCP and LLM audit retentions, or forever if either is 0. `admin` is a new audit log export type.
- **Redaction of configured URLs.** A URL that comes from configuration is treated as a value. Status and readiness report only its hostname (`targetHostname`). Authorize errors replace it with placeholders. The admin audit, Read-Only Admins, auditors and the UI never see it. A full `target` URL is shown only when the catalog fixes it.

## Rationale

- The hybrid encoding left every user row, owner string and user API response byte-for-byte unchanged, yet made a user and an agent with the same numeric ID distinct everywhere owners are stored or compared. A separate owner-kind column on every table, or reusing users with a flag, would have touched far more storage or blurred the two identity models.
- A Gateway table plus the existing outbox kept key and identity lifecycle in one transactional store, which the ODP preferred to a v1 resource.
- A single `api_keys` table with a CHECK constraint reused hashing, one-time disclosure, expiry, revocation and last-used tracking, and the database enforces exactly one owner.
- An allowlist for agents fails closed: a route added for users is not reachable by agents until someone decides it should be.
- The wildcard forms fit the existing `Subject` shape, so stored data needed no migration.
- Matching user key scopes (decision 6) gave one scope model in the API, CLI and UI, and an agent key does nothing its creator didn't ask for. Removing owners (decision 7) dropped metadata that granted nothing but still needed cleanup. Mirroring user cleanup (decision 7a) kept one, already-proven model for what deletion does to policies.
- Rejecting overrides of fixed values kept runtime sharing decisions unchanged and stopped a profile from silently changing what a vMCP's creator fixed for everyone.
- Random revision names, hostname-only URL reporting and the secret-free audit stop any API, log or audit row from becoming a side channel for a profile's secrets.
- Sharing readiness with the connect path means the readiness API can't promise a connection that would then fail.

## Consequences

- New code that reads an owner must go through `pkg/principal`, and new in-process principals used for policy matching must call `principal.RecordKind`. A principal without a kind matches no subject.
- New routes are closed to agents until they are added to `pkg/api/authz/agent.go`.
- Agents can be named in four policy systems and must never be named in access control rules or message policies. A policy system that adds subject matching must decide which side it is on.
- Profile renames are safe in the API. In catalog YAML they lose configuration and grants unless the profile has an explicit `id`.
- Every new store of per-profile data must implement `vmcp.ProfileDataStore` and register in `svcs.VMCPProfileData`, or it leaks data when profiles are deleted.
- Rolling back requires disabling agents and removing agent subjects, profile configuration and grants before downgrading (see [Rollout and rollback](#rollout-and-rollback)). The schema additions stay.
- Mid-stream MCP errors can't yet carry the reason code. That needs a hook in the MCP proxy library.

## Rollout and rollback

There is no flag. Each part is live once it merges, so the stack merges in order. A spec doesn't merge ahead of the specs that make its routes complete. Users' behavior doesn't change. Replicas of mixed versions are safe: older replicas ignore the `obot_principal_kind` claim and the `PrincipalKind` loopback claim, and refuse `agent:` subjects in loopback tokens.

Rollback:

1. Disable every agent (`obot agents disable <id>`), so no agent credential authenticates.
2. Remove `{agent,…}` subjects from model access policies, skill access rules, vMCP profiles and MCP guardrail filters. Replace `{user,"*"}` with `{selector,"*"}` or explicit users. Older versions refuse to save a policy that names an agent, and read `{user,"*"}` as the literal user ID `*`, so the users it covered lose access through it.
3. While still on this version, clear profile configuration (`POST …/profiles/{id}/configuration/reset`) and reset profile OAuth grants (`DELETE …/components/{id}/oauth`). Older versions ignore grant token rows (owner `vmcp-profile:…`) and profile credentials, which would otherwise stay at rest, and any vMCP write by an older version drops the status fields that track them, so a later upgrade can't find them. If a reset isn't possible, delete OAuth token rows whose `user_id` starts with `vmcp-profile:`.
4. Delete the agents, so their keys are revoked and their instances removed.
5. Downgrade the binary.

Schema additions stay in place, and nothing needs to be altered back: the `agents`, `agent_lifecycle_events` and `admin_audit_events` tables, `api_keys.agent_id` and the CHECK constraint, the audit and usage principal columns, the MCP audit vMCP, component and profile columns, and the token and pending-state grant columns. Older versions use gorm AutoMigrate, which never drops or tightens these, and their writes satisfy the CHECK. Revoked agent keys stay in `api_keys` with a NULL `user_id`. Older versions read it as user 0, list it in the admin key list, and can't authenticate it, because they parse only `ok1-` keys and match on `user_id`. A rollback rehearsal on Postgres 16 ran the pre-feature binary (`d2d8508c7`) on a database written by this version, with and without these steps, and then this version again. All three started, migrated and served user keys, admin key lists and audit logs.

## Departures from the ODP

Each of these differs materially from the accepted ODP.

1. **Key scope defaults (decision 6).** The ODP defaults omitted agent-key restrictions to `MCPServerIDs: ["*"]`, `CanAccessLLMProxy: true` and `CanAccessSkills: true`. Shipped: agent key scopes work as user key scopes do. An omitted scope isn't granted, at least one scope is required, and an empty MCP list means no MCP access.
2. **No owners (decision 7).** The ODP gives agents ownership metadata. Shipped: no owners. Only an immutable `CreatorUserID` is recorded.
3. **Deletion leaves subjects in place (decision 7a).** The ODP says deletion removes explicit agent subjects from vMCP profiles, model access policies and skill access rules. Shipped: subjects stay and match nobody, as a deleted user's do. Agent IDs are never reused, so a stale subject can't grant a later agent anything.
4. **Profiles never override fixed values.** The ODP lets profiles supply "optional overrides of vMCP-fixed values". Shipped: profiles set only `userAllowed` keys. Fixed, catalog-static, Secret-bound and prohibited keys are rejected.
5. **Agents work with authentication off (decision 9).** The ODP doesn't cover this mode. Shipped: the single owner manages agents, and agent keys authenticate as their agents ahead of the catch-all owner.
6. **No feature flag (decision 8).** The ODP's rollout adds the schema and enforcement "before enabling agent creation and policy editing". Shipped: there's no switch, and merge order is the only gate.
7. **MCP guardrail filters accept agent subjects.** The ODP limits agent subjects to vMCP profiles, model access policies and skill access rules, and says "do not expand wildcard semantics in unrelated policy systems". Shipped: MCP guardrail filters accept all agents, all users and explicit agents. Their `everyone`, and a filter with no subjects, apply to agents too. Filters restrict rather than grant, so this narrows what agents can do.
8. **OAuth grant fallback when the URL comes from another profile.** The ODP falls back to the wildcard grant when the explicit profile has none. Shipped: a grant is bound to the URL the agent reaches through the grant's profile. An agent whose URL comes from a different profile than its grant gets `oauth_invalid` rather than a fallback.
9. **Mid-stream errors carry no reason code.** The ODP says an already-started response uses the protocol's error path. Shipped: no challenge is ever issued, but a JSON-RPC error inside an already-started stream can't yet carry `agent_oauth_required` or the component.
10. **Refused agent keys get a plain 401.** The ODP keeps the existing authentication failure behavior for an invalid credential. Shipped: a refused agent key gets a plain 401 without `WWW-Authenticate` on MCP connections. The registry (`/v0.1`) still challenges.
11. **Admin change audit coverage.** The ODP audits lifecycle, key, profile, model and skill policy, and OAuth changes with the acting admin. Shipped: only changes made through the admin API, plus `system` grant invalidation and removal. Catalog-synced vMCP subject changes and controller-side changes are not recorded.
12. **Skills activity attribution.** The ODP attributes Skills activity to the agent and key. Shipped: skill downloads remain unaudited, for users and agents alike. That gap already existed.
13. **Catalog YAML can't name specific agents (decision 3).** The ODP enforces the selection rules in catalog validation, but doesn't limit catalog subjects. Shipped: catalog vMCPs accept `{agent,*}` and `{selector,*}` but reject explicit agent IDs, because agent IDs are per deployment.
14. **Grant reset doesn't revoke at the provider.** The ODP's "revocation" covers grants. Shipped: a reset deletes the grant locally without RFC 7009 revocation, as users' resets do.
15. **Additions beyond the ODP.** These are recorded so the follow-up ODP can accept them explicitly: CEL `principal.kind` and `principal.id`; anonymous registry discovery lists `{user,"*"}` vMCPs; the API key admin list filters by `?kind=user|agent`; localhost-callback grants through `obot mcp login`; the `admin` audit export type; and the rename of the user API key tab from "Agents" to "API Keys".

## References

- Specs and implementation notes: the PR stack from `bef63e781` to `5fe022506`.
- `pkg/principal`, `pkg/subjects`, `pkg/api/authz/agent.go`, `pkg/gateway/types/agent.go`, `pkg/gateway/types/apikeys.go`, `pkg/gateway/types/adminauditevent.go`
- `pkg/vmcp/readiness.go`, `pkg/vmcp/profileids.go`, `pkg/vmcp/profileconfig.go`, `pkg/gateway/client/mcpoauthgrant.go`
- `pkg/controller/handlers/cleanup/agent.go`, `pkg/controller/userwildcard.go`
- Docs: `docs/docs/security/agent-identities.md`, `docs/docs/mcp-gateway/agent-setup.md`, `docs/docs/reference/agent-identities.md`
- Manual QA checklist: `tests/integration/agent-identities-qa.md`
