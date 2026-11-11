# Formal verification rerun evidence

- Started (UTC): 2026-11-10T02:00:41Z
- Finished (UTC): 2026-11-10T02:00:44Z
- TLA+ Tools: v1.8.0, TLC build 2026.11.06.222357, revision d8fb231, SHA-256 e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f
- Restored model: PASS; ExactCallerCredentialOnly, TypeOK, NoDeadlock, and CHECK_DEADLOCK hold; 18 distinct states.
- Broken model: FAIL as expected; only ExactCallerCredentialOnly is violated; TypeOK, NoDeadlock, and CHECK_DEADLOCK hold; 19 distinct states.
- Expected counterexample: caller u2 requests i2, but fallback selects i1's credential owned by u1.
- Model boundary: two users, two instances, one component, one tool; transport, retries, token refresh, encryption, and upstream semantics are not modeled.
