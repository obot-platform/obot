# R05 formal rerun evidence

Date: 2026-11-09, Asia/Shanghai.

## Environment

- TLA+ Tools jar: `/tmp/tla2tools-1.8.0.jar`
- TLC: `2026.11.06.222357 (rev: d8fb231)`
- Jar SHA-256: `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`
- Java: Corretto OpenJDK 11.0.31

## Scope

The F01—F04 models and configs were copied read-only from their owning worktrees into this worktree's `r05-formal-rerun/` directory, then run with Java TLC directly. This avoids executing their project scripts, because those scripts rewrite formal evidence in worktrees that R05 must not modify.

F05 was rerun with its project-local command:

```bash
rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh
```

## Results

| Case | TLC result | Distinct states | Evidence |
| --- | --- | ---: | --- |
| F01 SnapshotFencing | PASS | 33 | `r05-formal-rerun/f01/SnapshotFencing.out` |
| F01 SnapshotFencingBroken | Expected invariant failure: `ExecutionEnteredAtMostOnce` | 22 before stop | `r05-formal-rerun/f01/SnapshotFencingBroken.out` |
| F02 OpenAPIWorkerResolver | PASS | 18 | `r05-formal-rerun/f02/OpenAPIWorkerResolver.out` |
| F02 OpenAPIWorkerResolverBroken | Expected invariant failure: `ExactCallerCredentialOnly` | 19 before stop | `r05-formal-rerun/f02/OpenAPIWorkerResolverBroken.out` |
| F03 OpenAPIHTTPProxy | PASS | 241 | `r05-formal-rerun/f03/OpenAPIHTTPProxy.out` |
| F03 OpenAPIHTTPProxyBroken | Expected invariant failure: `ExecutionRequiresUniqueAuthorizedMatch` | 21 before stop | `r05-formal-rerun/f03/OpenAPIHTTPProxyBroken.out` |
| F04 ExecutionBudgetLeases | PASS | 181,137 | `r05-formal-rerun/f04/ExecutionBudgetLeases.out` |
| F04 BrokenEarlyRelease | Expected invariant failure: `ConfirmedReleaseInsideLease` | 4,303 before stop | `r05-formal-rerun/f04/ExecutionBudgetLeasesBrokenEarlyRelease.out` |
| F04 BrokenStaleStart | Expected invariant failure: `ActiveLeaseSafety` | 877 before stop | `r05-formal-rerun/f04/ExecutionBudgetLeasesBrokenStaleStart.out` |
| F05 project skill | PASS; broken chart exactly breaks traffic gate and rolling-capacity rules | full baseline 65,706 | `../formal/EVIDENCE.md` |

Direct TLC baseline exit code is `0`; intentional invariant failures exit `12` under this TLC CLI. The `.rc` files beside each output preserve the observed exit code.

## Boundary

These are design-model checks only. They do not execute Go or Python implementation, Docker/Compose, TLS, PostgreSQL, a browser UI, a real business API, or a Kubernetes cluster, and they do not establish business exactly-once semantics.
