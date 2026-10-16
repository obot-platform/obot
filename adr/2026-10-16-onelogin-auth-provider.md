# 2026-10-16: Add OneLogin as an auth provider with SCIM provisioning

- **Status:** Accepted
- **Date:** 2026-10-16
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

- [2026-09-23: SCIM Support for
  Okta](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-23-okta-scim/README.md),
  whose design this decision reuses.

## Context

Customers want to use OneLogin as Obot's identity provider, with the same two
options Okta offers: reading each user's groups from the directory at sign-in,
or provisioning users and groups through SCIM.

The [Okta SCIM ADR](./2026-10-08-okta-scim-provisioning.md) built SCIM as a
provider-neutral facade with per-provider adapters, so that a second provider
would need only an adapter. OneLogin is that second provider. Testing its SCIM
connector against a capture server showed that it mostly speaks plain RFC 7644,
but that its defaults, and a few of its behaviors, are unsafe for Obot:

- Its default user template sends no stable user ID.
- Before creating a user, it looks the user up by a configurable value, and
  adopts whatever account the lookup finds. On the default value, the username,
  a person who re-uses a deleted person's username is matched to the old
  person's account.
- It can be set to delete users, or to deactivate them.
- It shows Obot's error text to its admin verbatim, and retries nothing except a
  `429`, so a failed change waits until an admin retries it by hand.

## Decision

**OneLogin is a regular enterprise auth provider.** Login uses OIDC, and a
user's identity is their numeric OneLogin user ID, which is both the OIDC
subject and the value that OneLogin's SCIM template can send. Obot treats
OneLogin **roles** as groups. In directory mode, the provider reads a user's
roles through OneLogin's v2 admin API and uses IDs of the form
`onelogin/<role id>`. In SCIM mode, OneLogin pushes roles as groups through a
rule. New SCIM groups use `onelogin/<SCIM UUID>`; groups adopted during migration
keep their existing IDs.

**SCIM support is a OneLogin adapter.** Shared SCIM code gained no OneLogin
rules. Only the "SCIM Provisioner with SAML (SCIM v2 Core)" connector is
supported. Setup requires three OneLogin settings. Obot validates the user IDs
in incoming requests and refuses user deletion, but cannot inspect the app's
settings directly:

- **The template sends the user ID as `externalId`.** Obot binds a SCIM user to
  a sign-in identity by it, as it does for Okta.
- **`userName` must equal the user ID.** Set the app's SCIM Username parameter
  to OneLogin ID and leave the template's `userName` mapped to that parameter,
  so OneLogin's pre-create lookup only ever finds the same person.
- **Both deprovisioning actions are Suspend.** Obot still refuses to delete
  users over SCIM. With Suspend, removing a user's access, suspending them, and
  deleting them all deactivate the Obot user, and restoring access reactivates
  the same account.

**Error messages are the setup UI.** Adapters supply provider-specific hints
that the SCIM server adds to its error details. Each OneLogin error names the
setting to change, and says to retry. Okta's hints are empty, so its errors are
unchanged.

**The provisioning-only landing page is provider-neutral.** It moved to
`/scim-provisioning`, and `/okta-scim` remains an alias for existing Okta apps.

## Rationale

**Refuse misconfigured user IDs.** A create with a missing or invalid
`externalId`, or a `userName` that differs from it, is refused with the fix in
OneLogin's event log. Requiring both to be the user's numeric ID prevents a
re-used username from matching a former employee's account, at the cost of one
setup step that OneLogin doesn't default to. This validates the request's
values; it cannot verify which SCIM Username parameter produced the lookup.

Deprovisioning settings cannot be checked at creation. With an action set to
Delete, creation succeeds, but a later user `DELETE` is refused with `501` and
the user keeps access until an admin changes the action to Suspend and retries.
With Do nothing, OneLogin sends no deprovisioning request, so Obot cannot detect
the missing change.

**Keep "Obot never deletes users over SCIM".** Supporting user `DELETE` would
have changed shared SCIM behavior, and the Okta rule that deactivation keeps
data and memberships, while giving administrators nothing that Suspend doesn't
already provide. Testing showed that Suspend deactivates users on app-access
removal, suspension, and deletion, and that restoring access reuses the same
account. Unlicensing sends no SCIM request and leaves the Obot user active.

**Roles, not OneLogin groups.** A OneLogin user belongs to at most one group,
and groups exist mainly to apply security policies. Roles are multi-valued, and
are what OneLogin admins use to grant app access, so they are the natural match
for Obot groups.

**The v2 admin API, one call per user.** OneLogin's published rate limit applies
to its v1 API, and is shared with every other integration on the account. Its v2
API doesn't count against that limit, so reading each user's roles directly is
simpler than maintaining a bulk cache of the directory. If OneLogin turns out to
limit v2, a bulk cache remains possible without changing anything visible to
administrators.

## Consequences

- OneLogin never sends a role's ID or a rename. After a role rename,
  administrators must reapply entitlement mappings to create the new Obot group
  and transfer memberships. Grants on the old group don't move. Administrators
  must fix them by hand, as documented. A future "merge groups" action would
  help here and for Okta.
- OneLogin doesn't send role changes made while a user's access is removed, or
  any change when a user is unlicensed. The documentation covers both. Obot has
  no way to detect them.
- Setup has more required steps than Okta's. The setup wizard, the
  documentation, and the error messages all name them.
- OneLogin retries a `429` every few seconds without backoff, so Obot's SCIM
  rate limit must stay well above OneLogin's bursts.
- Shared SCIM changes made for OneLogin, the error hints and a fast path for
  RFC 7644 member removes, apply to every provider.
- The provider lives in `enterprise-providers`, which may later move to
  `obot-platform/providers`.

## References

- [OneLogin auth provider setup](../docs/docs/configuration/auth-providers.md#onelogin)
- [Provision users and groups from OneLogin with
  SCIM](../docs/docs/configuration/onelogin-scim.md)
- [`pkg/scim/adapter/onelogin.go`](../pkg/scim/adapter/onelogin.go): the
  OneLogin adapter
- [`pkg/scim/adapter/adapter.go`](../pkg/scim/adapter/adapter.go): the adapter
  interface, including error hints
- [2026-10-08: Provision Okta users and groups through
  SCIM](./2026-10-08-okta-scim-provisioning.md)
