---
name: verify-openapi-execution-budgets
description: Verify the planned OpenAPI cross-replica execution-budget lease lifecycle when P04 scheduling, lease expiry, release, crash recovery, or Worker fencing changes.
---

# Verify OpenAPI execution budgets

Use this skill when a change touches P04's planned budget queue, global active quota, lease deadline/generation, release token handling, Obot/Worker crash recovery, or Worker pre-upstream fencing. Do not use it as evidence that implementation, PostgreSQL behavior, E2E, or deployment passes; the current model is design-level.

## Property

The critical property is that an active shared-quota lease is unexpired, belongs to one request row, counts toward the cross-replica limit, and can trigger only one fenced upstream attempt; quota is released immediately only on confirmed completion/cancel before the deadline, while uncertain expiry stays reserved until the reclaim grace period. A violation can over-admit business calls across Obot/Worker replicas or let a stale Worker invoke a business system after its lease fence.

## Coverage

- Design source: `docs/plans/openapi-workers/PLAN.md` sections 5 (PostgreSQL state and atomic transitions) and 7 (cancellation, crash, expiry, and fencing).
- Full model: `docs/plans/openapi-workers/formal/ExecutionBudgetLeases.tla` with two requests, two Obot replicas, one global active slot, one waiting slot, queue admission, waiting cancellation, dispatch, same-instance snapshot recovery, crash-lost dispatch, one upstream attempt, confirmed release/cancel, expiry, and grace reclaim.
- Chart models: `OpenAPIBudgetLeasesChart*.tla` are deliberately scalar projections of the full state machine used because `statechart` cannot import the full model's function-heavy XML. They preserve the normal path and the two shortest mutations; they do not replace the full TLC baseline.
- Expected mutations: `ExecutionBudgetLeasesBrokenEarlyRelease.cfg` releases an already-sent expired lease before grace; `ExecutionBudgetLeasesBrokenStaleStart.cfg` permits upstream start when only the fencing margin remains.
- Sync record: the uncommitted I04 implementation in this worktree was compared against every model action on 2026-11-10, starting from baseline `2be04f025d1c86144e8670d9e9341b263edc28ea`; no commit hash is available because committing is prohibited. The full restored model and both mutations were rerun after adding waiting cancellation and snapshot recovery. Latest result: 928,529 distinct states, all restored invariants hold, and each mutation triggers exactly its intended invariant.

## Rerun

```bash
rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh
```

The command pins TLA+ Tools v1.8.0 SHA-256, runs the complete cross-replica model and both full mutations, runs the three chart models with `/statechart`, checks exact invariant names and concrete counterexample values, rejects unexpected or unchecked rules, and rewrites `docs/plans/openapi-workers/formal/RUN.md`.

## Interpret failures

- `ActiveLeaseSafety` in the full model or `LeaseSafety` in a chart means a stale permit started upstream, an active lease is expired, or quota/count consistency broke. Map the TLC action back to the planned admission or Worker guard.
- `ConfirmedReleaseInsideLease` means quota was made reusable after the lease deadline without waiting for reclaim grace; check the planned release branch and reclaim timing.
- A TypeOK failure usually means the model abstraction changed, not a product finding. Restore the state-variable bounds before interpreting lease failures.
- A pass proves only the bounded two-replica/two-request design model. It does not cover release-token hashing, PostgreSQL locking, queue fairness across operations, HTTP cancellation, retry behavior, request/response bounds, or real deployment.

## Required update

After changing the design or implementing the package, update action names and guards in both the full and chart models, rerun the command, and update the sync commit and result in this file. Keep the full model as the source of coverage; never weaken it merely to make `statechart` parse it.
