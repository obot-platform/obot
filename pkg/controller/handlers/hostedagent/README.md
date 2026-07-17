# Hosted Agents — known gaps

This feature is intentionally incomplete. It delivers the resources,
authorization, UI, and a placeholder orchestrator; it does not run anything.
The items below are known and deliberate, not oversights.

## 1. Orchestration is fake

`hostedagent.go` is a real reconciler — it is registered in
`pkg/controller/routes.go` and runs on every create/update/watch event for
`HostedAgent` and `HostedAgentInstance` — but it performs no deployment. It
marks the object ready and assigns a synthetic
`{serverURL}/hosted/{name}-{random}` URL so the rest of the feature can be
exercised end to end.

Replacing it means rewriting the two handler bodies. Nothing else in the feature
depends on the URL being fake. Note the early-return guards: without them each
`Status().Update` retriggers the handler and mints a new URL forever.

## 2. User-attached resources (done — how it works)

`HostedAgentInstance.Spec.Manifest` carries `MCPServers`, `Skills`, and `Models`
that the *user* chose. Two things are enforced, on both create and update, in
`validateInstanceAgainstAgent` (`pkg/api/handlers/hostedagentinstances.go`):

1. the agent opted into the **kind**, via `AllowUserMCPServers` /
   `AllowUserSkills` / `AllowUserModels`; and
2. the user has access to each **specific ID**.

Each kind has its own access model, so there is a check per kind:

| Kind | Check | Why it is done this way |
|---|---|---|
| MCP servers | `authz.CheckMCPIDAccess` | IDs are MCP gateway handles (`/mcp-connect/{mcp_id}`) and may name a catalog entry, server, server instance, or system server. This is the same resolution the gateway applies at connect time, so the two cannot drift. Note it returns a `NotFound` **error** for an unknown ID rather than `false`, so that case is mapped explicitly. |
| Skills | `skillaccessrule.Helper.UserHasAccessToSkill` | The skill is loaded first so its repo ID is known. Passing an empty repo ID would skip repository-granted access and wrongly deny a user granted a whole repository. |
| Models | `modelaccesspolicy.Helper.UserHasAccessToModel` | Policies are keyed by concrete model IDs, so `obot://<alias>` references are resolved first via `resolveModelReference`. An unresolved alias would never match and would always deny. Wildcards are rejected: they express a policy, not a model to point an agent at. |

Errors follow the conventions already in the codebase: a missing resource is a
400 naming the ID (as in `accesscontrolrules.go`), and an inaccessible one is a
403 (as in `llmproxy.go` and `mcp.go`). Existence is not hidden, matching how
those paths already behave.

There is deliberately **no admin bypass**: these resources are used on behalf of
the instance's owner, so the same rules apply to everyone. In practice the
seeded wildcard rules (`acr1-everything`, `sar1-everything`, and the default
model policy) mean admins are unaffected.

One accepted limitation: the skill and model helpers are informer-backed with no
uncached fallback, so a rule or model created moments earlier may not be visible
yet. That staleness is accepted everywhere those helpers are used.

## 3. Admin-configured resources are existence-checked only

`HostedAgent.Spec.Manifest`'s `ModelProviders`, `Models`, `MCPServers`, and
`Skills` are set by an admin, so access is not a concern in the same way, but
they are also not validated for existence. A deleted MCP server or skill leaves a
dangling ID on the agent. `AccessControlRule` solves the equivalent problem with
a `PruneDeletedResources` controller handler
(`pkg/controller/handlers/accesscontrolrule/`) — the same approach would work
here.

## 4. Schedule questions store a raw cron string

A `schedule` question's answer is a cron expression, validated with `gronx`.
This differs from `v1.Schedule` (`{interval, hour, minute, weekday, timezone}`)
used by scheduled audit log exports, which derives cron in the controller and
cannot express arbitrary expressions such as `*/15 * * * *`. Raw cron was chosen
deliberately; the tradeoff is that the UI is a text field rather than a
structured picker.

Cron *shape* is checked in `apiclient/types` (field count) and parsed
authoritatively in the server, because `apiclient` is dependency free by design
and should not pull in `gronx`.
