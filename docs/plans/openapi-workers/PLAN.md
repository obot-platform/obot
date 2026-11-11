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
