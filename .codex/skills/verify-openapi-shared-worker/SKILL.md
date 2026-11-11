---
name: verify-openapi-shared-worker
description: Verify the shared OpenAPI Worker snapshot miss, one-shot snapshot supplement, and requestID execution-fencing design before changing its protocol or cache behavior.
triggers:
  - Change the shared OpenAPI Worker describe or execute protocol.
  - Change snapshot_required, snapshot resupply, requestID fencing, or retry behavior.
  - Change cold snapshot loading, compilation completion, or cache activation semantics.
  - Review a design or implementation claim about upstream execution after a snapshot miss.
---

# Verify OpenAPI shared Worker snapshot fencing

## Inputs

- Repository root of the `openapi-shared-worker` worktree.
- Current design or implementation under review, especially the snapshot envelope, `describe`/`execute` protocol, requestID state, cold-cache handling, and retry policy.
- The relevant commit being synchronized to the model.

## Why This Is A Skill

The failure is an interleaving bug, not a simple unit-test case: a cold miss, one snapshot resupply, compilation, execution, uncertain transport failure, and request reuse can interleave in ways that accidentally replay a business API call. The model records the exact project nouns and failure boundary so this reasoning is reusable without rediscovering the state machine.

The checked rule is: after a cold-cache request, the Worker can start a business upstream call only after exactly one valid snapshot is supplied and the requestID acquires the execution fence exactly once. If this rule breaks, Obot can replay a side-effecting business request after an uncertain failure or execute before the published immutable snapshot is loaded.

## Ordered Steps

1. Read this skill and the covered sections in `docs/plans/openapi-workers/PLAN.md`.
2. Compare the design or implementation against the model's request phases, digest binding, supply counter, execution-entry counter, and upstream-attempt counter.
3. If any corresponding behavior changed, update the TLA actions and invariants before running the command; do not weaken a rule to make TLC pass.
4. Run the command in the next section from the repository root.
5. Read both `/statechart` reports and compare the base state flow with the broken shortest path.
6. Record the tool version, state counts, timestamps, exit statuses, and non-sensitive output paths in `docs/plans/openapi-workers/formal/EVIDENCE.md`.
7. Update the synchronization commit and coverage list only after both runs have completed and the expected pass/fail pattern is present.

The model is design-level. It covers `docs/plans/openapi-workers/PLAN.md` sections 4.4, 4.5, and 4.6 for one request under a cold cache, including invalid snapshot, compilation failure, business completion, uncertain transport failure, same-request reuse, and digest mismatch. It does not cover warm-cache hits, LRU eviction, concurrent deduplication, helper cancellation, worker restart, transport body limits, or exactly-once business side effects.

## Re-run Command

Run from the repository root:

```sh
rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh
```

The script downloads pinned TLA+ Tools 1.8.0 (SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`) into a temporary file when `TLA2TOOLS` is not already set. It first runs a deliberately broken configuration with `EnforceFence = FALSE` and a bound of one deliberate reuse, then restores and runs the guarded configuration. The broken run must fail `ExecutionEnteredAtMostOnce` and `UpstreamRequiresOneSnapshotAndOneFence`; it may also fail `NoRetryAfterUncertainOutcome` when transport failure follows the duplicate entry. The restored guarded run must then satisfy every invariant. Any other failure means the model/config/view or setup is broken.

The wrapper parses the `/statechart` reports and exits non-zero when the broken model omits either required violation, reports any other violation, leaves a rule unchecked, or when the guarded model omits any required holding rule. Exit statuses alone are not treated as proof.

Read every result with the `/statechart` skill. `report.md` gives the shortest path in reader terms; `chart.mmd` shows the state flow. The passing chart shows the normal flow and the broken chart marks the state that bypasses the fence. The view intentionally hides only the terminal `Done` self-loop; rejection counters are states in the model but do not add visible labels.

## Success Criteria

- The guarded run exits `0`, has no invariant violations or deadlock, and generates a normal-flow Mermaid chart.
- The broken run exits `1` and reports at least `ExecutionEnteredAtMostOnce` and `UpstreamRequiresOneSnapshotAndOneFence` as violated; `NoRetryAfterUncertainOutcome` is also acceptable after duplicate transport failure.
- The counterexample reaches a duplicate entry only after the first request supplied a snapshot, entered execution once, and reached a terminal result.
- The evidence file names the actual command, tool version, start/end times, state counts, result, and output paths.
- The skill's coverage and synchronization commit still describe the current design.

## Constraints And Pitfalls

- Do not remove an invariant to make TLC pass; a failing invariant is a finding or evidence that the model no longer matches the design.
- Do not interpret transport failure as proof that the upstream side effect did not happen.
- Do not add automatic retries after a supplied snapshot and uncertain transport outcome.
- Do not claim coverage for warm-cache hits, LRU eviction, concurrent deduplication, helper cancellation, worker restart, body limits, or exactly-once side effects.
- Do not put snapshots, credentials, URLs, or response bodies into evidence.
- Do not update the synchronized commit merely because implementation code appeared; update it only after the model is compared and both runs pass the expected pattern.

## Verification Evidence

- Evidence is collected in `docs/plans/openapi-workers/formal/EVIDENCE.md`.
- Raw `/statechart` artifacts are kept under `docs/plans/openapi-workers/formal/evidence/base/` and `.../evidence/broken/`.
- The evidence must remain non-sensitive and reproducible with the one command above.

Coverage and synchronization:

- Design source: `docs/plans/openapi-workers/PLAN.md`, sections 4.4 (`describe`), 4.5 (`execute`), and 4.6 (cold compile/cache lifecycle).
- Model and configs: `docs/plans/openapi-workers/formal/SnapshotFencing.tla`, `SnapshotFencing.cfg`, and `SnapshotFencingBroken.cfg`.
- Statechart view: `docs/plans/openapi-workers/formal/snapshot-fencing.view.mjs`.
- Last synchronized commit: `2be04f025d1c86144e8670d9e9341b263edc28ea` (planning baseline; implementation has not started).

If any covered design or implementation changes, first update the model so phase, digest, supplement, fence, and upstream-attempt variables still match the real protocol nouns. Move the synchronization commit forward only after both configurations and `/statechart` have been rerun and evidence refreshed. A model pass is not an E2E, integration, or deployment pass.
