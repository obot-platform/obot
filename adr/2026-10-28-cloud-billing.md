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

An Obot Cloud environment is billed through obot-billing, a separate service that owns Stripe, the plans and each
environment's entitlements. Obot only reads its subscription from billing and holds itself to it. An environment is
billed when `OBOT_SERVER_BILLING_KEY` is set (`OBOT_SERVER_BILLING_URL` defaults to `https://billing.obot.ai`). Without
the key, nothing below about billing applies, and the Keygen license works as before.

### 1. Entitlements

Entitlements are what the environment may do. One struct, `types.Entitlements`, describes them for both sources:

- **Features** are booleans: community, enterprise, cloud, enterprise auth providers, enterprise model providers.
- **Limits** are nullable numbers: users, devices, hosted MCP server memory (MiB) and CPU (millicores). `null` is
  unlimited and 0 allows none.
- **Audit log retention** in days. `null` or 0 keeps the configured retention.
- Audit log storage and network egress are stored but not used yet.

### 2. Providers

One interface provides the entitlements, and everything that checks a feature or a limit takes it:

```go
type EntitlementProvider interface {
	Entitlements(context.Context) (types.Entitlements, error)
}
```

It has two implementations, in separate packages that don't know about each other:

| Provider | Package | What it does |
|---|---|---|
| `license.Provider` | `pkg/license` | The Keygen license, as before. It turns the license's entitlement codes into `types.Entitlements`. |
| `billing.Provider` | `pkg/billing` | The obot-billing subscription. It reads the entitlements from the stored `Subscription`, and also reports whether the environment is restricted. `billing.Client`, in the same package, is the client for obot-billing's API. |

Startup picks one. With a billing key it creates the billing provider and uses it as the entitlement provider. Without
one, the license provider is the entitlement provider and there is no billing provider. The Keygen license stays
activated either way, since the model proxy uses its key, so the services carry three things: the license provider,
the billing provider (nil when the environment isn't billed), and the entitlement provider, which is one of the two.

The billing provider reads the `Subscription` from the cache, since requests read it all the time: every request
checks the restriction, and every sign-in reads the user limit. The cache follows the leader's writes within moments,
and billing's answer is only as fresh as the last sync anyway. Startup and the first writes storage admits read it
before the cache starts, and those reads go to storage.

The checks that don't depend on the source are functions in `pkg/license` that take the entitlements or their
provider: usage (`Usage`), the limits it's over (`LimitViolations`), the violations `/api/version` reports
(`Violations`), whether a provider manifest's required features are missing (`EntitlementsMissing`), and the gate on
paths that need a configured auth provider's features. Nothing outside `pkg/license` reads entitlement codes.

### 3. What billing stores

Two resources, both in the default namespace, created by the leader when it's elected and recreated if deleted, and
only in a billed environment. Until they exist, the entitlements are empty, as before billing first answers:

- **`Subscription`** (`default/subscription`)
  - The `obot.ai/subscription-sync` annotation: set by `POST /api/license` to ask the leader for a sync, the way
    other kinds ask for one. The license page sends it when an owner comes back from the billing pages. There's no
    button for it.
  - `status.plan`, `status.status`, `status.periodEnd`, `status.cancelAtPeriodEnd`, `status.selfService` and
    `status.entitlements`: billing's last answer. The period end is stored as a `metav1.Time`, like every other
    stored time, and the API returns the answer as a `types.Subscription`.
  - `status.lastSyncTime` and `status.syncError`: when the leader last tried, and why it failed.
  - `status.overLimitSince`: when usage went over a limit, or unset while it's within them.
- **`EntitlementUsage`** (`default/entitlement-usage`)
  - `status.memoryMiB` and `status.cpuMillicores`: what hosted MCP servers request, as last counted. It's for display
    and the over-limit clock only. `license.Usage` is its one reader.

### 4. The leader's controllers

`pkg/controller/handlers/subscription` runs three handlers on the leader:

1. **`Sync`** (on the `Subscription`) asks billing for the subscription every 15 minutes, 5 minutes after a failure,
   and right away when the sync annotation is set, which it clears once the answer is stored. A failed sync keeps
   the last answer and records the error.
2. **`EnsureOverLimitSince`** (on the `Subscription`, after `Sync`) compares usage with the subscription's limits. It
   sets `overLimitSince` when usage first goes over, leaves it while usage stays over, and clears it once usage fits.
   It runs at each sync and whenever the `EntitlementUsage` changes.
3. **`CountUsage`** (on the `EntitlementUsage`) recounts what hosted MCP servers request and stores it. It runs
   whenever a vMCP, vMCP instance, filter, system MCP server or the K8s settings change.

### 5. How limits are enforced

Every limit is enforced when the object is created or updated, against a live count, under a lock.

- **Users.** Each request signed in through an auth provider reads the user limit from the entitlements. If the
  request has to create a user, the gateway takes its user lock, counts users and creates the user in one transaction,
  or refuses with a 403.
  SCIM user creation goes through the same code. The bootstrap and no-auth users aren't limited.
- **Devices.** Device enrollment reads the device limit from the entitlements, and the gateway counts and creates the
  device under its device lock, or refuses with a 403. Re-enrolling a device never counts.
- **Compute** (billed environments only). Compute is the memory and CPU that the MCP servers Obot runs request.
  The storage API server passes every create and update, patches included, to `mcp.ComputeAdmitter`, whichever
  client makes the write. A write of an object that's being deleted is made right away, since it has no servers. For
  the other writes of the kinds that use compute, it does this under one lock:
  1. Read the entitlements. If neither memory nor CPU is limited, make the write.
  2. List the objects that use compute from storage, without a cache, and add up what their servers request.
  3. Add it up again as if the object were written.
  4. Refuse with a 402 if the write raises memory or CPU past its limit. Otherwise make the write.

  A write that lowers compute is always made, even while over. The lock is a Postgres advisory lock held by a
  transaction while the write runs on other connections, so replicas take turns. SQLite runs one replica, so an
  in-process lock is enough.

What a server requests is its memory and CPU requests under the current K8s settings, with a limit counted when
there's no request, as Kubernetes does. One server counts for each multi-user component of a vMCP, each single-user
component of an instance, each enabled filter that isn't a guardrail, and each enabled system MCP server that isn't
a filter's. Remote servers don't run in a pod, so they don't count.

Compute is counted in advance, from Obot's own resources and never from what's deployed, so a server counts whether
or not it's running yet. Two tests pin the count to what runs: one compares it with the Deployment the backend builds
for a server, the other with the servers the vMCP and instance controllers create. The count is deliberately higher
in one case: a single-user component counts for every instance, even when no profile enables it for that instance's
user. Profiles follow the user's groups, which change without a write that could be refused, and counting only the
enabled components would mean looking up every instance's user while holding the lock. The temporary servers
launched for tool previews and filter tests have no Obot resource, so they don't count.

Callers that can hit the refusal read it with `mcp.ComputeLimitRefusal`: updating a vMCP reports which connections
kept their versions, the catalog sync applies its vMCPs one at a time to leave out the ones that don't fit and
records why, and the MCP gateway answers the client with the reason as a JSON-RPC error.

### 6. Restriction

A billed environment is restricted in two cases:

- `overLimitSince` is 3 days old.
- The last sync failed and the period billing last answered with ended 3 days ago or more. A billing outage keeps
  the last answer, but only that far past the period it was for. An enterprise subscription has no period end, so an
  outage never restricts it.

Every request from someone who isn't an admin checks it, against the cached `Subscription`, so a restriction can
start or lift a moment late. While restricted, the admission check that runs after
authentication refuses users who aren't admins or owners, and devices. Admins and owners keep access, including through their scoped API keys, so they can
bring usage back down. SCIM, tunnels and metrics keep working. Refused browsers are signed out and sent to the login
page, which says why. Every other client gets a 402. Nothing is deleted, and the restriction lifts as soon as usage
fits or billing answers again.

### 7. Where usage is shown

`/api/version` and the license page show usage from `license.Usage`: users and devices counted as they're read, and
compute from the stored `EntitlementUsage`, which is 0 where nothing counts it. `/api/version` reports a violation for
each limit usage is over, for a restriction, and for each configured auth provider that needs a missing feature.

The capacity page shows what MCP server deployments request against the cluster's capacity. When the entitlements
limit memory or CPU, `mcp.ComputeCapacity` shows their limits instead, with the compute counted from Obot's resources.

In a billed environment `GET /api/license` returns a `BillingStatus` in place of the license: the entitlements,
and for admins and auditors the subscription, usage and sync error. The entitlements are shown once, so the
subscription leaves its own out. `POST /api/license` asks for a sync, and `POST /api/license/manage` opens billing's
pages for an owner through a one-time link, a `types.BillingSession`. The Keygen license routes are off in a billed
environment.

### 8. Audit log retention

Only the leader deletes data past retention. Each pass reads the entitlements: a retention above 0 replaces the
configured days for MCP and LLM audit logs, capped at 100 years. If the entitlements can't be read, that pass keeps
every audit log.

## Rationale

- **A separate billing service** keeps Stripe keys and webhooks, which reach every customer, out of every environment.
  Obot authenticates with its environment's billing key and never sees a Stripe object.
- **One typed struct behind one interface** keeps the license and subscription checks from drifting apart. Billing
  and the Keygen license stay separate packages, and only startup knows which one applies.
- **Admitting compute in storage** covers every writer, including controllers and patches, without wrapping each
  one.
- **Live counts under a lock** are what make a limit hold. A stored count can lag, so it could admit two writes that
  only fit one at a time. Display tolerates the lag, so it reads the stored count and `/api/version` doesn't list
  every object that uses compute on each call. The limits themselves come from the cached `Subscription`: they
  change only when the leader syncs, so the cache costs a moment at most and saves a read on every request.
- **Restricting instead of refusing a downgrade** lets owners pay less without tidying first, and the 3 days give them
  time to adjust.
- **Empty entitlements before billing first answers** grant no features and set no limits, so the first owner can
  sign in with the activation link.

## Consequences

- In a billed environment, every create and update of a kind that uses compute takes the compute lock and lists
  those kinds. A new kind that changes compute has to be added to `mcp.Compute`.
- A change to the subscription reaches each replica through its cache, a moment after the leader stores it.
- An environment over its limits can't add users, devices or compute, but keeps running what it has until it's
  restricted.
- An environment whose billing never answers runs unlimited, without enterprise features.
- An environment whose billing stops answering keeps its last answer, and is restricted 3 days after that answer's
  period ends. Without a period end, it keeps the answer for as long as the outage lasts.
- Limits change only as fast as the leader syncs: every 15 minutes, or right away when an owner comes back from the
  billing pages.
- A provider manifest's required entitlement that names no feature Obot knows always counts as missing, so the
  provider can't be used. A license that carried that exact code used to grant it. No shipped provider requires
  one.
- The API reports features as booleans: `/api/version` has `cloud` and no longer has `licenseEntitlements` or
  `missingLicenseEntitlements`, and a provider's status has `entitlementsMissing` in place of `missingEntitlements`.
  `/api/version` also has `billingEnabled` in a billed environment, since a Keygen license can grant `cloud` too.

## References

- [Entitlement provider interface](../pkg/gateway/client/auth.go), [billing provider](../pkg/billing/provider.go),
  [billing client](../pkg/billing/client.go), [license provider](../pkg/license/provider.go) and the
  [checks against entitlements](../pkg/license/entitlements.go)
- [Compute: counting and admission](../pkg/mcp/compute.go)
- [`Subscription`](../pkg/storage/apis/obot.obot.ai/v1/subscription.go) and
  [`EntitlementUsage`](../pkg/storage/apis/obot.obot.ai/v1/entitlementusage.go)
- [Leader controllers](../pkg/controller/handlers/subscription/subscription.go)
- [Compute lock](../pkg/gateway/client/lock.go)
- [Storage admission](../pkg/storage/registry/generic/store.go)
- [Restriction at authentication](../pkg/api/authn/admission.go)
- [Billing routes](../pkg/api/handlers/billing.go)
- [obot-billing's API](https://github.com/obot-platform/obot-billing/blob/main/docs/api.md)
