---
status: draft
---

# Studio JWT Trust

## Overview

This design covers the Obot-side work to accept Studio as a JWT-issuing trust peer for service and per-user calls into Obot's existing API. Studio holds no Obot-direction credential — every Studio call mints a fresh short-lived Studio-signed JWT validated through Studio's published JWKS. Obot's existing catalog and MCP server endpoints stay as they are; the work in Obot is a pure authentication-and-authorization extension layered onto those endpoints.

Studio-side counterparts (in the Studio repo, `feat/mcp-catalog-support` branch):

- `docs/design/mcp-studio-obot-transport/` — Studio's OIDC publishing, JWKS, JWT mint helpers, outbound Obot client.
- `docs/design/mcp-studio-runtime/` — Studio's catalog enablement model, HTTP API, UI surfaces.
- `docs/functional/configure-connectors/` — the canonical functional spec.

## Design Scope

**Covers**

- Accepting Studio-signed JWTs at Obot's existing auth boundary, validated through Studio's JWKS.
- Two subject shapes in v1: `<Studio backend principal>` for admin work, `<Studio user identity>` for per-user calls.
- Reading the `active_role` claim on user-subject JWTs and refusing calls with absent or empty claims.
- Mapping each subject shape onto Obot's existing endpoint authorization (`pkg/api/authz/`). No new endpoints are added.
- Resolving the Obot user record for a user-subject JWT (creating it on first call if Obot's existing Studio OIDC provider has not yet seen the user).
- The image-curated MCP catalog the running Obot binary ships with.

**Does not cover**

- Studio's JWT mint, JWKS publishing, or outbound client — see the Studio repo, `docs/design/mcp-studio-obot-transport/`.
- Per-user Obot API keys — these are deleted from the design. Obot stores no Studio-managed API keys.
- Symmetric Obot→Studio trust (Obot minting JWTs to call Studio). Not required in v1; can be layered later without a contract change.
- New service-identity catalog or provisioning endpoints. The existing `pkg/api/handlers/systemmcpcatalogs.go`, `mcp.go`, and per-user server endpoints cover what Studio needs.
- The `generic-oauth-provider/` design, which already owns Studio as Obot's OIDC login provider for browser flows.

## Key Decisions

| Decision | Rationale |
|---|---|
| Reuse Obot's existing catalog and MCP server endpoints — add no new HTTP routes for the Studio integration | Obot already exposes `GET /api/system-mcp-catalogs/{catalog_id}/entries`, `POST/PUT/DELETE /api/system-mcp-servers/*`, and per-user `/api/mcp-servers/*` paths. The Studio integration needs no new shape — only the ability to authenticate as Studio and be authorized at those paths. |
| Extend the existing auth path under `pkg/api/authz/` — do not duplicate authz logic | Adding a parallel authz layer would create merge conflicts with upstream changes to authz. Treating Studio JWTs as one more authenticated caller shape, recognized by `Authorizer`, keeps the auth model uniform. |
| Recognize the Studio backend principal as a configured constant, not an Obot user record | Studio's backend must not appear in Obot's user catalog, must not own MCP credentials, and must not be reachable through browser auth. Treating it as a configured principal closes those vectors. |
| Trust the `active_role` claim on user-subject JWTs; do not call Studio per request | Studio refuses to mint a user-subject JWT for an ineligible user; Obot's per-call check of `active_role` is the second layer of defense. JWT TTL (Studio default 60-120s for user-subject) bounds staleness. No Obot→Studio callback required in v1. |
| Map user-subject JWTs to the same Obot user record the existing Studio OIDC provider creates at browser login | The `{ studioIssuer, studioSubject }` tuple is the canonical join key. The OIDC provider already stores this on the Obot user record per `generic-oauth-provider/`. Service-identity calls and browser logins resolve to the same Obot user. |
| Create the Obot user record on first user-subject JWT if it does not yet exist | A user-subject JWT may arrive before the user has done a browser login (e.g., Studio's User Settings panel triggering a registry read). On first sight, Obot creates the user record from the subject claims and proceeds. The next browser login reaches the same record. |
| Confine all integration code to `pkg/studio/` | The historical `2026-06-10-mcp-studio-integration.md` plan established this rebase-friendly placement — a single new package with one small additive change to each of three upstream files. Preserve that for fork maintenance. |
| Curate the MCP catalog at Obot image build time | The Studio deployment ships a Studio-curated Obot image with the supported MCP server set baked into the image's catalog configuration. Adding or removing a supported server is an Obot image release. Studio holds no manifest. |

## Architecture / How It Works

### Components

| Component | Responsibility |
|---|---|
| Studio JWKS validator | Fetches and caches Studio's JWKS from the configured Studio issuer; validates signature and standard claims (`iss`, `aud`, `exp`) on incoming Studio-issued tokens. Used for ID tokens (existing Studio OIDC login flow) and for backend- and user-subject service JWTs. |
| Subject-shape resolver | Inspects the validated `sub`. If equal to the configured Studio backend principal, sets a backend-subject context. Otherwise treats it as a Studio user identity, resolves or creates the Obot user record from claims, sets a user-subject context. |
| `active_role` claim enforcer | On user-subject contexts, refuses the request if the `active_role` claim is absent or empty. Returns `403 forbidden`. |
| Authorization mapper | At each Obot endpoint, asks the request context "is this caller authorized?". Backend-subject is authorized for admin-scoped catalog and system-MCP-server endpoints. User-subject is authorized for the same endpoints a Studio-eligible user would be — scoped to their own resources. |
| Studio configuration | Holds the Studio issuer URL, the Obot audience Studio uses when minting, and the Studio backend principal string. Populated from chart / config. |
| Image-curated catalog | The Obot image's catalog configuration carries the curated set of supported MCP server entries this deployment exposes. The set is fixed for the image's lifetime and changes only with a new image. |

### Where it plugs into Obot

Three upstream touch points, all additive:

| File | Change |
|---|---|
| `pkg/services/config.go` | Register a Studio JWT validator alongside existing auth handlers; provide it the Studio issuer URL and Obot audience. |
| `pkg/api/router/router.go` | Wire the Studio JWT auth handler so it runs on the existing endpoint set. No new route registration. |
| `chart/values.yaml` | Add Studio issuer / audience / backend-principal configuration. |

Everything else lives under `pkg/studio/`:

- `pkg/studio/jwks/` — JWKS fetch, cache, key rotation reactivity.
- `pkg/studio/jwt/` — JWT validation, subject-shape resolution, `active_role` claim check, request-context enrichment.
- `pkg/studio/authz/` — authorization mapping for backend-subject and user-subject onto Obot's existing endpoint scopes.
- `pkg/studio/user/` — Obot user record resolution / creation from Studio claims (joining with the Studio OIDC provider's storage).
- `pkg/studio/config/` — typed configuration struct, env binding, validation.

A `docs/studio/CHANGES.md` manifest tracks every upstream touchpoint for rebase hygiene. A `scripts/check-upstream-touchpoints.sh` script flags unexpected upstream touches introduced during execution.

### JWT validation

A Studio-signed JWT is validated against Studio's published JWKS in this order:

1. Parse the token; reject if not a JWT or unparseable.
2. Fetch the public key from Studio's JWKS using the token's `kid` header. JWKS is cached with a poll interval; cache misses trigger a refresh.
3. Verify the signature.
4. Verify standard claims: `iss` matches the configured Studio issuer; `aud` includes the configured Obot audience; `exp` is in the future; `nbf` (if present) is in the past; clock-skew tolerance capped at 60 seconds.

Validation failure short-circuits with `401 unauthorized` and a JSON error body carrying a stable error code (`studio_jwt_invalid_signature`, `studio_jwt_expired`, `studio_jwt_wrong_audience`, etc.) so the Studio side can map outcomes deterministically.

### Subject-shape resolution

After successful validation, the resolver inspects `sub`:

- **Backend-subject (`sub == <Studio backend principal>`).** Sets a backend-subject context on the request. The authorization mapper allows this context on admin-scoped catalog and system-MCP-server endpoints; any per-user route requires a user-subject and is refused.
- **User-subject (`sub == <Studio user identity>`).** Looks up the Obot user record by `{ studioIssuer, studioSubject }`. If found, sets a user-subject context for that user. If not found, creates the Obot user record from the subject claims (`sub`, `email`, `name`, `picture` when present) — the same shape Obot's existing Studio OIDC provider uses at browser login — and sets the user-subject context. Either way, enforces the `active_role` claim next.

Two subject shapes are accepted in v1. A token whose `sub` matches neither shape is refused with `401 unauthorized` (`studio_jwt_unknown_subject`).

### `active_role` claim enforcement

On user-subject contexts, the request is refused with `403 forbidden` (`studio_active_role_missing`) when:

- The `active_role` claim is absent.
- The `active_role` claim is present but parses to falsy / empty.

On backend-subject contexts, the `active_role` claim is not consulted.

The claim's exact value shape (boolean vs. list of role strings) is captured in the Studio mint helper contract (`docs/design/mcp-studio-obot-transport/` in the Studio repo). Obot reads it via a small adapter so the shape can evolve without rewriting authz logic.

### Authorization mapping

Studio integration adds no new authz rules to upstream Obot. It adds one decision layer that maps each request context onto existing authz scopes:

| Context | Treats as | Allowed endpoint families |
|---|---|---|
| Backend-subject | Admin-or-owner | `/api/system-mcp-catalogs/**`, `/api/system-mcp-servers/**`, `/api/mcp-catalogs/**`, `/api/mcp-servers/{id}/oauth-url` (the backend-fetches-connect-URL path) |
| User-subject (with `active_role`) | The corresponding Obot user | The user-scoped endpoint families the same user would reach through browser auth: `/api/mcp-servers/**` (user-scoped), `/api/mcp-server-instances/**`, `/api/mcp-servers/{id}/oauth` (disconnect), and other per-user MCP surfaces |
| User-subject (without `active_role`) | Refused with `403` before reaching any endpoint |

The exact endpoint paths and the authz rules they hit today live in `pkg/api/router/router.go` and `pkg/api/authz/authz.go`. This design intentionally does not duplicate that list; the mapping is "if backend-subject, treat as admin/owner; if user-subject, treat as that Obot user," and the rest follows from existing rules.

### Studio JWKS reactivity

Obot's cache of Studio's JWKS:

- Refreshes on a configurable poll interval (default 5 minutes).
- Refreshes on demand when a JWT presents a `kid` not in the current cache (single inflight refresh per `kid`).
- Tolerates Studio publishing both old and new keys during rotation (the validator picks the key matching `kid`).

This works the same for user-login ID tokens (already validated by the existing Studio OIDC provider) and for backend- and user-subject service JWTs.

### Image-curated MCP catalog

The Obot image ships pre-configured with the curated MCP server entries this deployment is allowed to expose. The image's catalog configuration enumerates each entry with `serverId`, display name, description, configuration-required hint, and any metadata Obot needs to provision it.

The catalog set is fixed for the lifetime of the running Obot image. Adding or removing a supported MCP server is an Obot image release; Studio does not need an image bump. Studio observes catalog membership changes through its existing reads of `/api/system-mcp-catalogs/{catalog_id}/entries`. Studio's organization enablement state may carry entries that are no longer in the image's catalog after an image bump; Studio's catalog page handles those (it deletes the orphan enablement row on the Vibedata Owner's next visit).

### Identity mapping with the existing Studio OIDC provider

The `generic-oauth-provider/` design already creates Obot user records for Studio-authenticated browser logins. This design uses the same `{ studioIssuer, studioSubject }` tuple as the join key. The order of events is not important: a user-subject service JWT may create the Obot user record before any browser login, or a browser login may create it before any service call. Either way, the next event for the same user resolves to the same Obot user record.

Email and display fields are not identity keys; they are display values. A Studio-side email change does not change the Obot user record's identity.

## Server provisioning lifecycle

Studio's catalog admin page asks Obot to provision or tear down a catalog entry via Obot's existing system-MCP-server endpoints. The Obot-side states the integration depends on are the same ones the system-MCP-server lifecycle already exposes:

| State | Meaning |
|---|---|
| Registered | The entry exists in Obot's image-curated catalog. |
| Provisioning | Obot is starting the underlying MCP server. |
| Ready | The MCP server is reachable through Obot's MCP Gateway. |
| ProvisionFailed | A provisioning attempt failed. |
| TearingDown | Obot is removing the underlying MCP server. |
| Removed | The entry is gone (Studio disable + teardown complete, or a new Obot image dropped the entry). |

Studio reads these via its `listCatalogEntries` and adjacent calls. Obot does not invent new states for this integration.

## Failure cases

| Situation | Expected behavior |
|---|---|
| Studio JWKS endpoint unreachable | Obot serves requests from its JWKS cache until cache expiry; refuses further requests once the cache is empty and refresh has been failing. Standard `503` with a stable error code (`studio_jwks_unreachable`). |
| JWT signed with a key not in Studio's JWKS | Refused with `401 studio_jwt_invalid_signature`. |
| JWT `aud` does not match Obot's configured audience | Refused with `401 studio_jwt_wrong_audience`. |
| JWT expired or not-yet-valid | Refused with `401 studio_jwt_expired`. |
| JWT `sub` matches neither subject shape | Refused with `401 studio_jwt_unknown_subject`. |
| User-subject JWT missing `active_role` | Refused with `403 studio_active_role_missing`. |
| User-subject JWT for a Studio user that has never browser-logged-in | Obot creates the user record from the subject claims and proceeds. |
| Backend-subject JWT hitting a per-user-only endpoint | Refused with `403 studio_backend_subject_not_allowed`. |
| User-subject JWT hitting an admin-only endpoint | Refused with `403` per the existing authz rule for that endpoint. |

## Invariants

- Obot accepts two Studio-signed JWT subject shapes in v1: `<Studio backend principal>` and `<Studio user identity>`.
- Obot stores no Studio-managed API keys; there is no per-user key store on the Obot side.
- The `{ studioIssuer, studioSubject }` tuple is the canonical join key between user-subject JWTs and Obot user records.
- The integration adds no new HTTP endpoints to Obot; it extends auth/authz on existing endpoints.
- All Studio-integration code lives under `pkg/studio/`. The upstream touch surface is three files: `pkg/services/config.go`, `pkg/api/router/router.go`, `chart/values.yaml`.
- Obot does not call Studio in v1. The `active_role` claim is the freshness signal; JWT TTL bounds staleness.
- The MCP catalog is curated at Obot image build time. The running image's catalog set is fixed for its lifetime.

## Configuration

| Configuration | Source |
|---|---|
| Studio issuer URL | `OBOT_STUDIO_ISSUER` (or chart equivalent). |
| Obot audience for Studio-issued JWTs | `OBOT_STUDIO_AUDIENCE` (or chart equivalent). |
| Studio backend principal value | `OBOT_STUDIO_BACKEND_PRINCIPAL` (or chart equivalent). |
| Studio JWKS poll interval | `OBOT_STUDIO_JWKS_POLL_SECONDS`, default 300, bounded 60-3600. |
| JWT clock-skew tolerance | Compiled-in default 60 seconds. |

These configuration keys are also referenced from the Studio side (`mcp-studio-obot-transport/`) so the deployment can validate Studio and Obot agree on issuer / audience / principal.

## Relationship to existing Obot designs

| Spec | Relationship |
|---|---|
| `docs/design/generic-oauth-provider/` | Already designs Studio as an OIDC login provider for browser flows. This design extends the trust to backend and per-user service JWTs through the same JWKS. |

## Cross-repo references

| Topic | Location |
|---|---|
| Studio JWT minting and outbound client | Studio repo, `docs/design/mcp-studio-obot-transport/`, branch `feat/mcp-catalog-support` |
| Studio catalog application layer | Studio repo, `docs/design/mcp-studio-runtime/`, branch `feat/mcp-catalog-support` |
| Studio functional spec | Studio repo, `docs/functional/configure-connectors/`, branch `feat/mcp-catalog-support` |

## Key source files

| File | Purpose |
|---|---|
| `pkg/api/router/router.go` | One additive change: wire the Studio JWT auth handler so it runs on the existing endpoint set. No new routes. |
| `pkg/api/authz/authz.go` | Existing authz rules; this design adds the subject-shape → existing-scope mapping layer in `pkg/studio/authz/`. |
| `pkg/services/config.go` | One additive change: register the Studio JWT validator alongside existing auth handlers. |
| `chart/values.yaml` | One additive change: Studio issuer / audience / backend-principal config. |
| `pkg/studio/` (new) | All integration code lives here. See [Where it plugs into Obot](#where-it-plugs-into-obot). |
| `docs/studio/CHANGES.md` (new) | Manifest of upstream touch points for rebase hygiene. |
| `scripts/check-upstream-touchpoints.sh` (new) | Flags unexpected upstream touches introduced during execution. |

## Open questions

1. **JWKS cache eviction during Studio downtime.** Current proposal: serve until cache expiry, then refuse with `503`. Alternative: extend cache indefinitely as long as keys are not rotated. Decision needed before implementation.
2. **`active_role` claim shape.** Boolean vs. list of role strings. The Studio mint helper currently emits a tightly-scoped role list; Obot's adapter should remain shape-agnostic. Document the wire format with the Studio side at impl time.
3. **Backend-subject access to per-user state.** The connect-URL fetch is backend-subject (Studio asks on the user's behalf with the user id in the request body). Verify Obot's existing `oauth-url` endpoint accepts a user id in the body, or whether a small change is needed there.
