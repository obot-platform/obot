---
name: verify-openapi-worker-bridge
description: Verify the P02 OpenAPI Worker per-call identity, instance, authorization, and business-credential isolation design with TLA+.
triggers:
  - Changing the planned pkg/openapiworker resolver or executor
  - Changing OpenAPI component bridge authorization or credential-context behavior
  - Reviewing cross-user token isolation or fallback behavior in P02
---

# Verify OpenAPI Worker Bridge

Use this skill when P02 changes how an OpenAPI invocation resolves the platform caller, vMCP instance, published component snapshot, original tool authorization, or user business credential. The critical rule is that a request may be dispatched only with the authenticated caller's valid credential from the requested instance's exact credential context.

## Inputs and coverage

- Read `docs/plans/openapi-workers/PLAN.md`, especially sections 4.3, 4.5, 4.6, and 11.
- The model is `docs/plans/openapi-workers/formal/OpenAPIWorkerResolver.tla`; its view is `formal/view.mjs`.
- Model rules are `ExactCallerCredentialOnly`, `TypeOK`, and `NoDeadlock`. The checker also reports `CHECK_DEADLOCK`.
- Current-source context for the fallback risk is `pkg/mcp/action.go` (`serverConfigForAction`, `serverInstanceCredentialContext`, and `serverInstanceHeaders`) and `pkg/vmcp/credential.go` (`InstanceConfigurationCredentialContext`). The future P02 implementation must map its actions to the resolver steps in the model.

## One rerun command

From the repository root:

```bash
bash .codex/skills/verify-openapi-worker-bridge/verify.sh
```

The command downloads TLA+ Tools v1.8.0 to a temporary path when `TLA2TOOLS` is not set, verifies its SHA-256, runs the restored and broken configurations, removes the temporary jar, and writes `docs/plans/openapi-workers/formal/RUN.md`.

## Required outcome

- Restored configuration must pass all four reported checks: `ExactCallerCredentialOnly`, `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK`.
- Broken configuration must fail exactly once with `ExactCallerCredentialOnly`; `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` must still hold, and no rule may be listed as unchecked.
- The required counterexample is caller `u2` requesting instance `i2` while the fallback chooses instance `i1`'s credential owned by `u1`.
- Consult the generated Mermaid chart with the `/statechart` skill. The normal chart is `formal/statechart-pass/chart.mmd`; the bug chart is `formal/statechart-broken/chart.mmd`.

## Failure interpretation

- A restored-model failure is blocking: the resolver design can dispatch another user's business token, dispatch a malformed state, or stop before a terminal state.
- A broken-model pass means the check no longer exercises fallback leakage; strengthen or fix the model before accepting the change.
- Another broken-model invariant means the intentional variant is no longer isolated; inspect `statechart-broken/tlc.out` and fix the model before interpreting the result.
- A statechart report saying `Rules not checked` is not a pass. Reduce the model or remove an earlier failure so every rule completes.

## Sync record and update requirements

- Last synchronized against worktree base commit `2be04f025d1c86144e8670d9e9341b263edc28ea` on 2026-11-09, before P02 implementation.
- If `pkg/openapiworker/resolver.go`, `pkg/openapiworker/executor.go`, `pkg/mcp/openapi_bridge.go`, the P02 resolver design, or the credential-context functions above change, map each new branch or fallback to the model before moving the sync record.
- Do not add an implementation fallback to catalog, server, admin, or another user's credential context without first changing this model and showing the resulting failure to the coordinator.
- The model deliberately does not verify HTTP transport, snapshot digest bytes, `snapshot_required` retries, JSON-only SSE behavior, encryption, token refresh, budgeting, auditing, or upstream semantics; use the P02 tests for those behaviors.
