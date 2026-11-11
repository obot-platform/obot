# P03 HTTP proxy formal verification evidence

- Started / finished: `2026-11-09T19:09:43+0800` / `2026-11-09T19:09:53+0800`
- Command: `rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh`
- TLA+ Tools release: v1.8.0, SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`
- Java:

```text
openjdk version "11.0.31" 2026-04-21 LTS
OpenJDK Runtime Environment Corretto-11.0.31.11.1 (build 11.0.31+11-LTS)
OpenJDK 64-Bit Server VM Corretto-11.0.31.11.1 (build 11.0.31+11-LTS, mixed mode)
```
- Statechart runner: `bun /Users/echo/.agents/skills/statechart/scripts/statechart.ts`
- Statechart tool:

```text
1.3.14
```

- Normal model: PASS — all four invariants and deadlock check completed with no errors.
- Broken variant: expected failure — only `ExecutionRequiresUniqueAuthorizedMatch` was violated.
- Normal states: `613 states generated, 241 distinct states found, 0 states left on queue.`
- Broken states before stop: `21 states generated, 21 distinct states found, 8 states left on queue.`
- Normal chart projection: PASS — `TLC reached 17 states and 29 steps. The chart draws 7 states and 9 arrows.`; report/chart under `statechart-normal/`.
- Broken chart projection: expected failures — unique-route fallthrough and whitelist bypass; `TLC reached 20 states and 35 steps. The chart draws 7 states and 11 arrows.`; report/chart under `statechart-broken/`.
- Full model uses escaped segment sequences and exact static-precedence matching. The chart model uses the same finite route shapes after strict raw parsing and omits standard-module imports required only by the chart runner; this projection does not weaken the full model invariants.
- Full TLC output: `normal.out`, `broken.out`.

The verifier checks the pinned artifact hash, TLC exit codes, exact pass marker, exact expected Broken invariant, absence of unexpected invariant/error output, statechart key states/arrows, both expected chart counterexamples, and final state summaries. It does not treat a bare process exit code as sufficient evidence.
