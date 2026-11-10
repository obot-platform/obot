# 2026-11-10: Sign in the initial Local owner when bootstrap setup creates it

- **Status:** Accepted
- **Date:** 2026-11-10
- **Supersedes:** None
- **Superseded by:** None

## Related issues

<!-- TODO: link the GitHub issue for this change. -->

## Related ODPs

None.

## Context

During bootstrap setup, the bootstrap user creates the first Local account by choosing its email and password. Setup then started a temporary login through the Local provider, so the same person immediately had to type back the password they had just chosen before `/api/setup/oauth-complete` could promote the account to Owner.

## Decision

`POST /api/setup/local-owner` creates the first Local account and, in the same request, starts a Local session for that account and sets its HTTP-only session cookie. It requires bootstrap mode to be enabled, a bootstrap-authenticated caller, and the Local provider to be the configured provider. It uses the existing one-account bootstrap creation limit, never requires a password change, and only issues a session for the account created by that request. The response tells the browser to navigate to `/api/setup/oauth-complete`, which authenticates the request as the new Local user ahead of the bootstrap credential and applies the existing promotion, bootstrap cookie removal, and redirect. `POST /api/local-auth/users` is unchanged.

## Rationale

The bootstrap credential already grants full administrative access, and the caller chose the account's password, so a session for that account grants nothing the caller does not already have. Placing the endpoint under `/api/setup/` keeps it unavailable once setup finishes, rather than adding a session-issuing create path to the general Local user API. Promotion is left to `oauth-complete` because the gateway user for the Local identity does not exist until a request is authenticated as it, and reusing that endpoint keeps one implementation of the promotion guards. Having the UI submit the Local login form with the password it still held was rejected: failures come back as redirects to the login page instead of API errors.

## Consequences

Any future change that lets bootstrap create more than one Local account, or create accounts outside setup, must not extend this endpoint to issue sessions for them. If the account is created but the session is not, the UI falls back to the existing temporary-login step, so the owner can still finish setup by signing in.

## References

- [Provisioned owner and provider switch ADR](2026-08-14-provisioned-owner-and-provider-switch.md)
- `pkg/api/handlers/setup/local_owner.go`
- `pkg/api/handlers/setup/oauth_complete.go`
