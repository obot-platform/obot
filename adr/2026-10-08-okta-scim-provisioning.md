# 2026-10-08: Provision Okta users and groups through SCIM

- **Status:** Accepted
- **Date:** 2026-10-08
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/field-issues#38](https://github.com/obot-platform/field-issues/issues/38)

Implemented in
[obot-platform/obot#8085](https://github.com/obot-platform/obot/pull/8085) and
[obot-platform/enterprise-providers#26](https://github.com/obot-platform/enterprise-providers/pull/26).

## Related ODPs

- [2026-09-23: SCIM Support for
  Okta](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-23-okta-scim/README.md),
  including Amendment A, which changes what happens when a SCIM-managed provider
  is deconfigured.

## Context

We wanted to add SCIM support so that Okta users could select which users
and groups to push to Obot, rather than giving Obot access to query about
all groups in the Okta workspace.

## Decision

Okta provisions users, account status, groups, and group memberships to Obot
through SCIM 2.0.

**SCIM is a facade over the existing directory model.** Obot serves SCIM at
`/scim/v2`. SCIM requests write the existing user, identity, group, and
membership tables. New binding tables map SCIM resource IDs to
existing rows, and existing user and group IDs never change:

- A user binds by the native Okta user ID, which Okta sends as `externalId`,
  through an identity of the same auth provider. Email is never used.
- A group binds by normalized display name, because Okta does not send its group
  ID. Groups that SCIM creates get IDs of the form `okta/<scim-uuid>`.

**One connection, owned by the auth provider.** An installation has at most one
SCIM connection, tied to the configured auth provider. It authenticates with its
own bearer token (`obot_scim_…`), of which Obot stores only a SHA-256 verifier.
Tokens expire after a year. Rotation keeps the previous token valid for up to 24
hours, but never past its own expiration, unless an Owner revokes it sooner. The
URL does not name the connection: authentication resolves it from the bearer
token, and the resulting connection principal serves only that connection. SCIM
requests pass through the normal authentication, authorization, rate limiting,
and audit logging. Only the connection principal can reach the SCIM routes, and
no other principal, Owners included, can reach them.

**Provider rules live in an in-server adapter registry.** `pkg/scim/adapter`
maps an auth provider name to an adapter, which holds every provider-specific
rule: how native user IDs are read, how new group IDs are formed, and which
configuration parameters SCIM replaces. The connection records its adapter type.
Auth provider manifests declare nothing about SCIM. Shared code never infers
provider behavior from names, prefixes, or ID shapes.

**Deactivation disables users and deletes nothing.** Users gain `disabled_at`
and `disabled_reason`. One admission check runs after the whole authenticator
chain, so a disabled user is denied on every credential path: browser sessions,
API keys, persistent and MCP OAuth tokens, device and tunnel flows,
impersonation, and hosted-agent keys through their owner. Each authenticator
reports the user's state from the read it already makes, and a user principal
without a reported state is denied. Disabling a user deletes their gateway auth
tokens in the same transaction. A durable outbox then deletes their MCP OAuth
refresh tokens from the controller store. Memberships, data, and API keys are
kept, so reactivation restores the same account. Okta never deletes users over
SCIM. An admin can delete a user once Okta has deactivated them.

**Setup is two permanent steps.**

| State | How it is reached | Effect |
| --- | --- | --- |
| `connected` | **Enable** on an Okta provider that synchronizes its directory, or configuring Okta without API Services credentials | Login-time directory synchronization stops. Enable deletes the provider's unreferenced, unbound groups. Users that Okta has not provisioned can still sign in. |
| `enforced` | **Enforce** | Users of the provider that Okta has not provisioned are disabled, and sign-in requires a provisioned user. |

Enforce is blocked until every group referenced by a role, policy, or vMCP
profile has been pushed, or its references removed, and until the acting Owner
has been provisioned. Group references are found by one shared finder,
`pkg/groupref`, which setup, the API, and auth-provider cleanup all use.

**Deconfiguring the provider removes SCIM.** Deconfiguring the Okta provider,
including switching to another provider, deletes its SCIM connection, groups,
memberships, and group role assignments, and removes its groups from access
policies, as auth-provider cleanup does for any provider. Users are kept, and
users that SCIM disabled stay disabled until an administrator enables them. This
is the only way to stop using SCIM. Configuring Okta again with API Services
credentials returns to directory synchronization.

## Rationale

Writing the existing tables keeps every user ID, group ID, role assignment, and
policy subject valid, so moving to SCIM needs no rewrite of authorization data.
A separate SCIM store would have required synchronizing two models.

Other approaches were rejected:

- **A staged copy of the directory with an atomic cutover.** This would need
  staging tables, a diff engine, and a fenced commit. Binding is already
  idempotent, and the Enable and Enforce reviews show the same consequences a
  diff would.
- **Exposing existing groups to Okta and using Link Group.** Testing against
  Okta showed that linking echoes the target group's members back, so stale
  members are never removed. Existing groups are therefore invisible to SCIM
  until a pushed group binds to them by name.
- **A reversible migration.** After Enable, memberships and the IDs of
  SCIM-created groups exist only in SCIM, so returning to directory
  synchronization would orphan them.
- **Keeping SCIM data when the provider is deconfigured.** The ODP originally
  kept it so SCIM could resume. Deleting it matches how every other provider is
  deconfigured, is simpler, and gives administrators a way to stop using SCIM.

Separating Enable from Enforce lets an administrator start provisioning, check
that Okta pushes what they expect, and only then cut off unprovisioned users.
Blocking Enforce on unpushed referenced groups makes the administrator decide
what happens to their access, instead of Obot keeping stale members or silently
emptying the group.

The admission check adds no database query, because every user credential path
already reads the user's row. Making it fail closed means a new or forgotten
credential path locks users out instead of admitting disabled ones.

## Consequences

- Group binding trusts Okta display names. A group recreated in Okta under the
  same name takes over the Obot group's roles and policies. Enable is blocked
  while two referenced groups share a name.
- Okta sends nothing when a user is suspended, so administrators must unassign
  users from the SCIM app to remove access. The documentation says so.
- Okta does not retry requests that fail with `503`. Changes sent while Obot is
  down or the provider is deconfigured must be retried from Okta's task list.
- Okta's built-in `Everyone` group cannot be pushed. Its references must be
  replaced with the all-users subject before moving to SCIM.
- SCIM writes are serialized by one lock. This is acceptable because each
  connection has a single client.
- Disabling a user does not close responses already streaming or in-memory MCP
  sessions. Every new request is denied.
- Every new authenticator that yields a user must report that user's lifecycle
  state, or its users are denied.
- Every new resource that can reference a group must be added to `pkg/groupref`,
  or Enable and Enforce will not account for it, and cleanup will not remove it.
- Supporting another identity provider means adding an adapter. Shared SCIM code
  should not need to change.
- Downgrading Obot after Enable is not supported.

## References

- [Provision users and groups from Okta with
  SCIM](../docs/docs/configuration/okta-scim.md)
- [`pkg/scim`](../pkg/scim): SCIM endpoint, filters, and PATCH handling
- [`pkg/scim/adapter`](../pkg/scim/adapter): provider adapter registry and the
  Okta adapter
- [`pkg/scim/setup`](../pkg/scim/setup): Enable, Enforce, and SCIM-first setup
- [`pkg/api/authn/admission.go`](../pkg/api/authn/admission.go): admission check
- [`pkg/gateway/client/lifecycle.go`](../pkg/gateway/client/lifecycle.go): user
  lifecycle and outbox
- [`pkg/groupref`](../pkg/groupref): group reference finder
- [RFC 7643: SCIM Core Schema](https://www.rfc-editor.org/rfc/rfc7643)
- [RFC 7644: SCIM Protocol](https://www.rfc-editor.org/rfc/rfc7644)
