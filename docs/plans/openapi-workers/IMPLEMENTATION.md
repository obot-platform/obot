# I01 Shared OpenAPI Worker Implementation

Date: 2026-11-09 Asia/Shanghai. Status: **partial** — I01 core implementation and local verification complete; cross-feature/deployment acceptance remains open.

## Scope And Contract

- Worktree: `/Users/echo/orca/workspaces/obot/openapi-shared-worker`.
- Branch: `feature/openapi-shared-worker`; baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.
- Freeze input: 00 `FROZEN-1.0`, G00 completion, C01/C02 freeze appendices, P01 `PLAN.md`, P01 `REVIEW.md`, and the review issue matrix.
- Approved write set respected: only `workers/openapi/**` plus this feature's `docs/plans/openapi-workers/**` implementation/evidence files were created. No branch/worktree, commit, push, root Makefile, Go, UI, chart, or original workspace change was made.

## Implemented

- Added the Python package under `workers/openapi/` with pinned dependencies and lock.
- Implemented canonical snapshot envelope parsing, length-prefixed `executionDigestV1`, schema digest equality, canonical JSON rejection, header policy validation, fixed compile helper process protocol, multi-snapshot LRU, active references, same-digest compile deduplication, and bounded compile queue.
- Implemented four JSON-only internal routes (`describe`, `execute`, `healthz`, `readyz`); `/mcp` and `/ensure` do not exist.
- Implemented requestID/digest binding, `snapshot_required` with worker instance ID, one accepted recovery attempt, invalid-resupply rejection, and one-shot execution fence before upstream dispatch.
- Implemented typed Worker lease checks, monotonic remaining deadline, strict raw path matching, top-level HTTP operation binding, protected/forwardable/credential header classes, credential prefix checks, no redirect following, SSE/upgrade/trailer rejection, bounded body/header reads, and business result preservation.
- Implemented DNS all-answer policy checking and pinned-IP transport construction with original Host/SNI; production startup requires mTLS materials, with explicit localhost test HTTP only.
- Added Docker build and `.dockerignore`.
- Dashboard increment (2026-11-10, user request "worker 需要链接上运维看板"): added a fifth route `GET /metrics` on the same authenticated mTLS listener, rendering Prometheus text format v0.0.4 with zero new dependencies (`openapi_worker/metrics.py`). Gauges: snapshot cache entries/bytes/limits, active references, pending compilations, compile total. Counters: internal requests by route/outcome, bounded-vocabulary errors by route/code, upstream results by status class/outcome, snapshot lifecycle events. Histograms: request latency by route, cold compilation latency. Label cardinality is bounded to fixed vocabularies (routes, known error codes, status classes, booleans); request IDs, digests, URLs, header names, and credential values never become labels. Tests in `tests/test_metrics.py` cover exposition format, counter correctness across describe/execute, upstream uncertain classification, and a no-leak assertion for request IDs/credentials/digests. The metrics test fixture also repairs a pre-existing cross-test isolation defect: stale compile-helper subprocesses from earlier tests were bound to a closed/different asyncio loop and poisoned later cold compiles; the fixture now reaps stale helpers and resets the registry, and `CompileHelperPool.compile`'s lazy `start()` restarts them in the current loop. Same file set mirrored to the 05 integration worktree (`openapi-k8s-acceptance/workers/openapi/`), where the full suite (28 tests) also passes.

## Files

Primary implementation:

- `workers/openapi/openapi_worker/snapshot.py`
- `workers/openapi/openapi_worker/compiler.py`
- `workers/openapi/openapi_worker/compiler_helper.py`
- `workers/openapi/openapi_worker/registry.py`
- `workers/openapi/openapi_worker/operations.py`
- `workers/openapi/openapi_worker/executor.py`
- `workers/openapi/openapi_worker/httpclient.py`
- `workers/openapi/openapi_worker/app.py`
- `workers/openapi/pyproject.toml`
- `workers/openapi/uv.lock`
- `workers/openapi/Dockerfile`
- `workers/openapi/.dockerignore`
- `workers/openapi/.gitignore`

Tests and documentation:

- `workers/openapi/tests/**`
- `workers/openapi/README.md`

Evidence:

- `docs/plans/openapi-workers/evidence/I01-VERIFICATION.md`
- `docs/plans/openapi-workers/formal/EVIDENCE.md`
- `docs/plans/openapi-workers/formal/evidence/base/`
- `docs/plans/openapi-workers/formal/evidence/broken/`

## Verification

All commands ran from `workers/openapi` unless noted.

- `uv run pytest -q`: **verified** — 22 passed in 0.45s; 2 non-failing deprecation warnings from installed uvicorn/websockets.
- `uv lock --check`: **verified** — 76 packages resolved from lock.
- `python -m compileall -q openapi_worker tests`: **verified**.
- `git diff --check`: **verified**.
- Fixed credential/artifact scan: **verified** — no fixed synthetic credential string.
- `docker build -t obot-openapi-worker:local .`: **verified** — image `sha256:aaee615dbc40cded87238f0fd07badcda048b5ecfd913d18890258561d72820c`.
- From repository root, `rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh`: **verified** — restored 33 distinct states, all rules hold; broken 57 distinct states with required fence/no-retry violations.

Detailed command results and behavioral assertions: `docs/plans/openapi-workers/evidence/I01-VERIFICATION.md`.

## Formal Model Status

- Skill: `.codex/skills/verify-openapi-shared-worker/SKILL.md`.
- Model remains synchronized in event semantics. Implementation preserves exactly one accepted recovery attempt, requestID/digest binding, and one execution fence/upstream attempt; no invariant was weakened.
- Canonical bytes/SHA equality is intentionally a code-level boundary per C01/D15 and is covered by snapshot tests, not claimed as TLA coverage.

## Remaining Work

- I02 must provide authoritative Go canonical manifest/snapshot bytes and share cross-language digest/path/header vectors.
- Full file-based production egress exceptions (exact host + ports + paired CIDRs), real certificate rotation, and actual mTLS positive/negative handshakes remain for I05.
- Real Obot bridge, PostgreSQL lease/admission, external HTTP proxy, multi-replica K8s, restart/eviction E2E, and full 8 MiB/chunk/compression matrix are not verified here.
- No real enterprise authorization, business API, historical JWT, production deployment, commit, or push was used.

## Handoff

- Worker implementation is uncommitted and ready for 00 review.
- No shared contract was changed.
- Integration consumers should use the four documented routes and the typed lease/error/result envelope in `workers/openapi/openapi_worker/app.py` and `workers/openapi/openapi_worker/executor.py`.

## Untracked Inventory

`git status --short` reports no tracked-file modification and three untracked roots: `.codex/`, `docs/plans/`, and `workers/`.

| Group | Path / selection | Origin |
| --- | --- | --- |
| I01 implementation | All files under `workers/openapi/` listed above, including `.gitignore` and `.dockerignore` | Created by this task; no generated `.venv`, pytest cache, or bytecode is included. |
| I01 handoff | `docs/plans/openapi-workers/IMPLEMENTATION.md`, `docs/plans/openapi-workers/evidence/I01-VERIFICATION.md` | Created by this task. |
| Refreshed formal output | `docs/plans/openapi-workers/formal/EVIDENCE.md` and final `formal/evidence/{base,broken}/` reports/charts | Formal artifacts predated I01; this task refreshed evidence with the required rerun. |
| Preexisting feature planning/review artifacts | `.codex/skills/verify-openapi-shared-worker/**`, `docs/plans/openapi-workers/{PLAN.md,REVIEW.md,formal/**,review-evidence/**}` | Produced by P01/F01/R01 before this implementation dispatch and left uncommitted as required. |
