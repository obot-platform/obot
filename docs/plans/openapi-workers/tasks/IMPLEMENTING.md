# Feature Codex 实施任务门槛与公共约束

此说明只在 00 完成 G00 并真实派发实施 Task 时生效。派发前不得依据待办任务内容提前实施。

## 进入检查

1. 读取 `/Users/echo/.codex/RTK.md` 并使用 `rtk`，遵守适用 AGENTS.md。
2. 读取完整交接文件、架构说明和 00 的 ROADMAP、CONTRACTS、ACCEPTANCE、ORCA-INDEX。
3. 验证 CONTRACTS 中存在 00 记录的冻结版本/摘要及实施许可，G00 已完成；不存在则通过 orchestration ask 升级，停止实施。
4. 读取自己的 PLAN 和 REVIEW，以及 00 汇总审核中自己的问题关闭证据。只按已批准写集实施。
5. 读取并执行自己的项目本地形式化验证 skill（例如 `.codex/skills/verify-openapi-*/SKILL.md`），记录进入实施前的模型检查结果；模型、设计或覆盖说明缺失时先通过 orchestration ask 升级，不得绕过。
6. 在既有 feature worktree 工作，不创建分支/worktree，不操作原工作区，不自行 commit/push、开 PR 或部署生产。

## 实施要求

保留 MCP/vMCP 基础、用户自己的业务 token、业务系统执行权限、普通 HTTP 出口、无 SSE、固定共享 Workers、无 Docker socket/集群管理员运行依赖。不重做远程 MCP，不新增 Endpoint 顶层对象、Eunomia、任意脚本转换或默认异步轮询。

业务凭证仅在当前请求运行时使用，不进入源码、快照、普通日志、计划或产物。不访问历史 JWT，验证采用合成凭证和假业务系统。

公共接口或写集需要变化时先通过 orchestration ask 请求 00 协调，不能直接在其他 worker 的写集中修改。依赖内容尚未以允许路径交接时先报告具体缺失，不用未经审核的临时替代实现绕过。

优先运行受影响的测试与真实流程，已有回归覆盖不得弱化。UI 修改遵守 UI 子目录 AGENTS 和多语言要求。使用独立端口及临时数据，只清理自己创建的资源。

若实现改变该 skill 覆盖的状态机、并发、重试、解析或生命周期行为，先更新模型和失败注入使它描述新设计，再运行同一验证命令。不得删除或削弱不变量来让检查通过；模型同步提交只有在代码与模型重新核对并复跑通过后才可推进。形式化通过不替代受影响单测、集成测试、E2E 或部署验证。

## 交付

在自己目录的 `IMPLEMENTATION.md` / `evidence/` 记录变更路径、基线、受影响流程、命令、实际结果、风险、未跟踪文件清单、交接写集、形式化 skill 路径、模型同步状态和复跑结果。结果按 verified / partial / blocked 区分；编译/单测/模型检查不等于 E2E 或部署通过。

使用 worker_done 交付，不自行合并其他 feature、不直接向用户请求继续指令。00 是唯一协调入口，Orca 消息负责唤醒，不运行自旋监控脚本。
