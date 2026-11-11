<<<<<<< HEAD
<<<<<<< HEAD
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
=======
# R02 集体交叉审核报告

日期：2026-11-09。审核 worktree：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge`。状态：complete（审核完成；下列 P1 未解决前不能冻结实施）。

## 1. 审核范围与方法

本次读取了 00 的 `ROADMAP.md`、`CONTRACTS.md`、`ACCEPTANCE.md`、形式化汇总索引，完整交接与架构说明，以及 01—05 的 `PLAN.md`、formal 模型/配置、原始输出、`EVIDENCE.md`（如存在）和项目本地验证 skill。审核重点是 R02 分配的启动模式、每调用凭证绑定、vMCP 工具发现、平台鉴权、无 Docker/K8s 动态托管，并对 01/03/05 做了更深入复核。

真实源码核对集中在：

- 启动与后端：`pkg/services/config.go:435`、`pkg/services/config.go:592-749`、`pkg/mcp/manager.go:45-90`、`pkg/mcp/manager.go:133-190`。
- OpenAPI 快照与 Header：`pkg/mcp/openapi.go:57-79`、`pkg/openapi/environment.go:21-100`、`pkg/openapi/headers.go:9-31`、`apiclient/types/mcpserver.go:204-280`。
- 凭证与 fallback：`pkg/mcp/types.go:349-375`、`pkg/mcp/action.go:181-266`、`pkg/mcp/action.go:269-332`、`pkg/mcp/action.go:407-445`、`pkg/gateway/client/credential.go:76-96`、`pkg/vmcp/credential.go:56-81`。
- vMCP 工具身份与授权：`apiclient/types/vmcp.go:41-65`、`apiclient/types/mcpserver.go:183-202`、`pkg/vmcp/profiles.go:25-103`、`pkg/vmcp/profiles.go:129-166`、`pkg/vmcp/legacy.go:11-55`。
- 平台鉴权与 MCP 出口：`pkg/gateway/server/apikey_auth.go:31-100`、`pkg/gateway/types/apikeys.go:35-90`、`pkg/api/authz/mcpid.go:33-104`、`pkg/api/authz/mcpid.go:115-219`、`pkg/api/authz/mcpid.go:231-264`。
- SSE 探测回退与现有响应包装：`pkg/mcp/backend.go:89-185`、`pkg/api/handlers/mcpgateway/handler.go:253-531`、`pkg/api/handlers/mcpgateway/handler.go:572-590`、`pkg/api/server/server.go:186-360`、`pkg/api/server/server.go:438-466`。
- 部署事实：`Dockerfile:28-95`、`run.sh:21-37`、`pkg/gateway/server/router.go:11-25`、`pkg/server/server.go:99-142`、`chart/templates/mcp.yaml:1-120`。

另按 01 PLAN 声明直接读取官方 `obot-platform/mcp-images` tag `v0.27.2` 的 `openapi-mcp/{server.py,network.py,config.py,pyproject.toml}`，确认其 `stateless_http=True` 但未传 `json_response=True`、逐请求认证隔离、完整缓冲响应、公网-only egress 和 30 秒启动 `SIGALRM` 等关键事实。没有访问真实业务 API、历史 JWT、生产服务、集群或用户现有 Docker 服务。

## 2. 总体结论

**Verdict：G00 不能冻结。** 00 草案的方向与源码事实总体相符：当前确实没有固定 Worker 后端，现有 Docker/K8s 后端不能满足无 daemon/无集群控制权目标；vMCP 授权身份已经是组件 ID + 原始工具名，凭证存储支持精确 context；OpenAPI 现有配置有专用 `/readyz`，但公共 `ensureServerReady` 在缺少 health path 时仍会自动 GET SSE。01—05 的计划大多准确识别了这些基础事实。

阻断点集中在跨 feature 契约尚未收敛：02 提供的 Executor/身份接口不足以安全支撑 03；Header、executionDigest、服务认证和 MCP loopback 身份语义在 01/02/03/05 之间仍存在不同假设。上述问题不是实现失败，因为当前仍在规划阶段；但按 REVIEWING 规则，P1 和协议不兼容必须由 00 修订后复核。

## 3. 发现

### R02-01 [P1] 02 Executor 接口缺少平台 principal，无法支撑 03 安全落地

- **影响**：03 的普通 HTTP 代理必须由 02 从认证结果解析用户、当前实例、组件快照、授权 operation、业务凭证和预算身份；02 当前 `Reference`/`Invocation`/`Executor` 没有这些字段或语义。若 03 自行解析或自填 userID，会造成越权调用、错误用户凭证绑定或预算键漂移。
- **依据**：P02 PLAN 4.3 的接口只有 `Reference` 与 `Invocation`，仅说明上下文重新解析；P03 PLAN 3 第 3 项和 6.1 明确要求 `PrincipalContext`、raw `HTTPInput`、用户已收窄 `ResolvedDescription`、Header 分类、typed error 与 `BudgetSourceID`。真实源码中 API Key 用户 UID、`authorized_mcp_ids`、归因和组均在 `pkg/gateway/server/apikey_auth.go:61-100`，这些信息不能由 03 从 Header/body 猜测。
- **责任 feature**：02 主责，00 冻结接口，03 提供消费方契约。
- **建议**：00 采纳 03 6.1 的 principal-in-context 接口，明确 02 是唯一 resolver；03 不得读取用户业务凭证或自行构造授权清单。
- **解决条件**：冻结版契约包含 principal 来源、raw path/query/body/Header、错误分类、用户收窄 operation、`BudgetSourceID` 和 `BudgetContext` 字段；02/03 各自新增编译级接口测试和双用户越权负例，证明 03 无法自选用户、组件、operation 或凭证。

### R02-02 [P1] Header 策略与 executionDigest 语义在 01/02/03 间未收敛

- **影响**：不同计划分别假设 Header 定义来自不可变快照、组件快照 + 用户实例策略，或 describe 返回的 Credential/Forwardable Header。若 digest 漏掉 required/prefix/scope，Worker 可能用旧执行计划或缺认证请求上游；若把用户实例值放进 digest，又会让本应共享的 Worker 缓存按用户拆分并把用户身份间接写入共享摘要。
- **依据**：P01 PLAN 4.2 只冻结 `Header 名、required、prefix`；P02 PLAN 4.4 要求 Header 名称/位置/前缀/必需性/作用域并叠加用户实例 policy；P03 PLAN 6.1 要求 `CredentialHeaders` 与 `ForwardableHeaders`。源码 `types.MCPConfig` 还有 `UserAllowed`、`Sensitive`、`Static`、Options 和 SecretBinding 等语义（`apiclient/types/mcpserver.go:257-280`），现有 `configureHeaders` 会把非 `UserAllowed` 固定值写入 `ServerConfig.Headers`（`pkg/mcp/types.go:349-375`），这正是 P02 PLAN 4.5 要禁止作为个人业务认证 fallback 的风险。
- **责任 feature**：00 冻结权威来源；01 定义 Worker envelope 可接受的不可变非敏感字段；02 实现 resolver；03 只消费分类结果。
- **建议**：区分三层：Worker execution plan 的共享非敏感 Header 定义、02 每调用解密后的最终 Header 值、03 入站允许转发的业务 Header。用户实例策略可影响 02 的 required 判定和最终值，但不得把凭证值或用户身份放入 executionDigest；若 required/prefix/scope 变化，必须显式发布并生成新共享 digest 或由 02 拒绝旧 plan。
- **解决条件**：00 契约给出字段矩阵和跨语言 digest 测试向量；01/02 用同 schema、不同 Header policy/prefix/required/scope 的测试证明缓存命中与拒绝行为；03 证明请求方同名认证 Header 不能覆盖注入值。

### R02-03 [P1] MCP 组件桥接的 loopback 身份不足以独立复算 vMCP Profiles

- **影响**：P02 要求 bridge 的 `tools/list`/`tools/call` 按当前用户 instance 与 Profiles 收窄。但 vMCP composite loopback token 只携带用户 UID、名称、邮箱和 `GroupMCP/GroupCompositeMCP/GroupAuthenticated`，不携带 `obot_groups` 与 `auth_provider_groups`（`pkg/api/handlers/mcpgateway/handler.go:572-590`）。而 Profiles 匹配依赖这两个 Extra 字段（`pkg/vmcp/profiles.go:25-41`）。若组件 bridge 直接用该 loopback principal 调 `MatchingProfiles`，共享 vMCP Profile 会失效或被错误放行；若只依赖父 MMMCP 过滤，P02 又不能声称组件 bridge 每调用独立完成 Profiles 授权。
- **依据**：P02 PLAN 4.6 要求组件继续走 `/mcp-connect/{connect_id}` 且 bridge 收窄工具；源码父 vMCP 在 `pkg/mcp/vmcp.go:95-179` 生成组件并调用 `InstanceGrant` 后再经 MMMCP loopback。当前组件 token 的确保留 UID，但丢失角色组上下文。
- **责任 feature**：02。
- **建议**：00/02 明确二者之一：组件 bridge 只执行原始工具/operation 检查，Profiles/instance 收窄由父 vMCP 已收窄的组件配置负责；或 bridge 通过 UID 读取当前有效用户组并在每调用重建完整 `kuser.Info`，同时保留 API Key scope 检查。不能把 loopback token 的三个组当作完整平台身份。
- **解决条件**：新增真实 MCP `tools/list`/`tools/call` 测试，覆盖共享 vMCP Profile 按 Obot group/IdP group 放行或拒绝、instance 组件选择、别名/前缀和跨组件同名工具；审计记录实际进入 resolver 的身份来源。

### R02-04 [P1] Worker 服务认证默认值在 01/02/05 间不兼容

- **影响**：01 生产默认 mTLS，02 候选 bearer token + TLS，05 建议生产 mTLS 且 bearer 只能叠加。若不冻结，02 可实现仅 bearer，01 Worker 可只支持 mTLS，05 Compose 无法互操作；更严重的是“集群内/TLS 终端”被误解为可明文传输用户业务 token。
- **依据**：P01 PLAN 4.8、P02 PLAN 4.1、P05 PLAN 第 3 节第 1 项和 5.4。00 CONTRACTS 只要求 TLS 与服务认证分离，尚未规定默认握手和最小配置。
- **责任 feature**：00 统一契约，01/02/05 分别实现端点、客户端和部署材料。
- **建议**：冻结生产最低要求为 TLS + mTLS；bearer 仅作为显式配置的第二因子或企业网关过渡方案。本地验收必须生成短期 CA/证书并实际验证证书链、SAN、错误证书拒绝和轮换流程。
- **解决条件**：契约写明必选/可选材料、Header 是否叠加、证书 SAN、CA 信任、轮换顺序和本地验收拒绝负例；01/02/05 提供同一套非敏感样例配置和互操作测试。

### R02-05 [P1] 03 raw path 与原始响应 writer 的公共文件写集尚未划出

- **影响**：普通 Go `ServeMux` 会解码 `%2F`、清洗 `//`、`.`/`..` 并可能重定向；统一 `headersResponseWriter` 会默认写安全 Header/Content-Type，`Wrap` 还可能回放平台 `Set-Cookie`，均会破坏业务响应字节语义。P03 已识别该问题，但 `pkg/api/server/server.go` 仍只是“00 明确授权后”才可写，02 的服务组装也可能与该公共路径冲突。
- **依据**：P03 PLAN 3 第 1/4 项、5.1、5.3 和 6.5；源码 `pkg/api/server/server.go:186-360`、`pkg/api/server/server.go:438-466`；当前 `/api-proxy` 不存在（`pkg/api/router/router.go:617-625`）。
- **责任 feature**：00 划分写集，03 实现，02 避免并行修改。
- **建议**：G00 明确将 raw-prefix 专用分支或等价 helper 独占划给 03，并要求 02 的服务组装补丁先合入/后合入的顺序；平台错误与业务响应 writer 分离。
- **解决条件**：I03 前完成写集记录；回归测试覆盖 `%2F`、重复斜杠、路径穿越、原始 status/Header/body、平台 Cookie 不混入，并证明管理 API、SCIM、UI 和 `/mcp-connect` 现有响应不被改变。

### R02-06 [P2] 预算 source identity 在 P02 接口中尚未落地

- **影响**：P04 的预算键需要 `sourceID::entryKey + operationKey`，且同一来源经多个 vMCP/component 共享额度。P02 当前 `Reference`/`Description` 未暴露 `BudgetSourceID`；仅靠 component ID、vMCP ID 或 schema digest 会把额度拆分，或让不同来源误合并。
- **依据**：P04 PLAN 4.1 要求 `ComponentCatalogReferences` 或同等 catalog entry 引用，缺失即失败；源码 `pkg/storage/apis/obot.obot.ai/v1/vmcp.go:52-55` 有该映射，但仅在 catalog sync 路径填充（`pkg/controller/handlers/mcpcatalog/vmcp.go:176-229`）。个人/迁移 vMCP 和 catalog entry 已删除时必须定义权威 fallback 或明确拒绝。
- **责任 feature**：02 暴露稳定身份，04 消费并计算 hash，00 冻结迁移/缺失行为。
- **建议**：02 的 `ResolvedReference` 必须包含 `BudgetSourceID`，并规定：优先发布组件 reference；无 reference 时从不可变 catalog entry 元数据派生；两者都缺失则 `budget_identity_unavailable`，不得改用 component/vMCP/schema digest。
- **解决条件**：同一来源两个组件、两个 vMCP、legacy retained snapshot 与缺失 reference 的测试证明共享或明确拒绝行为符合冻结契约。

### R02-07 [P2] 平台保护 Header 规则必须同时进入 01 编译计划、02 resolver 和 03 入站清理

- **影响**：现有 `reservedHeader` 不保留 `authorization`、`x-api-key` 或 `x-obot-*`（`pkg/openapi/headers.go:14-31`）。若只由 03 清理，MCP tool Header 参数或 02 转发仍可能把平台 Key 带到业务上游；若只由 01 清理，业务 API 自身要求的 `Authorization` 又会被误伤。需要明确“平台保护名”与“业务认证名”冲突时平台身份优先。
- **依据**：P03 PLAN 3 第 5 项、P01 PLAN 4.5 和 P02 PLAN 4.5 均有局部要求，但 00 CONTRACTS 未给出跨语言完整名单和冲突策略。
- **责任 feature**：00 冻结名单，01/02/03 同步实现测试。
- **建议**：将 `Authorization`、`X-API-Key`、`X-Obot-Api-Key`、Cookie、代理/转发/连接管理和 `X-Obot-*` 定义为平台保留；业务必须使用 `Authorization` 时，02 注入的当前用户业务值最后覆盖，请求方传入值先丢弃。平台认证建议文档继续引导使用 `X-API-Key`。
- **解决条件**：01/02/03 的共享测试向量覆盖自填平台 Header、业务 Authorization、`authtoken`、多值 Header 和 OpenAPI Header 参数，假上游断言只能看到当前用户业务凭证。

## 4. 重点链路复核结果

### 启动模式与无 Docker/K8s

**已确认**：当前 `MCPRuntimeBackend` 只有 Docker/Kubernetes/K8s（`pkg/mcp/manager.go:45-54`、`pkg/mcp/backend.go:27-30`）。Docker 初始化立即创建 Docker client、探测网络、清理容器并启动事件 watcher；K8s 路径会读取 kubeconfig/in-cluster、创建 client/cache/router并尝试 Namespace。P02 的 `openapi-worker` 尚不存在。

**结论**：02 的方向正确。但 `NewSessionManager` switch 和 `mcp.Options` 是必需接线点；P02 把 `manager.go` 列为需 00 扩展写集而非默认写集，G00 必须明确该文件的最小变更归属。固定模式还要显式禁用动态托管 API、MCP Network Policy、SecretBinding 和 K8s 细节，不能只是不调用 `ensure/deploy`。

### 凭证绑定

**已确认**：现有用户 Header 的精确 context 是 `userID-MCPServerInstanceName`（`pkg/mcp/action.go:407-438`），vMCP 用户配置另有 `vmcp-instance/{instanceID}` 组件限定 context（`pkg/vmcp/credential.go:61-70`）；凭证按 context/name 揭示（`pkg/gateway/client/credential.go:76-96`）。同时 `serverConfigForAction`/`ResolveServerStaticConfiguration` 可解析 server/catalog 固定值，`configureHeaders` 会把非 `UserAllowed` Header 固化到共享 ServerConfig。

**结论**：P02 的专用 resolver 是必要的，不能复用含 fallback 的 `serverConfigForAction`。对个人业务认证应只用精确用户/实例 context；未来真正服务级 Header 必须显式 scope，不能借用管理员配置。真实企业 token 交换仍未实现，不能宣称授权闭环。

### vMCP 工具发现与平台鉴权

**已确认**：vMCP 组件是 catalog entry 快照，ID 服务端分配且稳定（`apiclient/types/vmcp.go:41-58`）；`ToolOverride.Name` 是原始工具名，`OverrideName/Description/Enabled` 只影响展示和启用（`apiclient/types/mcpserver.go:183-202`）。Profiles 与 instance 选择以组件 ID + 原始工具名相交（`pkg/vmcp/profiles.go:44-103`）。API Key 身份从数据库 UID 得出，`mcpServerIds` 或 `*` 才给 `GroupMCP`（`pkg/gateway/server/apikey_auth.go:31-100`、`pkg/gateway/types/apikeys.go:73-90`）；`checkMCPID` 会再解析当前用户连接并校验 scope（`pkg/api/authz/mcpid.go:33-104`）。

**结论**：P01 的 `operationKey + legacyToolName` 分离和 P03 的组件限定白名单符合现有授权模型。主要缺口是 R02-01/R02-03 的 principal 与 loopback 身份，以及 03 不能复用管理 `vmcp_id` 可见性检查。

### MCP/SSE 行为

**已确认**：当前 OpenAPI wrapper 有 `/readyz`，所以常规路径先 GET health；但 `ensureServerReady` 在没有专用 health path 时会 POST 混合 Accept，非 200 后自动 GET SSE（`pkg/mcp/backend.go:89-185`）。官方 v0.27.2 `server.py` 只传 `stateless_http=True`，未传 `json_response=True`；上游 `network.py` 会共享连接池但逐请求重置 Header，完整缓冲业务响应并包装错误。

**结论**：P01 不暴露 `/mcp`、由 02 Go bridge 生成 JSON-only 的架构合理；P02 必须绕过 `ensureServerReady` 的公共回退并用真实请求断言方法、Accept、Content-Type 和 body。当前仍未实际启动官方镜像，`json_response=False` 在该环境的实际响应类型保持“源码推断，待端点实测”，不得写成已验证。

### 05 部署与验收

**已确认**：仓库没有 Obot 主 Compose/Worker Compose；官方镜像可用外部 `OBOT_SERVER_DSN`，`run.sh` 只有在 DSN 为空时才内嵌 PostgreSQL（`run.sh:21-37`）。现有 `/api/healthz` 同时检查 DB 和 controller，尚无 `/api/readyz`（`pkg/gateway/server/router.go:11-25`）。旧 chart 包含 Namespace、Role/RoleBinding 与 ClusterRole/Binding，不能作为无集群管理员路径（`chart/templates/mcp.yaml:1-120`）。

**结论**：05 放弃 K8s、交付两个固定 Compose project、Worker 不挂 DB secret/Docker socket 的方向符合 00 契约。Docker daemon 当前不可用不是计划缺陷，但不能用 `compose config` 冒充 build/up/E2E；证书轮换、别名解析、资源限制和滚动升级仍需真实 Compose 验收。

## 5. 形式化审核与独立复跑

复核了五份 TLA+ 模型/配置、原始 TLC/statechart 输出、EVIDENCE 和本地 skill。五个 skill 的入口脚本均存在且 `bash -n` 通过；但除本 worktree 新增的 `review-evidence/` 外，没有在其他 worktree 执行会覆盖 formal 产物的项目脚本。

本 worktree 独立复跑使用 TLA+ Tools v1.8.0，SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`，输出在 `docs/plans/openapi-workers/review-evidence/formal-rerun/`：

| 模型 | 结果 | 关键状态/反例 |
| --- | --- | --- |
| F01 normal | PASS | 65 generated / 33 distinct，0 left |
| F01 broken | 预期 FAIL | `ExecutionEnteredAtMostOnce` 精确失败 |
| F03 normal | PASS | 613 generated / 241 distinct，0 left |
| F03 broken | 预期 FAIL | `ExecutionRequiresUniqueAuthorizedMatch` 精确失败 |
| F03 chart normal/broken | PASS / 预期 FAIL | Broken 同时给出 unique-route 与 whitelist 反例 |
| F05 normal | PASS | 563,869 generated / 65,706 distinct，0 left |
| F05 full broken | 预期 FAIL | 首个失败为 `OpenAPITrafficGated` |
| F05 chart normal/broken | PASS / 预期 FAIL | Broken 同时给出 traffic gate 与 rolling capacity 反例 |

另核对 02 原始输出：恢复模型 18 distinct 全规则通过，Broken 19 distinct 且仅 `ExactCallerCredentialOnly` 失败，反例是 `u2` 请求 `i2` 时读取 `u1/i1` 凭证。04 原始输出：恢复模型 181,137 distinct 全规则通过；early-release 只破坏 `ConfirmedReleaseInsideLease`，stale-start 只破坏 `ActiveLeaseSafety`。

模型边界声明充分：01 不覆盖热缓存/LRU/重启/业务 exactly-once；02 不覆盖传输、digest、刷新或 SSE；03 不覆盖 Header/body/预算/网络；04 不覆盖 PostgreSQL 锁、token 哈希、公平队列或部署；05 不执行 Docker/Compose、不验证 TLS 字节或轮换。各 PLAN/skill 均没有把设计级通过外推为 E2E。

## 6. 已验证 / 需验证 / 剩余局限

### 已验证

- 读取了 00/01—05 必审材料和相关真实源码，未依据汇总结论替代原始证据。
- 01/03/05 的完整模型和 chart 失败注入在本 worktree 独立复跑，结果与原始证据一致。
- 02/04 原始 TLC 输出、反例和边界完成交叉核对。
- 00 的主要源码事实、凭证 fallback 风险、vMCP 授权身份、API Key scope 和现有 SSE fallback 得到当前源码确认。

### 需要后续验证

- 官方 v0.27.2 镜像真实 POST 响应是否为 SSE；只能待 01/05 本地端点测试。
- 02 固定模式和 Go bridge 实现、03 raw-prefix、04 PostgreSQL 预算、05 Compose 生命周期均未实施。
- mTLS 握手、证书轮换、Compose 别名、readiness 摘除、两副本全局额度和双用户并发隔离均需实现后的真实运行证据。
- 真实企业 token 交换、真实业务 API 和生产部署保持 `UNVERIFIED/GAP`。

### 局限

本次是设计级交叉审核，没有修改或运行实现代码，没有调用业务 API，没有启动/停止服务，也没有把模型结果解释为部署或 exactly-once 结论。审核产物只写入本 worktree 的 `REVIEW.md` 与非敏感 `review-evidence/`。
>>>>>>> feature/openapi-worker-bridge
=======
# R04 集体交叉审核报告

日期：2026-11-09 Asia/Shanghai。审核 worktree：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets`。基线：`2be04f025d1c86144e8670d9e9341b263edc28ea`。

## 1. 结论

**Verdict：发现 P1，G00 不能冻结实施。**

- 未发现 P0 凭证跨用户泄漏或已发生的未授权业务执行；当前共享 Worker、固定后端、HTTP 代理和预算实现均尚未开始。
- 确认 5 项 P1：Worker 租约 fence 字段语义不一致、取消/截止释放结果不一致、入队前凭证与请求体检查顺序不一致、Go/Python `executionDigest` 缺少可落地来源、生产服务认证方式不一致。这些都属于跨 feature 协议不兼容或关键需求无法按共同契约实现。
- 另有 7 项 P2 和 1 项 P3，覆盖等待队列取消、多层级公平性验证、预算来源身份、operation 状态规模、describe 响应上限、MCP 审计/JSON 有界接线、传输层重试和复跑入口一致性。
- 五个 formal verifier 均在本 worktree 的隔离副本中复跑：恢复模型全部通过，故意破坏模型全部只触发预期反例。复跑结果和边界见 `docs/plans/openapi-workers/review-evidence/R04-FORMAL-RERUN.md:1`。
- 本审核未修改其他 PLAN、formal 产物、业务实现或全局配置；未调用真实业务 API，未访问历史 JWT，未启动/停止服务，未部署或操作集群。

## 2. 审核范围与方法

已完整读取：

- 00 的 `ROADMAP.md`、`CONTRACTS.md`、`ACCEPTANCE.md`、`PLAN.md` 和 `REVIEWING.md`。
- 交接文件 `/Users/echo/mcp-platforms/obot/.codex-runs/ORCA-HANDOFF-openapi-workers-2026-11-09.md` 与架构说明 `/Users/echo/mcp-platforms/obot/.codex-runs/ARCHITECTURE-current-platform-2026-11-09.md`。
- 01—05 各自的 `PLAN.md`、formal 模型/配置/输出、`EVIDENCE.md`（如存在）和项目本地 verify skill。
- R04 指定重点为 01/02/03，同时核对 04/05 的部署与验收接口。

真实源码重点复核：

- OpenAPI 快照与 operation 缺口：`pkg/openapi/import.go:32`、`pkg/openapi/document.go:32`。
- 预算来源身份：`pkg/mcp/source.go:7`、`pkg/storage/apis/obot.obot.ai/v1/vmcp.go:54`。
- 现有运行后端：`pkg/mcp/manager.go:149` 只接受 Docker/Kubernetes。
- 现有审计风险：`pkg/api/handlers/mcpgateway/proxy_audit.go:582` 无界累积响应、`pkg/api/handlers/mcpgateway/proxy_audit.go:688` 固定脱敏名单。
- 现有响应包装风险：`pkg/api/server/server.go:186`、`pkg/api/server/server.go:308`、`pkg/api/server/server.go:438`。
- PostgreSQL 迁移基础：`pkg/gateway/db/db.go:32`、`pkg/storage/server.go:47`。

## 3. 发现

### R04-01 [P1] Worker 租约 fence 在“opaque metadata”与必检字段之间冲突

- **状态**：CONFIRMED。
- **影响**：P04 的跨副本安全性质要求 Worker 在距 `LeaseDeadline` 不足 fencing margin 时不得发起业务上游调用；P01/P02 却把预算上下文定义为不透明，P01 还声明 Worker 只消费绝对截止时间。若 HTTP context deadline 大于租约截止减 margin，旧 Worker 仍可能发起新业务调用，导致已回收额度被 stale permit 使用，破坏全局限额。
- **依据**：P01 非Goal 写 Worker 只消费绝对截止时间和调用元数据（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:80`），execute 只携带“不透明预算元数据”（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:158`）。P02 同样称 lease 对 02 opaque（`../openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md:140`）。P04 的 `BudgetContext` 明确包含 `LeaseDeadline` 并要求 Worker 拒绝缺失、过期或近过期调用（`docs/plans/openapi-workers/PLAN.md:116`、`docs/plans/openapi-workers/PLAN.md:127`），还要求执行截止为 `min(HTTP deadline, leaseDeadline-margin)`（`docs/plans/openapi-workers/PLAN.md:277`）。F04 的 stale-start 反例在 `now=2`、`deadline=3`、margin `1` 时触发（`docs/plans/openapi-workers/review-evidence/r04-formal-rerun/f04/docs/plans/openapi-workers/formal/EVIDENCE.md:10`）。
- **责任 feature**：04 主责语义；01 负责 Worker 协议与执行前检查；02 负责公共 Go 类型和序列化；00 冻结唯一方案。
- **建议**：二选一冻结：(a) Worker 可见 typed `LeaseID/LeaseDeadline/BudgetKeyHash` 并按 margin 拒绝；(b) 完全 opaque，Obot 在派发前把 HTTP context deadline 设为 `leaseDeadline-margin`，Worker 只看到一个更早的绝对截止时间。不能保留两种含义。
- **解决条件**：00 契约和 P01/P02/P04 使用同一字段/责任矩阵；新增缺失、过期、仅剩 margin、时钟回拨的负例，断言 DNS 和业务上游连接均为 0；若协议名词变化，F04 模型与 skill 同步记录。

### R04-02 [P1] 取消、deadline 与租约释放缺少跨层 typed outcome 协议

- **状态**：CONFIRMED。
- **影响**：P03 在总 deadline 耗尽时写“取消上游并交还租约”，P04 又要求无法证明上游停止时保守占用到 grace。若 03/02 把任何 `cancelled`、504 或传输失败当作可立即释放，会在旧业务请求仍可能执行时释放全局额度，造成跨副本超发；反之各 feature 自行猜测又会导致语义漂移。
- **依据**：P03 错误表要求 504 后“交还租约”（`../openapi-http-proxy/docs/plans/openapi-workers/PLAN.md:164`），同节又说收到结果或失败后幂等释放（`../openapi-http-proxy/docs/plans/openapi-workers/PLAN.md:171`）。P04 要求可观察 `cancelled` 才释放、无法确认停止则等到 deadline+grace（`docs/plans/openapi-workers/PLAN.md:261`），Worker crash/分区不释放（`docs/plans/openapi-workers/PLAN.md:270`）。F04 模型中 `CancelConfirmed` 只允许 `upstream="none"`（`docs/plans/openapi-workers/formal/ExecutionBudgetLeases.tla:174`），而 P01 的错误结果只有 `phase` 等字段，没有冻结 `upstream_attempted=false` 或“确认停止”的可互操作证明。
- **责任 feature**：04 主责状态机；01 定义 Worker outcome；02/03 传递和映射；00 冻结状态矩阵。
- **建议**：定义唯一 `Outcome` 枚举与字段：pre-dispatch cancel、upstream_attempted=false、business result returned、transport unknown、deadline reached while sent、worker lost、audit/release failed。只有明确 `upstream_attempted=false` 或已返回最终业务结果才允许立即幂等释放；其余进入 reclaim grace。
- **解决条件**：契约给出客户端断开、Obot crash、Worker crash、deadline、Worker cancelled、业务结果、内部响应超限六类分支；两 Obot/两 Worker 假上游测试证明 grace 前峰值不超限、grace 后恢复且无重复 active；P03 错误表和 F04 模型名词一致。

### R04-03 [P1] 入队前的凭证资格与请求体大小检查顺序在 03/04 间不一致

- **状态**：CONFIRMED。
- **影响**：P04 要求业务凭证解析和请求体大小都在预算获取前完成；P03 要求先 `Acquire` 再读取 body，并把凭证值揭示推迟到 Execute。实现会在 409/413 是否占用队列、慢 body 是否占用租约、谁是 Acquire owner 上分叉，影响公平排队、错误语义和有界资源。
- **依据**：P04 固定顺序第 1—3 步为凭证解析、请求体大小解析、预算入队（`docs/plans/openapi-workers/PLAN.md:103`）。P03 明确“读取 body 前先 Acquire”，获取租约后再按 1 MiB 上限读取（`../openapi-http-proxy/docs/plans/openapi-workers/PLAN.md:170`），其接口又要求业务凭证值只在 Execute 内注入（`../openapi-http-proxy/docs/plans/openapi-workers/PLAN.md:225`）。
- **责任 feature**：04/03 各负其责，02 提供 resolver eligibility 与 Header/body typed error，00 冻结顺序。
- **建议**：区分“凭证存在且可用资格检查”和“凭证值揭示”：在 Acquire 前检查当前用户实例必需业务授权资格、operation、声明 Content-Length 与总 deadline；Acquire 后流式读取实际 body 并立即在发送 Worker 前揭示凭证值。若 chunked body 无法预知大小，则按 lease deadline 和 1 MiB reader 在租约内流式截断/拒绝。
- **解决条件**：P03/P04/00 顺序一致；编译级和 HTTP 回归证明缺凭证、错误 Key、无实例、禁用 operation、声明超限均在入队前拒绝；慢/chunked body 在租约内取消并幂等释放；请求体不会在入队前无界缓冲。

### R04-04 [P1] Go 侧 executionDigest 缺少 operation 清单来源，跨语言一致性不可实现

- **状态**：CONFIRMED。
- **影响**：P01/P02 的 `executionDigest` 都包含排序 operation 清单，但当前 Go 导入器只保存 schema/baseURL/Header 建议，不生成 operation 清单。P02 计划由 Go 组装 digest，却没有说明 Go 如何复刻 FastMCP 3.4.7 的 operation 语义。结果是冷启动 digest 不匹配、每次补送失败，或 Go/Python 各自生成不同缓存身份。
- **依据**：P01 要求固定长度前缀编码并包含排序 `operationKey`（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:120`）。P02 要求 Go 侧 digest 输入同样包含 operation 清单（`../openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md:148`）。源码注释和结构明确“MCP tools are generated and listed by the hosted wrapper”，Result 只有 schema/baseURL/Header/metadata（`pkg/openapi/import.go:32`）；`inspect` 也只解析 destination、metadata 和 credential headers（`pkg/openapi/document.go:32`）。
- **责任 feature**：02 主责 Go canonical manifest；01 提供跨语言向量和 Worker 重算；00 冻结算法。
- **建议**：不要让 Go 和 Python 各自解析出两套清单。优先由 02 冻结 canonical snapshot manifest 生成器并持久化 operation 清单，或规定冷启动首包先带 `schemaDigest` + raw snapshot，Worker 返回 canonical `executionDigest` 后再执行；无论哪种都必须提供 Go/Python 同一向量。
- **解决条件**：共享测试向量覆盖同 schema/不同 BaseURL/Header policy/operation 变化、同 schema 多组件、旧 digest 不命中；Go/Python 对每条向量输出完全相同 `executionDigest`；P01/P02 编译级接口测试通过。

### R04-05 [P1] 生产 Worker 服务认证在 01/02/05 间未收敛

- **状态**：CONFIRMED。
- **影响**：内部 execute 携带用户业务 token。若 01 实现 mTLS-only、02 实现 bearer-only、05 按 mTLS+可选 bearer 挂载，三者无法互通；若为集成而降级 bearer 或明文，会把业务凭证暴露在不一致的信任边界。
- **依据**：P01 生产默认 mTLS（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:240`）。P02 配置字段是 token file/token 并建议 bearer（`../openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md:81`）。P05 明确识别冲突并建议生产 mTLS、bearer 仅叠加（`../openapi-k8s-acceptance/docs/plans/openapi-workers/PLAN.md:73`、`../openapi-k8s-acceptance/docs/plans/openapi-workers/PLAN.md:167`）。
- **责任 feature**：00 冻结最低要求；01/02/05 分别实现服务端、客户端和材料。
- **建议**：生产最低要求冻结为 TLS + 客户端证书 mTLS；bearer 只能作为显式第二因子或受命名网关过渡方案。本地验收生成短期 CA/证书，真实握手验证。
- **解决条件**：三份 PLAN 和 CONTRACTS 的必选/可选材料、SAN、CA、轮换顺序完全一致；无证书、错证书、错 CA、过期证书均拒绝；Compose E2E 记录真实 mTLS 握手结果。

### R04-06 [P2] 等待队列票据没有即时取消转换

- **状态**：CONFIRMED design gap；不是当前实现缺陷。
- **影响**：客户端在 waiting 阶段断开时，P04 状态机只允许 active 行 Release/Cancel；等待票只能等 deadline/reclaim。这会造成断开请求继续占用最多 2 秒队列/接口票据，放大 429 并削弱公平性，但不会直接释放 active 业务名额。
- **依据**：P04 `Release/Cancel` 明确“只有 active 能转终态”（`docs/plans/openapi-workers/PLAN.md:199`）；等待公平依赖 wait deadline（`docs/plans/openapi-workers/PLAN.md:215`）。验收 B-05 要求客户端断开和截止时间有幂等释放、无持续泄漏（`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/ACCEPTANCE.md:49`）。
- **责任 feature**：04。
- **建议**：增加 `CancelWaiting(request_id, owner/release_token)` 短事务，按状态条件 `waiting -> cancelled`，递减 waiting_count；重复取消幂等；Obot crash 后仍由 reclaimer 兜底。
- **解决条件**：真实 PostgreSQL 并发测试覆盖 waiting 断开、另一副本同时 Admit、重复 Cancel、错误 token、Obot crash；状态和 waiting_count 重算一致，后续请求立即恢复。

### R04-07 [P2] 公平排队和多层级名额原子性没有设计级反例覆盖

- **状态**：CONFIRMED coverage gap。
- **影响**：F04 只有一个共享 active slot、一个 operation 和一个 waiting ticket，未建模 platform/default/dedicated 三层计数、多 operation 公平轮转或配置下调/更新。TLC 通过不能发现真实锁序死锁、三表计数漂移或慢接口独占。
- **依据**：P04 的核心表和锁序是三层级（`docs/plans/openapi-workers/PLAN.md:161`、`docs/plans/openapi-workers/PLAN.md:199`），公平算法在 `docs/plans/openapi-workers/PLAN.md:211`。F04 边界明确不覆盖 PostgreSQL 锁、多 operation 公平和队列取消（`docs/plans/openapi-workers/review-evidence/r04-formal-rerun/f04/docs/plans/openapi-workers/formal/EVIDENCE.md:77`）。
- **责任 feature**：04；05 负责最终两副本假上游验收。
- **建议**：在实施前增加小型多 operation/multi-scope TLA/statechart，或把该风险明确降级为强制 PostgreSQL 测试门。至少建模两个 operation、两个池、平台层和一条 waiting cancel，注入计数跳变/慢接口占满。
- **解决条件**：模型或真实 PostgreSQL 测试证明接口/池/平台均不超限、两个 runnable operation 可预测轮转、慢接口不阻塞快接口、取消和回收后计数重算；边界同步到 skill。

### R04-08 [P2] sourceRef 规范化、落库形态与 legacy 缺失策略不足

- **状态**：CONFIRMED；等价 URL 数据需要实现期验证。
- **影响**：当前 sourceID 只去掉 scheme 前缀和尾斜杠，大小写、默认端口、用户信息或等价路径别名会产生不同 `sourceID::entryKey`，把同一接口额度拆成多行。P04 又一方面称只存 budget key hash，另一方面表结构保存 raw `source_ref` 并建唯一索引，存储/泄漏和迁移语义不一致。legacy component 的 reference 是可选字段，缺失时可能整链路拒绝。
- **依据**：P04 budget identity 与“只存 SHA-256”声明（`docs/plans/openapi-workers/PLAN.md:88`），但 request/operation 表明确保存 `source_ref` 并唯一索引（`docs/plans/openapi-workers/PLAN.md:150`、`docs/plans/openapi-workers/PLAN.md:176`）。当前规范化实现只有三个字符串操作（`pkg/mcp/source.go:7`）；`ComponentCatalogReferences` 为 `omitempty`（`pkg/storage/apis/obot.obot.ai/v1/vmcp.go:54`）。
- **责任 feature**：04 主责；02 暴露稳定 source identity；00 冻结 canonicalization/迁移。
- **建议**：定义结构化 canonical source identity（scheme 归一、host 小写、默认端口/credentials 处理、path 规则、entryKey trim/case），明确历史 reference 迁移；无法唯一确定时继续 `budget_identity_unavailable`。若必须可观测，可存 source 分类标签而非完整 source URL。
- **解决条件**：PostgreSQL 测试覆盖等价 URL、两个 vMCP/component、legacy retained snapshot、缺失 reference；断言共享同一 budget key 或明确拒绝，且没有重复 active 行。

### R04-09 [P2] operation budget state 数量和 admission pump 扫描缺少长期上界

- **状态**：CONFIRMED plan gap。
- **影响**：每个 distinct source/operation 惰性创建 `openapi_budget_states(scope=operation)` 后没有保留期限、总数上限或冷数据 compaction。admission pump 每 20ms 从 runnable operations 中选择，可能随历史 operation 状态增长放大查询和锁范围；即使队列只有 64，状态表和索引也可无界增长。
- **依据**：operation 状态行由 Enqueue 惰性创建（`docs/plans/openapi-workers/PLAN.md:203`）；公平查询读取所有 runnable operations（`docs/plans/openapi-workers/PLAN.md:215`）；P04 只给终态 request 行 15 分钟清理，没有 operation state 清理（`docs/plans/openapi-workers/PLAN.md:206`）。
- **责任 feature**：04。
- **建议**：为 operation state 增加空闲 TTL/最大行数/分页 candidate set，或仅对存在 waiting/active request 的 operation 动态计算公平状态；回收器按 budget_key 批量重算后删除冷行。管理查询必须分页。
- **解决条件**：真实 PostgreSQL 测试注入大量历史/冷 operation，断言状态行、pump 延迟、索引扫描行数、内存均有上限，热 operation 公平性不变。

### R04-10 [P2] describe/内部 JSON 响应没有独立硬上限

- **状态**：CONFIRMED design gap。
- **影响**：00 和 P01 都说 describe 返回有界 JSON，但 P01 只限制 schema、编译后 plan、operations、parameters 和 JSON Schema 节点，没有限制 describe HTTP envelope。一个 16 MiB plan 中的重复 schema/description/metadata 在序列化后可超过 16 MiB，导致 02/03 文档接口或 Worker ASGI 层以未声明内存/响应大小运行。
- **依据**：00 要求内部业务接口有界 JSON（`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/CONTRACTS.md:39`）。P01 describe 返回 operation 清单和输入 schema（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:139`），限制表只到 plan 字节和节点数（`../openapi-shared-worker/docs/plans/openapi-workers/PLAN.md:200`）。
- **责任 feature**：01；02/03 按同一上限读取。
- **建议**：定义 describe JSON envelope 独立硬上限，例如从 plan 上限和 operation 元数据余量推导；超限时返回 `description_too_large`，并支持分页/按 operation 查询或要求运营精简 schema。execute 错误/结果 envelope 已有推导上限，也应在同一表格冻结。
- **解决条件**：构造最大 operations/schema 节点/描述长度的边界 schema，断言 ASGI 序列化前后、02 client 读取和 03 docs API 均在配置上限内；超限不发业务上游且无伪成功。

### R04-11 [P2] Agent MCP 出口的 JSON 转换和有界审计接线未落到 02/03 接口

- **状态**：CONFIRMED integration gap。
- **影响**：00/P04 要求 HTTP 与 MCP 共用响应/审计边界，但 P02 只写“审计脱敏/不含 token”，没有明确 `tools/call` 业务 body 转 MCP JSON/text 时复用 P04 bounded component。8 MiB 原始 body 经 JSON/base64/文本转义可产生更高但未声明的内存，且现有 MCP audit 确实无界缓冲、固定脱敏名单不认识自定义 `authtoken`。
- **依据**：P04 要求 01/02/03 复用 bounded components（`docs/plans/openapi-workers/PLAN.md:300`），并规定 MCP JSON/text 转换不超过原始响应上限（`docs/plans/openapi-workers/PLAN.md:298`）。P02 审计要求只到日志脱敏（`../openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md:257`）。源码 `auditResponseBody.Read` 将所有响应写入 buffer（`pkg/api/handlers/mcpgateway/proxy_audit.go:582`），固定名单见 `pkg/api/handlers/mcpgateway/proxy_audit.go:688`。
- **责任 feature**：04 提供共同组件；02 接入 MCP bridge；03 接入 HTTP audit。
- **建议**：冻结 `ToBoundedMCPContent` 的上限/截断/失败语义：业务 result 保留原状态，但工具内容转换超限返回平台错误或显式截断内容，不能伪装完整业务 body。审计默认元数据，启用正文时 32 KiB 截断，动态脱敏所有声明业务认证 Header。
- **解决条件**：MCP tools/call 与 HTTP 代理同一 8 MiB 响应/超限响应测试；审计记录大小、截断标记、脱敏结果；测试产物扫描不到合成 token。

### R04-12 [P2] “不自动重试”尚未约束 Go transport 层和连接重放

- **状态**：CONFIRMED plan gap；实际 transport 行为需实现后注入验证。
- **影响**：P01 requestID fence 是 Worker 内存态，Worker 重启后丢失；Go HTTP transport 可能在复用连接失效时对部分请求做底层重试。如果 02 仅在应用层不调用第二次，不能证明一个业务 execute 只有一次网络 attempt，可能违反 04 的一次上游尝试性质。
- **依据**：P02 只写 snapshot 补送后失败不自动重试 execute（`../openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md:144`），未定义禁用 transport retry/连接重放；F04 假设唯一 upstream attempt（`docs/plans/openapi-workers/formal/ExecutionBudgetLeases.tla:138`），F01 只证明 Worker 自身状态机不重试，不覆盖 Obot transport。
- **责任 feature**：02 主责 client transport；01 定义 requestID 语义；04 标记 uncertain outcome。
- **建议**：02 使用不透明重试策略的自定义 transport或显式禁用 Go transport retry；每次 execute 只允许一次 Worker attempt；连接池不得在请求已开始写入后重放。Worker 重启后不承诺 requestID 去重，Obot 必须永久拒绝同一不确定 requestID 的自动补发。
- **解决条件**：注入死连接、半发送、Worker kill、重定向和连接重建；`httptrace`/假 Worker/假上游断言写入次数与上游计数均不超过一次；审计保留 uncertain 结果。

### R04-13 [P3] P02 verify skill 的复跑命令未按 RTK 约定书写

- **状态**：CONFIRMED documentation issue。
- **影响**：项目约定所有 shell 命令经 `rtk` 执行。P02 skill 写 `bash .codex/.../verify.sh`，与其它 feature 不一致，可能让后续复跑绕过本机 token 优化约定。这不影响本次模型结论。
- **依据**：`../openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/SKILL.md:21` 的 Re-run Command 使用裸 `bash`（`../openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/SKILL.md:26`）；R04 实际复跑已通过 wrapper 执行并记录于 `docs/plans/openapi-workers/review-evidence/R04-FORMAL-RERUN.md:15`。
- **责任 feature**：02。
- **建议**：改为 `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh`。
- **解决条件**：更新后用该命令复跑一次，保持 restored pass / broken expected fail。

## 4. 重点链路复核

### 跨副本计数与租约

P04 采用 PostgreSQL 行级状态、短事务、lease deadline/fencing/grace reclaim 的方向正确，F04 的两个反例也准确对应提前释放和 stale start。阻断点不在模型核心性质，而在 P01/P02/P03 的协议字段、outcome 和调用顺序未与 P04 对齐（R04-01—R04-03）。

### 公平排队

“慢接口跳过、runnable operation 按最久未放行优先”的设计能满足避免全局队首阻塞的目标，但目前只是 prose + future tests。三层计数、配置变更、等待取消和历史 operation 规模仍需要模型或强制 PostgreSQL gate（R04-06、R04-07、R04-09）。

### 取消与崩溃回收

F04 对保守回收的抽象正确。P04 也明确不确定结果不能虚假回滚。需要补的是可互操作 proof 字段：waiting cancel、pre-upstream cancel、upstream attempted、worker lost、deadline while sent（R04-02、R04-06、R04-12）。

### 响应与审计上限

P04/P01 对原始请求、响应、解压、base64、Header 和审计正文的硬边界总体完整，base64 推导公式正确。缺口是 describe envelope 和 MCP `tools/call` 转换/审计没有成为 01/02 的明确接线责任（R04-10、R04-11）。

### 01/02/03 接口

P03 对 P02 提出的 principal、raw input、Header 分类、typed error 和 `BudgetSourceID` 需求是必要的；R04-03/R04-04 从预算顺序和 digest 来源进一步确认该接口还不能直接实施。服务认证分歧单独构成 R04-05。

## 5. Formal 审核与复跑

为避免修改其他 worktree，五个模型/配置/view/skill 被复制到 `docs/plans/openapi-workers/review-evidence/r04-formal-rerun/` 后复跑。F03 复制脚本只把 `git rev-parse` 根路径固定为隔离副本，模型、配置、不变量、预期退出码和反例断言未改。

| Feature | 恢复模型 | 故意破坏 | 结论 |
| --- | --- | --- | --- |
| F01 | PASS，33 distinct，全部规则 hold | 57 distinct / 120 steps，重复执行/fence/no-retry 反例 | 冷 miss 一次补送与 requestID fence 有效；不覆盖 digest 字节、热缓存、重启 |
| F02 | PASS，18 distinct | 仅 `ExactCallerCredentialOnly` 失败，19 distinct | 精确实例凭证隔离有效；不覆盖 Key scope/digest/transport |
| F03 | PASS，613 generated / 241 distinct | 仅授权执行不变量失败；chart 给出歧义路由和白名单绕过 | 授权匹配抽象有效；不覆盖 strict parser/Header/body |
| F04 | PASS，181,137 distinct | early release 与 stale start 各只触发目标不变量 | 租约核心性质有效；不覆盖 PostgreSQL/多 operation/公平/HTTP |
| F05 | PASS，65,706 distinct | traffic gate 与 rolling capacity 预期失败 | Compose 设计状态机有效；不执行 Docker/YAML/TLS |

详细状态数、工具 SHA、时间戳和图表位置见 `docs/plans/openapi-workers/review-evidence/R04-FORMAL-RERUN.md:21`。所有通过均为设计级证据，不得外推为实现、PostgreSQL 并发、E2E、Compose 部署或生产验证。

## 6. 已验证 / 需要验证 / 剩余局限

### 已验证

- 读取 00 契约/验收/总计划、五份 feature PLAN/formal/skill 和相关真实源码；未以其他 REVIEW 或汇总替代证据。
- 当前源码确认：Go importer 不生成 operation 清单；source normalization 过于简单；现有 backend 只有 Docker/K8s；现有 MCP 响应审计无界缓冲且固定脱敏名单不足；统一 response writer 会污染业务响应。
- 五个 formal verifier 在本 worktree 隔离副本中复跑，均达到预期 restored pass / intended broken fail。
- `git diff --check` 通过；当前 HEAD 仍为基线 `2be04f025d1c86144e8670d9e9341b263edc28ea`。

### 需要后续验证

- Go/Python executionDigest 与 canonical operation manifest 向量。
- typed lease/outcome 协议、等待取消和真实 PostgreSQL 多副本/多 operation 公平测试。
- Go transport 无重放、DNS pinning、mTLS、响应/审计/MCP 转换上限。
- 两 Obot/两 Worker、假业务系统、Compose 双栈和真实企业授权交换均未实施或执行。

### 剩余局限

本次是设计级审核，没有修改业务代码或运行实现流程。R04-12 的 transport 行为、R04-08 的历史 URL 别名数据和所有 E2E/部署结论均保持待验证，不应写成当前已发生故障或已通过。
>>>>>>> feature/openapi-execution-budgets
