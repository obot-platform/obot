# OpenAPI 共享 Workers Roadmap

日期：2026-11-09。状态：FROZEN-1.0 / implementation；契约已冻结，实施调度开启。

## 用户目标与固定约束

用户登录 Obot 并完成业务系统授权后，平台保存用户自己的业务 token。Agent 用平台 Key 阅读运营维护的工具信息并构建应用，应用之后通过普通 HTTP 请求调用平台代理；业务系统负责执行和数据权限。

保留 MCP/vMCP、目录、组件快照、用户实例和现有加密凭证存储。只改造 OpenAPI runtime 和关联执行链。Obot 与 OpenAPI Workers 均为 Kubernetes 无状态 `Deployment`，Worker 可按 `executionDigest` 路由并横向扩容；跨副本状态进入现有 PostgreSQL。部署到已有 Namespace，不创建 Namespace / ClusterRole / ClusterRoleBinding，不要求集群管理员，不挂载 Docker socket。禁用 SSE，不引入 Eunomia、Endpoint 顶层对象、任意脚本转换或默认 jobId 轮询。不重做已有远程 MCP。

不自行 commit/push、开 PR、部署生产或改业务账号权限。不触碰原工作区及其 README 修改，不使用历史真实 JWT。业务 token 不得出现在计划、源码、快照、普通日志和测试产物中。

## 工作区

仓库 ID：`c7918928-0734-4283-b8b2-8bc85657f341`。

起点：`skin/ztocc-theme` @ `2be04f025d1c86144e8670d9e9341b263edc28ea`。

既有路径前缀：`/Users/echo/orca/workspaces/obot/`。

| 编号 | worktree | 分支 | 职责 |
| --- | --- | --- | --- |
| 00 | openapi-workers-roadmap | feature/openapi-workers-roadmap | 统一入口、契约、依赖、验收、协调 |
| 01 | openapi-shared-worker | feature/openapi-shared-worker | Python/FastMCP 多快照共享执行 |
| 02 | openapi-worker-bridge | feature/openapi-worker-bridge | Obot 接入与无动态托管模式 |
| 03 | openapi-http-proxy | feature/openapi-http-proxy | 应用 HTTP 出口与调用文档 |
| 04 | openapi-execution-budgets | feature/openapi-execution-budgets | 接口调度、跨副本预算与有界处理 |
| 05 | openapi-k8s-acceptance | feature/openapi-k8s-acceptance | Kubernetes 无状态部署、多副本集成和验收 |

完整 selector 为 `id:<仓库ID>::<绝对worktree路径>`。不得重复创建这些分支或 worktree。

## 阶段与依赖

P00：00 自身的计划、ROADMAP、CONTRACTS、ACCEPTANCE、ORCA-INDEX。

P01—P05：由各自 worktree 中的 Orca Codex 独立核查源码并落盘 PLAN。规划时允许指出契约草案的问题，禁止提前修改实现代码。

F01—F05：各 feature 在自己的既有 worktree 补齐设计级形式化验证。每个任务选择一个测试难以覆盖且故障代价高的状态机/并发/重试/解析性质，用合适工具建立可复跑模型，实际运行模型检查，生成项目本地验证 skill，并用 `/statechart` 输出 Mermaid 状态图与最短反例/正常流转说明。

R01—R05：全部 P 与 F 任务结束后交叉审核，写本 feature 的 REVIEW。审核任务的交付是发现和审查证据；有发现不等于审核任务本身失败，但发现未解决时冻结任务不能完成。

G00：00 汇总审查、协调修订、核实重大问题已解决或由用户明确接受，冻结共享契约与写集。仅 00 能完成此任务，不把自我声明的“无问题”当作集体审核。

| 形式化任务 | 依赖 |
| --- | --- |
| F01 共享 Worker 状态机 | P01 |
| F02 Obot 接入状态机 | P02 |
| F03 HTTP 代理状态机 | P03 |
| F04 预算租约状态机 | P04 |
| F05 Compose 生命周期 | P05 |

| 实施任务 | 依赖 |
| --- | --- |
| I01 共享 Worker | G00 |
| I02 Obot 接入 | G00 |
| I04 接口预算模块 | G00 |
| I03 HTTP 代理 | G00、I01、I02 |
| I05 集成与验收 | G00、I01、I02、I03、I04 |

I05 内先检查各实现的集成交付，再应用限定写集补丁并运行全链路。I04 的公共接线不能在 I02/I03 正在写相同文件时并行进行。

## 交叉审核分配

| 审核者 | 必读内容 | 审核重点 |
| --- | --- | --- |
| R01 / 01 Codex | 所有 PLAN/FORMAL、CONTRACTS、ACCEPTANCE，重点 02/04 | Worker 协议、FastMCP、加载隔离、缓存、逐请求认证、预算边界、模型与反例 |
| R02 / 02 Codex | 所有 PLAN/FORMAL，重点 01/03/05 | 启动模式、凭证绑定、vMCP 发现、平台鉴权、无 Docker/K8s 依赖、模型与反例 |
| R03 / 03 Codex | 所有 PLAN/FORMAL，重点 01/02/04 | 原始 HTTP 语义、发布白名单、路径匹配、Key 范围、错误归属、模型与反例 |
| R04 / 04 Codex | 所有 PLAN/FORMAL，重点 01/02/03 | 跨副本计数、公平排队、取消和崩溃回收、大小上限、审计、模型与反例 |
| R05 / 05 Codex | 所有 PLAN/FORMAL、ACCEPTANCE | 部署权限、固定副本、恢复、真实可执行的验收、证据边界、模型与反例 |

REVIEW 对每项问题给出严重程度、契约或源码依据、影响、责任 feature 和可验证的解决条件。P0/P1 以及会造成跨 feature 协议不兼容的问题均阻断 G00。其他问题明确解决或记录限度，不能静默丢弃。

## 调度与协作

- 用户只与 00 沟通，feature worker 通过 orchestration ask/send/worker_done 返回。
- 默认规划、审核、实施各阶段都最多同时两个 worker。
- 规划优先 P01/P02，再按空位启动 P04/P03/P05。P05 完成后按空位并发推进 F01—F05；全部 P 与 F 完成前不派发审核或实施。
- 审核在所有 P/F 产物可读后开始；每个 reviewer 在自己的 worktree 写入，不覆盖他人计划或形式化证据。
- G00 完成后优先同时启动 I01/I02；有空位可推进 I04；I03 等待接口可用。
- Orca Run 仅保存任务与消息，不是自动执行器；00 每次被唤醒后依据依赖、门槛、写集与并发数调度。
- 不写自旋 polling/watch 脚本，不通过原聊天继续指挥，不盲目改全局 Codex 配置。

## 变更与集成归属

01 独占新增 Python Worker 包和其测试/镜像构建定义；02 负责共享 Go 协议类型、Resolver/Executor、桥接和服务组装；03 负责代理 handler/授权规则/应用文档与相应 UI；04 负责预算包、PostgreSQL 状态表/迁移、租约与审计有界组件；05 负责已有 Namespace 内的无状态 Kubernetes `Deployment`/`Service`、TLS 材料挂载、多副本滚动升级和隔离 E2E。

具体路径必须由各 PLAN 基于源码补齐，冲突由 00 在冻结时消除。任何调整共享字段、生成文件或公共接线的请求先发给 00，不在其他 worker 的写集中直接修改。

最终使用限定路径的 `git diff --binary` 和审查后的未跟踪文件清单交接；应用前进行检查。保留本地未提交状态，不把用户原工作区修改带入集成。

## 汇报与完成标准

ORCA-INDEX 是唯一返回入口。任务状态、卡片状态与证据结论分别记录：规划进行中不能写成代码实施中，计划完成不能写成 feature 完成，派发接受不能写成实际运行验证通过。

最终按 ACCEPTANCE 区分源码/静态验证、本地真实运行和 Docker Compose 部署。状态只采用 verified / partial / blocked，并附具体未完成项。
