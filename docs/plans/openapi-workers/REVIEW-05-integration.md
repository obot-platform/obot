# R05 集体交叉审核报告

日期：2026-11-09。审核者：05 worktree / `feature/openapi-k8s-acceptance`。结论：**partial / blocking for G00**——审核已完成并落盘证据，但存在 5 个 P1，契约冻结前必须由 00 协调处理。

本报告只评审设计和证据，不实施、不修改其他 worktree、不部署、不访问真实业务系统或历史凭证。

## 1. 审核范围与完成标准

已完整读取：

- 00：`PLAN.md`、`ROADMAP.md`、`CONTRACTS.md`、`ACCEPTANCE.md`、`REVIEWING.md`。
- 交接：`ORCA-HANDOFF-openapi-workers-2026-11-09.md`、`ARCHITECTURE-current-platform-2026-11-09.md`。
- 01—05：各 worktree 的 `PLAN.md`、formal 模型/配置、形式化证据和项目本地 verify skill。
- 真实源码：运行后端、K8s/Chart 权限、OpenAPI 快照、vMCP/授权/凭证、HTTP server、审计和数据库迁移链路。

完成标准：

1. 覆盖 01—04 的共享协议、恢复和资源边界，不只检查 05。
2. 核对已有 Namespace/固定工作负载要求与当前 Compose 目标的关系。
3. 复核形式化性质、故障注入、图表和证据边界；至少复跑一组项目本地命令，其余模型以不写他人 worktree 的方式复跑 TLC。
4. 每项发现给出严重程度、影响、证据、责任 feature 和可验证解决条件。

## 2. 总体判断

各 PLAN 对现有源码事实的整体核查质量较高：P01 准确识别 v0.27.2 的 SSE、完整缓冲和启动转换问题；P02 准确识别 Docker/K8s 依赖和凭证 fallback 风险；P03 准确识别 raw path 与管理授权不能复用；P04 的 PostgreSQL 租约方向与现有迁移基础相符；P05 明确没有执行 Compose，不把设计模型冒充部署。

但共享契约尚未达到冻结条件：服务认证、预算身份、Worker 租约 fencing、双 Worker 快照恢复和部署目标范围仍存在跨 feature 分歧。P1 见下表。形式化模型本身均可复现预期通过/失败模式，但 P02 的身份模型缺少同一用户多实例反例，P05 的 `NoDeadlock` 证据和 broken 图表可读性有缺陷。

## 3. 发现

| ID | 严重程度 | 状态 | 发现 | 影响 | 依据 | 责任 | 建议与解决条件 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R05-01 | P1 | 已确认 | 原交接要求“公司已有 Namespace 中普通 Deployment/Service”，当前 00/P05 改成 Compose-only 并声明真实集群不再是目标，缺少显式需求变更记录或替代验收。 | 若用户仍要求公司 Namespace 部署，当前交付无法满足；若需求确已改为 Compose-only，原验收项被静默删除。 | Handoff L102/L183；P05 PLAN §1/§4 声明真实集群不再是目标；当前 `chart/templates/mcp.yaml:4` 创建 Namespace、`:83` ClusterRole、`:92` ClusterRoleBinding，不能作为无集群管理员路径。 | 00/05 | 00 二选一并记录：A. 恢复预置 Namespace 的固定 Deployment/Service 交付与 RBAC/挂载/验收；B. 记录用户明确接受 Compose-only，并把原 Namespace 目标标记为已废弃而非遗漏。解决前不冻结。 |
| R05-02 | P1 | 已确认 | 双 Worker 共享网络别名下，`snapshot_required` 的“同请求补送一次”可能被路由到另一个未加载 Worker，不能保证恢复。 | 正常负载均衡、重启或单 Worker 缓存淘汰后，第一次 miss 在 worker-1、补送落到 worker-2 仍 miss；F-01/F-02 可随机失败或永远无法证明两个 Worker 均热。 | P01 PLAN §4.4/§4.6：每 Worker 本地缓存、miss 后补送一次、无持久缓存；P02 PLAN §4.3 只定义一个 Worker client 和一次补送；P05 PLAN §5.1 两个 Worker 共用 `openapi-worker` alias，§5.5 要求重启恢复。 | 01/02/05 | 冻结确定性方案：Obot 首次执行主动携带快照、Worker 按 digest 向 Obot 拉取、按 digest 粘路由并预热替代副本，或显式向 `worker-1/2` 逐个预热。用两 Worker 随机路由、重启、缓存淘汰和两个 Obot 的测试证明一次恢复策略稳定生效且补送前上游计数为 0。 |
| R05-03 | P1 | 已确认 | Worker 服务认证没有冻结：P01 默认 mTLS，P02 建议 bearer，P05 只是提出 mTLS 可叠加 bearer。 | Obot client、Worker server 和 Compose secrets 交付会各自实现不同认证，导致集成失败或为联调降级 TLS。 | P01 PLAN §4.8；P02 PLAN §4.1；P05 PLAN §3.1/§5.4。 | 00/01/02/05 | 00 冻结必选与可选层次，例如生产必须 mTLS、bearer 只能叠加；同步更新 P01/P02 字段和 P05 secrets/轮换/负例。验收需真实错误 client cert/token、过期证书和降级 HTTP 均拒绝。 |
| R05-04 | P1 | 已确认 | P02 的 Executor/Description 契约未提供稳定预算来源，P03/P04 都要求 `BudgetSourceID`/`sourceRef`。 | 03 无法生成 04 的全局预算键，或各自回退到 component/executionDigest，导致同一来源接口额度被拆分或不同来源误合并。 | P02 PLAN §4.3 `Reference/Invocation` 无 source identity；P03 PLAN §6.1 要求 `BudgetSourceID`；P04 PLAN §4.1 要求 `ComponentCatalogReferences` 的 `sourceID::entryKey`；真实字段存在于 `pkg/storage/apis/obot.obot.ai/v1/vmcp.go:54`。 | 00/02/03/04 | 在冻结协议中把 sourceRef 暴露到 ResolvedDescription/Budget key，并规定 legacy 缺失、多来源、entryKey 空的处理。测试两个 vMCP/component 别名命中同一预算行，不同 sourceRef 不合并。 |
| R05-05 | P1 | 已确认 | P01/P02 把预算上下文视为不透明，P04 的过期 fencing 却要求 Worker 解释 LeaseDeadline 并在发上游前检查。 | 若 Worker 不检查 lease deadline/margin，旧 Worker 可在租约过期后继续触达业务系统，P04 formal 的 `ActiveLeaseSafety` 与实现脱节。 | P01 PLAN §4.5 称预算元数据不透明；P02 PLAN §4.3 称 lease 对 02 不透明；P04 PLAN §4.3/§7.3 要求 Worker 拒绝缺失/过期 lease，并用 `leaseDeadline - fencingMargin`。 | 00/01/02/04 | 冻结 Worker 可见的 LeaseID、LeaseDeadline、key hash 字段与必检规则；保留 release token 只在 Obot/DB 侧。增加缺失、过期、剩余时间不足、时钟偏差负例和假上游计数 0 断言。 |
| R05-06 | P2 | 已确认 | P01 默认拒绝 private/loopback，P05 假业务系统在 Compose 私网，但 P05 未列出允许该测试上游的显式 egress 配置和负例。 | 按默认配置验收必然被 Worker 拒绝；若联调时放宽为允许全部私网，又无法证明 DNS/origin 约束没有被削弱。 | P01 PLAN §4.7；P05 PLAN §6 fake-business 只描述共享网络，未给出 Worker egress override。 | 01/05 | P05 acceptance override 显式只允许 fake-business 服务名/CIDR；断言 fake 调用成功，同时 loopback、其他私网 host、混合 DNS 和 origin 漂移拒绝。 |
| R05-07 | P2 | 已确认 | 03 的 operation 匹配依赖 Worker `Describe`，但 04 要求执行预算在 Worker 调用前获取；冷编译/补快照发生在 Acquire 前后没有统一顺序。 | 大量未授权/未入队请求可通过 Describe 触发编译资源消耗；或者每个合法请求先 Worker 后预算，破坏 04 的统一入口和排队语义。 | P03 PLAN §5.2/§5.5/§6.1；P04 PLAN §4.2 的固定顺序和 “describe 不消耗接口预算” 边界。 | 00/02/03/04 | 冻结：匹配使用本地发布快照，或 Describe/预热独立于 execute 且有独立并发预算，Execute 必须先 Acquire 再调用 Worker。测试慢编译、队满、无资格和 Worker 不可用时的调用顺序。 |
| R05-08 | P2 | 已确认 | P02 formal 每个用户只有一个实例，无法表达同一用户在两个实例间借错凭证；工具/组件授权也全部为 true。 | 模型只能证明跨用户 fallback，不能证明“exact instance context”中同一用户错误实例/token 的泄露路径，覆盖弱于宣称。 | P02 PLAN §11.2 明确两个用户、两个实例且一一对应；模型 `Credentials` 按 instance 存储，但 `InstanceOwner` 使每个用户只有一个可达实例。 | 02 | 扩展为 u1 拥有 i1/i2 且 token 不同、i2 缺失，u2 拥有 i3；再增加一个禁用工具/组件。Broken fallback 必须出现 u1:i2 借 u1:i1 的反例，恢复模型仍拒绝。 |
| R05-09 | P2 | 已确认 | P05 formal 的 `NoDeadlock == TRUE` 是恒真式，TLC 输出警告；broken 图中路由门槛被删除后的 unsafe edge 仍标成“after every gate holds”。 | 证据把恒真式列为通过规则，图表读者无法看懂反例是破坏 gate 后放行，违背 formal 证据可读性要求。 | `docs/plans/openapi-workers/formal/ComposeLifecycle.tla:261`；`statechart-base/tlc.out:11` 警告；`statechart-broken-chart/chart.mmd:30`。 | 05 | 删除恒真 invariant，显式 `CHECK_DEADLOCK TRUE` 并记录原始输出；为 `BreakRouteGate` 生成“unsafe route gate admits request”独立边标签。复跑后证据不得再列出恒真 `NoDeadlock` 为 invariant。 |

### 需要验证但未构成缺陷

- Compose v5.5 对固定服务、资源限制、多服务 alias 和 stop grace 的实际行为：P05 已标 `UNVERIFIED`，需在实现后以解析后的 config 和容器清单验证。
- P01 对 FastMCP 3.4.7 私有 IR 的兼容性：已固定依赖，仍需实施时用真实 wheel 兼容样例和 `uv lock --check` 验证。
- P03 Go 1.27 raw path/ServeMux 行为：计划判断与当前源码结构相符，但必须以代码回归和真实 HTTP 请求验证。
- 真实企业授权交换、生产 TLS 入口、生产容量和真实集群权限均未验证；不能由模型或静态检查外推。

## 4. 真实源码核查

| Feature | 核查结论 | 关键源码证据 |
| --- | --- | --- |
| 01 | 现有 OpenAPI 是每配置 wrapper：端口 `/mcp`、schema 文件、`/readyz`；没有固定 Worker。 | `pkg/mcp/openapi.go:57`；`pkg/mcp/manager.go:45`。 |
| 02 | 当前 backend 只接受 Docker/Kubernetes；K8s 初始化要求 local config；凭证 reveal 支持有序 context fallback，必须为 OpenAPI 改专用无 fallback resolver。 | `pkg/mcp/manager.go:149`；`pkg/services/config.go:592`；`pkg/gateway/client/credential.go:76`；`pkg/mcp/action.go:407`。 |
| 03 | `/api-proxy` 不存在；普通 `/api/` 由 404 catch；`Wrap` 会添加平台 Header/Cookie/audit 语义，业务响应需专用分支；管理 `vmcp_id` 不能替代调用资格。 | `pkg/api/router/router.go:611`；`pkg/api/server/server.go:186`；`pkg/api/authz/resources.go:109`。 |
| 04 | gateway 迁移已存在且可扩展；现有 MCP 审计存在完整缓冲和固定 Header 脱敏名单，不能复用于新链路。 | `pkg/services/config.go:477`；`pkg/api/handlers/mcpgateway/proxy_audit.go:68`；`proxy_audit.go:688`。 |
| 05 | 现有 Helm/K8s 路径创建 Namespace 和 ClusterRole/Binding；现有 healthz 是 DB+controller 复合状态，独立 readyz 尚不存在。 | `chart/templates/mcp.yaml:4`、`:83`、`:92`；`pkg/gateway/server/router.go:16`。 |

上游 `mcp-images` tag `v0.27.2` 通过 `git ls-remote` 核实为 `64b62ab7c8862b15bc29b5a5805f3999e486136f`；raw tag 中 `pyproject.toml` 固定 `fastmcp==3.4.7`、`httpx==0.28.1`、`httpcore==1.0.9`，`server.py` 只传 `stateless_http=True`，`network.py` 完整 `bytearray` 缓冲。P01 对现有 wrapper 的源码判断成立。

## 5. 形式化审核与复跑

### 复跑结果

- `REAL`：本 worktree 执行 `rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh`，退出 0；完整基线 65,706 distinct states，normal chart 37 states / 8 states / 13 arrows，broken chart 64 states / 10 states / 28 arrows，并精确触发 `OpenAPITrafficGated` 与 `PlannedRollRetainsCapacity`。
- `REAL`：为避免写入 01—04 worktree，把其 TLA/config 复制到本 worktree `review-evidence/r05-formal-rerun/` 后直接运行 pinned TLC 2026.11.06.222357：
  - F01 baseline 0 / 33 distinct；broken TLC invariant violation，22 distinct。
  - F02 baseline 0 / 18 distinct；broken `ExactCallerCredentialOnly`，19 distinct。
  - F03 baseline 0 / 241 distinct；broken `ExecutionRequiresUniqueAuthorizedMatch`，21 distinct。
  - F04 baseline 0 / 181,137 distinct；early release 违反 `ConfirmedReleaseInsideLease`；stale start 违反 `ActiveLeaseSafety`。
- 上述复跑只证明模型/配置的当前通过与故障注入模式，不是实现、E2E、Compose 部署或 exactly-once 证明。

### 模型审计

| Feature | 性质与伤害 | 结论 |
| --- | --- | --- |
| 01 | snapshot miss 一次补送、requestID 一次执行 fence，防重放副作用。 | 与设计对应，故障注入真实。但它只覆盖单请求/单进程冷缓存，不覆盖双 Worker 路由、重启、LRU 和并发去重；不能支撑 R05-02 的恢复结论。 |
| 02 | 精确用户/实例凭证，防跨租户 token 泄露。 | 能证明跨用户 fallback 反例，但同一用户多实例未建模，见 R05-08。 |
| 03 | 唯一路由 + 三层白名单 + raw path/query 保留，防越权调用。 | 模型、反例和图表可读；严格 URI parser 被抽象，需实现回归补齐。 |
| 04 | 跨副本 active lease、过期 fencing、保守回收，防超发和 stale upstream。 | 与 P04 设计对应，故障注入具体。PostgreSQL 锁、公平队列和 HTTP 取消仍按边界留给实现测试。 |
| 05 | 流量门槛和计划内滚动升级保留容量。 | 两个目标性质有效，但 deadlock/图表证据质量缺陷见 R05-09；不解析 Compose/TLS，不证明部署。 |

## 6. 证据边界

- `REAL`：只包含本报告列出的本地源码检查、上游 tag/raw 文件读取、TLC/formal 复跑和 artifact 阅读。
- `FACT`：源码/文件事实均已标注当前 worktree 或对应 feature worktree。
- `UNVERIFIED`：Docker daemon 当前不可用，未执行 Compose config/build/up、镜像构建、E2E、浏览器 UI、真实集群或生产网络验证。
- `GAP`：01—04 业务实现尚未开始；05 部署文件尚未存在；真实企业 token 交换、生产容量和真实集群权限不在本轮验证。
- 未读取/复制历史 JWT，未调用真实业务 API，未修改用户服务或全局配置，未 commit/push。

## 7. 审核产物

- 本报告：`docs/plans/openapi-workers/REVIEW.md`。
- F05 项目本地复跑：`docs/plans/openapi-workers/formal/EVIDENCE.md` 及 `statechart-*`。
- 01—04 只读模型复跑证据：`docs/plans/openapi-workers/review-evidence/r05-formal-rerun/`。
- 复跑摘要：`docs/plans/openapi-workers/review-evidence/R05-FORMAL-RERUN.md`。
