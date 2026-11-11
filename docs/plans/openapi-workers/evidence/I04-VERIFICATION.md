# I04 verification evidence

Date: 2026-11-10 09:10 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-execution-budgets`. Baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Commands and results

- `rtk go test -count=1 ./pkg/gateway/db ./pkg/gateway/types ./pkg/openapi/executionbudget`
  - **PASS**: 61 tests across 3 packages.
- `rtk go test -race -count=1 ./pkg/openapi/executionbudget`
  - **PASS**: 11 tests; PostgreSQL lifecycle tests skip when the temporary DSN is not exported.
- `rtk go vet ./pkg/gateway/db ./pkg/gateway/types ./pkg/openapi/executionbudget`
  - **PASS**: no diagnostics.
- Direct pinned linter invocation after `rtk make lint` installed v2.14.0 but could not find it on Make's PATH:
  - `rtk proxy sh -c 'lint="$(go env GOPATH)/bin/golangci-lint"; "$lint" run pkg/openapi/executionbudget/... pkg/gateway/db/... pkg/gateway/types/...'`
  - **PASS**: 0 issues.
- Temporary PostgreSQL 16.15 via `initdb`/`pg_ctl` on `127.0.0.1:55432`, random isolated schemas, stopped and deleted by cleanup trap:
  - `OBOT_TEST_POSTGRES_DSN='<isolated-local-postgres-dsn>' rtk go test -race -count=1 -v ./pkg/openapi/executionbudget ./pkg/gateway/db -run TestOpenAPIBudget`
  - **PASS**: 4 PostgreSQL/race tests across 2 packages: lifecycle, fairness/cross-operation limits, concurrent acquire, and concurrent migration.
- `rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh`
  - **PASS**: full restored model, 1,218,530 generated / 928,529 distinct / 0 left; `TypeOK`, `ActiveLeaseSafety`, `ConfirmedReleaseInsideLease`, and `SnapshotRecoverySafety` hold.
  - **FAIL-AS-EXPECTED**: early release and stale start each trigger exactly the intended invariant.
  - **PASS/FAIL-AS-EXPECTED**: normal chart completes; both broken charts trigger exactly their target invariant and have no unchecked rule.
- `rtk git diff --check`
  - **PASS**.
- `rtk gofmt` was applied to every new/modified Go file before the final test/lint run.

## Verified behavior

- Real PostgreSQL migration is serialized across three replica connections and creates the required unique/state indexes.
- One active lease survives repeated release with the correct token; wrong token is rejected; transport-unknown remains active instead of immediately releasing quota.
- `snapshot_required` keeps the lease active and permits a later terminal outcome on the same request.
- Waiting cancellation is idempotent, rejects a wrong token, and removes the waiting count/runtime state when idle.
- Two runnable operations share the platform cap and release/admit in predictable oldest-operation/oldest-ticket order.
- Concurrent callers cannot exceed the configured platform/pool/operation active limit.
- Canonical identity merges equivalent scheme/host/default-port/dot/duplicate-slash/percent cases and rejects ambiguity/userinfo/query/fragment.
- Request/body/MCP readers reject one byte over limit; gzip decoding is bounded on both compressed and decompressed byte counts; audit headers and body are independently bounded and synthetic auth values are absent from captured output.
- Typed lease fields serialize as numeric Unix milliseconds required by FROZEN-1.0.

## Defects found and fixed

- Migration: concurrent DDL race, invalid default seed, wrong operation-pool CHECK, missing platform/pool runtime rows.
- Lifecycle: ineffective row locking around Release/Cancel, runtime-state create/delete race, unserialized local admission transactions, missing transaction deadline, snapshot miss release semantics, and non-millisecond protocol fields.
- Identity/limits: root/default-port/percent normalization gaps, pool limits not constrained by platform capacity, and audit header limit omission.
- Bounded IO: limit detection could be masked by an exact-size stream; gzip/deflate lacked a shared bounded component.
- Formal coverage: waiting cancellation and same-instance snapshot recovery were absent from the full/chart models.

## Boundaries

- **Verified**: code compilation, vet, target lint, unit/race bounds, migration, isolated real PostgreSQL lifecycle/fairness/concurrency, and design/model counterexamples.
- **Partial**: P04 is not yet wired into HTTP/MCP Executor paths; PLAN section 12's exhaustive queue/default/dedicated/platform matrices and two-replica E2E remain for downstream integration/acceptance.
- **Not verified / not run**: real business API, historical or real JWT, production service, Kubernetes cluster, deployment, two Obot/two Worker topology, Worker crash fencing, DNS pinning, or production throughput.
- Temporary PostgreSQL data was local and removed; no credential was persisted in evidence or source.
