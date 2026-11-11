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
