---
name: verify-openapi-compose
description: Verify the OpenAPI Workers two-Compose-stack startup, traffic gate, and rolling-upgrade lifecycle design with TLC.
triggers:
  - Changing deploy/compose/obot.yaml or deploy/compose/openapi-workers.yaml
  - Changing P05 startup, health/readiness, shared-network, service-auth, drain, or rollout design
  - Reviewing F05 formal verification or Compose lifecycle failure behavior
---

# Verify OpenAPI Compose lifecycle

Use this skill when a change can affect whether OpenAPI traffic is admitted only after the shared network, trusted service authentication, ready PostgreSQL/Obot capacity, and ready Worker capacity all exist, or whether a planned one-replica-at-a-time upgrade can leave zero serving Obot/Worker capacity.

## Inputs and coverage

- Read `docs/plans/openapi-workers/PLAN.md` sections 5.1, 5.5, 11, and 12.
- The model is `docs/plans/openapi-workers/formal/ComposeLifecycle.tla`; variables map directly to `sharedNetwork`, PostgreSQL readiness, `obot[1..2]`, `worker[1..2]`, service-auth material, active OpenAPI calls, rollout state/target, and an extra-failure flag.
- The full design model uses `ComposeLifecycle.cfg`. `ComposeLifecycleChart.cfg` bounds only the normal startup/roll path for a readable Mermaid chart. `ComposeLifecycleBrokenChart.cfg` bounds the two mutation paths so every remaining rule is checked.
- This is design-level verification. It does not parse Compose YAML, start Docker, replace Compose E2E, validate TLS bytes, or prove request/lease behavior.

## Re-run

```bash
rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh
```

The command recreates the three output directories and `docs/plans/openapi-workers/formal/EVIDENCE.md`. It downloads TLC 1.8.0 only when needed and verifies its pinned SHA-256. `STATECHART_TS`, `JAVA`, and `TLA2TOOLS` can be overridden.

Read every invariant result with the pstack `/statechart` skill. The relevant reports are under `docs/plans/openapi-workers/formal/statechart-base/`, `statechart-chart/`, and `statechart-broken-chart/`.

## Success and failure interpretation

- Success requires all of `TypeOK`, `NoDeadlock`, `OpenAPITrafficGated`, and `PlannedRollRetainsCapacity` to pass in both non-broken configs.
- The broken config must fail exactly `OpenAPITrafficGated` and `PlannedRollRetainsCapacity`, while `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` continue to hold and no report says rules were not checked.
- `OpenAPITrafficGated` failing means traffic can pass without one of: explicit shared network, PostgreSQL readiness, trusted service auth, an Obot replica, or a Worker replica. Check Compose ordering/readiness and the Obot routing/backend guard, not just the TLA model.
- `PlannedRollRetainsCapacity` failing means an upgrade drained the last serving Obot or Worker. Check rollout scripts and readiness removal; do not hide the finding by weakening the invariant or enabling a second active call.
- `TypeOK` or `NoDeadlock` failing is a model/tool defect or an orchestration state omission; fix the model or design before trusting either target property.

## Synchronization

- Last confirmed model target: commit `2be04f025d1c86144e8670d9e9341b263edc28ea`, with uncommitted F05 PLAN/formal/skill files in this worktree.
- Re-run and move this record forward only after confirming the model still matches changed `PLAN.md` sections and, once implemented, `deploy/compose/obot.yaml`, `deploy/compose/openapi-workers.yaml`, readiness wrappers, and rollout/drain scripts.
- If stack topology leaves two replicas, add a modeled third state or update the target abstraction and rerun both baseline and mutation checks. If a new lifecycle stage is introduced, model it rather than describing it only in prose.
- Preserve the distinction between the full 65,706-state baseline and bounded chart runs. Never replace the baseline with the chart-only constraint.
