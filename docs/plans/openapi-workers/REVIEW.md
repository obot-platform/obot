# R01 collective cross-review

Date: 2026-11-09 Asia/Shanghai. Reviewer worktree: `/Users/echo/orca/workspaces/obot/openapi-shared-worker`. Baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Verdict

**Overall: blocking issues found; CONTRACTS DRAFT-0.2 must not freeze yet.**

- No P0 credential breach or unauthorized implementation execution was found.
- Seven confirmed P1 issues block implementation freeze: four cross-feature protocol/order incompatibilities and three P03 routing/authorization/response-writer blockers.
- Seven P2/P3 issues cover budget-model scope, migration identity, Compose egress, protocol naming, digest-binding model coverage, and statechart readability.
- F01–F05 formal artifacts were independently rerun in isolated copies under this worktree. All restored models passed and every broken variant produced its intended counterexample. Model passes remain design-level only.
- Current source and official upstream tag support the plans' main facts: current OpenAPI is one wrapper workload per configuration, FastMCP is pinned at 3.4.7, current credential resolution has fallback risk, current response audit/redaction is insufficient, and no shared Worker/HTTP proxy/budget implementation exists yet.

## Findings

### R01-01 — P1 — Worker lease fencing contract is internally incompatible

- **Status:** confirmed.
- **Impact:** P04 requires a Worker to reject missing/expired/near-expiry leases before upstream, but P01 declares budget metadata opaque and says the Worker consumes only the absolute deadline. If the HTTP deadline is longer than `leaseDeadline - fencingMargin`, a stale Worker request can initiate a business call after the shared quota is no longer valid. This undermines the F04 stale-start property at the real protocol boundary.
- **Evidence:** P01 non-goal says Worker only consumes absolute deadline and call metadata (`PLAN.md:80`); execute carries “不透明预算元数据” (`PLAN.md:161`). P02 also keeps lease context opaque (`PLAN.md:142`, `PLAN.md:229`). P04 defines `LeaseID`, `LeaseDeadline`, requires Worker rejection (`PLAN.md:114-130`), and requires `min(HTTP context deadline, leaseDeadline - fencingMargin)` (`PLAN.md:275-278`). The F04 model enforces this in `WorkerStartsUpstream`, unlike the P01 protocol text.
- **Responsible feature:** 04 for fencing semantics, 01 for Worker execution contract, 02 for public Go types; 00 must freeze one design.
- **Recommendation:** Either make `LeaseID`, `LeaseDeadline`, and fencing margin typed Worker-visible fields, or remove Worker-side validation and require Obot to derive the HTTP context deadline from `leaseDeadline - fencingMargin` before dispatch. Do not leave both meanings of “opaque” in the shared contract.
- **Close condition:** CONTRACTS and P01/P02/P04 use one typed deadline model; P01 adds missing, expired, and within-margin negative tests proving DNS/upstream is never contacted; P02 serializes the agreed fields; F04 model/actions and sync record are updated if protocol nouns change.

### R01-02 — P1 — Production Worker service authentication is not converged

- **Status:** confirmed.
- **Impact:** P01 defaults production to mTLS, P02 exposes bearer-token fields and recommends bearer over TLS, while P05 expects mTLS plus optional bearer. I02 cannot produce a compatible client and I05 cannot provision matching secrets from the current plans. A late downgrade to bearer-only or HTTP would expose a service boundary that carries per-request business tokens.
- **Evidence:** P01 requires production mTLS (`PLAN.md:238-244`). P02 only defines Worker URL/token file/token and generic TLS transport fields, and recommends `Authorization: Bearer` (`PLAN.md:79-90`). P05 itself identifies the conflict and proposes mTLS plus optional bearer (`PLAN.md:73-82`, `PLAN.md:167-178`).
- **Responsible feature:** 02 primary; 01 and 05 jointly affected.
- **Recommendation:** Freeze production TLS with required mTLS. Bearer may remain an optional second factor or explicitly named enterprise-gateway transition, never a substitute for TLS/mTLS. Add Obot client certificate/key and Worker client-CA configuration to P02.
- **Close condition:** P01/P02/P05 name identical required/optional materials; P02 has missing/expired certificate and wrong-CA negative tests; I05 performs a real mTLS handshake and rejects no-client-certificate/wrong-identity cases.

### R01-03 — P1 — P02 Executor interface is insufficient for P03 security boundary

- **Status:** confirmed; independently confirms P03's own blocker.
- **Impact:** P02's proposed `Executor` has no admitted-principal argument and returns a generic `Description`. P03 needs a user-narrowed operation list, raw HTTP input type, header classification, budget source identity, and typed errors. Without this, P03 would either duplicate credential/authorization resolution or trust caller-selected component/operation data, risking cross-user access and hidden-operation bypass.
- **Evidence:** P02 interface (`PLAN.md:115-135`) versus P03 required `PrincipalContext`, `HTTPInput`, `ResolvedDescription`, and principal-aware methods (`PLAN.md:190-226`).
- **Responsible feature:** 02 primary; 00 must freeze the public API.
- **Recommendation:** Adopt the P03 shape while keeping all resolver and credential-value logic inside 02. `PrincipalContext` must be extracted only from an admitted request principal/context and never accept a caller-provided user ID.
- **Close condition:** P02/P03 compile against one interface; tests prove wrong user, wrong instance, revoked component/tool, hidden operation, missing credential, and raw-input passthrough cannot bypass P02's resolver.

### R01-04 — P1 — Credential/body/budget acquisition order conflicts across P03 and P04

- **Status:** confirmed.
- **Impact:** P04 fixes credential resolution and “request body size” before budget acquisition, while P03 acquires before reading the body and defers credential values to Execute. Implementations can diverge on 409-before/after queueing, queue-slot occupancy during slow uploads, and whether request bytes are buffered before quota admission. This affects denial semantics, quota fairness, and bounded-resource behavior.
- **Evidence:** P04 fixed order (`PLAN.md:101-110`); P03 requires operation match plus Acquire before body read (`PLAN.md:168-174`) and says credential values are revealed only inside Execute (`PLAN.md:217-226`).
- **Responsible feature:** 03 and 04, coordinated by 00.
- **Recommendation:** Freeze one sequence: authenticate and authorize operation; verify required business credential eligibility before consuming a queue slot; validate declared `Content-Length`/size policy before admission; Acquire; stream the bounded actual body under lease deadline; reveal the final credential value only immediately before Worker dispatch. Chunked bodies are read only after admission and count against the lease deadline.
- **Close condition:** P03/P04 order text and tests agree. Tests show missing credential and oversized declared body do not enqueue; slow/chunked body under lease is cancelled and released; body bytes are never buffered without the agreed admission policy.

### R01-05 — P1 — Ordinary ServeMux wildcard routing cannot preserve the HTTP path contract

- **Status:** confirmed from current Go 1.27 documentation.
- **Impact:** `%2F` is unescaped segment-wise for matching, and repeated slash/dot segments are sanitized with redirects. A normal `{path...}` handler therefore loses raw path bytes and can change request semantics, causing wrong operation matching or bypass.
- **Evidence:** Current `net/http.ServeMux` documentation states segment-by-segment unescaping and sanitizing redirects; P03 independently identifies this (`PLAN.md:62-69`).
- **Responsible feature:** 03.
- **Recommendation:** Use a dedicated pre-mux `/api-proxy/` raw-prefix branch that preserves `URL.RawPath`, strictly parses escaped segments, and sets an explicit routing pattern for authz/tracing.
- **Close condition:** Regression tests cover `%2F`, `%2E`, repeated slashes, traversal, illegal encoding, empty path, query order, and no redirect; existing management/UI/MCP routes are unchanged.

### R01-06 — P1 — API proxy cannot reuse management `vmcp_id` resource authorization

- **Status:** confirmed from current source.
- **Impact:** Existing `vmcp_id` resource handling is bound to vMCP management read/manage checks. Reusing it can let management visibility authorize a proxy before the current user's instance, component selection, and original tool are checked, or leak resource existence.
- **Evidence:** `pkg/api/authz/resources.go:176-225` binds `vmcp_id` and runs `checkVMCP`; `pkg/api/authz/vmcp.go:94-118` uses read/manage visibility. Current per-connection MCP checks are separate in `pkg/api/authz/mcpid.go:107-133` and `mcpid.go:223-255`.
- **Responsible feature:** 03 with 02's resolver.
- **Recommendation:** Add an API-proxy-specific authorization entry using canonical vMCP connection semantics, then require 02 to re-resolve current user instance, component, Profile, override, and operation on every call.
- **Close condition:** Negative tests reject management-visible-but-not-callable users, wrong API Key scope, another user's instance, revoked component/tool, and hidden resources without upstream contact.

### R01-07 — P1 — Unified APIServer response writer pollutes business responses

- **Status:** confirmed from current source.
- **Impact:** Default `Content-Type`, `X-Content-Type-Options`, session cookies, and API no-cache behavior are platform response semantics. Applying the normal wrapper to upstream business results would alter a business response and can confuse clients or mix authenticated platform sessions with business `Set-Cookie`.
- **Evidence:** `pkg/api/server/server.go:186-195` installs `headersResponseWriter` early; `server.go:438-459` adds default headers; `server.go:308-312` replays provider cookies; `server.go:358-362` adds API no-cache headers. P03 identifies this at `PLAN.md:62-69`.
- **Responsible feature:** 03.
- **Recommendation:** Create a dedicated proxy response writer. Business results preserve only upstream end-to-end headers/body/status; platform errors use the platform JSON envelope and error header.
- **Close condition:** Browser-level tests verify business 200/403/429/5xx, non-JSON, HEAD, multi-value headers, and upstream `Set-Cookie` semantics; refreshed platform cookies and default platform headers do not contaminate business responses.

### R01-08 — P2 — F04 model omits real multi-scope admission and waiting cancellation

- **Status:** confirmed coverage gap; not an implementation claim of failure.
- **Impact:** F04's full model has one shared active slot, one operation, and no platform/default/dedicated pool hierarchy. It does not model queued-request cancellation or multi-operation fairness. A passing model can therefore hide over-admission caused by real three-level count transitions, lock-order/deadlock bugs, or waiting tickets occupying queue slots after client disconnect until timeout.
- **Evidence:** Model coverage and boundary (`PLAN.md:471-488`); state transitions only allow `active` to terminal for Release/Cancel (`PLAN.md:199-207`); admission/fairness algorithm is prose plus future PostgreSQL tests (`PLAN.md:211-222`).
- **Responsible feature:** 04.
- **Recommendation:** Before implementation freeze, add a small multi-operation/multi-scope model or a separate statechart for admission atomicity and waiting cancellation. Alternatively, record an explicit design decision that this risk is reserved exclusively for real PostgreSQL tests, with the exact test matrix.
- **Close condition:** Model or tests demonstrate platform/pool/operation invariants, two-operation fairness, and immediate dequeue/cancel of a waiting ticket; no active lease is released early merely because the request was still queued.

### R01-09 — P2 — Legacy components can lack the required budget source reference

- **Status:** confirmed schema possibility; migration impact needs P04 decision.
- **Impact:** P04 rejects a missing `ComponentCatalogReferences`, while the current vMCP field is optional. Existing/migrated components without this field cannot enter the shared budget mode and may appear to lose OpenAPI capability if the new backend is selected globally.
- **Evidence:** P04 requires sourceRef and rejects missing/ambiguous identity (`PLAN.md:85-99`); current field is `omitempty` (`pkg/storage/apis/obot.obot.ai/v1/vmcp.go:54-55`).
- **Responsible feature:** 04 and 00.
- **Recommendation:** Add migration/backfill from retained component catalog references, or explicitly document a legacy compatibility policy and keep the old runtime available until re-publication. Do not silently substitute vMCP/component IDs for a stable catalog source identity.
- **Close condition:** A legacy-snapshot test identifies the exact outcome, migration evidence covers backfilled references, and capability/UI behavior does not silently hide existing OpenAPI components.

### R01-10 — P2 — Compose acceptance omits Worker private-egress allowlist configuration

- **Status:** confirmed planning gap.
- **Impact:** P01 defaults to rejecting loopback/private/link-local destinations unless explicitly allowlisted. P05's fake business system and company intranet APIs live on private networks, but P05 does not specify the Worker egress allowlist variable, certificate/name binding, or test-only CIDR. The E2E can fail closed or be “fixed” with an unsafe broad private-network allowance.
- **Evidence:** P01 requires explicit private CIDR/domain policy and DNS-all-answer checks (`PLAN.md:53-63`, `PLAN.md:226-236`, `PLAN.md:394-404`). P05 places fake business on the test shared network (`PLAN.md:203-228`) but has no corresponding egress configuration.
- **Responsible feature:** 05 with 01; 00 freezes names and safety defaults.
- **Recommendation:** Define a narrow test-only fake-business hostname/CIDR allowlist and a production intranet allowlist. Keep the default deny-private policy and reject unlisted private answers even when another DNS answer is public.
- **Close condition:** Compose config exposes only the intended allowlist; E2E proves the allowed fake business is reachable while another private hostname/IP is refused before upstream connection.

### R01-11 — P2 — P02 introduces an undefined `ensure` protocol noun

- **Status:** confirmed naming/protocol mismatch.
- **Impact:** 00 and P01 expose only `describe`, `execute`, `healthz`, and `readyz`. P02 repeatedly says `describe/ensure/execute`, which can be read as a third Worker endpoint and produce incompatible generated clients or tests.
- **Evidence:** Shared contract paths do not include `ensure`; P02 type/client/step text names `describe/ensure/execute` (`PLAN.md:105-113`, `PLAN.md:231-235`).
- **Responsible feature:** 02.
- **Recommendation:** Rename the concept to an Obot-side `EnsureSnapshotReady` client helper implemented as `describe` with an optional immutable snapshot envelope, or explicitly add an endpoint to the shared contract. The default recommendation is no new endpoint.
- **Close condition:** P02 and frozen CONTRACTS contain one endpoint list and one helper name; generated protocol tests contain no undocumented route.

### R01-12 — P2 — F01 formal model does not represent the supplied snapshot digest

- **Status:** confirmed model abstraction; implementation tests are planned but not yet run.
- **Impact:** The TLA model treats `SupplySnapshot(..., "valid")` as a matching valid snapshot and only carries the request-bound digest. It can prove “one valid snapshot”, but not that the bytes' `executionDigest` equal the request digest before execution. A serialization/digest bug could therefore pass this model while P01's planned protocol test would be needed to catch it.
- **Evidence:** `SnapshotFencing.tla` request shape binds one request digest and supply counter, but `SupplySnapshot` has only valid/invalid outcome; the evidence and skill explicitly exclude byte/digest implementation details.
- **Responsible feature:** 01.
- **Recommendation:** Either add a `suppliedDigest` variable and wrong-digest transition, or record digest equality as a code-level boundary and keep the existing protocol digest-mismatch test mandatory for I01 acceptance.
- **Close condition:** Updated model shows wrong-digest rejection, or CONTRACTS/I01 evidence requires the existing digest-mismatch test to pass before implementation settlement.

### R01-13 — P2 — Protected-header list and path-conflict policy still require contract freeze

- **Status:** confirmed open cross-feature decision.
- **Impact:** Existing OpenAPI reserved-header validation does not treat `authorization`, `x-api-key`, or `x-obot-*` as inherently protected. P03 has a stronger list and static-precedence/ambiguity policy, but 01/02/03 must implement exactly the same rules or a caller may override platform-injected credentials or route to another operation.
- **Evidence:** Current reserved list (`pkg/openapi/headers.go:14-24`) omits platform identity names; P03 identifies the header and ambiguity decisions (`PLAN.md:62-73`); F03's model already uses static precedence and rejects ambiguity.
- **Responsible feature:** 00 to freeze; 01/02/03 to implement the same matcher/header classes.
- **Recommendation:** Freeze unconditional stripping of platform authentication, proxy, forwarding, connection-management, Worker-service, and `X-Obot-*` headers; business credential headers are applied last. Freeze static-segment precedence with equal-specificity ambiguity rejection.
- **Close condition:** Shared cross-language test vectors cover header names/canonicalization, credential override, platform headers, ambiguous templates, `%2F`, and repeated query values.

### R01-14 — P3 — Several broken statecharts understate the concrete fault

- **Status:** confirmed readability issue; TLC counterexample values are present and correct.
- **Impact:** Human reviewers can misread the intended failures: F02's broken path still says “found for the exact instance” instead of showing `u2/i2` borrowing `u1/i1`; F04's stale-start transition says “before the fence” even in the broken no-margin case; F05's unsafe admission arrow says “after every gate holds”. The raw TLC outputs contain the needed values, but the charts should communicate the injected bypass themselves.
- **Evidence:** Copied charts under `review-evidence/r01-formal-rerun-20261109T184839/`; concrete counterexamples are present in the corresponding TLC outputs and scripts.
- **Responsible feature:** 02, 04, and 05 chart views.
- **Recommendation:** Add caller/requested/source identity to the F02 view, mark F04's transition as broken/no-margin, and label F05's broken admission as bypassing gates. Also align F02's documented rerun command with the project `rtk` convention.
- **Close condition:** Rerun shows the concrete values/fault wording on the chart and report; all scripts retain exact expected TLC counterexamples.

## Formal review and independent rerun

The 00 summary was used only as an index. R01 read each original PLAN, formal model/config/output/evidence, local skill, and reviewer-relevant source. To avoid modifying source feature formal artifacts, models/skills were copied to:

```text
docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/
```

The copied F03 verifier was changed only to locate its copied root instead of calling `git rev-parse`; model/config/invariants were unchanged. Results:

| Feature | Restored result | Intended broken result | Review conclusion |
| --- | --- | --- | --- |
| F01 | 65 generated / 33 distinct, all rules pass | 57 distinct / 120 steps; fence/upstream/no-retry violations | Core cold-miss property is real; supplied-digest equality remains abstract (R01-12). |
| F02 | 18 distinct, all rules pass | 19 distinct; only credential isolation fails; `u2/i2` borrows `u1/i1` | Resolver fallback harm is well modeled; chart needs concrete identity (R01-14). |
| F03 | 613 generated / 241 distinct, all rules pass | Full model violates unique authorized match; chart gives ambiguity and whitelist bypass | Strongest routing/whitelist model; strict URI parser/body/header remain implementation tests. |
| F04 | 233,082 generated / 181,137 distinct, all rules pass | Early release and stale start each trigger exactly one invariant | Stale lease/conservative reclaim is solid; real three-scope admission/fairness/wait cancel is out of scope (R01-08). |
| F05 | 65,706 distinct baseline, all rules pass | Broken chart 64 states triggers traffic gate and roll capacity failures | Lifecycle gate is useful; it does not execute Compose or TLS (R01-14 chart wording). |

No copied statechart report contains `Rules not checked`. Full commands, timings, logs, reports, and charts are indexed in:

```text
docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/R01-FORMAL-RERUN.md
```

## Feature coverage matrix

| Feature | Reviewed PLAN | Reviewed formal/skill/source | Findings owned or materially affected |
| --- | --- | --- | --- |
| 01 Shared Worker | Yes | PLAN 2/4/5/7/8/12, TLA/config/view/output, local skill, official tag and FastMCP wheel | R01-01, R01-02, R01-10, R01-12, R01-13 |
| 02 Worker bridge | Yes | PLAN 2/4/5/7/11, TLA/config/view/output, local skill, current Go authz/credential/backend source | R01-01, R01-02, R01-03, R01-11, R01-13, R01-14 |
| 03 HTTP proxy | Yes | PLAN 2/3/5/6/8/12, full and chart TLA/config/output, local skill, router/authz/server source | R01-03, R01-04, R01-05, R01-06, R01-07, R01-13 |
| 04 Budgets | Yes | PLAN 2/4/5/6/7/8/9/12/16, full and chart TLA/config/output, local skill, DB/audit source | R01-01, R01-04, R01-08, R01-09 |
| 05 Compose/acceptance | Yes | PLAN 2/3/5/6/7/8/10/12/14, TLA/config/output, local skill, current deployment facts | R01-02, R01-10, R01-14 |
| 00 coordination | ROADMAP, CONTRACTS, ACCEPTANCE, ORCA-INDEX, formal summary | Draft contracts/acceptance were checked against all feature PLANs and local source | R01-01 through R01-04 and R01-13 require 00 freeze decisions |

## Source and dependency conclusions

- FastMCP/upstream facts are accurate: official `mcp-images` tag `v0.27.2` pins `fastmcp==3.4.7`; `fastmcp-slim` 3.4.7 has the declared SHA-256; the cached 3.4.7 source exposes `json_response` and `stateless_http`. P01 correctly states that the current upstream server does not pass `json_response=True`.
- Current Obot source has no shared Worker client/backend, `/api-proxy`, cross-replica budget lease, or Compose deliverable. This review did not treat any PLAN or formal pass as implementation/E2E success.
- Current ordered credential contexts and fixed server headers validate P02's no-fallback resolver. Current vMCP authorization and component snapshots validate the component/original-tool identity model.
- Current response buffering/redaction and fixed header list validate P04's bounded/redacted audit requirement.
- Current server wrapper/authz/ServeMux behavior validates P03's raw-prefix, dedicated authorization, and response-writer blockers.
- Other feature worktrees retained their expected untracked PLAN/formal/skill outputs; this review created no tracked implementation change outside this worktree.

## Review limitations

- This is a source/model/plan review, not implementation, browser, Compose, real PostgreSQL, real cluster, or real business-API verification.
- Formal reruns used copied planning artifacts; they do not validate Python Worker code, Go resolver implementation, PostgreSQL transactions, TLS bytes, Docker lifecycle, cancellation propagation, payload limits, or audit implementation.
- Upstream behavior was verified from the official tag and cached pinned wheel source, not by launching the official image.
- No historical JWT, real business credential, real business API, user service, production deployment, commit, push, branch, or worktree creation was accessed.
