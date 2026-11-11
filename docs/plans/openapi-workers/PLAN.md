<<<<<<< HEAD
# 00 总计划与协调

日期：2026-11-09。阶段：planning and formal verification。结论：partial；本文件落盘不代表实施完成。

## 问题与目标

把现有 Obot OpenAPI runtime 改造成固定共享 Workers 执行链，保留 MCP/vMCP 与用户业务凭证基础，增加普通 HTTP 应用出口、跨副本接口预算和两个 Docker Compose 部署单元；每个 feature 设计在交叉审核前完成可复跑的设计级形式化验证。

用户只与 00 中的协调 Codex 沟通。每个 feature 必须由自己 worktree 中的 Orca Codex 先规划，全部计划完成后交叉审核，由 00 处理分歧和冻结共享契约，再按依赖实施。

## 现有基础与证据

- 完整需求来源：`/Users/echo/mcp-platforms/obot/.codex-runs/ORCA-HANDOFF-openapi-workers-2026-11-09.md`。
- 架构来源：`/Users/echo/mcp-platforms/obot/.codex-runs/ARCHITECTURE-current-platform-2026-11-09.md`。
- 六个既有分支和 worktree 已通过 Orca 与 Git 核实，起点均为 `2be04f025d1c86144e8670d9e9341b263edc28ea`。
- 接管时六个 feature worktree 均干净，尚无计划文件或项目 Run；原工作区仅有用户自己的 README 修改。
- 已读取根 AGENTS.md、UI 子目录约定、ADR 指南，以及当前安装 Orca 的 CLI、orchestration 和 coordinator-loop 指南。
- 源码定位：`pkg/services/config.go`、`pkg/mcp/manager.go`、`pkg/mcp/backend.go`、`pkg/api/handlers/mcpgateway/handler.go`、`pkg/vmcp`、`pkg/gateway/client/credential.go`。
- 静态核查发现：现有启动仅支持 Docker/Kubernetes 运行后端；健康探测存在 SSE 回退；代理审计存在完整响应缓冲。以上是源码发现，尚非新链路运行验证。

## 范围与非目标

范围是计划、契约、调度、跨 feature 写集协调、集成审查、证据和统一汇报。00 不代写其他 feature 的初始 PLAN，不在实施 worker 正在修改的文件中直接接管实现。

不新增远程 MCP 类型、Endpoint 顶层管理对象、业务权限引擎、Eunomia、任意脚本转换或默认异步任务机制。不提交、不推送、不创建 PR、不部署生产，不访问历史真实 JWT，不改原工作区。

## 接口与快照契约

以本目录 `CONTRACTS.md` 为唯一共享契约。当前版本为 DRAFT-0.2，供 feature 规划、形式化验证与交叉审核；没有冻结记录时禁止实施。

冻结至少覆盖：快照摘要、稳定 operation 身份、参数位置绑定、用户与临时凭证上下文、业务 HTTP 语义、错误、截止时间与取消、共享预算、缓存、发布更新、Worker 服务认证和无 SSE 行为。

## 文件归属

- 00 独占本目录的 PLAN、ROADMAP、CONTRACTS、ACCEPTANCE、ORCA-INDEX、REVIEW、任务说明和协调证据。
- 01—05 各自独占对应 worktree 的 PLAN、REVIEW、F 形式化模型/证据/项目本地 verify skill、实施与验证交付。
- 共享 Go 类型、服务组装、生成文件默认由 02 负责。03 接手路由时需由 00 明确移交写权限；04 对公共链路的接线另行串行安排。
- 最终集成在 05 既有 worktree 中通过审核后的限定路径补丁进行，不用提交交换代码，不覆盖各 feature 自己的计划。

## 依赖与门槛

完整 DAG 和交叉审核分配见 ROADMAP。F01—F05 分别依赖对应 P 任务；R01—R05 等全部 P/F 完成；所有实施任务都依赖 00 的契约冻结任务，该任务又依赖五个 feature 的交叉审核结果。

规划与实施默认都最多两个 worker 同时工作。计划任务完成、审核任务完成、实施完成分别记录，不能相互替代。

## 实施步骤

1. 00 落盘总计划、契约草案、验收矩阵和索引，建立真实 Orca Run 与任务 DAG。
2. 在 01、02 既有 worktree 启动规划 Codex；有空位后推进 04、03、05 的规划。
3. 五份 PLAN 全部落盘后，按最多两个 worker 并发执行 F01—F05：每个 feature 选择一个高代价状态机/并发性质，实际运行 TLC/合适验证工具，生成项目本地 verify skill、失败注入反例、恢复运行和 Mermaid 状态图。
4. 全部 F 完成后让各 feature Codex 按 ROADMAP 分配交叉审核，在自己的 worktree 写 REVIEW；审核覆盖 PLAN、模型、工具输出、反例、skill 复跑和边界。
5. 00 汇总问题，为发现指定责任人，协调修订和针对性复核；未解决的阻断问题不得通过冻结门槛。
6. 记录冻结版本、内容摘要、审查证据与写集，再真实启动 01、02 实施；按依赖和并发上限继续。
7. 集成与验收完成后汇报修改、实际运行结果和剩余缺口。

## 验证与证据

索引记录绝对计划路径、分支、selector、真实 Run/Task/Dispatch ID、阶段、结果和证据位置。派发回执只证明派发接受；启动证明需依据返回的 ready/turn-started 状态。没有回执不能声称已并行。

每次 worker 报告后检查产物与关键证据，只有存在缺口或矛盾时补做验证。状态使用 verified / partial / blocked；运行测试、静态审查和未执行项分开标注。
F 交付必须包含工具版本/摘要、模型与设计映射、抽象边界、实际通过/失败结果、预期反例、Mermaid 图和项目本地 skill 的一条复跑命令。00 抽查复跑；不运行模型或只写伪证据不能过关。

## 失败行为与恢复

worker 启动失败或结果不确定时读取当前版本 recovery-and-cleanup 指南，不盲目重试、不新建重复工作区。通过 orchestration 消息提问和交付，不用 worker 终端输入替代任务通信，不写 polling/watch 脚本。

Orca 原生唤醒到达后继续调度。外部聊天缺少 ORCA_TERMINAL_HANDLE 的已知情况不构成修改全局 Codex 配置的理由。

## 风险

- 跨语言快照与 operation 命名可能漂移，必须以共享样例和真实兼容性检查消除。
- 旧 MCP 代理中的会话、SSE、审计行为不能因新 HTTP 通路而漏掉。
- 租约过期、连接断开不一定证明业务系统已经停止执行，恢复语义需保守设计。
- 真实企业授权交换、生产容量和 Compose build/up/E2E 尚未验证，不能由设计模型或静态检查推导。

## 交付物

计划与协调文件、真实 Run/Task DAG、五份 feature PLAN、五组 F 形式化模型/证据/项目本地 verify skill、五份 feature REVIEW 与 00 汇总审核、冻结契约、各阶段 Dispatch/证据索引，以及最终本地可审查变更与验收结论。
=======
# P04 接口调度与资源预算计划

日期：2026-11-09。分支：`feature/openapi-execution-budgets`；基线：`2be04f025d1c86144e8670d9e9341b263edc28ea`。状态：planning；本文件只规划，不表示实施、测试或 E2E 通过。

## 1. 问题与目标

当前 OpenAPI 执行链没有跨 Obot/Worker 副本的接口级在途计数、有限排队、统一截止时间、崩溃回收或全链路大小预算。直接使用每个副本的内存信号量会造成各副本独立放大限额；把上游请求包在数据库事务里又会占住连接并放大 PostgreSQL 故障。P04 的目标是基于现有 PostgreSQL/GORM 状态基础，为普通 HTTP 出口和 Agent MCP `tools/call` 提供同一个预算入口，并给 Worker、内部 JSON、HTTP 返回和审计链路提供可验证的硬边界。

完成标准：

- 同一目录来源的同一 operation 经多个 vMCP、多个平台入口和多个 Worker 发布时，在 PostgreSQL 中共享一个预算键；不按用户、组件、别名、Worker 或快照版本拆分额度。
- 平台总量、默认池、专属池、接口并发和队列均为有限值；未配置专属限制的 operation 使用默认池并仍受平台总量约束。
- 入队、放行、释放和过期回收都是短数据库事务中的原子状态转换；上游请求期间不持有数据库事务或连接。
- 队列有总上限和接口上限，等待有时间预算；慢接口不会阻塞其他可运行接口，放行策略可观察且可重复测试。
- 取消、Obot 崩溃、Worker 崩溃、网络分区、租约过期和数据库不可用都有明确行为；过期调用不能发起新的上游请求，已提交给业务系统的请求不虚假声明可回滚。
- 请求体、响应体、解压、Header、base64/JSON 编码、HTTP 返回和审计正文全部有界；自定义业务认证 Header 脱敏。
- 预算存储不可用时新调用失败关闭，不回退为各副本内存限额。

## 2. 已核查现有基础

以下标注来自当前 worktree 源码和 00/01/02 草案阅读结果；未运行任何业务 API、生产服务或集群。

### 2.1 PostgreSQL 与迁移事实

`FACT(local)`：

- `pkg/services/config.go:465` 由 `storage.Start` 建立数据库访问，随后创建 gateway GORM 连接并运行 `gatewayDB.AutoMigrate()`；平台已区分存储层和 gateway 层。
- `pkg/gateway/db/db.go:32` 的 gateway 迁移使用事务，PostgreSQL 下通过 `pg_advisory_xact_lock` 串行化多副本迁移；`pkg/gateway/db/db.go:137` 集中 `AutoMigrate` gateway 类型。
- `pkg/storage/server.go:27` 说明存储层也已有跨副本 PostgreSQL advisory lock；预算状态不需要引入新的迁移集群。
- `pkg/services/config.go:520` 已有 SQL leader election，但预算准入必须面向所有 Obot 副本，不能只在 leader 上执行请求。
- `pkg/gateway/client/claude.go:36`、`pkg/gateway/client/localauth.go:145` 等已有短事务 advisory lock / `FOR UPDATE` 模式，可作为实现风格参考。
- `pkg/gateway/db/automigrate_postgres_test.go:15`、`pkg/storage/server_postgres_test.go:16` 已有 `OBOT_TEST_POSTGRES_DSN` 加独立 schema 的并发迁移测试模式，P04 可复用而不新增测试依赖。

### 2.2 OpenAPI 身份与快照事实

`FACT(local)`：

- `apiclient/types/openapi.go:69` 保存规范化 JSON schema 快照；`pkg/openapi/import.go:22` 固定单份 schema 1 MiB，并在导入和快照解析处使用限制读取。
- `apiclient/types/vmcp.go:44` 的 vMCP component 是 catalog entry 快照，包含 `SourceDigest`；`pkg/storage/apis/obot.obot.ai/v1/vmcp.go:55` 已把 component 映射到规范化 `sourceID::entryKey` 引用。
- `pkg/mcp/source.go:7` 的 sourceID 是去掉 scheme 和尾斜杠后的 source URL；`pkg/controller/handlers/mcpcatalog/vmcp.go:185` 会把 component 解析为显式 source reference。
- `pkg/vmcp/drift.go:14` 的 source digest 服务于快照漂移检测，包含 manifest 语义；它不能直接当作预算身份，否则同一来源更新快照后额度会被拆分。
- `pkg/mcp/manager.go:490` 已把用户 ID、审计元数据和逐请求 Header 值从部署身份排除，说明现有架构已有“资源身份与调用身份分离”的方向。

### 2.3 当前缺口

`FACT(local)`：

- 没有找到 OpenAPI 专用的跨副本接口预算、队列或租约表；现有 `ErrInsufficientCapacity` 来自 Kubernetes 部署容量，不约束业务接口在途请求。
- `pkg/api/handlers/mcpgateway/proxy_audit.go:68` 的 `auditResponseBody` 使用 `bytes.Buffer` 完整累积响应；即使入口有 10 MiB hook 限制，这不是按 OpenAPI 契约的 8 MiB 响应边界，也不能复用于新链路。
- `pkg/api/handlers/mcpgateway/proxy_hooks.go:24` 现有 hook 请求上限为 10 MiB，与 00 草案的 1 MiB 原始请求体不一致。
- `pkg/api/handlers/mcpgateway/proxy_audit.go:688` 的 Header 脱敏名单是固定集合，不包含目录配置的 `authtoken` 等自定义认证 Header。
- `pkg/gateway/client/auditlogpersister.go:32` 的 MCP 审计队列有条数容量但没有 OpenAPI 字节预算；`pkg/gateway/client/egressauditpersister.go:12` 的 egress 队列有独立 64 MiB 上限，不能证明 OpenAPI 单条正文有界。

`UNVERIFIED`：

- P03 与 P05 的 PLAN 在本次核查时尚未落盘；02 的 `BudgetContext`、03 的响应返回路径和 05 的真实拓扑仍需交叉审核后冻结。
- 当前机器可用的 `OBOT_TEST_POSTGRES_DSN`、Docker、K8s 和集群时钟误差未在规划期验证。
- 真实业务 token 交换、真实企业 API 和生产集群不属于本任务验证范围。

## 3. 范围与非目标

### 范围

- 新增独立 PostgreSQL 预算状态包、GORM 类型、迁移和索引。
- 定义统一 Go 预算接口：身份解析、入队、等待、租约获取、幂等释放、取消、过期回收和状态查询。
- 定义平台/池/接口级限额、队列限额、默认值、可配置上限和配置校验。
- 提供有界请求/响应/Header/审计捕获组件和自定义 Header 脱敏组件。
- 为 02 提供可随 `Executor` 传递的 `BudgetContext`；为 03 提供响应与审计有界处理接口。
- 提供真实 PostgreSQL 并发、公平性、崩溃回收和索引/迁移测试。

### 非目标

- 不实施 Python Worker、Obot 固定 Worker backend、外部 `/api-proxy` 路由、部署清单或 05 集成验收。
- 不修改现有 OpenAPI schema 导入、vMCP 快照、凭证存储和业务权限边界。
- 不新增 Redis、消息队列、独立调度服务、Docker socket 依赖或 K8s 管理员权限依赖。
- 不用每个 Worker 的内存信号量或进程池数量冒充跨副本接口额度。
- 不把请求排队改成默认 `jobId` 轮询，不引入 SSE。
- 不承诺业务系统 exactly-once：平台只保证不自动重试、过期不发起新上游调用和保守回收。
- 不在规划阶段修改 `pkg/services/config.go`、`pkg/openapiworker`、router 或生成文件；公共接线必须由 00 冻结写集后串行完成。

## 4. 契约与快照身份

以下为 `CANDIDATE`，需 G00 冻结。

### 4.1 预算键

```text
sourceRef    = normalizedSourceID + "::" + entryKey
operationKey = UPPERCASE(HTTP method) + " " + schema path template
budgetKey    = SHA-256(sourceRef + "\n" + operationKey)
```

约束：

- `sourceRef` 必须来自发布组件的 `ComponentCatalogReferences` 或由当前 catalog entry 解析出的同等引用；缺失、空 entryKey 或一个 component 对应多个来源时返回 `budget_identity_unavailable`，不得回退到 vMCP ID、component ID 或 schema digest。
- `operationKey` 使用 00/01/02 冻结的 path template；operationId、旧工具名、显示别名、前缀、用户、实例、Obot 副本、Worker 副本和 `executionDigest` 均不参与预算键。
- 同一 `sourceRef + operationKey` 的 HTTP 代理调用、MCP `tools/call`、多个 vMCP 发布和多个 Worker 副本共享同一行接口预算。
- 快照版本更新不拆分额度；快照删除或 operation 不存在时由 02/03 在预算获取前拒绝，不创建悬空预算。
- budget key 只存储 SHA-256，不存储可能含内部路径或凭证的原始 source URL；日志可输出非敏感 `sourceRef` 分类标签。

### 4.2 调用顺序

统一入口顺序固定为：

1. 平台认证、vMCP/component/operation 资格与业务凭证解析。
2. 从发布快照解析 `sourceRef`、`operationKey`、请求体大小和绝对截止时间。
3. PostgreSQL 预算入队/租约获取。
4. 选择 Worker 并把 `BudgetContext`、绝对截止时间和逐请求认证 Header 一起发送。
5. Worker 执行上游请求，Obot 传递取消并接收有界内部结果。
6. 调用结果在 Obot 侧完成有界转换/审计后，幂等释放租约。

`describe`/`tools/list` 不消耗接口执行预算，但仍有独立传输、JSON 和 schema 缓存预算；`execute` 冷加载计入该调用总截止时间，冷编译等待会占用队列/租约，不额外发起第二次上游执行。业务上游 4xx/5xx 是成功取得的执行结果，应正常释放租约。

### 4.3 `BudgetContext`

建议 02 的公共类型保留以下不透明字段，由 04 提供：

```go
type BudgetContext struct {
    LeaseID       string
    LeaseDeadline time.Time
    BudgetKeyHash string
    Pool          string
}
```

- 02/03 不解释或修改该结构，只随调用传递；Worker 只记录 LeaseID、key hash 和 deadline，不接触释放 token。
- Obot 本地保存随机 release token，数据库只存哈希；重复 `Release`/`Cancel` 返回幂等结果。
- `BudgetContext` 不包含用户 ID、业务 token、Header 值或内部地址。
- Worker 收到缺失 LeaseID/deadline 的预算必需调用时拒绝；过期或距过期小于 fencing 余量的调用返回 `lease_expired`，不连接业务上游。

## 5. PostgreSQL 状态模型

`CANDIDATE`，按现有 gateway GORM 迁移实现，不新增外部状态服务。

### 5.1 表

#### `openapi_budget_limits`

平台和池级限额，启动/迁移时确保默认行：

| scope/key | max_in_flight | max_waiting |
| --- | ---: | ---: |
| `platform` | 32 | 64 |
| `pool:default` | 16 | 0 |
| `pool:dedicated` | 16 | 0 |

池级 `max_waiting=0` 表示不单独设置池队列，只受平台总队列和接口队列约束。行级 CHECK 要求数值大于零；平台 `max_in_flight` 不小于两个池之和，平台 `max_waiting` 不小于单接口队列上限。

#### `openapi_operation_budgets`

专属接口配置，唯一键 `(source_ref, operation_key)`：

- `pool`: 固定 `dedicated`；
- `max_in_flight`: 1—12；
- `max_waiting`: 1—16；
- `updated_at`、乐观版本号。

没有行的 operation 使用 default pool、并发 4、队列 8。配置上限必须小于等于 dedicated pool 并发，且专属池内单接口默认硬上限为 12，避免一个 operation 合法配置为 16 后独占整个专属池。

#### `openapi_budget_states`

计数和公平状态，唯一键 `(scope, key)`：

- `scope in (platform, pool, operation)`；
- `active_count`、`waiting_count`；
- `last_admitted_at`；
- `updated_at`。

计数由状态转换维护，并由回收器从 request 行重算校正；不得只信任计数。

#### `openapi_execution_requests`

同一行承担队列票据和租约：

- 身份：`request_id` 唯一、`source_ref`、`operation_key`、`budget_key`、`pool`；
- 状态：`waiting`、`active`、`completed`、`cancelled`、`expired`、`failed`；
- 时间：`enqueued_at`、`wait_deadline`、`deadline`、`admitted_at`、`finished_at`；
- 隔离：`release_token_hash`、`lease_generation`、`obot_instance_id`、可选 `worker_instance_id`；
- 结果：`outcome_code`、`response_bytes`、`retryable`、`last_error_code`。

不存储用户 ID、业务 token、Header 值、完整 URL、请求体或响应体。`id bigserial` 即公平序列。

### 5.2 索引

迁移显式创建并测试：

- `openapi_execution_requests(request_id) UNIQUE`
- `openapi_execution_requests(state, deadline)`：过期回收；
- `openapi_execution_requests(state, id)`：全局队列顺序；
- `openapi_execution_requests(budget_key, state, id)`：接口队列与计数校正；
- `openapi_execution_requests(pool, state, id)`：池观测；
- `openapi_execution_requests(finished_at)`：终态清理；
- `openapi_budget_states(scope, key) UNIQUE`
- `openapi_operation_budgets(source_ref, operation_key) UNIQUE`

所有查询必须限制状态和时间；管理查询必须分页，禁止全表扫描式状态接口。

### 5.3 原子状态转换

固定锁序为 `platform -> pool -> operation`，所有转换使用短事务：

1. **Enqueue**：锁三行状态，校验平台队列、接口队列和 deadline，插入 `waiting` 票据并递增等待数。operation 状态行不存在时用 `ON CONFLICT` 惰性创建并重试一次；创建失败则失败关闭。
2. **Admit**：同一锁序下读取限额和计数，只选择“operation、pool、platform 均有空位”的 waiting 票据，原子改为 `active`、写入 lease generation/deadline、递增 active、递减 waiting。
3. **Release/Cancel**：按 request ID 和 release token 哈希条件更新；只有 `active` 能转终态。成功后递减 active。重复释放返回当前终态，不重复扣减。
4. **Expire/Reclaim**：只回收 `deadline + reclaim_grace` 已过的 waiting/active 行；每批最多 500 行，用状态条件更新和 `FOR UPDATE SKIP LOCKED` 避免多副本重复处理，然后重算受影响计数。
5. **Cleanup**：终态行保留 15 分钟后批量删除；删除失败只影响观测，不影响限额正确性。

事务超时默认 250ms，禁止在事务中做 DNS、Worker HTTP、上游 HTTP、审计持久化或日志外呼。PostgreSQL 不可用时 `Enqueue/Admit/Release` 返回明确存储错误；新调用返回 503。

### 5.4 等待与公平放行

每个 Obot 副本运行一个本地 admission pump，不按请求各开一个数据库循环：

- 请求入队后注册本地 channel 和 `wait_deadline`。
- pump 每 20ms（加最多 10ms 抖动）执行一次短事务，从所有 runnable operation 中选择一个 waiting 票据放行，并通知本副本已放行请求；其他副本放行的票据最晚在下个 pump 周期被发现。
- runnable 指接口、池和平台均有剩余 active 名额且票据未超过 wait deadline。
- 公平排序：候选 operation 按 `last_admitted_at` 最旧优先；相同时间按最老票据 `id` 优先。新 operation 的初始 `last_admitted_at` 为其状态创建时间。
- 慢接口已满时会被跳过，不会作为队首阻塞其他接口；它自己最多保留单接口队列 8 张票据。
- 默认每 Obot 副本同时最多一个 admission pump 事务，两副本默认约 100 TPS 以下短事务；该值是上限不是容量承诺，压测后可调。

这是有界轮询的候选实现，不依赖 PostgreSQL LISTEN/NOTIFY 或 Redis。轮询延迟计入 2 秒等待预算；等待耗尽返回 429，调用总 deadline 耗尽返回 504。

## 6. 默认值与硬上限

### 6.1 初始默认值

| 项目 | 默认 | 约束 |
| --- | ---: | --- |
| 平台总在途 | 32 | 1—128 |
| 默认池总在途 | 16 | 1—平台总在途 |
| 专属池总在途 | 16 | 1—平台总在途 |
| 未配置专属限制的接口并发 | 4 | 1—默认池并发 |
| 专属接口并发 | 显式配置，默认 4 | 1—12，且 ≤ 专属池 |
| 平台总队列票据 | 64 | 1—1024 |
| 单接口队列票据 | 8 | 1—16，专属可配置 |
| 最长队列等待 | 2s | 1s—30s，且 < 总截止时间 |
| 单调用总截止时间 | 30s | 1s—120s |
| Worker 执行 fencing 余量 | 1s | 250ms—5s |
| 过期回收宽限 | 1s | 250ms—5s |
| 终态保留 | 15m | 1m—24h |
| 原始请求体 | 1 MiB | 1 KiB—8 MiB |
| 原始业务响应 | 8 MiB | 1 KiB—32 MiB |
| 响应解压后字节 | 8 MiB | 与配置响应上限一致，最大 32 MiB |
| 端到端 Header 序列化 | 64 KiB | 4—128 KiB |
| 审计正文 | 32 KiB | 0—64 KiB，默认仅元数据 |
| schema 单份 | 1 MiB | 沿用导入器，不放宽 |

平台总量不表示吞吐容量；它只限制同时处于 `active` 的调用。上线后必须按实际 PostgreSQL、Worker CPU/内存和业务延迟调整。

### 6.2 编码上限

- 内部 base64 envelope 按“原始字节上限 + Header/元数据余量”推导：`4 * ceil(rawLimit/3) + 64 KiB`；不得把 JSON envelope 上限也写成 8 MiB。
- 1 MiB 请求体对应约 1.38 MiB base64；8 MiB 响应体对应约 10.67 MiB base64，再加 64 KiB envelope 余量。
- Obot 读取 Worker JSON 前先按推导上限限制；base64 解码分配前再次校验原始上限。
- MCP JSON/text 转换不得超过原始响应上限；不能先无界解码再判断。
- 审计正文启用时最多采样 32 KiB，输出显式 `truncated=true` 与采样字节数；截断数据不能表现为完整 JSON 成功。

## 7. 故障、取消与过期许可

### 7.1 取消

- 客户端断开或调用方 context 取消时，Obot 立即取消到 Worker 的 HTTP 请求；Worker 取消上游连接。
- Worker 返回可观察的 `cancelled` 后，Obot 幂等释放租约。
- 如果断开后无法确认 Worker 已停止，租约保持 active 到 deadline + reclaim grace，不提前释放。
- Obot 优雅关闭时停止新预算获取，等待本地请求到较短关闭窗口；未确认完成的租约不释放，交由回收器。

### 7.2 崩溃与网络分区

- Obot 在获取租约后崩溃：Worker 侧 HTTP context 关闭会触发取消，但新 Obot 不宣称知道执行结果；数据库租约保守占用至 deadline，之后回收。
- Worker 崩溃或 Obot→Worker 分区：只要无法证明上游未执行，不释放额度；已发送给业务系统的请求可能继续在业务侧执行，这是不确定结果，禁止自动重试。
- Worker→业务系统网络分区：上游 transport 返回错误或 deadline，Worker 关闭连接并返回平台错误；业务系统已接收的副作用仍不能回滚。
- PostgreSQL 暂时不可用：新调用 503；已获取租约且已发送的调用可按自身 context 完成，释放/审计稍后重试，恢复后由状态重算校正计数。

### 7.3 过期许可 fencing

- `leaseDeadline` 来自 admission 事务中的数据库 UTC `clock_timestamp()` 加剩余预算。
- Worker 实际执行截止时间为 `min(HTTP context deadline, leaseDeadline - fencingMargin)`；到期前必须停止读取、返回或记录结果，不能发起重试或新的上游请求。
- Obot 回收器只在 `leaseDeadline + reclaimGrace` 后释放 active 名额，给调度和时钟误差留余量。
- 该设计在主机时钟误差小于配置余量时保证过期 Worker 不发起新业务请求；任意时钟漂移、宿主机冻结和实时系统攻击不属于平台可证明边界。生产必须监控 Obot/Worker/PostgreSQL 时钟偏差，并把偏差告警阈值设为小于 fencing margin。
- 已在过期前发出的业务请求无法从平台侧撤销；租约回收只恢复平台额度，不把业务副作用标记为失败或成功。

### 7.4 错误映射

| 情况 | 结果 |
| --- | --- |
| 预算身份缺失/快照引用不唯一 | 内部配置错误，拒绝且不触达上游 |
| PostgreSQL 不可用/迁移失败 | 503，`budget_store_unavailable` |
| 平台或接口队列满 | 429，`budget_queue_full`，带 Retry-After |
| 等待预算耗尽 | 429，`budget_wait_timeout` |
| 调用总 deadline 耗尽 | 504，`deadline_exceeded` |
| Worker/内部传输失败 | 503/502 按错误阶段分类，结果不确定且不自动重试 |
| 请求/响应/Header/base64 超限 | 413/502 按发生阶段分类，不返回半截业务 body |
| 上游业务 4xx/5xx | 保留业务状态和有界业务响应，正常释放租约 |

`Retry-After` 不超过剩余总 deadline，且默认不小于 100ms；排队拒绝不得泄漏内部地址、凭证或完整请求输入。

## 8. 大小与审计边界

### 8.1 执行链路

P04 提供以下可测试组件，01/02/03 在集成阶段必须复用：

- `LimitRequestBody`：`http.MaxBytesReader`/有限 reader，1 MiB 原始上限，超限不继续读取。
- `LimitWorkerEnvelope`：按推导 base64/JSON 上限读取内部响应，超限关闭连接并释放租约。
- `LimitUpstreamBody`：同时限制压缩读取字节和解压后字节，覆盖无 Content-Length、分块、gzip/deflate 膨胀和谎报长度。
- `LimitHTTPResponse`：03 返回前按原始响应和 Header 上限流式写出，超限返回平台错误，不写出半截成功。
- `CaptureBoundedAudit`：最多读取 32 KiB 正文，保留截断标记；默认只记录元数据。

Worker 与 Obot 都不能先 `io.ReadAll` 再检查。所有错误路径必须关闭底层 body，防止连接池泄漏。

### 8.2 Header 脱敏

审计脱敏集合必须包含：

- 平台默认：`Authorization`、`Cookie`、`Set-Cookie`、`Proxy-Authorization`、`X-Api-Key`、`X-Auth-Token`、`X-Obot-Api-Key`、Worker 服务认证 Header；
- 当前发布快照/用户实例中声明的全部业务认证 Header 名，例如 `authtoken`；
- `MCPConfig.Sensitive=true` 的 Header，不区分大小写；
- 未知名采用保守策略：请求/响应 Header 只记录名称和值长度，不记录值。

审计只保存脱敏后的 Header 名和 `[REDACTED]`，测试用合成 token 扫描产物，不得在日志、PLAN 之外的证据或测试快照中保存原文。现有 `sanitizedMCPHeaders` 固定名单不能直接满足本要求。

### 8.3 审计失败

- 默认审计为元数据模式，审计队列满或持久化失败时丢弃正文并记录有界计数，不造成无界内存。
- 如果部署策略声明“必须审计正文”，队列满时在执行前拒绝新调用，而不是执行后静默丢审计。
- 审计与租约释放解耦：审计失败不能导致额度永久占用；数据库释放成功后审计后台按有界重试/丢弃策略处理。

## 9. 文件归属与写集

### 9.1 P04 独占新文件

```text
pkg/openapi/executionbudget/*.go
pkg/openapi/executionbudget/*_test.go
pkg/gateway/types/openapi_execution_budget.go
pkg/gateway/db/openapi_budget_migration.go
pkg/gateway/db/openapi_budget_migration_test.go
docs/plans/openapi-workers/PLAN.md
```

预算包内部建议拆分：`errors.go`、`config.go`、`manager.go`、`postgres.go`、`admission.go`、`cleanup.go`、`boundedio.go`、`audit.go`。

### 9.2 必需公共写集

`pkg/gateway/db/db.go` 只允许新增一行调用 `AutoMigrateOpenAPIBudgets(tx)`。该文件同时被迁移和 gateway 类型公共改动使用，I04 不得与 I02/I03 并行修改；默认在 I02 settlement 后由 00 单独授权 I04 接入。

不把跨 feature 协议类型放入 `apiclient/types`，不修改 `pkg/storage/openapi/generated/**`、`apiclient/types/zz_generated.deepcopy.go` 或根 `Makefile`。

### 9.3 后续接线归属

| Feature | P04 交付 | 对方责任 |
| --- | --- | --- |
| 01 | 绝对 deadline、BudgetContext 和响应/解压上限测试契约 | Worker 执行前检查 lease/deadline，过期不触上游，禁止自动重试 |
| 02 | `Manager.Acquire/Release` 与不透明 `BudgetContext` | 在 Executor 调用前后接线，传递取消，不改预算算法 |
| 03 | 有界响应和审计组件 | HTTP handler 在写出与审计前调用，不能复用无界 `bytes.Buffer` |
| 05 | 验收场景与指标口径 | 启动两 Obot/两 Worker/真实 PostgreSQL 并记录证据 |

如需要管理 API 或 UI 配置专属限额，00 在 I04 核心稳定后单独分配 handler/router/UI 写集；P04 先交付存储与 service 层，避免与 03 的 router 冲突。

## 10. 依赖

- 依赖 G00 冻结：`sourceRef` 生成规则、`operationKey`、`BudgetContext`、错误码、大小默认值和租约 deadline 语义。
- 核心状态机可在 I02 之前用接口桩和真实 PostgreSQL 测试独立实现。
- 与 01/02 的真实 Worker 集成依赖其 `execute` 契约可用。
- 与 03 的 HTTP 返回/审计集成依赖其 PLAN 落盘并接受有界组件。
- 两 Obot/两 Worker 故障验收依赖 I05 拓扑，不能用 SQLite 单进程替代。

## 11. 分步实施

1. 定义错误、配置结构和默认值校验；先写非法上限、池约束和编码上限推导测试。
2. 新增 GORM 类型和 PostgreSQL 迁移，加入 gateway 迁移调用，验证多副本 fresh schema 并发迁移。
3. 实现 Enqueue/Admit/Release/Cancel 短事务与 release token 哈希；写并发原子性和幂等测试。
4. 实现 admission pump 与 runnable 公平选择；写慢接口隔离、多 operation 轮转和队列上限测试。
5. 实现过期回收、计数重算、终态清理和索引查询计划测试；模拟 Obot/Worker 崩溃与数据库恢复。
6. 实现有界 reader、base64 envelope、压缩、Header 和审计组件；覆盖分块、膨胀、谎言 Content-Length、截断和自定义 Header 脱敏。
7. 提供不透明 `BudgetContext` 和 02/03 集成说明，冻结公共接线补丁的限定写集。
8. 与 01/02/03 集成后运行两 Obot/两 Worker/真实 PostgreSQL 并发与故障验收；05 汇总证据。

## 12. 验证证据要求

### 12.1 单元与真实 PostgreSQL 测试

所有跨副本限额测试必须设置 `OBOT_TEST_POSTGRES_DSN`，使用独立 schema 并自动清理；未设置时跳过且不得报告通过。SQLite 只能用于纯解析/上限函数的显式单进程测试，不得用于全局额度、公平性或崩溃回收结论。

至少运行：

```bash
OBOT_TEST_POSTGRES_DSN='<isolated-postgres-dsn>' \
  go test -race ./pkg/openapi/executionbudget ./pkg/gateway/db -run 'TestOpenAPIBudget'

go test -race ./pkg/openapi/executionbudget -run 'TestBounded|TestAuditRedaction|TestLimit'

go test -race ./pkg/gateway/db -run TestGatewayAutoMigrateConcurrentlyOnFreshPostgresSchema

git diff --check
```

必须覆盖：

- 两个 Manager 代表两个 Obot 副本，同一预算键并发获取，接口/池/平台计数均不超限。
- 同一 sourceRef/operation 通过两个不同 vMCP/component 别名入队，仍命中同一状态行。
- 未配置专属限制、专属限制、专属池占满和平台总量占满分别返回预期 429/503。
- 慢接口队首不可运行时，快接口仍可放行；两个 runnable operation 轮转且顺序可预测。
- 平台队列 64、接口队列 8 和等待 2s 耗尽的 `Retry-After`。
- Release/Cancel 幂等、错误 token 不生效、requestID 不允许复用。
- Obot 崩溃后租约保守占用到 deadline；回收后无重复 active；Worker 过期调用不触达假上游。
- PostgreSQL 断开时新调用失败关闭，恢复后计数重算。
- 迁移并发、CHECK/UNIQUE/索引存在和关键查询使用索引。

### 12.2 大小与审计测试

- 1 MiB 请求通过，1 MiB+1 拒绝；8 MiB 响应和推导 base64 envelope 通过，超限拒绝。
- 分块、无 Content-Length、Content-Length 谎报、gzip/deflate 膨胀、Header 超限和 JSON 转换分别有界。
- 超限时无伪成功半截 body，底层连接关闭，租约释放。
- 审计 32 KiB 截断标记正确；`Authorization`、`authtoken` 和快照声明的自定义 Header 值均不可见。
- 测试产物扫描不到运行期合成 token 原文。

### 12.3 两副本两 Worker 验收

最终由 I05 执行，P04 提供断言和指标：

- 拓扑：两个 Obot 进程、两个 Worker 进程、一个真实 PostgreSQL、一个本地假业务系统；不使用 SQLite。
- 每个请求标记 Obot/Worker 实例，假上游只统计已进入 handler 的调用。
- 接口限额设为 2 时，四个客户端经两个 Obot/两个 Worker 并发调用，同一 `budgetKey` 假上游峰值不超过 2。
- 杀死一个 Worker 后，未确认执行不释放额度；deadline 后可回收且后续调用不超限。
- 客户端断开传播到 Worker 和假上游连接；数据库短暂停用时返回 503，恢复后无重复名额。
- 8 MiB 响应、超限响应、慢接口和快接口同压，验证总预算、池隔离、公平性和有界内存。

证据记录命令、拓扑、开始/结束时间、配置、实际峰值、状态表查询、进程日志摘要和产物路径；不得把规划或单元测试写成 E2E 通过。

## 13. 失败行为汇总

| 场景 | 行为 |
| --- | --- |
| 预算身份不唯一 | 拒绝调用，不创建本地替代键 |
| 配置非法 | 启动/保存失败，不回退无限 |
| 队列满/等待超时 | 429 + Retry-After，不触达 Worker/上游 |
| 数据库不可用 | 新调用 503，不回退内存信号量 |
| 放行后 Worker 失败 | 租约保守到 deadline，结果不确定，不自动重试 |
| 客户端取消且 Worker 确认 | 幂等释放，上游连接关闭 |
| 客户端取消但无法确认 | 不提前释放，等待 fencing/回收 |
| 调用总 deadline | 504，Worker 停止新上游动作，回收额度 |
| 响应/编码/审计超限 | 明确平台错误或截断审计，不返回半截成功 |
| 下调限额 | 已执行调用不取消，新调用等待回到新限额内 |

## 14. 风险与契约分歧

| 风险/分歧 | 影响 | 处理 |
| --- | --- | --- |
| 00 未定义 sourceRef 权威来源 | 可能按 component 拆分额度或不同来源误合并 | 冻结为 `ComponentCatalogReferences`/catalog entry 的 `sourceID::entryKey`，缺失即失败 |
| 快照更新与预算键长期绑定 | 同一来源不同版本共享额度 | 这是目标行为；需要版本级隔离时必须另立显式预算策略，不能默认改键 |
| 轮询 admission pump 增加数据库 QPS | 高副本数下可能放大负载 | 每副本单 pump、20ms+抖动、短事务和指标告警；必要时 G00 再评审 LISTEN/NOTIFY，不引入 Redis |
| 公平算法不是完美公平 | 新洪峰仍可能让旧 operation 等待到预算耗尽 | 使用最久未放行优先并验收轮转；明确等待上限，不承诺完美 WFQ |
| 计数漂移 | 可能永久少放行或超限 | 每次状态转换锁序更新，回收器从 request 行重算，测试崩溃/断连恢复 |
| 时钟漂移 | 过期 Worker 可能晚停 | 1s fencing/reclaim 余量 + 时钟偏差监控；声明非任意时钟攻击下的硬实时保证 |
| 现有代理审计无界 | 03 复用会把 8 MiB+ 响应缓冲进内存/审计 | 新 OpenAPI 链路必须使用 P04 bounded capture，禁止直接复用 `auditResponseBody` |
| 固定 Header 脱敏名单 | `authtoken` 泄漏 | 按发布快照和 `Sensitive` 动态扩展脱敏集合 |
| 公共迁移文件冲突 | I02/I03/I04 并行覆盖 | 00 将 `db.go` 一行接入排到 I02 settlement 后，I04 独占其余新文件 |
| 管理接口未接线 | 专属限额无法由运营修改 | 核心存储先独立实现；API/UI 写集由 G00 单独分配，不隐藏功能或宣称已完成 |

## 15. 交付物

- 本计划：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/docs/plans/openapi-workers/PLAN.md`。
- 实施阶段交付 `pkg/openapi/executionbudget`、GORM 类型、迁移、索引、清理器和真实 PostgreSQL 测试。
- 交付有界请求/响应/编码/审计组件与自定义 Header 脱敏测试。
- 交付给 01/02/03 的 `BudgetContext`、deadline 和大小边界集成契约。
- 交付给 05 的两 Obot/两 Worker/真实 PostgreSQL 并发与故障验收断言。
- 未交付：业务代码实施、公共接线、管理 API/UI、部署、真实企业 API 验证和集群验收。

## 16. 形式化验证

### 16.1 已验证性质

关键性质：跨副本 active 租约必须未过期、唯一绑定一个请求行并计入共享在途名额；Worker 只能在距租约截止时间仍有 fencing 余量时发起一次上游调用；只有租约内确认完成/取消可以立即释放名额，已发送但结果不确定的过期租约必须保守占用到 reclaim grace 之后。违反该性质会在多个 Obot/Worker 副本间超发业务调用，或让崩溃/过期后的旧许可继续触达业务系统。

### 16.2 模型与结果

- 完整模型：`docs/plans/openapi-workers/formal/ExecutionBudgetLeases.tla`，覆盖两个请求、两个 Obot 副本、一个共享 active 名额、一个等待票据、入队/放行/派发/Obot 崩溃/唯一上游尝试/确认释放/取消/到期/宽限回收。
- 工具：TLA+ Tools v1.8.0（TLC build `2026.11.06.222357`），固定 SHA-256；恢复配置完整探索 233,082 generated / 181,137 distinct states，0 left，`TypeOK`、`ActiveLeaseSafety`、`ConfirmedReleaseInsideLease` 全部通过。
- 失败注入一：删除“过期后等待 grace”的释放约束，只触发 `ConfirmedReleaseInsideLease`；反例在 `deadline=3` 时把 `expiredPending` 改为 `completed` 并释放名额。
- 失败注入二：删除 Worker 发起上游的严格 fencing 检查，只触发 `ActiveLeaseSafety`；反例在 `now=2` 对 `deadline=3` 发起上游，剩余时间不大于 1 秒 margin。
- 图表：`OpenAPIBudgetLeasesChart*.tla` 是完整状态机的标量投影，用 `/statechart` 生成正常流和两个最短反例；完整模型仍是覆盖依据。图表隐藏重复 `Tick`/`Done` 自环及同可见 phase 的交错。
- 证据与复跑：`docs/plans/openapi-workers/formal/EVIDENCE.md`、`docs/plans/openapi-workers/formal/RUN.md`、`rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh`。脚本解析工具版本、状态数、精确失败规则、具体反例值和未检查规则，不只依赖退出码。

### 16.3 边界

该验证只覆盖设计级有界状态机，不证明 PostgreSQL 锁序/事务、release token 哈希、多 operation 公平排序、HTTP 取消传播、网络分区、重试、大小边界、审计、时钟漂移或部署正确性；这些仍按第 12 节真实测试和 05 验收执行。后续修改第 5/7 节或实现 `pkg/openapi/executionbudget` 时，必须按 `.codex/skills/verify-openapi-execution-budgets/SKILL.md` 同步模型并复跑。
>>>>>>> feature/openapi-execution-budgets
