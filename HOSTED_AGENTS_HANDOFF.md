# Hosted Agents — handoff

Status as of 2026-07-16. This records what was built, what was deliberately left
undone, and what to do next. It is a handoff note, not product documentation —
delete it once the feature lands.

Deeper per-topic notes live in
[`pkg/controller/handlers/hostedagent/README.md`](pkg/controller/handlers/hostedagent/README.md).
This file is the map; that file is the detail on the placeholder orchestrator.

---

## 1. What this feature is

An admin registers an **agent**: name, description, light/dark icon URLs, and a
docker image. The agent references configured services (model providers, models,
MCP servers, skills), carries env vars where some are sensitive, and is either:

- **shared** — one multi-tenant instance for everyone, launched from a URL; or
- **per-user** — each user creates their own named instances, capped by
  `maxInstancesPerUser`.

Per-user agents can additionally ask the user **questions** at instance-creation
time, and can let users attach **their own** MCP servers, skills, and models.

Access is gated by **Agent Access Policies**.

### The naming trap

`Agent` and `/admin/agents` were already taken by an unrelated nanobot feature,
so everything here is named `HostedAgent` in Go and `/hosted-agents` in URLs.
**The UI deliberately says "Agents"** — `hosted-agent` never appears in
user-facing copy. The older feature is expected to be deleted; until then the
sidebar shows both "Obot Agent Management" (old) and "Agent Management" (new).

---

## 2. Why it is built this way

These decisions are not obvious from the code and are expensive to rediscover.

**Access policies were cloned from `SkillAccessRule`, not `AccessControlRule`.**
The three admin "Access Policies" in this product are three *independent*
implementations, not one shared mechanism. `AccessControlRule` (MCP) is the
oldest and has catalog/workspace dual-scoping this feature does not need;
`ModelAccessPolicy` drops the resource `Type` field, which this feature does
need. `SkillAccessRule` is the newest, smallest, flat/unscoped one, and matches
an admin-registered global resource exactly. Cloning it also keeps this feature
entirely out of MCP's `acr1-everything` wildcard semantics.

**`MCPServers` holds MCP *gateway* IDs**, the same handles used by
`/mcp-connect/{mcp_id}`. That ID is polymorphic: `pkg/api/authz/mcpid.go`
dispatches on its prefix to a server instance (`msi1`), a server (`ms1`), a
system server (`sms1`), or otherwise a catalog entry — and authorizes it. This
is why the admin form lists catalog entries *and* servers: most admin-configured
MCP servers are `MCPServerCatalogEntry`, not `MCPServer`.

**Questions follow `MCPCatalogEntryFieldManifest`** (`{key, name, description,
required, sensitive}`, flat list) rather than a new schema language, with a
`type` added for validation. A `schedule` answer is a raw cron string
(precedent: `refreshSchedule` in `pkg/imagepullsecrets`), *not* the structured
`v1.Schedule` used by audit-log exports — that type cannot express
`*/15 * * * *`.

**`apiclient` stays dependency free.** Cron *shape* is checked there (field
count); authoritative `gronx` parsing happens in the server. Do not add `gronx`
to `apiclient` — it would land on every API consumer.

**Sensitive env values never touch the resource.** They are stripped from the
manifest and stored in the gateway credential store under
`hosted-agent-{name}`; a blank sensitive value on update means "unchanged".
Verified: the literal secret appears in zero `hostedagent` rows.

---

## 3. What exists

### Storage — `pkg/storage/apis/obot.obot.ai/v1/`

| Type | Prefix | Notes |
|---|---|---|
| `HostedAgent` | `ha1` | `Spec.Manifest` + `SourceID`/`RelativePath`/`CommitSHA`. Field selectors `spec.perUser`, `spec.sourceID`. `DeleteRefs` → `AgentSource`. |
| `HostedAgentInstance` | `hai1` | `Spec{UserID, HostedAgentName, Manifest}`. Field selectors `spec.userID`, `spec.hostedAgentName`. `DeleteRefs` → `HostedAgent`. |
| `HostedAgentAccessRule` | `haar1` | Clone of `SkillAccessRule`. Resource enum: `hostedAgent | selector`. |
| `AgentSource` | `as1` | Clone of `SkillRepository`. `{displayName, repoURL, ref}` + sync status. |

All registered in `scheme.go`; stores and `/status` derive from the scheme by
reflection, so nothing in `pkg/storage/registry/` needed touching.

### API — `pkg/api/handlers/`

```
/api/hosted-agents                 CRUD + POST {id}/reveal      (admin; GET is policy-filtered for users)
/api/hosted-agent-instances        CRUD                          (per-user)
/api/hosted-agent-access-rules     CRUD                          (admin)
/api/agent-sources                 CRUD + POST {id}/refresh      (admin)
```

The UI calls access rules "Access Policies" via a rename shim in
`operations.ts`, matching the existing Skill Access Policy convention.

### Controllers — `pkg/controller/handlers/`

- `hostedagent/` — **placeholder orchestrator**. Real reconciler, no deployment.
- `agentsource/` — **placeholder sync**. Real reconciler (throttle, status,
  force-sync annotation, prune), but `buildAgentsFromSource` returns nothing.

### Seed

`pkg/controller/data/everything-hosted-agent-access-rule.yaml` →
`haar1-everything` (selector `*` / subject `*`), mirroring
`everything-skill-access-rule.yaml`. **Only seeds when no MCPCatalogs exist**
(i.e. a fresh DB) — on an existing dev DB you must create it by hand.

### UI — `ui/user/src/`

```
routes/admin/hosted-agents/{,[id]/}                  Agents + Sources tabs
routes/admin/hosted-agent-access-policies/{,[id]/}
routes/hosted-agents/{,[id]/}                        user-facing
lib/components/admin/HostedAgentForm.svelte
lib/components/admin/HostedAgentQuestionsEditor.svelte
lib/components/admin/HostedAgentEnvEditor.svelte
lib/components/admin/HostedAgentAccessPolicyForm.svelte
lib/components/admin/SearchHostedAgents.svelte
lib/components/hosted-agents/{HostedAgentCard,HostedAgentInstanceForm}.svelte
```

Reuses `SearchModels` (provider grouping, `obot://` aliases, wildcards),
`SearchSkills`, `SearchUsers`, `Table`, `Confirm`, `SensitiveInput`.

---

## 4. Unrelated fixes that rode along

Two pre-existing bugs were fixed to get `make check-hooks` green. **Both touch
other people's code and deserve their own review.**

**`ReverseProxy.Director` → `Rewrite`** (`ui/handler.go`,
`pkg/api/handlers/mcpgateway/handler.go`, `pkg/gateway/server/llmproxy.go`).
Not cosmetic: `Director` made ReverseProxy set `X-Forwarded-For` automatically,
`Rewrite` does not. Each site was migrated to preserve exact behavior —
`llmproxy` deliberately does **not** use `SetXForwarded()`, because that would
send `X-Forwarded-Host`/`Proto` to external model providers, which `Director`
never did. `ui/handler_test.go` pins the forwarding behavior.

**Skill metadata was silently dropped** (`apiclient/types/skill.go`). `Skill`
embedded both `Metadata` (tag `metadata`) and `SkillManifest` (field
`MetadataValues`, also tag `metadata`). At equal depth `encoding/json` drops
*both* — a populated skill serialized as `{"id":...,"name":...}` with no
metadata at all, so skill frontmatter has never reached any client. Fixed by
renaming the tag to `metadataValues`.

> **API change worth knowing:** `/api/skills` now emits `metadataValues` where
> it previously emitted nothing. The storage key moves `spec.metadata` →
> `spec.metadataValues` and self-heals on the next hourly sync, since upsert
> rewrites `Spec` wholesale. No client read it, so risk is low.

---

## 5. Required next steps

### 5.1 Real orchestration (required — the feature does nothing without it)

`pkg/controller/handlers/hostedagent/hostedagent.go` marks agents/instances
ready and assigns a synthetic `{serverURL}/hosted/{name}-{random}` URL. It
deploys nothing.

Replace the two handler bodies. Nothing else depends on the URL being fake.
**Keep the early-return guards** — without them each `Status().Update`
retriggers the handler and mints a new URL forever.

Note the deliberate split: shared agents carry `Status.URL`; per-user agents
carry no status at all (their instances do). The admin list renders `-` rather
than `pending` for per-user agents because of this.

### 5.2 Real agent discovery (required for Agent Sources)

`pkg/controller/handlers/agentsource/agentsource.go`: `buildAgentsFromSource`
returns `nil, nil` and `placeholderFetcher.Fetch` checks nothing out. Everything
around it is real.

Model it on `pkg/controller/handlers/skillrepository/` — note that **it does not
clone git**: it uses the GitHub REST API + zipball (`github.go`) and discovers
by convention (any directory containing `SKILL.md`). Decide the agent equivalent
of `SKILL.md`. Populate `SourceID`/`RelativePath`/`CommitSHA` on each discovered
`HostedAgent`; the `spec.sourceID` field index and the prune loop already work.

Also unimplemented: `ValidateAgentSourceURL` only checks https + host + path,
whereas skills hard-require a GitHub URL. Tighten if the fetcher is
GitHub-only.

### 5.3 Delete or rename the old Agents feature

`/admin/agents` (nanobot `ProjectV2Agent`) still exists. Until it goes, the
sidebar has two "Agent Management" sections. When it is removed, consider
renaming `HostedAgent` → `Agent` throughout so code matches the UI.

---

## 6. Recommended next steps

### 6.1 Prune dangling admin-configured resources

`HostedAgent.Spec.Manifest`'s `ModelProviders`, `Models`, `MCPServers`, and
`Skills` are **not** existence-checked. Deleting an MCP server leaves a dangling
ID on the agent. Admin-authored, so not a security issue — but
`AccessControlRule` solves exactly this with `PruneDeletedResources`
(`pkg/controller/handlers/accesscontrolrule/`); copy that.

⚠️ If you do, note the trap in that handler: its `switch` has **no `default`**,
so an unknown resource type is silently dropped and the rule is rewritten
without it. Don't reproduce that.

### 6.2 Re-check access at use time

User-attached resources *are* access-checked at the API (§7), but the helpers
are informer-backed and can be stale, and access can be revoked after an
instance is created. Whatever wires these into a running agent should re-check
rather than trust the stored list.

### 6.3 Smaller items

- **Structured schedule input.** A cron text field is unfriendly. If a picker is
  wanted, `ui/user/src/lib/components/nanobot/taskSchedule.ts` has the closest
  extractable helpers.
- **Questions are per-agent only.** They are rejected on shared agents, since
  there is no per-user creation step to ask at.
- **`HostedAgentInstance` has no `reveal`.** Sensitive *question answers* are
  stored in the instance manifest in plain text, unlike agent env vars, which go
  to the credential store. If sensitive answers must be protected, route them
  through the credential store the way `hostedagents.go` does.

---

## 7. Security posture

**Enforced today**, on both create and update, in `validateInstanceAgainstAgent`
(`pkg/api/handlers/hostedagentinstances.go`):

- the agent opted into the resource **kind** (`allowUserMCPServers` /
  `allowUserSkills` / `allowUserModels`); and
- the user has access to each **specific ID**.

Per-kind, because each kind has its own access model:

| Kind | Check | Trap it avoids |
|---|---|---|
| MCP servers | `authz.CheckMCPIDAccess` | Returns a `NotFound` **error**, not `false`, for unknown IDs — unmapped, a typo becomes a 500. |
| Skills | `skillaccessrule.UserHasAccessToSkill` | The skill is loaded first for its repo ID; an empty repo ID silently skips repository-granted access and wrongly denies. |
| Models | `modelaccesspolicy.UserHasAccessToModel` | Policies are keyed by concrete model IDs, so `obot://<alias>` is resolved first or it would always deny. |

Errors: **400** for missing (as `accesscontrolrules.go` does), **403** for
inaccessible (as `llmproxy.go` does). **No admin bypass** — these resources run
on behalf of the instance owner. Seeded wildcard rules mean admins are
unaffected in practice.

Instance ownership is enforced in `pkg/api/authz/hostedagent.go`
(`Spec.UserID == user` **and** the user still has access to the parent agent).
`POST /api/hosted-agent-instances` carries no ID in its path, so the authorizer
cannot gate it — that check lives in the handler.

---

## 8. How to verify

```bash
make generate && go build ./... && make test
cd ui/user && pnpm run ci
make check-hooks          # required before handing work back (see CLAUDE.md)
```

Run the server against a throwaway DB on free ports so it does not collide with
a running dev instance:

```bash
OBOT_SERVER_DSN="sqlite://file:/tmp/verify.db?_journal=WAL&cache=shared" \
OBOT_DEV_MODE=true OBOT_BOOTSTRAP_TOKEN="token" \
go run main.go server --dev-mode --http-listen-port 28080 --storage-listen-port 28443
```

Then `Authorization: Bearer token` as the admin. **Use a fresh DB** or
`haar1-everything` will not be seeded and every agent will be invisible.

Behaviors worth re-checking after any change, each of which caught a real bug:

1. **URL stability** — poll a shared agent for ~12s. A changing URL means the
   idempotency guard broke and the reconciler is looping.
2. **Secret isolation** — create an agent with a sensitive env var; the literal
   value must appear in zero `hostedagent` rows, and `POST {id}/reveal` must
   return it.
3. **Access denial** — attach a real skill (accepted, 201), narrow
   `sar1-everything` to another user, attach the *same* skill (must be 403),
   restore. Rejecting bogus IDs proves nothing; flipping a real one proves the
   check works.
4. **Cascade** — deleting a per-user agent removes its instances; deleting an
   `AgentSource` must **not** touch hand-registered agents (they have no
   `SourceID`, and `cleanup.Cleanup` skips empty refs).
5. **Quota** — the `maxInstancesPerUser + 1`-th instance must be a 400.

---

## 9. Environment notes

- **`make check-hooks` is the authoritative gate** (`CLAUDE.md`), not
  `make lint`. It runs the correct golangci-lint via the `tool` block in
  `go.mod`; a stale `golangci-lint` on `PATH` fails with a Go-version mismatch
  and is not the one the hooks use.
- **`go vet` is not a substitute.** It missed both issues the linter caught
  (`redefines-builtin-id`, a redundant embedded-field selector).
- The `go-lsp` hook is **file-scoped**: it only re-runs on packages you touch, so
  a pre-existing warning can appear the moment you add an unrelated file to that
  package. That is what surfaced the skill.go bug.
