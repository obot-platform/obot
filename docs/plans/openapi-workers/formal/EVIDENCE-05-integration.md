# F05 Compose lifecycle formal verification evidence

## Result

- Started / finished: `2026-11-10T08:48:44+0800` / `2026-11-10T08:48:55+0800` (Asia/Shanghai).
- Tool: TLA+ Tools `2026.11.06.222357 (rev: d8fb231)`, jar SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`; Bun `1.3.14`; `openjdk version "11.0.31" 2026-04-21 LTS`.
- Full baseline config: `docs/plans/openapi-workers/formal/ComposeLifecycle.cfg`; `65,706` distinct states; `TypeOK`, `NoDeadlock`, `OpenAPITrafficGated`, `PlannedRollRetainsCapacity`, and `CHECK_DEADLOCK` all hold.
- Normal-flow chart config: `docs/plans/openapi-workers/formal/ComposeLifecycleChart.cfg`; `37` states; chart: 8 states and 13 arrows; the same four named checks and deadlock check hold.
- Broken chart config: `docs/plans/openapi-workers/formal/ComposeLifecycleBrokenChart.cfg`; `64` states; exactly `OpenAPITrafficGated` and `PlannedRollRetainsCapacity` break; `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` hold; no rules are unchecked.
- Model SHA-256: `43e02b6a1e7bf103fa857b58c63134629f69254b41e0eade8868bca2a0eb155e`.

## Commands

The project-local repeat command is:

```bash
rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh
```

The script downloads TLA+ Tools 1.8.0 only when `TLA2TOOLS` is absent, verifies the pinned SHA-256, and runs the three statechart commands above. Set `STATECHART_TS` or `JAVA` to override their default local paths.

## Expected counterexamples

- Missing route gate: from the empty deployment, `BreakRouteGate` admits one OpenAPI call before the shared network, PostgreSQL, service authentication, Obot, or Worker is ready, reaching `Unsafe traffic admitted`.
- Missing rolling guard: after one Obot and both Workers are ready, `BreakRollGuard` starts an Obot rollout and drains the only ready Obot replica, reaching `Rolling upgrade, capacity lost`.

The script compares the report's broken-rule headings with exactly those two names, rejects `Rules not checked`, and requires the remaining rules to hold.

## Scope and chart abstraction

- The full model explores arbitrary interleavings of network setup/removal, PostgreSQL readiness/failure, service-auth staging/trust/withdrawal, two Obot replicas, two Workers, one active call, planned one-at-a-time rolls, and an extra-failure flag. It proves the design-scope properties, not Docker Engine or Compose behavior.
- The chart model is the same TLA+ module and invariants under `ChartConstraint`. It follows the intended path through explicit network, PostgreSQL readiness, service-auth trust, both stacks becoming ready, admitted traffic, and a capacity-retaining rollout. It is not a substitute for the 65,706-state full baseline.
- The normal chart merges per-replica starting/ready transitions and repeated request admit/complete transitions that stay in the same visible state (36 hidden self-label steps in this run). It excludes unrelated failure/recovery interleavings; those remain covered by the full baseline.
- The broken chart is similarly bounded to the two mutation paths so statechart can complete every rule rather than stop at the full model's 100,000-state diagnostic bound.
- Not modeled: actual Docker daemon, container image contents, Compose parsing, Kubernetes, TLS handshake bytes, certificate rotation, request payload limits, leases, upstream behavior, and simultaneous double failures during a roll. A pass here does not claim E2E or deployment success.
