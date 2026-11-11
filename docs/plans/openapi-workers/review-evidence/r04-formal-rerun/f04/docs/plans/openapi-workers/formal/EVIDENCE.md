# F04 execution-budget lease formal verification evidence

## Result

- Property: a cross-replica active lease is unexpired, uniquely bound to one request, counted against the shared quota, and permits only a pre-fence upstream attempt; uncertain expiry remains reserved until reclaim grace. Violating this can over-admit business calls or let a crashed/stale Worker invoke upstream after its permit expires.
- Started / finished (UTC): `2026-11-09T11:09:53Z` / `2026-11-09T11:10:02Z`.
- Tool: TLA+ Tools v1.8.0, TLC build `2026.11.06.222357 (rev: d8fb231)`, jar SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`; Java output: `openjdk version "11.0.31" 2026-04-21 LTS`.
- Full restored model: `ExecutionBudgetLeases.cfg` — 233082 generated / 181137 distinct states, 0 left; `TypeOK`, `ActiveLeaseSafety`, and `ConfirmedReleaseInsideLease` all hold.
- Full early-release mutation: only `ConfirmedReleaseInsideLease` is violated. At `now=3`, `r1` changes from `expiredPending` to `completed` with `finishedAt=3` and `deadline=3`, releasing `reservedCount` before grace.
- Full stale-start mutation: only `ActiveLeaseSafety` is violated. At `now=2`, `r1` starts upstream for `deadline=3`; with fencing margin 1, `upstreamStart + FencingMargin < deadline` is false.
- Normal chart: 115 states; `TypeOK`, `LeaseSafety`, `ConfirmedReleaseInsideLease`, and `NoDeadlock` hold; no rule is unchecked.
- Early chart: 139 states; exactly `ConfirmedReleaseInsideLease` breaks, `TypeOK`, `LeaseSafety`, and `NoDeadlock` hold, and no rule is unchecked.
- Stale chart: 136 states; exactly `LeaseSafety` breaks, `TypeOK`, `ConfirmedReleaseInsideLease`, and `NoDeadlock` hold, and no rule is unchecked.
- Full model SHA-256: `a3bbae28228cabe4cba057a7ca19dae2515165c828dbf69bc1a4c83baff202df`.

## Commands

The repeat command is:

```bash
rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh
```

The script downloads TLA+ Tools only when `TLA2TOOLS` is absent, verifies the pinned SHA-256, runs the full model and both full mutations with Java TLC, and runs the normal plus two broken chart models with `/statechart`. It individually asserts all three non-empty `chart.mmd` files, parses exact invariant names, concrete lease values, absence of unexpected or unchecked rules, and rewrites this evidence file.

Raw full-model outputs are in `docs/plans/openapi-workers/formal/runs/latest/`. Chart artifacts are `statechart-chart/chart.mmd`, `statechart-broken-early/chart.mmd`, and `statechart-broken-stale/chart.mmd`, each beside `report.md` and `tlc.out`.

## Normal Mermaid chart

```mermaid
---
title: "OpenAPI execution-budget lease lifecycle"
---
stateDiagram-v2
  state "Legend" as legend {
    lg0: Idle
    lg1: Admission
    lg2: Execution
    lg3: Conservative hold
    lg4: Recovery
    lg5: Confirmed release
  }
  class lg0 blue
  class lg1 purple
  class lg2 teal
  class lg3 orange
  class lg4 yellow
  class lg5 teal
  s0: No request yet
  s1: Bounded queue ticket waiting
  s2: Lease held, upstream not sent
  s3: Lease held, upstream already sent
  s4: Confirmed result released
  s5: Expired, quota still held
  s6: Expired quota reclaimed
  [*] --> s0
  s0 --> s1: a replica takes a bounded queue ticket
  s1 --> s2: the shared quota admits one request
  s2 --> s3: a Worker starts the only upstream attempt before the fence
  s3 --> s4: an observed completion releases quota inside the lease
  s2 --> s5: time advances and a due lease expires
  s3 --> s5: time advances and a due lease expires
  s5 --> s6: the reclaimer releases quota after the grace period
  class s0 blue
  class s1 purple
  class s2 teal
  class s3 teal
  class s4 teal
  class s5 orange
  class s6 yellow
```

The chart hides repeated `Tick` and `Done` self-steps and interleavings that leave the same visible phase. Those remain in the TLC state graph. The two shortest broken-path charts are `statechart-broken-early/chart.mmd` and `statechart-broken-stale/chart.mmd`.

## Mapping and boundaries

- Full `phase` maps to `openapi_execution_requests.state`; `deadline` maps to the UTC lease deadline; `reservedCount` maps to active/unreclaimed shared quota; `origin` maps to `obot_instance_id`; `delivery` models Obot→Worker dispatch; `upstreamStart` models the only permitted business attempt.
- `Tick` atomically observes due waiting/active deadlines. `ReclaimExpiredAction` releases quota only after `deadline + ReclaimGrace`; `ObotCrash` removes an undelivered dispatch so it cannot later start upstream; `WorkerStartsUpstream` requires the full fencing margin.
- The scalar chart models are projections for readable Mermaid output because the full function-heavy module is beyond the current statechart XML importer. They do not relax or replace the full baseline invariants.
- Not covered: PostgreSQL transactions/locks, release-token hash comparison, queue fairness among operations, HTTP cancellation propagation, retry/network behavior, payload limits, audit, clock drift beyond the modeled margin, and real deployment. A pass is not implementation, E2E, or deployment evidence.
