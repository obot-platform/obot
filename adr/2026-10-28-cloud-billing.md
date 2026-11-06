# 2026-10-28: Obot Cloud billing through obot-billing

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#8008](https://github.com/obot-platform/obot/issues/8008) — the plans, subscription behavior and
  environment lifecycle.

## Related ODPs

- [2026-09-23: Cloud Billing](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-23-cloud-billing/README.md)

## Context

Obot Cloud sells a plan priced per user, whose devices, memory, CPU, audit log storage and network egress grow with
its users, and an add-on that sells extra memory and CPU. Owners have to buy and change that plan themselves, and each
environment has to hold to what it pays for: its users, its devices, the memory and CPU its hosted MCP servers request,
and its audit log retention.

Those limits have to hold on every replica and for every writer: API handlers, controllers such as the catalog sync,
and MCP clients whose connections start servers. A Cloud environment shouldn't hold payment credentials that could
reach other customers, and a billing outage mustn't take a paying environment down.

## Decision

**obot-billing owns the subscription; Obot only reads it.** A separate, shared service, obot-billing, owns the Stripe
account, the plan catalog, trials, the billing pages and each environment's entitlements. Obot never sees a Stripe
object. It authenticates to billing with its environment's billing key, a deployment secret the environment's Terraform
generates and billing stores only as a hash. Owners reach billing's pages from the License page through a one-time
link that Obot opens with `POST /api/v1/sessions`.

**One entitlement source.** With `OBOT_SERVER_BILLING_KEY` set, the billing plan replaces the Keygen license as the one
source of features and limits. Both sources produce the same typed struct of features and nullable limits, the license
through one mapping from its entitlement codes, so features are gated on the same paths either way and limits are
never added across sources. The Keygen license key, if any, still authenticates the model service.
`OBOT_SERVER_BILLING_URL` defaults to `https://billing.obot.ai`, and both settings are hidden.

**The leader syncs a `BillingSubscription`.** The leader reads `GET /api/v1/subscription` every 15 minutes, every 5
after a failure, and when an admin clicks Check now, and stores the answer in a `BillingSubscription` that every
replica reads. A failed sync keeps billing's last answer, across restarts. Until billing first answers, no plan
applies: every feature is on, nothing is limited, and the environment is never restricted.

**Hosted MCP servers are limited by the memory and CPU they request.** Usage is measured from what's stored: the
servers vMCPs and vMCP instances run, under the current K8s settings, with a container's limit counted when it sets no
request, as Kubernetes does. The embedded storage API server checks every create and update of those kinds, patches
included, whichever client makes it. Under one Postgres advisory lock it reads the plan, counts usage and makes the
write, and it refuses with a 402 naming the limit only a write that raises usage past a limit. Writes that lower usage
always go through. An MCP client whose connection wouldn't fit gets the reason as a JSON-RPC error, and no server
starts. This replaces the capacity check against a namespace `ResourceQuota`'s headroom, which Terraform keeps on
billing environments only as a safety net above the most the plan can grant.

**Users and devices** are limited where the gateway creates them, as before.

**Restriction.** The leader recounts usage at each sync and whenever vMCPs, instances or the K8s settings change. The
environment is restricted if usage stays over a limit for 3 days, or if billing is unreachable for 3 days past the last
known period end. While restricted, only admins and owners can use it, including through their scoped API keys.
Everyone else is signed out and their page loads go to the login page, which says why; their MCP clients get the reason
as a JSON-RPC error; devices keep reporting. Nothing is deleted or evicted, and the restriction lifts once usage fits.

**Retention follows the plan.** The audit log cleanup reads its retention on every pass, and only the leader runs it.
A plan's retention above 0 replaces the configured one, capped at 100 years; 0 keeps the configured one. Audit log
storage and network egress are stored with the plan but not shown or enforced.

**The license API and page cover both sources.** `GET /api/license` returns the entitlements as an object, and with
billing the subscription, usage and sync error for admins and auditors only. `POST /api/license` starts a sync,
`POST /api/license/manage` opens the billing pages for owners, and the Keygen-only routes are off with billing. Messages
members see name the limit without usage numbers.

## Rationale

A separate service keeps Stripe keys and webhooks, which reach every customer, out of every instance, and keeps Stripe
code out of Obot. Running trials in billing keeps Stripe out of provisioning until an owner pays.

One typed entitlement source keeps the license and plan checks from drifting apart. Encoding plan limits as license
entitlement names, as first proposed, would have summed them when several matched, which is wrong for a ceiling like
memory.

Checking hosted usage in storage covers every client, including controllers, nah's apply and patches, without wrapping
each one. Wrapping the API's client missed controller writes, and routing clients by API group adds nothing when every
counted kind is in one group. Counting what's stored rather than keeping a running total can't drift, survives
restarts, and makes re-applying an unchanged server free. Memory and CPU track what hosted servers cost to run better
than a count of servers.

Restricting rather than refusing a downgrade lets owners pay less without tidying first, and covers every way usage
can end up over a limit. The 3-day grace gives owners time to adjust after a downgrade, and keeps an outage at renewal,
before billing confirms the new period, from restricting a paid environment. It matches the provisioner's grace after
a subscription ends.

Serving every feature before billing first answers lets a new environment work at once instead of waiting on billing;
the provisioner registers it before sending the activation email, so billing normally answers first.

## Consequences

- Storage depends on a limiter that's set once the gateway exists. Every create and update of a counted kind takes the
  lock and lists those kinds, and a new kind that changes hosted usage has to join a meter.
- Go callers read refusals, which arrive as Kubernetes status errors, with `billing.Refusal`.
- An environment over its plan can't add users, devices or hosted servers, but keeps running what it has until it's
  restricted. Restriction is all or nothing for non-administrators.
- An environment whose billing never answers runs unlimited.
- Obot's limits change only as fast as the leader syncs, unless an admin clicks Check now.
- Plan definitions, prices, trials and Stripe behavior change in obot-billing, not here; Obot changes only when a new
  kind of limit needs enforcing.

## References

- [Billing client, subscription and plan limits](../pkg/billing)
- [Entitlements](../pkg/entitlements/entitlements.go)
- [`BillingSubscription`](../pkg/storage/apis/obot.obot.ai/v1/billingsubscription.go)
- [Sync and restriction](../pkg/controller/handlers/billingsubscription/billingsubscription.go)
- [Hosted usage](../pkg/mcp/hosted.go)
- [Storage's limited store](../pkg/storage/registry/generic/store.go)
- [Restriction gate and sign-out](../pkg/api/server/entitlements.go)
- [License API](../pkg/api/handlers/license.go)
- [obot-billing's API](https://github.com/obot-platform/obot-billing/blob/main/docs/api.md)
