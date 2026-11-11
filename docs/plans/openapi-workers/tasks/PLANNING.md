# Feature Codex 规划任务公共说明

本次任务仅限规划。用户要求每个 feature 由自己 worktree 中的 Codex 编写 PLAN，大家交叉审核，00 冻结后再实施。禁止提前修改业务代码。

## 必读顺序

1. 非浏览器 shell 前读取 `/Users/echo/.codex/RTK.md`，后续 shell 使用 `rtk`。
2. 完整阅读 `/Users/echo/mcp-platforms/obot/.codex-runs/ORCA-HANDOFF-openapi-workers-2026-11-09.md`。
3. 阅读其架构说明 `/Users/echo/mcp-platforms/obot/.codex-runs/ARCHITECTURE-current-platform-2026-11-09.md`。
4. 阅读你 worktree 的适用 AGENTS.md；UI 如纳入计划也读取 UI 子目录约定。
5. 阅读 00 目录 `/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/` 下 ROADMAP、CONTRACTS、ACCEPTANCE。CONTRACTS 是评审草案，不是实施许可。
6. 使用 `rtk proxy orca skills get orchestration` 读取当前版本 worker 协议；依据派发 preamble 使用 task/dispatch 和 worker_done，不要创建或接管 coordinator Run。

## 写集和产物

你只允许在自己的 worktree 中新建/更新 `docs/plans/openapi-workers/PLAN.md`，必要的非敏感规划证据也放同目录。可读其他 feature 和原仓库明确指定的交接/架构文件，不写其他 worktree。

PLAN 必须包含：问题与目标、已核查现有基础、范围与非目标、接口与快照契约、具体文件归属、依赖、分步实施、验证证据要求、失败行为、风险和交付物。说明哪些是源码事实、哪些是候选设计、哪些尚未验证。不要把计划或静态检查写成 E2E 已通过。

核查你负责链路的真实源码，指出草案中不可实现、遗漏或与既有行为冲突的条目。列明跨 feature 所需接口以及公共写集冲突，提议具体解决方式，不用泛泛“待定”代替分析。

## 固定限制

保留已有 MCP/vMCP、目录、组件快照、用户实例和加密凭证存储，用户使用自己的业务 token，业务系统执行权限。保留普通 HTTP 应用出口，禁用 SSE，固定共享 Worker 副本，不依赖 Docker socket 或 K8s 集群管理员。不新增远程 MCP 类型、业务权限引擎、Endpoint 顶层管理对象、Eunomia、任意脚本变换或默认异步轮询。

不访问历史 JWT 或其他敏感备份，不复制真实凭证。不改原工作区及其 README，不操作用户服务，不自行 commit/push、不建分支/worktree/PR、不部署生产。

用户只和 00 沟通。有问题通过 orchestration ask 发给 coordinator，不直接向用户索取已有资料，不为消息上下文问题改全局 Codex 配置。

## 完成报告

PLAN 落盘后做与计划有关的必要检查，报告绝对文件路径、核查源码、契约分歧、写集、依赖、验收方法和未验证项。通过 worker_done 报告规划完成并停止；不要继续实施或自行派发其他 agents。

验证命令非零且导致规划依据无法确认时应报告 partial/blocked；纯粹记录预期失败作为证据时写清原因，不能忽略退出码。
