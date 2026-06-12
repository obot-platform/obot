---
status: draft
---

# Generic OAuth Provider Support

## Overview

Obot should support one native generic OAuth / OpenID Connect auth provider. The provider appears as a first-class auth provider in the admin UI and API, but it still follows the existing Obot convention of one registry-backed provider instance per provider type.

The goal is Langfuse-style custom provider support: an admin can configure an issuer URL, client ID, client secret, display name, and optional scopes to connect Obot to Entra, Studio, Keycloak, Okta, or another OIDC-compatible identity provider without adding a bespoke provider integration.

## Design Scope

**Covers**

- A single built-in generic provider registry entry named `generic-oauth-auth-provider`.
- API and UI support for configuring the generic provider through the existing auth provider flow.
- OIDC issuer discovery from `/.well-known/openid-configuration`.
- A precise trust model for email/account linking.
- Issuer-bound identity storage so the same provider type can be safely reconfigured.
- A provider daemon contract for OIDC security, profile mapping, validation, and runtime errors.
- Documentation for configuring common OIDC providers.
- Backend, provider/runtime, UI, and route-level tests for provider configuration and login wiring.

**Does not cover**

- Multiple generic provider instances, such as `Studio` and `Entra` configured side by side.
- Dynamic create/delete APIs for auth provider records.
- Vendor-specific group management APIs for every possible OIDC provider.
- Standard OIDC group-claim mapping in v1.
- Non-OIDC OAuth providers that cannot expose a stable issuer discovery document.

## Key Decisions

| Decision | Rationale |
|---|---|
| Add one `generic-oauth-auth-provider` registry entry. | Obot already models providers as registry-backed resources with one instance per provider type. This avoids dynamic provider CRUD and matches the current provider model. |
| Require OIDC issuer discovery. | Issuer discovery lets Obot derive authorization, token, userinfo, and key endpoints from one URL instead of asking admins for brittle endpoint sets. |
| Required fields are display name, issuer URL, client ID, client secret, and email domains. | These match the Langfuse-style configuration model and give admins the minimum operator-controlled inputs for a standard OIDC login. Email domains remain the primary Obot-side access gate. |
| Scope defaults to `openid email profile`. | This is the common baseline for OIDC login and can be overridden for providers that require custom scopes. |
| Make trusted email linking an explicit provider configuration value. | The provider is first-class, but the trust decision should be visible to admins and bound to the configured issuer, not hidden in a hardcoded provider-name allowlist. |
| Persist issuer identity with Obot identities. | OIDC `sub` values are only unique within an issuer. Generic provider identities need issuer binding to avoid collisions after reconfiguration. |
| Preserve the existing one-configured-provider rule. | `AuthProviderHandler.Configure` currently rejects configuring a second provider. This feature adds a provider type; it does not change active-provider cardinality. |
| Defer generic group mapping. | Vendor-specific group APIs belong in dedicated providers. Standard group-claim mapping can be added later with a separate design and tests. |
| Validate the issuer before reporting the provider as usable. | Generic configuration has more typo and compatibility risk than GitHub/Google. Admins need synchronous validation or an explicit test action before login depends on the provider. |

## Architecture

The provider registry remains the source of available auth provider types. Obot loads provider YAML files from configured provider registries and creates `AuthProvider` resources from them. The new generic provider is delivered as one registry entry named `generic-oauth-auth-provider`.

```text
provider registry
  └─ auth-providers/generic-oauth-auth-provider.yaml
        │
        ▼
AuthProvider resource
        │
        ▼
/api/auth-providers list/configure/reveal/deconfigure
        │
        ▼
gateway credential store
        │
        ▼
dispatcher launches generic provider command with env vars
        │
        ▼
generic OIDC provider daemon
        │
        ▼
proxy and gateway complete login through provider daemon
        │
        ▼
identity is stored with provider name + issuer fingerprint + subject
```

The existing auth provider API remains the primary surface. The generic provider appears in `GET /api/auth-providers` with provider metadata and configuration parameters. Admins configure it through `POST /api/auth-providers/generic-oauth-auth-provider/configure`.

The generic provider daemon is a required deliverable. If the provider image already contains a suitable generic OIDC command, the implementation must use its actual environment contract. If it does not, the implementation must add the command to the provider image/repo before the Obot registry entry can be enabled.

## Provider Daemon Contract

The provider daemon owns the OIDC protocol boundary. Obot should not treat it as a black box with unspecified behavior.

The daemon must:

- Read the environment variables defined in the provider registry manifest.
- Fetch and validate the issuer discovery document.
- Verify that the discovery document's `issuer` matches the configured issuer after canonicalization.
- Generate authorization requests with `state`, `nonce`, and PKCE.
- Validate returned `state` and `nonce`.
- Exchange authorization codes using the configured client credentials.
- Validate ID tokens against issuer, audience, signature, expiry, not-before, issued-at, and allowed clock skew.
- Resolve and cache JWKS with rotation support.
- Reject tokens signed by unknown keys or unexpected algorithms.
- Validate userinfo responses when used, including userinfo `sub` consistency with the ID token `sub`.
- Return the provider profile fields required by Obot's existing auth-provider contract.
- Expose `/obot-get-user-info` for profile refresh with the same field semantics.

The daemon should support standard OIDC providers first. Provider-specific knobs such as custom audience, client authentication method, response mode, or private-key JWT are not required in v1 unless the chosen daemon already supports them.

## Provider Configuration

Obot can bootstrap a registry-backed auth provider from startup environment by setting `OBOT_AUTH_PROVIDER_ID` to the provider resource name. The selected provider's registry manifest defines the required and optional configuration environment variables. The generic OAuth / OIDC provider uses `OBOT_AUTH_PROVIDER_ID=generic-oauth-auth-provider`; leaving `OBOT_AUTH_PROVIDER_ID` unset while setting any legacy `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*` variable keeps the same generic OAuth startup behavior for compatibility.

The registry entry should expose these required parameters:

| Field | Environment variable | Notes |
|---|---|---|
| Provider Name | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME` | Display name shown on the login button and in operator-facing provider details. |
| Issuer URL | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER` | Base issuer URL. Must support OIDC discovery. |
| Client ID | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID` | OAuth/OIDC client ID. |
| Client Secret | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET` | OAuth/OIDC client secret. Sensitive. |
| Email Domains | `OBOT_AUTH_PROVIDER_EMAIL_DOMAINS` | Existing shared auth-provider access-control field. Empty values should be rejected for the generic provider. Use `*` only when the admin intentionally allows all domains trusted by the issuer. |
| Trust Email Linking | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING` | Whether this issuer can link logins to existing Obot users by email. Defaults to `true` for the generic provider, but must be re-confirmed when the issuer changes. |

The registry entry should expose these optional parameters:

| Field | Environment variable | Default | Notes |
|---|---|---|---|
| Scope | `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE` | `openid email profile` | Space-delimited scopes. |

The admin UI should continue to display the callback URL in the provider configuration dialog. Operators must add that exact URL to the identity provider's allowed redirect URI list.

The card label should be fixed as `Custom OAuth / OIDC`. The configured provider name should control the login button label.

## API Behavior

The existing auth provider API remains the main contract:

- `GET /api/auth-providers` includes `generic-oauth-auth-provider`.
- `GET /api/auth-providers/generic-oauth-auth-provider` returns the provider metadata and status.
- `POST /api/auth-providers/generic-oauth-auth-provider/configure` stores the generic provider fields only after issuer validation succeeds. If the implementation chooses an explicit test-connection action, the provider must not be reported as usable until that validation succeeds.
- `POST /api/auth-providers/generic-oauth-auth-provider/reveal` reveals saved values to authorized admins. Existing Obot provider reveal behavior returns stored secrets to admins; if redaction is added later it should apply consistently across providers.
- `POST /api/auth-providers/generic-oauth-auth-provider/deconfigure` removes the credential and stops the provider daemon.

The configure path should keep rejecting a second configured provider. Generic OAuth is a new provider type, not a change to the current active-provider cardinality model.

The API should treat trusted email linking as provider configuration, not as a global "add trusted provider ID" API. The stored trust record must include the generic provider ID and the validated issuer identity. Existing GitHub and Google behavior can remain hardcoded initially, but the implementation should prefer a shared helper that can answer "is this provider/issuer trusted for email linking?" instead of growing the static `verifiedAuthProviders` list.

Configuration validation must verify:

- Required fields are present and non-empty.
- The issuer URL is HTTPS unless a dev/test mode explicitly allows HTTP.
- The discovery document is reachable.
- The discovery document issuer matches the configured issuer.
- Required endpoints exist.
- The configured client can produce a valid authorization URL. Client-secret correctness is proven during the interactive authorization callback, unless the chosen daemon supports a safe provider-specific validation probe.
- TLS validation succeeds through the system trust store or an explicitly documented custom CA mechanism.

## Identity and Account Linking

Generic OAuth can be trusted for email/account linking because admins are intentionally configuring the issuer as their login authority. That trust is explicit provider configuration, defaults to enabled for `generic-oauth-auth-provider`, and is scoped to the configured issuer.

Generic identities must be bound to both:

- Provider resource identity: `default/generic-oauth-auth-provider`.
- Issuer identity: canonical issuer URL or a stable issuer fingerprint derived from the validated discovery document.

Current identities are keyed by provider namespace/name and provider user ID. The implementation should add issuer binding for generic OAuth identities, either by adding an issuer/fingerprint field to the identity model or by including a canonical issuer component in the generic provider user ID before hashing. The implementation plan must choose one approach and include migration/backward-compatibility handling.

Account linking rules:

- For generic OAuth, a login can link to an existing Obot user by email only when `trustedForEmailLinking` is enabled for the same provider and issuer trust boundary.
- If the ID token or userinfo response includes `email_verified=false`, Obot must not link by email.
- If `email_verified=true`, Obot can link by email subject to email-domain restrictions and the trust setting.
- If `email_verified` is absent, Obot can link by email only when `trustedForEmailLinking` is enabled; the docs must call this out as an issuer-level trust decision.
- Reconfiguring the generic provider to a different issuer must not allow the new issuer to claim identities or sessions created by the previous issuer.

The provider daemon should return stable identity fields:

- Provider user ID from the OIDC `sub` claim.
- Issuer from the validated OIDC discovery document.
- Email from the OIDC `email` claim.
- Email verification from `email_verified` when present.
- Username/display name from `preferred_username`, `name`, or `email`, in that order.
- Icon URL from `picture` when present.

## Reconfiguration and Sessions

Changing the issuer URL for `generic-oauth-auth-provider` is a trust-boundary change, not a routine credential edit.

When the configured issuer changes, Obot should:

- Stop the existing provider daemon.
- Treat the new issuer as a distinct identity namespace.
- Require the admin to re-confirm `trustedForEmailLinking` for the new issuer before email-based account linking is enabled.
- Avoid linking the new issuer's `sub` values to identities from the previous issuer.
- Invalidate or require reauthentication for sessions created under the previous issuer.
- Emit or preserve enough audit context for operators to understand that the login authority changed.

If this is too large for v1, the UI/API should require deconfigure/reconfigure for issuer changes and warn that existing sessions will be invalidated.

## UI Behavior

Admin -> Auth Providers should show a `Custom OAuth / OIDC` card. The card uses the same configure, modify, reveal, and deconfigure flows as other providers.

The configure dialog should present:

- Provider Name
- Issuer URL
- Client ID
- Client Secret
- Scope
- Email Domains
- Trust this issuer for account linking

The UI should make these constraints visible:

- Email domains are required for generic OAuth.
- `*` allows every email domain accepted by the issuer.
- Trusting account linking lets this issuer attach logins to existing Obot users with matching verified or issuer-trusted email addresses.
- Changing the issuer changes the identity trust boundary.
- Changing the issuer requires re-confirming account-linking trust.
- The callback URL must be copied exactly into the external provider.

The generic provider should sort with the common auth providers. The existing auth-provider page already uses a preferred-order list; adding the generic provider there is sufficient if the current alphabetical fallback does not place it appropriately.

## Error Handling

Configuration should fail clearly when required fields are missing or validation fails. Runtime login should fail with actionable provider errors when:

- The issuer URL is invalid.
- The issuer discovery document cannot be fetched.
- The discovery document issuer does not match the configured issuer.
- The provider lacks required OIDC endpoints.
- The token response cannot be validated.
- ID token validation fails for issuer, audience, signature, expiry, not-before, issued-at, nonce, or algorithm.
- The JWKS endpoint does not contain the signing key.
- Userinfo returns a different `sub` than the ID token.
- The userinfo or ID token response lacks a usable subject or email.
- The email domain restriction rejects the user.
- The provider reports `email_verified=false` for an email-linking attempt.
- TLS validation fails for the issuer, discovery document, token endpoint, userinfo endpoint, or JWKS endpoint.

Discovery and JWKS data can be cached inside the provider daemon, but the daemon must handle retry after startup failure and JWKS rotation. The implementation should define cache TTLs in the provider code or document the daemon library defaults if using a proven OIDC library.

## Documentation

`docs/docs/configuration/auth-providers.md` should add a first-class generic OAuth / OIDC section. The section should explain:

- Required fields.
- Optional scope override.
- Redirect URI configuration.
- Example issuer formats for Entra, Keycloak, Okta, and Studio.
- The requirement for OIDC discovery.
- That Obot supports one configured provider at a time.
- That the generic provider has a `Trust this issuer for account linking` setting, enabled by default for first-class generic provider behavior.
- How `email_verified` affects account linking.
- Why changing the issuer is a trust-boundary change.
- How to configure trust for private/internal issuer certificates, or that the deployment must add the issuer CA to the system trust store.
- When a dedicated enterprise provider is still preferable.

Dedicated providers may still be preferable when Obot needs vendor-specific group APIs, profile pictures, account-state checks, or provider-specific authorization rules.

## Key Source Files

| File | Purpose |
|---|---|
| `pkg/controller/handlers/provider/provider.go` | Loads provider registry YAML files into `AuthProvider` resources. |
| `pkg/storage/apis/obot.obot.ai/v1/authprovider.go` | Defines the stored `AuthProvider` spec and status. |
| `apiclient/types/authprovider.go` | Defines API-facing auth provider types. |
| `pkg/api/handlers/authprovider.go` | Implements list, configure, reveal, and deconfigure auth provider APIs. |
| `pkg/gateway/server/dispatcher/dispatcher.go` | Launches provider commands with stored credential environment variables. |
| `pkg/gateway/types/identity.go` | Defines identity storage and needs issuer binding for generic OAuth. |
| `pkg/gateway/client/identity.go` | Defines verified-provider identity linking behavior. |
| `pkg/gateway/client/user.go` | Maps provider profile responses into user display name and icon fields. |
| `pkg/proxy/proxy.go` | Consumes auth-provider state and provider username/email fields during login. |
| `ui/user/src/routes/admin/auth-providers/+page.svelte` | Renders the auth provider cards and configure/deconfigure flows. |
| `ui/user/src/lib/components/admin/ProviderConfigure.svelte` | Renders provider configuration parameters. |
| `docs/docs/configuration/auth-providers.md` | Documents auth provider setup for operators. |

## Test Strategy

Backend tests should cover:

- The registry loader accepts the generic provider manifest and creates the expected `AuthProvider`.
- Auth provider status reports missing generic fields before configuration.
- Configure/reveal/deconfigure round-trips generic provider credentials.
- Configure validation rejects missing email domains, invalid issuers, issuer mismatch, and unreachable discovery documents.
- The one-configured-provider rule still rejects configuring generic OAuth when another provider is configured.
- Generic OAuth is treated as trusted for identity linking only when `trustedForEmailLinking` is enabled for the same issuer boundary.
- Reconfiguring the generic provider to a different issuer does not allow identity takeover through matching `sub` or email.
- Reconfiguring the generic provider to a different issuer requires re-confirming `trustedForEmailLinking`.
- `email_verified=false` prevents email-based linking.
- Existing sessions are invalidated or forced to reauthenticate when the issuer changes.

Provider/runtime tests should cover:

- OIDC discovery from a fake issuer.
- Discovery issuer mismatch.
- Discovery documents missing required endpoints.
- Authorization URL generation with default and custom scopes.
- `state`, `nonce`, and PKCE handling.
- ID token validation for issuer, audience, signature, expiry, not-before, issued-at, and allowed clock skew.
- JWKS cache refresh and key rotation.
- Userinfo `sub` mismatch with ID token `sub`.
- Token/userinfo handling with `sub`, `email`, `email_verified`, `name`, `preferred_username`, and `picture`.
- Clear failures for invalid issuer, missing discovery endpoints, missing subject, missing email, and `email_verified=false` link attempts.

UI tests should cover:

- The generic provider appears in the auth provider list.
- The configure dialog shows required and optional fields.
- Required-field validation prevents incomplete submission.
- The UI warns when the issuer is changed after prior configuration.
- Reveal populates existing generic provider values without changing existing provider-wide secret reveal semantics.

Documentation verification should use the repo's existing docs build or lint command when available.

## Open Questions

1. `[implementation]` Should issuer binding be stored as a new identity field or encoded into the generic provider user ID before hashing?
2. `[implementation]` If the current provider image lacks a generic OIDC command, which repository owns adding and releasing that command?
3. `[implementation]` Should configure perform synchronous validation, or should the UI add an explicit test-connection action that gates the configured status?
