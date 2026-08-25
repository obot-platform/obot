# 0001: Provision the initial local owner with a restricted setup capability

- **Status:** Accepted
- **Date:** 2026-08-14
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#4604](https://github.com/obot-platform/obot/issues/4604) — predecessor bootstrap-first provider setup flow

## Related ODPs

- [ODP 0001: Secure activation for a provisioned Local-auth owner](https://github.com/obot-platform/obot-design-proposals/tree/main/proposals/0001-provisioned-local-owner-activation)

## Context

Provisioned Obot environments need to give a predetermined owner access without exposing the general bootstrap credential or requiring that owner to configure an identity provider. Allowing an unauthenticated caller to select an email and set its password would permit account takeover. Treating an environment-provided password as permanent would also leave reusable credentials in provisioning state.

## Decision

Obot can provision one initial Local-provider owner from an email and high-entropy setup secret. It stores only the secret's hash and binds it to that local user. The emailed URL carries the secret in its fragment; the UI removes the fragment and exchanges the secret for an HTTP-only session.

A setup session is marked as requiring a password change. Authorization restricts it to its own profile, password completion, UI assets, and sign-out. The setup link remains usable until it expires or password completion succeeds, so interrupted setup can resume. Completion atomically transitions the account out of its pending state, clears the setup-secret hash, invalidates every other session, and preserves only the completing session; the first concurrent completion wins. A pending secret can be rotated from deployment configuration, but an unchanged secret cannot have its absolute expiration extended and a completed account cannot be rearmed or reset that way.

When initial-owner provisioning is configured, bootstrap-token generation and authentication are disabled. The configured email is assigned the Owner role through the existing explicit-role mechanism.

## Rationale

A bearer setup capability supports a one-click provisioning email while keeping the owner email insufficient to claim the account. A URL fragment avoids sending the secret in HTTP request targets and common access logs. Keeping the capability resumable until password completion avoids stranding a user who closes the browser or loses the first session; backend restrictions limit those sessions to completing setup. Hashing, expiration, rotation, and revocation reduce the impact of database disclosure or stale provisioning state.

## Consequences

Provisioners must generate a unique, high-entropy random secret with at least 32 characters (the documented example uses `openssl rand -hex 32`), store it as a deployment secret, and construct the activation URL. Length alone is not a substitute for entropy. Anyone possessing an unexpired setup link can race to complete owner setup, so the delivery channel remains security-sensitive. Obot must preserve the restricted-session authorization boundary for future APIs and must never treat deployment configuration as account recovery after setup completes.

## References

- [Local auth implementation](../pkg/localauth/)
- [Authentication setup documentation](../docs/docs/installation/enabling-authentication.md)
