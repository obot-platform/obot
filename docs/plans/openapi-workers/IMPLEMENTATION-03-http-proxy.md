# I03 HTTP proxy implementation

- Date: 2026-11-10 Asia/Shanghai.
- Baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.
- Shared contract: `FROZEN-1.0`; I02R2 operation metadata completion evidence reviewed before implementation.
- Result: **verified for the I03 handler/server/authz slice; integration E2E remains blocked on merging I02's executor package into this checkout**.

## Changes

- Added `pkg/api/handlers/apiproxy/`:
  - strict canonical VMCP/component/raw-route parsing;
  - component-scoped static-precedence operation matching with unique-match and ambiguity rejection;
  - 405/404 classification and method `Allow` generation;
  - exact raw path and raw query forwarding;
  - protected/credential/forwardable header separation;
  - business status/Header/body preservation, including multi-value Header and `Set-Cookie`;
  - 1 MiB request and 8 MiB response boundaries;
  - SSE-only request rejection, HEAD response handling, and platform JSON errors with request ID/retry fields.
- Added `pkg/api/authz/apiproxy.go` and tests:
  - dedicated API-proxy authorization entry;
  - requires `GroupMCP`, canonical VMCP ID, current-user connection/profile access, and API-key scope when present;
  - delegates component/operation narrowing to the frozen I02 resolver semantics.
- Updated `pkg/api/authz/resources.go` only to route API-proxy requests into that dedicated check.
- Updated `pkg/api/server/server.go`:
  - adds a pre-mux raw `/api-proxy/` branch that bypasses ServeMux cleaning and `headersResponseWriter`;
  - keeps authentication, rate limiting, activity recording, and audit logging;
  - suppresses platform session cookies and platform API cache/security headers from business responses;
  - emits API-proxy platform JSON errors without redirecting or exposing sensitive internal errors.

## Verification

All commands ran from this worktree.

```sh
rtk gofmt -w pkg/api/authz/*.go pkg/api/handlers/apiproxy/*.go pkg/api/server/server.go pkg/api/server/apiproxy_test.go
rtk go test ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server
rtk go vet ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server
rtk git diff --check
rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh
```

Results:

- `go test`: **PASS — 644 tests in the three affected packages**.
- `go vet`: **PASS**.
- `git diff --check`: **PASS**.
- Formal verifier: **PASS** for the normal model, expected failure for Broken, and both expected chart counterexamples.

Detailed non-sensitive evidence: `evidence/I03-VERIFICATION.md`.

## Formal model sync

- Skill: `.codex/skills/verify-openapi-http-proxy/SKILL.md`.
- Model/config: `formal/OpenAPIHTTPProxy.tla` and `formal/OpenAPIHTTPProxy.cfg`.
- Model sync commit remains `2be04f025d1c86144e8670d9e9341b263edc28ea`.
- No matching state-machine behavior changed: implementation still uses strict raw-segment parsing, component/method identity, static precedence plus ambiguity rejection, all three whitelist layers, and raw path/query execution. Therefore no model mutation was required.

## Boundaries and remaining work

- This checkout does not contain I02's unmerged `pkg/openapiworker` package, so router/service construction to the concrete Worker executor and a real fake-upstream E2E cannot be compiled here without crossing feature worktrees or copying unmerged I02 files.
- The I03 handler uses the frozen Describe/Execute semantics and I02R2 `method/pathTemplate` metadata; integration should adapt its local Executor interface to `openapiworker.Executor` after I02 is merged.
- No real JWT, historical token, real business API, production deployment, commit, push, branch, or worktree was used.
