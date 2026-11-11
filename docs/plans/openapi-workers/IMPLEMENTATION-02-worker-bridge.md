# I02 OpenAPI Worker Bridge Implementation

Date: 2026-11-09 Asia/Shanghai. Status: **I02R passed for settlement, with I03 interface unblock** — I02 now speaks the delivered I01 Python Worker wire protocol, real-process describe/execute interoperability passes with synthetic credentials and a fake business upstream, and authorized operation descriptions expose method/path/parameter metadata for I03 routing.

## Inputs and write set

- Baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.
- Freeze: 00 `FROZEN-1.0`, G00 completion, local C01 appendix, P02 `PLAN.md` / `REVIEW.md`, and review closure evidence.
- The default I02 write set was followed for protocol/backend/bridge/service handler/version files.
- 00 explicitly approved only two minimal extensions by reply `msg_b1ed1f1b26d1`: `pkg/mcp/backend.go` adds the `openapi-worker` constant and `pkg/mcp/manager.go` adds the minimal `NewSessionManager` switch branch. No other manager/backend behavior was changed.
- I02R additionally applied 00 reply `msg_52f88cb590a0`: only I01 `workers/openapi/openapi_worker/snapshot.py` and `workers/openapi/tests/test_snapshot.py` were changed to allow canonical `authorization` as a credential exception. Forwardable `Authorization` and every other protected name remain rejected.
- No branch/worktree, commit, push, real JWT, historical credential, production service, Kubernetes cluster, or original workspace was used.

## Implemented

- Added `pkg/openapiworker` with:
  - Principal-bearing `Reference`, `Invocation`, `Executor`, typed errors, bounded request/result types, and typed lease context.
  - Canonical manifest generation, schema/execution digests, strict path matching, protected/forwardable/credential header classes, and snapshot envelopes.
  - Fixed Worker HTTP client with production TLS + mTLS requirements, localhost HTTP only in dev mode, optional second-factor bearer header, JSON-only responses, redirect rejection, absolute millisecond deadlines, and one `snapshot_required` describe recovery.
  - I01 wire envelope `manifestBase64` + `policyBase64` + `schemaBase64`, paged describe responses, nested `input.kind`, typed lease fields, top-level business credentials, and list-shaped response headers.
  - OpenAPI header parameters become forwardable policy entries; protected names are excluded except exact credential `Authorization`, whose exact-instance decrypted value is applied last by the Worker.
  - Per-call resolver for API key scope, current VMCP/component/instance ownership, Profiles, instance grants, tool overrides, exact user-instance credential context, and hashed budget source identity.
  - Authorized operation descriptions additionally expose canonical `method`, `pathTemplate`, and parameter binding metadata while retaining `operationKey`; this is a public metadata extension only and does not alter Worker execute semantics.
  - Executor that re-resolves authorization/credentials for every describe/execute, injects credentials last, sends snapshots, preserves business results, and performs no execute retry.
- Added `pkg/mcp/worker_backend.go`:
  - `openapi-worker` backend selection without Docker client or kubeconfig initialization.
  - OpenAPI/remote/VMCP/composite remain non-hosted; NPX/UVX/containerized management and logs return `ErrNotSupportedByBackend`.
- Added JSON-only MCP OpenAPI bridge in `pkg/api/handlers/mcpgateway/openapi_bridge.go`:
  - `initialize`, `tools/list`, and `tools/call` are stateless JSON responses.
  - SSE-only Accept and GET are rejected as JSON errors; no `/messages` or SSE endpoint is introduced.
  - Parent VMCP component requests use the authenticated/hydrated principal and the same resolver/executor path.
- Added `Services.OpenAPIWorkerExecutor` handoff and machine-readable `/api/version` `hostingCapabilities`.
- Added regressions for backend boundaries, unavailable Docker/Kubeconfig startup, mTLS success/negative handshakes, digest/path/header vectors, exact caller and same-user second instance isolation, missing credentials, disabled tools, JSON-only MCP behavior, and no execute retry.
- Source drift detection increment (2026-11-10, user request "自动探测上游 schema 变化但不自动生效"): added read-only periodic probing so upstream OpenAPI changes (added/removed interfaces, parameter and schema field changes) become visible without ever taking effect.
  - `pkg/openapi/drift.go`: `ProbeSourceDrift` compiles the stored snapshot and a freshly imported source schema into canonical operation manifests through the same `BuildManifest` and diffs the operation sets. Reports bounded `DriftReport` (added/removed/changed operation keys, schema digests); no schema bodies, URLs, or credentials in the report.
  - `v1.MCPServerCatalogEntryStatus.OpenAPISourceDrift`: new status block (`Detected`, `LastProbedAt`, `NextProbeAt`, digests, per-direction operation key lists, bounded `ProbeError`). Status only; snapshot mutation and republishing remain the explicit administrator flow.
  - `mcpservercatalogentry.ProbeOpenAPISourceDrift` handler on the leader-elected router: skips non-OpenAPI entries, inline content sources, and snapshotless entries; probes through the credential-free SSRF-protected importer with a 30s bound; records fetch/parse/compile failures as `ProbeError` instead of crashing; reschedules itself via `RetryAfter` with a 6h default interval and respects `NextProbeAt` backoff. Wired in `pkg/controller/routes.go` with the same importer construction the save flow uses.
  - Inline content sources are classified as non-driftable by design: the content is the configuration. Header configs do not participate because they shape the policy document, never the diffed operation signatures; both sides compile under identical empty header config.
  - Tests: `pkg/openapi/drift_test.go` (no-change, added+removed+changed, description-only change, missing inputs) and `openapidrift_test.go` (skip conditions, status persistence, reschedule, backoff, fetch failure, unconfigured importer). Same file set mirrored to the 05 integration worktree; both suites pass.

## Formal verification

- Skill: `.codex/skills/verify-openapi-worker-bridge/SKILL.md`.
- Command: `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh`.
- Restored model: PASS, 18 distinct states; all required invariants and deadlock checks hold.
- Broken model: expected failure, 19 distinct states; only `ExactCallerCredentialOnly` fails, with the required cross-user credential fallback counterexample.
- The formal artifact still documents its original two-user/two-instance boundary. D28 same-user multi-instance, disabled component/tool, and exact-instance behavior are therefore recorded as a model gap covered by mandatory implementation regressions in `pkg/openapiworker/resolver_test.go`; the model itself was not changed in I02.

## Verification

See `docs/plans/openapi-workers/evidence/I02-VERIFICATION.md` for command-by-command evidence.

## I02R interop finding

- The original I02 mismatch is resolved: snapshot, describe, execute, lease, credentials, digest, path matching, and header policy now use the delivered I01 wire shapes.
- `TestRealI01PythonWorkerDescribeAndExecute` launches the actual I01 `uvicorn` process on a random localhost port (not a Go fake), starts a random-port TLS fake business upstream, cold-describes the Go-produced snapshot, and executes through the Python Worker.
- The real request forwards `X-Business`, ignores caller-supplied `Authorization`, injects only the exact synthetic instance credential, binds `/items/42`, and returns the fake business body through I01's list-shaped headers/result envelope.
- I01's `Authorization` exception is covered by both positive credential policy and negative forwardable-policy tests; no real JWT or business system was accessed.

## Remaining work

- Add real Obot+Worker E2E with unavailable Docker/Kubeconfig, two users, two OpenAPI documents, business errors, large/chunked/non-JSON responses, cancellation, and multi-replica behavior after I04/I05.
- Real enterprise authorization exchange, certificate rotation, Kubernetes deployment, and production access remain `UNVERIFIED`.

## Untracked inventory

All work remains uncommitted as required. New implementation roots are `pkg/openapiworker/`, `pkg/mcp/worker_backend*.go`, and `pkg/api/handlers/mcpgateway/openapi_bridge*.go`. Tracked-file edits are limited to the approved manager/backend wiring, backend regression, service handoff, gateway handler, version capability, and tests. `.codex/` plus preexisting `docs/plans/openapi-workers/` planning/review/formal artifacts remain uncommitted from earlier feature stages.
