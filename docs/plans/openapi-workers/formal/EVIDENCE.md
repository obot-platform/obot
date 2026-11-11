# F01 Snapshot Fencing Formal Evidence

Date: 2026-11-09. Worktree: `/Users/echo/orca/workspaces/obot/openapi-shared-worker`. Baseline commit: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Property And Harm

**Property:** after a cold-cache request, the Worker can begin a business upstream call only after exactly one valid snapshot is supplied and the requestID acquires the execution fence exactly once.

**Harm if violated:** Obot can execute before the published immutable snapshot is loaded, or replay a side-effecting business request after an uncertain transport outcome.

## Tool Choice

The property is an event-ordering and state-machine property, so it is checked with TLA+/TLC rather than bounded input generation or SMT. `/statechart` turns the TLC result into reader-facing state charts and shortest violation paths.

- TLA+ Tools release asset: `v1.8.0`
- TLA+ Tools SHA-256: `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`
- TLC build: `TLC2 Version 2026.11.06.222357 (rev: d8fb231)`
- Java: Corretto `11.0.31`
- Host used for the recorded run: macOS `26.5.2`, arm64
- Statechart skill: `/Users/echo/.agents/skills/statechart`

## Model Mapping

Model and design nouns use one request (`r1`) and two possible digests so digest rebinding is representable:

| TLA element | P01 design meaning |
| --- | --- |
| `request[r].phase` | `new`, cache miss, `snapshot_required`, compiling, ready, executing, business result, uncertain transport, or snapshot/compile failure |
| `request[r].digest` | requestID-to-executionDigest binding |
| `request[r].supplies` | number of snapshot supplements accepted for this requestID |
| `request[r].entries` | number of times the requestID enters the execution state machine |
| `request[r].upstreamAttempts` | number of business upstream attempts begun by this requestID |
| `StartExecute` / `ReturnSnapshotRequired` | execute observes cold cache and returns `snapshot_required` before upstream |
| `SupplySnapshot` / `CompileCompletes` | one valid or invalid snapshot supplement, then compile success/failure |
| `AcquireExecutionFence` / `BeginUpstream` | one-shot requestID fence, then business upstream attempt |
| `UpstreamCompletes` / `UpstreamTransportFails` | business result versus uncertain transport outcome |
| `RejectReuseWithoutFence` / `RejectDigestMismatch` | reuse rejection and requestID/digest mismatch rejection |

## Model Abstraction

Verified scope:

- one request under a cold cache;
- at most one accepted snapshot supplement;
- valid and invalid snapshot, compile success and compile failure;
- business completion and uncertain transport failure;
- same-request reuse and digest rebinding;
- no automatic retry after an uncertain transport outcome.

Not modeled:

- warm-cache hits, LRU eviction, byte/count limits, active references, and concurrent same-digest compilation;
- helper cancellation/replacement, process crash, worker restart, or shutdown draining;
- HTTP serialization, DNS/egress checks, body/header limits, or authentication isolation;
- exactly-once business side effects. An uncertain transport result remains uncertain even when the model proves the Worker does not itself retry.

The broken configuration is bounded to one deliberate reuse (`entries < 2`) so TLC reports a finite counterexample. This does not weaken any invariant; it only bounds the fault-injection depth.

## Re-run

From the repository root:

```sh
rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh
```

The wrapper downloads the pinned TLA+ Tools `v1.8.0` jar to a temporary path when `TLA2TOOLS` is not set, verifies its SHA-256, removes previous base/broken evidence directories, runs `/statechart` for the broken config and then the restored guarded config, and deletes the temporary jar on exit. It also parses both reports: Broken must contain the two required fence violations and may only additionally contain `NoRetryAfterUncertainOutcome`; every guarded rule must be reported as holding, and an unchecked broken rule is rejected.

Recorded wrapper run: start `2026-11-09T16:24:08+08:00`, end `2026-11-09T16:24:18+08:00`, wrapper exit `0`.

## Passing Configuration

- Config: `docs/plans/openapi-workers/formal/SnapshotFencing.cfg`
- `EnforceFence = TRUE`, `MaxRejections = 1`
- TLC start/finish: `2026-11-09 16:24:17` / `2026-11-09 16:24:17`
- Result: no errors and no deadlock
- States: 65 generated, **33 distinct**, 0 left on queue; depth 10
- Rules holding: `TypeOK`, `SnapshotSuppliedAtMostOnce`, `ExecutionEnteredAtMostOnce`, `UpstreamAttemptedAtMostOnce`, `UpstreamRequiresOneSnapshotAndOneFence`, `SnapshotRequiredNeverTouchesUpstream`, `NoRetryAfterUncertainOutcome`, `NoDeadlock`
- Raw evidence: `docs/plans/openapi-workers/formal/evidence/base/`

Normal reader flow:

1. Request not seen.
2. First execute observes cold cache.
3. Worker asks for the snapshot.
4. Caller sends one valid snapshot.
5. Snapshot compiles to a ready plan.
6. Worker takes the one-shot fence.
7. Worker starts the business API call.
8. Business response returns, or transport outcome becomes uncertain without a second upstream attempt.

Mermaid chart: `docs/plans/openapi-workers/formal/evidence/base/chart.mmd`. It has 10 states and 9 arrows. The chart omits 20 terminal `Done` self-loop steps and does not separately draw 18 transitions whose source and target reader label are the same.

## Broken Configuration

- Config: `docs/plans/openapi-workers/formal/SnapshotFencingBroken.cfg`
- `EnforceFence = FALSE`, `MaxRejections = 1`, one deliberate reuse bound
- TLC first violation start: `2026-11-09 16:24:14`
- /statechart complete reruns explored **57 distinct states and 120 generated steps**; the first TLC trace stopped after 38 generated/22 distinct states at `ExecutionEnteredAtMostOnce`.
- Expected result: TLC exit `1`
- Violations: `ExecutionEnteredAtMostOnce`, `UpstreamRequiresOneSnapshotAndOneFence`, and `NoRetryAfterUncertainOutcome` when transport failure follows duplicate entry
- Rules still holding: `TypeOK`, `SnapshotSuppliedAtMostOnce`, `UpstreamAttemptedAtMostOnce`, `SnapshotRequiredNeverTouchesUpstream`, `NoDeadlock`
- Raw evidence: `docs/plans/openapi-workers/formal/evidence/broken/`

Shortest reader path to `ExecutionEnteredAtMostOnce` and `UpstreamRequiresOneSnapshotAndOneFence`:

1. Request not seen.
2. First execute observes cold cache.
3. Worker asks for the snapshot.
4. Caller sends the snapshot once.
5. Compiled plan becomes ready.
6. Worker takes the one-shot fence.
7. Worker starts the business API call.
8. Business response returns.
9. The same request enters execution again.

`NoRetryAfterUncertainOutcome` continues with one more step: after duplicate entry, transport outcome becomes uncertain.

Mermaid chart: `docs/plans/openapi-workers/formal/evidence/broken/chart.mmd`. It has 13 states and 14 arrows; `Execution entered again`, `Business result returned after duplicate entry`, and `Transport outcome uncertain after duplicate entry` are marked as breaking. It omits 36 terminal `Done` self-loop steps and does not separately draw 33 same-label transitions.

## Boundary

This is a design-level model check of the P01 PLAN, not implementation verification. No real business API, OpenAPI schema, credential, E2E service, Docker image, deployment, or production cluster was used. A passing guarded model does not imply E2E, integration, deployment, or exactly-once business execution.

## I01 Implementation Rerun

- Time: 2026-11-09 22:13:54–22:14:25 Asia/Shanghai.
- Command: `rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh`.
- Exit: `0`; wrapper required broken exit `1` and restored exit `0`.
- Broken: 57 distinct states / 120 generated steps; `ExecutionEnteredAtMostOnce`, `UpstreamRequiresOneSnapshotAndOneFence`, and `NoRetryAfterUncertainOutcome` violated as expected.
- Restored: 33 distinct states / 56 generated steps; every rule held and no deadlock was reported.
- Raw final artifacts: `docs/plans/openapi-workers/formal/evidence/broken/` and `docs/plans/openapi-workers/formal/evidence/base/`.
- Model synchronization: unchanged. The implementation retains the modeled cold-miss, one accepted recovery attempt, requestID/digest binding, and one-shot execution fence; no model noun or invariant was weakened. Canonical byte equality remains an explicit code-level test boundary as frozen by C01/D15.
