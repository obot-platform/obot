# I03 verification evidence

- Started / finished: 2026-11-10 08:20–08:40 Asia/Shanghai.
- Worktree: `/Users/echo/orca/workspaces/obot/openapi-http-proxy`.
- Credential policy: unit/static/model evidence only; no real or historical JWT, no real business API, no credential values in evidence.

## Affected Go tests

Command:

```sh
rtk go test ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server
```

Actual result:

```text
Go test: 644 passed in 3 packages
```

Coverage:

| Scenario | Result |
| --- | --- |
| `%2F`, `%2E`, duplicate slash, trailing slash, invalid encoding, control-character paths | **verified rejected** |
| Exact escaped path and raw query forwarded unchanged | **verified** |
| Static route precedence and ambiguous template rejection before execute/body read | **verified** |
| Same path method mismatch returns 405 + `Allow`; missing path returns 404 | **verified** |
| Browser/API-key/wildcard scope authorization | **verified allowed** |
| Wrong key scope, other user's instance, missing MCP group, non-canonical VMCP | **verified rejected** |
| Request body over 1 MiB and worker response over 8 MiB | **verified rejected without business body** |
| SSE-only Accept rejected; mixed JSON/SSE allowed | **verified** |
| Business status, binary body, multi-value Header, `Set-Cookie` preserved | **verified** |
| Platform security/default headers, no-cache headers, and refreshed session cookies excluded from business response | **verified** |
| Platform error carries `code`, safe message, `requestID`, `retryable`, and `X-Obot-Error-Code`; HEAD has no body | **verified** |

## Static checks

Commands and results:

```text
rtk go vet ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server
PASS

rtk git diff --check
PASS
```

## Formal verification

Command:

```sh
rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh
```

Environment:

- TLA+ Tools v1.8.0, SHA-256 `e1f11b75c4b5bc0478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`.
- Java Corretto 11.0.31.
- Bun statechart runner 1.3.14.

Actual results:

- Normal model: PASS; 613 states generated, 241 distinct states, 0 states left on queue.
- Broken model: expected violation of only `ExecutionRequiresUniqueAuthorizedMatch`; 21 states generated, 21 distinct states, 8 states left on queue.
- Normal chart: PASS; 17 states, 29 steps, 7 drawn states, 9 arrows.
- Broken chart: PASS with expected unique-route and whitelist-bypass counterexamples; 20 states, 35 steps, 7 drawn states, 11 arrows.
- Full evidence: `../formal/runs/latest/EVIDENCE.md`.

## Not verified here

- Full Obot process / fake-upstream / real browser E2E: **not run**, because I02's concrete `pkg/openapiworker` implementation exists only in its separate unmerged worktree and this checkout cannot compile router/service integration without copying it.
- PostgreSQL global budget and K8s multi-replica behavior: I04/I05 scope; no deployment attempted.
- UI and public user docs: no UI/doc behavior changed in this slice.
