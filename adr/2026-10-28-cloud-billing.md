# 2026-10-28: Obot Cloud billing through obot-billing

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8008

## Related ODPs

- [2026-09-23: Cloud Billing](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-23-cloud-billing/README.md)

## Context

Obot Cloud environments are billed through obot-billing. Their plan grants features and limits users, devices, the
memory and CPU hosted MCP servers request, and audit log retention. The limits have to hold on every replica and for
every writer: API handlers, controllers such as the catalog sync, and MCP clients whose connections start servers.

## Decision

- **Source.** With `OBOT_SERVER_BILLING_KEY` set, the billing plan replaces the Keygen license as the one source of
  entitlements, so features are gated on the same paths either way. The Keygen license key, if any, still
  authenticates the model service.
- **Sync.** The leader syncs the plan into the `BillingSubscription` every 15 minutes, or when an admin asks, and
  every replica reads it from storage. A failed sync keeps billing's last answer.
- **Limits.** Users and devices are checked where the gateway creates them. Hosted MCP server memory and CPU are
  counted from vMCPs, vMCP instances and K8s settings, and the embedded storage API server checks every create and
  update of those kinds, patches included. It reads the plan, counts and writes under one Postgres advisory lock, and
  refuses with 402 only a write that raises usage past a limit.
- **Restriction.** If a new plan leaves usage over a limit for 3 days, or billing is unreachable for 3 days past the
  last known period end, the leader restricts the environment. Only admins and owners can use it then. Other users are
  signed out, their MCP clients get a JSON-RPC error, and devices keep reporting.
- **Management.** Owners change the plan on billing's pages, through a one-time session for each click. Messages and
  pages members see leave out usage numbers.

## Rationale

One entitlement source keeps the license and plan checks from drifting apart. Checking hosted usage in storage covers
every client, including controllers, nah's apply and patches, without wrapping each one. Wrapping the API's client
missed controller writes, and routing clients by API group adds nothing when every counted kind is in one group.
Memory and CPU track what hosted servers cost to run better than a count of servers. The 3-day grace gives owners time
to adjust after a downgrade, and keeps an outage at renewal, before billing confirms the new period, from restricting a
paid environment. It matches the provisioner's grace after a subscription ends.

## Consequences

Storage depends on a limiter that's set once the gateway exists. Every create and update of a counted kind takes the
lock and lists those kinds, and a new kind that changes hosted usage has to join a meter. Go callers read refusals,
which arrive as Kubernetes status errors, with `billing.Refusal`. An environment over its plan can't add users,
devices or hosted servers, but keeps running what it has until it's restricted.

## References

- [Billing client, subscription and plan limits](../pkg/billing)
- [Storage's limited store](../pkg/storage/registry/generic/store.go)
- [Sync and restriction](../pkg/controller/handlers/billingsubscription/billingsubscription.go)
- [Restriction gate and sign-out](../pkg/api/server/entitlements.go)
