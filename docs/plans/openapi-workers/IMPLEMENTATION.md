<<<<<<< HEAD
<<<<<<< HEAD
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
=======
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
>>>>>>> feature/openapi-worker-bridge
=======
# I04 接口预算与租约实施

日期：2026-11-10 09:10 Asia/Shanghai。状态：partial——I04 预算/租约模块、迁移、有界组件、形式化模型和隔离真实 PostgreSQL 验证完成；业务入口接线和两 Obot/两 Worker E2E 按依赖留给 I02R/I03/I05。

## 基线与写集

- 分支 / HEAD：`feature/openapi-execution-budgets` / `2be04f025d1c86144e8670d9e9341b263edc28ea`。
- I04 写集：`pkg/openapi/executionbudget/**`、`pkg/gateway/types/openapi_execution_budget.go`、`pkg/gateway/db/openapi_budget_migration.go`、`pkg/gateway/db/openapi_budget_migration_test.go`、`pkg/gateway/db/db.go` 的三行接入、本目录实施/证据/formal/skill 产物。
- 未修改共享契约、业务入口、其他 feature PLAN、原工作区、全局配置或集群；未 commit/push。
- 除 `pkg/gateway/db/db.go` 为修改外，其余交付文件均为未跟踪文件；`docs/plans/openapi-workers/review-evidence/**` 是本任务开始前已存在的规划审核证据，本次未修改。

## 变更范围

- 配置校验与默认值：平台/池/接口、队列、deadline、fencing/reclaim、终态保留和请求/响应/Header/审计字节边界。
- Canonical budget identity：结构化 source URL/entryKey 规范化、RFC 百分号编码、dot-segment 处理和 SHA-256 source/operation/budget hash；缺失或多义引用 fail closed。
- PostgreSQL 状态：预算/operation 配置、三层运行态、执行请求行、唯一键、状态/时间索引、CHECK、默认行、platform/pool 种子和并发迁移 advisory lock。
- 生命周期：短事务入队、platform→pool→operation 锁序、公平 admission、waiting 即时取消、release token 哈希、typed outcome、snapshot miss 保留租约、不确定结果保守 grace 回收、计数重算、运行态删除和终态双上限清理。
- Typed Worker lease：协议字段全部为 Unix millisecond 数字，包含 issued/deadline/fence/dispatched、budget hash、clock skew 和 fencing margin。
- 有界组件：请求/上游/内部 envelope 读取、gzip/deflate 压缩与解压双限、HTTP 写出、MCP base64 内容、审计 Header/正文双限和动态脱敏。
- Formal model：完整模型和三个 chart 模型补入 waiting cancel、same-instance snapshot recovery，以及 `SnapshotRecoverySafety`；保留原有 early release / stale start 反例。

## 已验证结果

详见 `docs/plans/openapi-workers/evidence/I04-VERIFICATION.md` 和 `docs/plans/openapi-workers/formal/EVIDENCE.md`。

## 实施中修复的问题

- 并发 PostgreSQL DDL 迁移竞态、非法默认种子、operation pool CHECK、platform/pool runtime 缺行。
- Release/Cancel 原先的行锁在自动提交语句中无效；现已与状态更新、计数重算和清理放进同一短事务。
- runtime operation state 创建与空闲删除存在竞态；已移动到入队事务内。
- 同一 Manager 的 admission 事务现在串行化；所有状态事务默认 250ms deadline。
- typed lease JSON 时间从 Go time 字符串修正为冻结契约要求的 Unix millisecond 数字。
- 补齐 `snapshot_required` 保留 active 租约的 typed outcome 和模型行为。
- source root/default port/percent encoding 规范化、pool-platform 配置关系和审计 Header 硬上限。
- LimitedReader 现在越过上限立即失败；gzip/deflate 同时限制压缩输入和解压输出。

## 剩余工作与边界

- I02R 消费 typed lease/outcome；I03 将 admission coordinator 和有界响应/审计组件接入 HTTP/MCP 入口。
- I05 需要两 Obot/两 Worker/真实 PostgreSQL/假上游验证真实取消传播、Worker lost、重启恢复、扩缩容、全局峰值和部署行为。
- 本任务验证是代码、迁移、模型和隔离 PostgreSQL 流程；不是业务 API E2E、Kubernetes 部署或生产验收。
- SQLite 仅保留为仓库既有显式单进程迁移开发路径；`Manager` 对 SQLite/非 PostgreSQL 执行申请仍 fail closed，不得用于全局额度结论。
>>>>>>> feature/openapi-execution-budgets
