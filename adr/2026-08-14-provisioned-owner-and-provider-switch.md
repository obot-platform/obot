# 2026-08-14: Provision the first owner and prove a replacement auth provider before it serves logins

- **Status:** Accepted
- **Date:** 2026-08-14
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7565](https://github.com/obot-platform/obot/issues/7565) — set up a trial with an initial owner and a password setup link, without asking the user to configure an auth provider first.

## Related ODPs

- [2026-08-14: Set up the first Local auth owner from deployment settings](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-14-provisioned-local-owner-activation/README.md)

## Context

Provisioned Obot environments need to give a predetermined owner access without exposing the general bootstrap credential or requiring that owner to configure an identity provider first. Allowing an unauthenticated caller to select an email and set its password would permit account takeover, and treating an environment-provided password as permanent would leave reusable credentials in provisioning state.

Those environments start on Local auth and are expected to move to an external provider, which makes changing providers a normal step rather than an edge case. Obot serves logins from one configured auth provider. Replacing it by deconfiguring the old one and configuring the new one leaves an interval where none is configured, and a misconfigured replacement makes that interval permanent: nobody can sign in, and no signed-in session remains to correct it. The replacement also needs an owner, and an identity under a new provider does not exist until someone signs in through it, so the outgoing owner cannot grant a role to it in advance.

## Decision

Obot can provision one initial Local-provider owner from an email and high-entropy setup secret. It stores only the secret's hash and binds it to that local user. The emailed URL carries the secret in its fragment; the UI removes the fragment and exchanges the secret for an HTTP-only session. A setup session is marked as requiring a password change, and authorization restricts it to its own profile, password completion, UI assets, and sign-out. The setup link remains usable until it expires or password completion succeeds, so interrupted setup can resume. Completion atomically transitions the account out of its pending state, clears the setup-secret hash, invalidates every other session, and preserves only the completing session; the first concurrent completion wins. A pending secret can be rotated from deployment configuration, but an unchanged secret cannot have its absolute expiration extended and a completed account cannot be rearmed or reset that way. When initial-owner provisioning is configured, bootstrap-token generation and authentication are disabled.

A replacement provider's settings are staged into a separate credential context rather than the provider's own. Provider configuration is read from the provider's own context, so a staged provider is not configured, does not serve logins, and does not appear as the current login provider. Staging, discarding, and switching are submitted as `ProviderConfigurationChange` objects under one fixed name, so concurrent changes serialize and the one-configured-provider and one-staged-provider rules are evaluated under that serialization rather than in the API handler. A one-time login through the staged provider is authorized only for the owner who started the switch, and the staged provider is loginable only for a request presenting that verification. The identity it returns is granted the Owner role and recorded in the existing temporary setup user cache. Activation requires that record to name the staged provider, then promotes the staged credential before deconfiguring the outgoing provider, in one reconcile. Deconfiguring the provider that is currently serving logins is refused.

## Rationale

A bearer setup capability supports a one-click provisioning email while keeping the owner email insufficient to claim the account. A URL fragment avoids sending the secret in HTTP request targets and common access logs. Keeping the capability resumable until password completion avoids stranding a user who closes the browser or loses the first session; backend restrictions limit those sessions to completing setup. Hashing, expiration, rotation, and revocation reduce the impact of database disclosure or stale provisioning state.

Keeping the staged settings out of the provider's own credential context means nothing else has to learn about staging: every existing reader of "which provider is configured" continues to give the right answer during a switch, and a restart mid-switch leaves the outgoing provider serving. Verification is recorded rather than inferred from the caller's session. An earlier revision gated activation on the request's own auth provider, which could not survive a refresh, could not be read back to drive the interface, and disagreed with anything shown to a second administrator. The temporary setup user cache already stores exactly this — which identity signed in through which provider — with a lifetime that matches a pending switch, so the switch reuses it instead of introducing a parallel record.

The Owner grant happens at verification because that is the first moment the identity exists, and from that point the browser is signed in as it rather than as the owner who started the switch. Deferring the grant to activation strands the switch: the browser returns as an ordinary user and cannot reach the control that would promote it. The grant is safe there because the callback requires the identity to have come from the staged provider, and a login through the staged provider is only authorized for the owner who started the verification. Promoting the credential before deconfiguring means a failure in the second half leaves the outgoing provider configured, which is the state an operator can recover from.

## Consequences

Provisioners must generate a unique, high-entropy random secret, store it as a deployment secret, and construct the activation URL. Anyone possessing an unexpired setup link can race to complete owner setup, so the delivery channel remains security-sensitive. Obot must preserve the restricted-session authorization boundary for future APIs and must never treat deployment configuration as account recovery after setup completes.

One replacement may be staged at a time, and re-staging discards any verification recorded against the previous settings, because that verification no longer describes what activation would promote. Switching between any two providers uses this path, including switching back to Local. A switch does not move Local users or their work: the identity the replacement provider returns is a new Obot user, so anything the outgoing provider's users set up stays with those accounts. Deconfiguring the last configured provider is no longer possible through the API, so an operator who wants no auth provider at all must change deployment configuration instead.

The temporary setup user cache now backs two flows. A bootstrap owner confirmation and a provider switch cannot be in progress at the same time, which is acceptable because both establish the first owner for a provider Obot is about to depend on, but it is a coupling future work has to respect.

## References

- [Enabling authentication](../docs/docs/installation/enabling-authentication.md)
- [Auth providers](../docs/docs/configuration/auth-providers.md)
