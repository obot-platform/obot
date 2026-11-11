# F02 formal verification evidence

- Verified property: `ExactCallerCredentialOnly`
- Property in plain language: a dispatched OpenAPI request must use the valid business credential stored for the authenticated caller's requested instance, never another user's or instance's credential.
- Harm when violated: one tenant can issue requests with another tenant's token, causing cross-tenant data access, unauthorized writes, and false audit attribution.
- Final rerun command start/end (UTC): 2026-11-09T08:36:38Z / 2026-11-09T08:36:43Z
- Restored TLC start/end: 2026-11-09 16:36:39 / 2026-11-09 16:36:39 Asia/Shanghai
- Broken TLC start/end: 2026-11-09 16:36:41 / 2026-11-09 16:36:41 Asia/Shanghai
- Tool: TLA+ Tools v1.8.0, TLC build `2026.11.06.222357`, revision `d8fb231`; jar SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`
- Java: Corretto OpenJDK 11.0.31
- Restored result: PASS; 18 distinct states, 22 generated states, depth 8, zero states left on queue; `ExactCallerCredentialOnly`, `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` all hold.
- Broken result: FAIL as intended; 19 distinct states and 21 generated states before stopping at the violation; only `ExactCallerCredentialOnly` breaks, while `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` hold and no rule is listed as unchecked.
- Expected counterexample: `u2` requests `i2`; cross-instance fallback selects `i1`'s credential owned by `u1`; the request is dispatched.
- Statechart outputs: `statechart-pass/` and `statechart-broken/`. The normal chart hides four terminal/stuttering `Done` steps.
