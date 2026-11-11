---
name: verify-openapi-http-proxy
description: Verify the P03 raw path/query-to-operation matching and layered operation-whitelist state machine with pinned TLC; use when API proxy routing, matching, operation enablement, authorization identity, or raw HTTP semantics change.
---

# Verify the OpenAPI HTTP proxy model

## Property

The proxy may execute a request only after one raw path template is uniquely selected for the target component and method, the operation passes ToolOverride, Profile, and instance allowlists, and the exact escaped path segments plus raw query are preserved for execution. A violation lets an ambiguous, cross-component, disabled, or caller-selected operation reach the upstream API, causing unauthorized calls or mistaken business attribution.

## Inputs

- Design authority: `docs/plans/openapi-workers/PLAN.md`, sections 3 (P1 path matching), 5.1 (raw route), 5.2 (operation whitelist), and 5.3 (raw HTTP semantics).
- Model: `docs/plans/openapi-workers/formal/OpenAPIHTTPProxy.tla`.
- Finite scenarios and all four invariants: `docs/plans/openapi-workers/formal/OpenAPIHTTPProxy.cfg`.
- Deliberate failure variant: `OpenAPIHTTPProxyBroken.tla` and `OpenAPIHTTPProxyBroken.cfg`.
- Chart-only equivalent projection and its two expected failures: `OpenAPIHTTPProxyChart*.tla/.cfg`.
- Statechart projection: `statechart.mjs`.
- Current sync commit: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Rerun

```sh
rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh
```

The script downloads TLA+ Tools v1.8.0 only when `TLA2_TOOLS_JAR` is absent and rejects any SHA-256 other than `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`. It verifies the normal model passes every configured invariant, the Broken variant violates exactly `ExecutionRequiresUniqueAuthorizedMatch`, and no unexpected TLC error or rule failure is hidden. It also runs the chart projection with Bun/statechart, asserts its normal key states and the two expected Broken counterexamples, and writes evidence to `docs/plans/openapi-workers/formal/runs/latest/EVIDENCE.md`.

## Interpretation

- `ExecutionRequiresUniqueAuthorizedMatch` failing means an ambiguous route, wrong component/method, or any whitelist layer was bypassed. The TLC counterexample names the request, candidates, and selected operation; replay those values against the route matcher and resolver before changing the model.
- `RawRequestSemanticsPreservedOnExecution` failing means escaped path segments or the raw query were decoded, split, reordered, or regenerated before execution.
- `RejectionNeverCallsUpstream` or `OnlyAuthorizedExecutionsAreRetained` failing means a platform rejection was mislabeled as a business result or an unauthorized execution stayed observable.
- In the chart projection, `ExecutionRequiresUniqueRoute` is the ambiguity fallthrough failure and `ExecutionRequiresWhitelist` is the disabled-operation bypass failure. Their reports under `statechart-broken/` give the shortest reader-facing paths.
- A normal pass covers only the finite raw-segment/query scenarios and the state-machine abstraction in the model. It does not cover strict URI validation itself, Header separation, body streaming, budgets, networking, E2E behavior, or deployment.

## Statechart

The verifier generates normal and Broken reader-facing charts with the `/statechart` skill, the pinned Java/TLC environment, `OpenAPIHTTPProxyChart*.tla/.cfg`, and `statechart.mjs`. The chart shows raw parse, unique route resolution, whitelist decision, business execution, and platform rejection. The reset transition is retained so terminal states are visible and deadlock is checked; no model step is hidden.

## Update requirement

This is a design model at the P03 planning-complete commit; no proxy implementation exists yet. If implementation adds or changes the raw-prefix parser, matcher precedence, method/405 classification, ToolOverride/Profile/instance intersection, component identity, or raw query forwarding, update both TLA model/config scenarios and this coverage record first, then rerun the verifier. Move the sync commit forward only after confirming the model still names the real implementation functions and state transitions. Do not claim implementation coverage from this planning model.
