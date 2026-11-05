# 2026-11-02: Provision Microsoft Entra ID users and groups through SCIM

- **Status:** Accepted
- **Date:** 2026-11-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

- [2026-09-23: SCIM Support for
  Okta](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-23-okta-scim/README.md),
  whose design this decision extends. No new ODP was written.

## Context

The [Okta SCIM ADR](./2026-10-08-okta-scim-provisioning.md)
expected another provider to need only an adapter, and OneLogin needed only an
adapter and small shared additions. Testing
Entra's provisioning service against a capture server (four runs, the fixtures
of which are replayed in `pkg/scim/testdata/phase0`) showed that Entra breaks
four assumptions of the shared code:

- **A user can't bind by the provider user ID.** Obot keys an Entra identity by
  the OIDC `sub`, which is pairwise per application, and which the provisioning
  service can't send. The stable ID that Entra can send is the user's object ID,
  which Obot stored only in the encrypted `provider_group_lookup_id` column. A
  user that SCIM creates has no `sub` until they sign in.
- **Groups shouldn't bind by name.** Entra group names need not be unique, but
  every group create carries the group's object ID as `externalId`.
- **There are no directory-only auth provider configuration parameters.**
  Entra sign-in and Microsoft Graph group reads use the same credentials.
  Directory synchronization differs from SCIM only in the Graph application
  permissions granted in Entra, so omitting parameters can't choose SCIM.
- **Entra sends requests that Obot refused or lost data on:** `DELETE
  /Users/{id}` on permanent deletion, which Entra never retries after a `501`;
  `addresses` and enterprise-extension attributes, which Entra re-sends on every
  evaluation of a user whose read-back lacks them; and group creates with no
  members, followed by one `Add` PATCH per member.

## Decision

**Users bind by object ID, through a hashed lookup column.** Adapters declare a
`UserKey`. Entra's is `UserKeyGroupLookupID`, so a SCIM user binds through the
new `identities.hashed_provider_group_lookup_id`, which `encryptIdentity` keeps
in sync on every write and a batched, idempotent migration backfills. SCIM for a
lookup-ID adapter can't be set up until the backfill finishes, nor while a live
user of the provider has no recorded lookup ID, since SCIM would provision that
person as a new user. While SCIM is connected, a sign-in that would create an
identity without a lookup ID is refused for the same reason.

**SCIM-created Entra users get a pending identity that sign-in claims.** Its
provider user ID is `OBOT_SCIM_PENDING/<object ID>`. When a sign-in finds no
identity for `hash(sub)`, and the provider's adapter binds by lookup ID, Obot
reads the object ID from `/obot-get-user-info` outside the transaction. It then
looks up the pending identity by its pending key, so there is at most one
candidate, and re-keys and re-encrypts it in place to `sub`. The user's username
becomes `sub`, so every Entra user looks the same whichever path created them.

**Sign-in and SCIM serialize per person.** A SCIM create and a first sign-in of
the same person can't find each other's identity by the key each knows, since
one keys by the pending key and the other by `sub`. On Postgres, both take a
transaction-scoped advisory lock keyed by the auth provider and the lookup ID,
so they can't both create an identity and a user for that person. SQLite's
single connection already serializes them.

**Groups bind by `externalId`, with deterministic IDs.** Adapters declare a
`GroupKey`. With Entra's `GroupKeyExternalID`, a pushed group binds to, or is
created as, `entra/<object ID>`, the ID that directory synchronization already
uses. Once bound, a group's `externalId` can't be removed or changed to one that
identifies another group. Bound group names stay unique per connection for
every adapter.

**SCIM-first is chosen by an explicit parameter.** An adapter without directory
parameters declares a `SCIMChoiceParameter` (Entra:
`OBOT_ENTRA_AUTH_PROVIDER_SCIM`). The configuration form sends it as `"true"` to
choose SCIM. `CheckConfiguration` reads it and always drops it, so it is never
stored and the provider never sees it. Its absence means directory
synchronization, which keeps every existing Entra configuration valid.

**Shared SCIM code stores and returns `addresses` and the enterprise extension.**
They are kept in the binding's encrypted profile, patched (including filtered
paths such as `addresses[type eq "work"].locality`), returned, and projected per
RFC 7643 and RFC 7644, and used for nothing else. This applies to every
provider. Shared code also gained `externalId` list filters for users and
groups, and stores each group's `externalId` as sent.

**Entra user `DELETE` deletes the user.** An adapter `Rules` struct holds
flags that only toggle shared behavior. Entra sets `DeletesUsers`, so `DELETE
/Users/{id}` disables the user, retires the binding, and deletes the user as the
admin API does, in one transaction. Unlike the admin API, it doesn't require the
SCIM binding to be deactivated first. It refuses Obot's last Owner, and its last
Admin when there is no other Owner, with a `409`. The cleanup of the user's identities, sessions, API keys, agents, vMCPs,
and MCP servers is a `deleted` event in the user lifecycle outbox, which the admin
API now also uses. Okta and OneLogin still answer `501`.

**Group members are replaced at bind, as for other providers.** Entra's empty
create drops a bound legacy group's members until Entra's `Add` PATCHes arrive.

## Rationale

**Bind by object ID rather than re-key every Entra identity.** Re-keying all
identities from `sub` to the object ID would migrate every Entra install, SCIM
or not, and change usernames. Reading `oid` from the ID token at sign-in would
need changes to oauth2-proxy state, `auth-providers-common`, and the proxy,
because Obot's serialized proxy state drops the ID token. The extra
`/obot-get-user-info` call happens only on an Entra user's first sign-in.

**Deterministic group IDs rather than name matching.** The object ID is always
sent, survives renames, and is already the ID of every directory-synchronized
group, so migration keeps role assignments and policy subjects without the
duplicate-name problems of name binding.

**Store the extra attributes rather than drop them.** Entra re-sends every
mapped attribute that the read-back lacks, as `Add` operations, each time it
evaluates a user, as capture run 004 showed
(`pkg/scim/testdata/phase0/entra-run-004.json`). Dropping them would add a PATCH
per affected user on every evaluation, and show those users as updated in
Entra's provisioning logs every time.

**Delete users rather than deactivate them.** Entra expects `DELETE` to remove
the user, and sends it on permanent deletion, and automatically 30 days after a
soft delete. Deactivating instead would keep the rule that Okta and OneLogin
follow, but leave Entra's intent unfulfilled, and every deleted user's account
for an Obot admin to delete by hand. Admins who want to keep the data
clear **Delete** under the user mapping's **Target object actions**, and Entra
then sends nothing. OneLogin keeps `501`, because its delete action also fires
when a user's app access is removed.

**Accept the member gap at bind.** Keeping legacy members as unconfirmed until
an `Add` confirmed them would close the gap, but needs a shared membership
column and Enforce logic. The gap affects only migration from directory
synchronization, and the Enable review and documentation warn about it.

## Consequences

- "Shared SCIM code should not need to change" no longer holds. Adding Entra
  generalized the adapter interface (`UserKey`, `GroupKey`,
  `NativeUserIDFromExternalID`, `NativeGroupIDFromExternalID`,
  `SCIMChoiceParameter`, `Rules`, and the native group ID in `NewGroupID`), the
  identity and group binding schemas, the sign-in path, and user deletion. A
  future provider whose IDs don't fit these keys may again need shared changes.
- Sign-in has a claim path that runs for every provider whose adapter binds by
  lookup ID. It must stay behind that adapter check.
- While an Entra SCIM connection exists, a user's first sign-in fails if
  `/obot-get-user-info` fails or reports no object ID. Before, that failure was
  only logged.
- The rule that Obot never deletes users over SCIM, which the
  [OneLogin ADR](./2026-10-16-onelogin-auth-provider.md) kept, is now per
  adapter. An Entra
  user who is soft-deleted and not restored loses their Obot account and data
  about 30 days later, with no Obot admin involved. The documentation and setup
  steps name the setting that prevents it.
- During migration, members of bound groups lose group-based access until
  Entra's first cycle adds them back, which takes longer for larger groups. The
  loss is not only temporary: each removed membership raises a user group
  change, which deletes the user's connections to shared vMCPs they no longer
  match a profile of, removes tools from their selections that their remaining
  profiles don't grant, and prunes their personal vMCPs of catalog entries they
  can no longer access, deleting a personal vMCP left empty. This includes
  access that came through a role the group gave. The Enable review and
  documentation advise adding those users individually to vMCP profiles and MCP
  access policies before enabling. Access that came through nested groups ends,
  because Entra provisions only direct members.
- Unassigning a group in Entra deletes it once SCIM is enforced, with its role
  assignments and policy subjects. Reassigning it restores the same group ID,
  but not those grants. vMCP profiles keep their group subjects, so they grant
  access again, though what members lost when the group was emptied isn't
  restored. Okta and OneLogin groups that SCIM creates get new IDs, so this is
  new with Entra.
- Entra quarantines the provisioning job after two failed cycles, which can be
  as little as about 20 minutes of Obot answering `503` or being unreachable. A
  quarantined job leaves quarantine only when a later cycle, which can be up to
  a day away, succeeds, or when an admin selects **Restart provisioning**.
  Obot's error hints and the SCIM page say so.
- `addresses` and the enterprise extension are now part of every provider's
  SCIM user schema.

## References

- [Provision users and groups from Microsoft Entra ID with
  SCIM](../docs/docs/configuration/entra-scim.md)
- [`pkg/scim/adapter/entra.go`](../pkg/scim/adapter/entra.go): the Entra adapter
- [`pkg/scim/adapter/adapter.go`](../pkg/scim/adapter/adapter.go): the adapter
  interface, `UserKey`, `GroupKey`, and `Rules`
- [`pkg/gateway/client/identity.go`](../pkg/gateway/client/identity.go): the
  claim of a pending identity at sign-in
- [`pkg/gateway/client/identitymigration.go`](../pkg/gateway/client/identitymigration.go):
  the lookup ID backfill
- [`pkg/gateway/client/scimuser.go`](../pkg/gateway/client/scimuser.go): user
  binding and `DeleteSCIMUser`
- [`pkg/gateway/client/scimgroup.go`](../pkg/gateway/client/scimgroup.go): group
  binding by `externalId`
- [`pkg/scim/setup/configuration.go`](../pkg/scim/setup/configuration.go): the
  SCIM choice parameter
- [`pkg/scim/entra_replay_test.go`](../pkg/scim/entra_replay_test.go): replay of
  the captured Entra traffic
- [2026-10-08: Provision Okta users and groups through
  SCIM](./2026-10-08-okta-scim-provisioning.md)
- [2026-10-16: Add OneLogin as an auth provider with SCIM
  provisioning](./2026-10-16-onelogin-auth-provider.md)
- [Microsoft: How application provisioning works in Microsoft Entra
  ID](https://learn.microsoft.com/en-us/entra/identity/app-provisioning/how-provisioning-works)
