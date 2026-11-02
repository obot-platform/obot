# 2026-11-02: Stop linking identities by email and stop assigning roles by email

- **Status:** Accepted
- **Date:** 2026-11-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Obot had two features that tied users to their email addresses. At sign-in, a
new identity from Google or GitHub (treated as providers that verify email)
attached to an existing user with the same email whose `verified_email` flag was
true or unset, so one person's Google and GitHub logins became one user.
Separately, `OBOT_SERVER_AUTH_OWNER_EMAILS` and `OBOT_SERVER_AUTH_ADMIN_EMAILS`
gave Owner or Admin to any user with a listed email, through whichever provider
they signed in. Roles granted that way could not be lowered, were flagged as
explicit in the API and UI, and the bootstrap setup flow listed them. The
initial Local-auth owner (`OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL`, see
[2026-08-14](./2026-08-14-provisioned-owner-and-provider-switch.md)) relied on
the same mechanism: its email was added to the owner list.

Neither feature is needed any more. Obot serves logins from one configured auth
provider and supports switching providers through a verified handoff, so
cross-provider linking has no remaining use. An email asserted by an identity
provider is weak grounds for merging accounts or granting privileged roles.

## Decision

- Sign-in never matches an existing user by email. An identity is linked only to
  the user it was created with, and a new identity always creates a new user.
  The `users.verified_email` column is dropped by a migration, and the MCP OAuth
  `userinfo` endpoint no longer returns `email_verified`.
- `OBOT_SERVER_AUTH_OWNER_EMAILS` and `OBOT_SERVER_AUTH_ADMIN_EMAILS` are
  removed, along with everything built on them: the explicit-role lock on
  demotion, the `explicitRole` user field, the
  `GET /api/setup/explicit-role-emails` endpoint, and the setup UI that used
  them.
- The initial Local-auth owner gets Owner once, when its first sign-in through
  the Local provider creates its user. After that it is an ordinary Owner that
  can be demoted, and later sign-ins don't raise it again. Before that first
  sign-in, its pending local account is protected like an Owner's from mutation
  by non-Owners. Canceling a bootstrap temporary login no longer demotes a user
  who is already an Owner.

## Rationale

Granting the initial owner a role once, at creation, keeps the provisioning flow
working. It avoids keeping a general email-to-role mechanism for a single
account. Roles that follow an email are hard to reason about: they apply through
any provider that asserts the address, and they override decisions made in the
UI. Without email matching, a user's role comes only from what is stored on the
user and from group role assignments.

## Consequences

- A person who signs in through two auth providers gets two users.
- Deployments that set the removed environment variables keep roles already
  granted, because roles are stored on the user. Emails on those lists that
  never signed in are no longer elevated. To give a role to users before their
  first sign-in, assign it to an identity provider group.
- Server configuration that still sets the removed variables is ignored.

## References

- `pkg/gateway/client/identity.go`
- `pkg/gateway/client/client.go`
- `pkg/gateway/db/migrations.go`
- [Auth Providers](../docs/docs/configuration/auth-providers.md)
