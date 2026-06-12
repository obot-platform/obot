---
status: draft
---

# OIDC JWT Authenticator

## Overview

This design covers a generic JWT authenticator that Obot uses to accept JWTs issued by the configured generic OIDC provider. Pairs with the existing `generic-oauth-auth-provider` (one configured OIDC issuer per Obot deployment, browser-login flow already designed in [`../generic-oauth-provider/`](../generic-oauth-provider/README.md)). The same trust anchor — the provider's issuer URL and JWKS — is reused for service-identity and per-user JWTs presented at Obot's existing API endpoints. No new endpoints are added; the integration is a pure authentication extension.

Obot is the JWT consumer in v1. The configured OIDC issuer (e.g. Studio) is the JWT minter. Symmetric Obot→issuer JWT trust (Obot signing JWTs back to the issuer) is not required and is not covered here.

### Consumers

The first consumer is Studio's MCP integration:
- Studio repo, `feat/mcp-catalog-support` branch, `docs/design/mcp-studio-obot-transport/` (Studio's JWT mint helpers and outbound Obot client).
- Studio repo, `docs/design/mcp-studio-runtime/` (catalog enablement, HTTP API, UI).
- Studio repo, `docs/functional/configure-connectors/` (canonical functional spec).

Future consumers configuring a different OIDC provider follow the same shape — the design and code stay generic.

## Design Scope

**Covers**

- Accepting JWTs from the configured `generic-oauth-auth-provider` at Obot's existing auth boundary, validated through the provider's JWKS.
- One subject shape in v1: every JWT's `sub` is a user identity. Studio is, in effect, an MCP client of Obot identical in pattern to Claude Desktop — Claude Desktop authenticates with Obot API keys; Studio authenticates with Studio-signed JWTs. Same identity model, different credential format.
- Reading a configured eligibility claim (`studio_eligible` for the Studio integration; the claim name is configurable for future providers) and refusing calls when the claim is absent or false.
- Reading a configured roles claim (`studio_roles` for the Studio integration) and mapping it to Obot's existing groups: if the claim contains `vibedata_owner`, the caller is treated as `[Admin, Owner]`; otherwise the caller is an authenticated Obot user. Unrecognized roles map to `[Authenticated]` only.
- Resolving or creating the Obot user record from `{ issuer, sub }` — same identity mapping the OAuth provider's browser flow uses.
- A `pkg/oidcjwt/` package that implements the authenticator and a thin `go-oidc` verifier wrapper.
- One additive change to the existing authenticator union (`pkg/services/config.go`).

**Does not cover**

- Issuer-side JWT minting, JWKS publishing, or outbound client — those live in the consumer's repo. For Studio, see `docs/design/mcp-studio-obot-transport/` in the Studio repo.
- Multiple simultaneously-configured OIDC providers. Obot's existing `generic-oauth-auth-provider` is single-instance; this design inherits that constraint.
- Symmetric callback flows (Obot signing JWTs to call the issuer).
- New service-identity catalog or provisioning endpoints. Obot's existing `pkg/api/handlers/systemmcpcatalogs.go`, `mcp.go`, and per-user MCP-server endpoints cover what consumers need.
- Per-user API keys, broker, or any other key-storage mechanism — the JWT is the credential, minted fresh per call by the consumer.

## Key Decisions

| Decision | Rationale |
|---|---|
| Reuse Obot's existing API endpoints — add no new HTTP routes for this integration | Obot already exposes `GET /api/system-mcp-catalogs/{catalog_id}/entries`, `POST/PUT/DELETE /api/system-mcp-servers/*`, and per-user `/api/mcp-servers/*` paths. The integration needs no new endpoint shape — only the ability to authenticate using a JWT and be authorized at those paths. |
| Plug into the existing authenticator union (`pkg/services/config.go`), not a parallel authz layer | Obot composes its auth from a union of `authenticator.Request` implementations. Adding one more authenticator to that union is the minimum-surface, rebase-friendly integration point. Authz rules stay in `pkg/api/authz/authz.go` unchanged. |
| Map JWTs to Obot users via the existing identity layer, then derive groups from the configured roles claim | Same `{ issuer, sub }` mapping the browser-login flow uses. The roles claim (e.g. `studio_roles`) is read per call and mapped to Obot's existing `Admin`/`Owner`/`Authenticated` groups — `adminAndOwnerRules` in `pkg/api/authz/authz.go:26` is reached by mapping `vibedata_owner` → `[Admin, Owner]`. No new authz rules needed. |
| Trust the configured eligibility claim; do not call the issuer per request | The issuer (Studio) is responsible for emitting `studio_eligible: false` on JWTs minted for ineligible users. Obot's per-call check is the second layer. JWT TTL bounds the staleness window. No Obot→issuer callback required in v1. |
| Reuse the existing OIDC provider's issuer URL and JWKS discovery | One trust anchor per deployment. The authenticator reads the issuer URL from the same configuration that the browser-login flow uses and uses OIDC discovery for JWKS. |
| Configure the audience, eligibility-claim name, and roles-claim name via env vars piggybacking on the existing `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*` prefix | Single configuration surface. Operators configure one provider; the JWT authenticator picks up its fields from the same place. No backend-principal env var — there is no backend-principal subject. |
| Confine all integration code to `pkg/oidcjwt/` | Single new package keeps the upstream-merge surface minimal. Authn/authz upstream files stay unchanged in structure; the only modification is one additive line in the authenticator union. |
| Use `github.com/coreos/go-oidc/v3` for OIDC discovery, JWKS caching, key rotation, and standard claim validation | This avoids maintaining custom JWKS cache and verifier code in the fork. `github.com/golang-jwt/jwt/v5` remains in use only for the unverified issuer pre-check that preserves authenticator-union fall-through semantics. |

## Architecture / How It Works

### Components

| Component | Responsibility |
|---|---|
| Configured generic OIDC provider | Single-instance `generic-oauth-auth-provider` registry entry. Provides the issuer URL, JWKS discovery, and the OAuth client used for browser login. Existing — see [`../generic-oauth-provider/`](../generic-oauth-provider/README.md). |
| Verifier wrapper (`pkg/oidcjwt/verifier.go`) | Uses `go-oidc` for OIDC discovery, JWKS caching, key rotation, and standard claim validation. Adds a pre-check that parses `iss` without verification so JWTs for other issuers can fall through to the rest of the authenticator union. |
| JWT authenticator (`pkg/oidcjwt/authenticator.go`) | Implements `authenticator.Request`. Inspects the `Authorization: Bearer …` header; if present and validates, resolves or creates the Obot user via the existing identity layer, checks the eligibility claim, then maps the roles claim to Obot groups: `vibedata_owner` (or whichever role string is configured as admin-mapped) → `[Admin, Owner]`; other recognized roles → `[Authenticated]`; missing / empty roles + `studio_eligible: true` → `[Authenticated]`. Returns a `user.Info` carrying the Obot user + derived groups. |
| Configuration (`pkg/oidcjwt/config.go`) | Typed config struct populated from existing env / chart values. Holds issuer URL, audience, eligibility-claim name, roles-claim name, and the role-to-admin-mapping table (default: `vibedata_owner` → admin/owner). |

### Where it plugs into Obot

Four additive touch points only. All other code lives in `pkg/oidcjwt/`.

| File | Change |
|---|---|
| `pkg/services/config.go` | One additive change: append a `oidcjwt.NewAuthenticator(...)` instance to the `authenticators` union just before `authn.NewAuthenticator(authenticators)` is called. |
| `chart/values.yaml` | Add `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE`, `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME`, `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME`, and `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES` (comma-separated, default `vibedata_owner`) under the existing top-level `config:` map. The chart already renders `config:` through a secret and `envFrom`. |
| `go.mod`, `go.sum` | Add `github.com/coreos/go-oidc/v3`. |
| `docs/studio/CHANGES.md` (new) | Manifest tracking every upstream touchpoint for rebase hygiene. |

A `scripts/check-upstream-touchpoints.sh` script (new) verifies CI catches any unexpected upstream touches introduced during execution. The expected files above are the allow-list.

### JWT validation order

For each incoming request with an `Authorization: Bearer …` header:

1. Parse the bearer as a JWT; on parse failure, return `(nil, false, nil)` so other authenticators in the union can try.
2. Parse the JWT without verification and read `iss`. If it differs from the configured issuer, return `(nil, false, nil)` so other authenticators can try.
3. Hand the JWT to `go-oidc` for signature, issuer, audience, expiry, and not-before validation.
4. If `go-oidc` validation fails after the issuer pre-check matched, return an error. This is a real auth failure and surfaces as `401 Unauthorized` from the API server's wrapper.

Authenticator-union semantics in Obot: each authenticator returns `(response, ok, err)`. `(nil, false, nil)` means "not mine, try the next." A real error surfaces as 401. This authenticator returns a real error only when the token is structurally ours (matching `iss`) but fails validation.

### Subject resolution and group mapping

After successful validation, all JWTs go through one path:

1. **Resolve or create the Obot user.** Use the same generic OAuth identity key the browser flow uses: `ProviderIssuer = <issuer>` and `ProviderUserID = "iss:<issuer>\x00sub:<sub>"`. If absent, create the record using the JWT's `email`, `email_verified`, `preferred_username`, `name`, and `picture` claims where present. Username selection follows the generic provider: prefer `preferred_username`, then `name`, then `email`, then `sub`.
2. **Check the eligibility claim.** Configured name (default `studio_eligible`). If absent or falsy, return a real error (401).
3. **Derive groups from the roles claim.** Configured name (default `studio_roles`); configured admin-mapping list (default `["vibedata_owner"]`). If the JWT's roles claim intersects the admin-mapping list → groups = `[types.GroupAdmin, types.GroupOwner, types.GroupAuthenticated]`. Otherwise → groups = `[types.GroupAuthenticated]`. The existing `adminAndOwnerRules` in `pkg/api/authz/authz.go` accepts the admin-mapped caller on `/api/system-mcp-catalogs/**`, `/api/system-mcp-servers/**`, `/api/mcp-catalogs/**` without further changes.
4. **Return `user.Info`** from the resolved/created Obot user with the derived groups. Include `auth_provider_user_id` in `Extra` for downstream setup/OAuth flows — same pattern the browser-flow authenticator uses.

### Eligibility claim

The claim name is configured (default `studio_eligible`). Recognized shapes:

- Boolean: `true` passes; `false`/missing fails.
- Array of strings: non-empty array passes; empty/missing fails (forward-compatible).

Boolean is the v1 default for the Studio integration; the validator handles arrays transparently to keep the contract flexible for future providers.

### Roles claim

The claim name is configured (default `studio_roles`). Expected shape: array of role strings. The authenticator intersects the roles claim with the configured admin-mapping list (default `["vibedata_owner"]`) to decide whether to add `[Admin, Owner]` groups. Unrecognized roles are ignored — they do not deny the call (the eligibility claim is the deny gate).

### Identity mapping with the existing OIDC provider

The existing `generic-oauth-auth-provider` (browser flow) normalizes the configured issuer by trimming trailing slashes, then creates Obot users keyed by `ProviderIssuer = <normalized issuer>` and `ProviderUserID = "iss:<normalized issuer>\x00sub:<sub>"` through the identity layer at `pkg/gateway/client/identity.go`. This authenticator reuses the same lookup-or-create call and exact provider-user-ID shape. Order of events is not important: a service-identity JWT may create the Obot user record before any browser login, or vice versa. Subsequent events for the same user resolve to the same record.

Email and display fields are display values only, not identity keys.

### Configuration

All values piggyback on the existing `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_*` env-var prefix so operators configure one provider, one trust anchor.

| Env var | Purpose |
|---|---|
| `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER` (existing) | Issuer URL. Drives OIDC discovery → JWKS URL, etc. |
| `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE` (new) | Audience Obot expects on JWTs. Empty disables this authenticator entirely (browser-login still works). |
| `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME` (new) | The claim name read for eligibility gating. Default `studio_eligible`. |
| `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME` (new) | The claim name read for group mapping. Default `studio_roles`. |
| `OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES` (new) | Comma-separated list of role values that map to `[Admin, Owner]`. Default `vibedata_owner`. |

If `AUDIENCE` is empty, the authenticator is inert (validation fails fast for any presented JWT). This lets deployments that don't use service-identity JWTs keep the authenticator wired without behavioral effect.

## States & failure cases

| Situation | Behavior |
|---|---|
| No `Authorization: Bearer …` header | Authenticator returns `(nil, false, nil)`; next authenticator in union tries. |
| Bearer is not a JWT (different scheme) | Same as above. |
| JWT `iss` does not match configured issuer | Return `(nil, false, nil)` — let other authenticators try. |
| JWT `iss` matches but `kid` is unknown after `go-oidc` refresh | Return real error — surfaces as 401. |
| JWT signature invalid for the configured issuer | Return real error — 401. |
| JWT `aud` does not match configured audience | Return real error — 401. |
| JWT expired (`exp` in the past, beyond skew) | Return real error — 401. |
| JWT with roles claim including a value in the admin-mapping list (default `vibedata_owner`) | `user.Info` with `[Admin, Owner, Authenticated]` groups; existing authz accepts on admin-scoped paths. |
| JWT with roles claim containing only non-admin values, or empty | `user.Info` with `[Authenticated]` groups; existing authz rejects admin-only endpoints with 403. |
| Admin-mapped JWT hitting a non-admin endpoint | Existing authz rejects with 403 — same behavior as any admin user hitting an unrelated path. |
| Non-admin JWT hitting an admin endpoint | Existing authz rejects with 403. |
| JWT missing eligibility claim | Return real error — 401 (we know the JWT is "ours" because it validated). |
| JWT with `studio_eligible: false` | Same — 401. |
| JWT for an `{ issuer, sub }` not in the Obot user table | Create the user record (same shape as browser-login first-time path); proceed. |
| `AUDIENCE` env var empty | Authenticator returns `(nil, false, nil)` for any JWT (effectively disabled). Other authenticators still run. |

## Tests

- Unit: verifier wrapper (valid token, different issuer fall-through, wrong audience real error, custom claim extraction).
- Unit: subject resolution and group mapping. JWT with `studio_roles: [vibedata_owner]` → admin/owner groups. JWT with `studio_roles: [domain_contributor]` or empty → authenticated only. Eligibility-claim missing or false → 401. New user record created on first call.
- Unit: authenticator union semantics (returns `(nil, false, nil)` for non-JWT bearers and JWTs with a different issuer; returns error for "ours" but invalid).
- Integration: spin up a test issuer (signed with a static keypair), present a JWT with `studio_roles: [vibedata_owner]` to `GET /api/system-mcp-catalogs/{catalog_id}/entries`, assert 200 and a catalog response shape. Present a JWT with `studio_roles: []` to the same endpoint, assert 403.

## Open questions

None remaining for v1. Earlier open items (claim shape, conversation-token strategy, retry curves) are resolved or owned by consumer-side docs.

## Cross-repo references

| Topic | Location |
|---|---|
| First consumer: Studio's MCP integration | `~/src/studio` (Studio repo), branch `feat/mcp-catalog-support`, docs under `docs/design/mcp-studio-obot-transport/` and `docs/design/mcp-studio-runtime/` |
| Studio's functional spec | Studio repo, `docs/functional/configure-connectors/` |
| Studio's JWT minting + outbound client design | Studio repo, `docs/design/mcp-studio-obot-transport/` |
| Provider compatibility source | Providers repo, `~/src/providers/generic-oauth-auth-provider/`; Google/GitHub provider images remain provider-specific OAuth proxy flows and are not issuers for this generic JWT authenticator. |

## Source file map (planned)

| File | Purpose |
|---|---|
| `pkg/oidcjwt/config.go` (new) | Typed config struct. Reads env vars; validates ranges. |
| `pkg/oidcjwt/verifier.go` (new) | `go-oidc` provider/verifier wrapper plus issuer pre-check. |
| `pkg/oidcjwt/authenticator.go` (new) | Implements `authenticator.Request`. Composes config + verifier wrapper + identity layer. |
| `pkg/oidcjwt/authenticator_test.go` (new) | Unit tests. |
| `pkg/oidcjwt/integration_test.go` (new) | Integration test against a real Obot router with a test issuer. |
| `pkg/services/config.go` (modify) | One additive line: append `oidcjwt.NewAuthenticator(...)` to the authenticators union. |
| `chart/values.yaml` (modify) | Add new env keys under the existing top-level `config:` map. |
| `docs/studio/CHANGES.md` (new) | Manifest of upstream touchpoints. |
| `scripts/check-upstream-touchpoints.sh` (new) | Verifies upstream-touchpoint allow-list. |
