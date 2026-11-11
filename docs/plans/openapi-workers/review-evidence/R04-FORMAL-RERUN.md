# R04 formal verification rerun evidence

Date: 2026-11-09 Asia/Shanghai. Reviewer worktree: `/Users/echo/orca/workspaces/obot/openapi-execution-budgets`.

## Method

- The five source feature models, configs, views, local verify scripts, and skills were copied read-only into `review-evidence/r04-formal-rerun/f01` through `f05`.
- The reruns wrote only to this worktree. Source feature formal artifacts were not modified.
- TLA+ Tools `v1.8.0` was used from `/tmp/tla2tools-1.8.0.jar`, SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`.
- The copied F03 verifier originally used `git rev-parse --show-toplevel`, which would resolve to the P04 repository rather than the isolated F03 copy. Only that copied script's `ROOT` derivation was changed to four parent directories. No model, config, invariant, view, expected exit code, or counterexample assertion was changed.
- Original and copied full-model SHA-256 values match for F01--F05. Full-model prefixes: F01 `718a3564d081`, F02 `f818ac0de0de`, F03 `d094d38a7d82`, F04 `a3bbae28228c`, F05 `43e02b6a1e7b`.

## Commands

```bash
rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh
rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh
rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh
rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh
rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh
```

Each command was executed from its corresponding isolated copy. Console output and UTC timestamps are beside this file as `f01.console.out`, `f01.status`, and so on.

## Results

| Feature | Restored result | Intended broken result | Evidence |
| --- | --- | --- | --- |
| F01 snapshot fencing | PASS; 33 distinct states, all listed rules hold | FAIL as expected; 57 distinct states / 120 steps; duplicate execution, missing fence prerequisite, and retry-after-uncertain violations | `f01/docs/plans/openapi-workers/formal/evidence/base/report.md:3`; `f01/docs/plans/openapi-workers/formal/evidence/broken/report.md:3` |
| F02 worker resolver | PASS; 18 distinct states | FAIL as expected; only `ExactCallerCredentialOnly`; 19 distinct states, `u2/i2` borrows `u1/i1` | `f02/docs/plans/openapi-workers/formal/RUN.md:6` |
| F03 HTTP proxy | PASS; 613 generated / 241 distinct, 0 left | FAIL as expected; only `ExecutionRequiresUniqueAuthorizedMatch`; charts expose route ambiguity and whitelist bypass | `f03/docs/plans/openapi-workers/formal/runs/latest/EVIDENCE.md:20` |
| F04 execution budgets | PASS; 233,082 generated / 181,137 distinct, 0 left | Early-release and stale-start each fail exactly their intended invariant | `f04/docs/plans/openapi-workers/formal/EVIDENCE.md:8` |
| F05 Compose lifecycle | PASS; 65,706 distinct states | Broken chart fails exactly traffic gate and rolling capacity invariants | `f05/docs/plans/openapi-workers/formal/EVIDENCE.md:7` |

All five verifier wrappers exited `0`. A wrapper exit of `0` means both the restored model passed and the deliberately broken model produced its required counterexample; it is not implementation, PostgreSQL, E2E, Docker Compose, or deployment evidence.

## Review interpretation

- F04 correctly demonstrates the high-cost lease properties for one shared slot: active count consistency, grace before release, and a pre-fencing-margin upstream start. The concrete stale-start counterexample has `upstreamStart=2`, `deadline=3`, and margin `1`.
- F04 explicitly does not cover PostgreSQL locks, release-token hashing, multi-operation fairness, queue cancellation, HTTP cancellation propagation, body/audit bounds, clock drift, or deployment. Those gaps remain implementation-test obligations.
- F01 supports one-shot requestID execution after one supplied snapshot, but does not verify supplied-digest byte equality, warm-cache/LRU behavior, or Worker restart persistence.
- F02 supports exact caller-instance credential isolation but does not cover transport retries, Key scope, digest generation, or JSON-only behavior.
- F03 supports unique authorized route matching in the abstracted parser domain but not the strict URI parser itself, body/Header separation, budgets, or networking.
- F05 supports Compose lifecycle gating and rolling capacity only at the design-state level; it does not parse YAML or start Docker.
