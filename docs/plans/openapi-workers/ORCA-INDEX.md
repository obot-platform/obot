# Orca 项目入口：OpenAPI 共享 Workers

更新日期：2026-11-10 15:39 Asia/Shanghai。唯一用户沟通入口：00 总计划与协调 Codex。

## 当前真实状态

- Run：`run_e4b29a6d58f8`，Orca 重导入后已由新 00 会话重新绑定。
- coordinator mailbox：`orca_session_id:codex_6a58d271_1826_49d6_8402_3211beced2c1`。
- 阶段：`FROZEN-1.0` 已冻结，用户确认无状态 Kubernetes Worker + 现有 PostgreSQL 方向；I01—I05R 实施、集成与隔离验收已完成。
- 共享契约：已冻结。交叉审核：完成，55 个原始发现去重为 30 簇，全部由 C01/C02 覆盖。业务代码实施与隔离验收：完成。
- 规划/审核/实施 worker 同时上限：2。
- 已建立 25 个真实任务：五个规划、五个形式化、五个交叉审核、两个契约修订、一个 00 冻结、五个实施任务、I02R/I02R2 修订和 I05R 最终集成任务。
- P01/P02 PLAN 均已落盘并经 00 独立检查后手动结算；因重导入导致 worker identity changed，两个原 Dispatch 均只做无进程动作 fence，证据见 `evidence/P01-SETTLEMENT.md` 与 `evidence/P02-SETTLEMENT.md`。
- P04 已按 Roadmap 用新 repo selector 在既有 worktree 启动规划 Dispatch `ctx_29e62fe13483`。
- P03 已按 Roadmap 用新 repo selector 在既有 worktree 启动规划 Dispatch `ctx_d6df8fe45947`。
- P03/P04 均已通过 `worker_done` 正常完成，PLAN 是唯一 feature 产物。
- P05 曾按两个 Docker Compose 单元完成规划；实施前用户改为已有 Namespace 内的无状态 Kubernetes Deployment，可横向扩容，并保留现有 PostgreSQL。
- pstack 技能已安装到 `~/.agents/skills/`：`setup-pstack`、`create-formal-verification-skill`、`statechart`；证据见 `evidence/PSTACK-FORMAL-SKILLS.md`。
- 按用户点名的 skillify 路线安装 `~/.agents/skills/skillify/SKILL.md`；只借用其流程固化质量门槛，不采用其 Claude/OMC 专属存储路径。
- 新增 F01—F05 形式化任务：每个 feature 必须实际运行模型检查、生成项目本地验证 skill 和 Mermaid 状态图。全部 P/F 完成前不派发交叉审核或实施。
- 旧 quota automation `2b76e693-617c-43bd-bc4d-1b10bf8a7243` 已禁用，避免 15:56 重复唤醒；未修改全局 Codex 配置。
- 五份 REVIEW 与独立复跑证据齐备；R01/R02/R03/R04/R05 分别报告 7/5/4/5/5 个 P1。R04/R05 的 Task/Dispatch 均已确认 `succeeded/completed`。
- C01/C02 契约修订均已完成并经 00 做结构/覆盖/格式核验；C01 覆盖 14 簇，C02 覆盖 14 簇，D10 保留 A/B 用户决策且未选择。
- 用户已作出最终部署决策：无状态 Worker 部署为已有 Namespace 内的 Kubernetes `Deployment`，可横向扩容；保留现有 PostgreSQL，不切换 MySQL。G00 已按该决策冻结。
- I01 核心实现完成：22 项 Worker 测试、lock、编译、Docker build 和 formal 复跑均通过。
- I02 首轮发现线协议差异后由 I02R 收敛完成；真实 I01 Python Worker describe/execute、TLS 假上游、Authorization 例外和 formal 复跑均通过。
- I02R2 已补齐 I03 所需的 method/pathTemplate/parameters 元数据；空 key、未匹配或不匹配 key 均零次 Worker execute。
- I03 HTTP Proxy 核心完成：raw-prefix、专属授权、唯一匹配、业务响应与边界测试/formal 均通过。
- I05R 已完成：四 feature 合并、真实双用户 Worker 互通、UI/docs、隔离 K8s 2 Obot + 2 Worker 扩缩容/mTLS/健康/raw route 验收全部通过；临时集群已清理。
- I04 预算/租约、真实 PostgreSQL 验证和 formal 复跑已完成。
- G00 决策门已解除；`evidence/REVIEW-ISSUE-MATRIX.md` 仅是修订拆分依据，不是关闭证据。

## 计划与工作区

当前仓库 ID：`8acef906-8a50-4889-8580-9f57625d0a0d`。历史仓库 ID：`c7918928-0734-4283-b8b2-8bc85657f341`。起点：`2be04f025d1c86144e8670d9e9341b263edc28ea`。

完整 selector 为 `id:8acef906-8a50-4889-8580-9f57625d0a0d::<下列worktree绝对路径>`。

```text
00 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-workers-roadmap
01 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-shared-worker
02 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-worker-bridge
03 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-http-proxy
04 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-execution-budgets
05 id:8acef906-8a50-4889-8580-9f57625d0a0d::/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance
```

| Feature | 分支 | Worktree | PLAN 绝对路径 | 阶段 / 证据结论 |
| --- | --- | --- | --- | --- |
| 00 | feature/openapi-workers-roadmap | /Users/echo/orca/workspaces/obot/openapi-workers-roadmap | /Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/PLAN.md | 草案已落盘 / partial |
| 01 | feature/openapi-shared-worker | /Users/echo/orca/workspaces/obot/openapi-shared-worker | /Users/echo/orca/workspaces/obot/openapi-shared-worker/docs/plans/openapi-workers/PLAN.md | PLAN + F01 形式化完成 / formal planning complete，等待集体交叉审核 |
| 02 | feature/openapi-worker-bridge | /Users/echo/orca/workspaces/obot/openapi-worker-bridge | /Users/echo/orca/workspaces/obot/openapi-worker-bridge/docs/plans/openapi-workers/PLAN.md | PLAN + F02 形式化完成 / formal planning complete，等待集体交叉审核 |
| 03 | feature/openapi-http-proxy | /Users/echo/orca/workspaces/obot/openapi-http-proxy | /Users/echo/orca/workspaces/obot/openapi-http-proxy/docs/plans/openapi-workers/PLAN.md | PLAN + F03 形式化完成 / formal planning complete，等待集体交叉审核 |
| 04 | feature/openapi-execution-budgets | /Users/echo/orca/workspaces/obot/openapi-execution-budgets | /Users/echo/orca/workspaces/obot/openapi-execution-budgets/docs/plans/openapi-workers/PLAN.md | PLAN + F04 形式化完成 / formal planning complete，等待集体交叉审核 |
| 05 | feature/openapi-k8s-acceptance | /Users/echo/orca/workspaces/obot/openapi-k8s-acceptance | /Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/docs/plans/openapi-workers/PLAN.md | PLAN + F05 形式化完成 / formal planning complete，等待集体交叉审核 |

## 协调文件

- ROADMAP：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/ROADMAP.md`
- CONTRACTS：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/CONTRACTS.md`
- ACCEPTANCE：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/ACCEPTANCE.md`
- 00 PLAN：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/PLAN.md`
- 规划任务公共说明：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/tasks/PLANNING.md`
- 形式化任务公共说明：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/tasks/FORMAL.md`
- 实施任务公共说明：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/tasks/IMPLEMENTING.md`
- G00 冻结清单：`/Users/echo/orca/workspaces/obot/openapi-workers-roadmap/docs/plans/openapi-workers/tasks/G00.md`
- G00 预审问题矩阵：`evidence/PRELIMINARY-G00-ISSUES.md`（仅为待复核汇总，不是关闭证据）
- R01—R05 去重问题矩阵：`evidence/REVIEW-ISSUE-MATRIX.md`（修订拆分依据，不是关闭证据）
- C01 修订完成证据：`evidence/C01-REVISION-COMPLETION.md`
- C02 修订完成证据：`evidence/C02-REVISION-COMPLETION.md`
- I01 实施完成证据：`evidence/I01-IMPLEMENTATION-COMPLETION.md`
- I04 实施完成证据：`evidence/I04-IMPLEMENTATION-COMPLETION.md`
- I02R 实施完成证据：`evidence/I02R-IMPLEMENTATION-COMPLETION.md`
- I02R2 实施完成证据：`evidence/I02R2-IMPLEMENTATION-COMPLETION.md`
- I03 实施完成证据：`evidence/I03-IMPLEMENTATION-COMPLETION.md`
- I05R 最终实施与验收证据：`evidence/I05R-IMPLEMENTATION-COMPLETION.md`
- v1 验收审阅包（首选视觉入口）：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/docs/plans/openapi-workers/ACCEPTANCE-REVIEW-v1.html`（`v1-acceptance-r3`，按 FastAPI `/docs` → 管理员导入 → 发布开关 → MCP `tools/list` → 授权 `tools/call` → HTTP `/api-proxy` 与 `openapi.json` → 架构扩缩容 的 11 步主线展示 13 张真实截图）
- v2 验收审阅包（当前首选视觉入口）：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/docs/plans/openapi-workers/ACCEPTANCE-REVIEW-v2.html`（`v2-acceptance-r1`，v1 主线全部保留 + 四步 v2 增量：Worker `/metrics` 看板、上游 schema 漂移探测（只探测不生效）、MCP/HTTP 双出口业务级审计、`/usage` 布局修复；证据 `evidence/OPS-ACCEPTANCE.md`、`evidence/AUDIT-E2E-ACCEPTANCE.md`）
- v1 验收审阅包（Markdown）：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/docs/plans/openapi-workers/ACCEPTANCE-REVIEW-v1.md`（`v1-acceptance-r2`，已补真实 vMCP UI/E2E 截图与验收前修复记录）
- 契约修订任务公共说明：`tasks/REVISING.md`
- F01—F05 形式化汇总：`evidence/FORMAL-VERIFICATION-SUMMARY.md`
- 各 feature 审核证据预定位置：各自同目录的 `REVIEW.md`。

## Task 与 Dispatch

计划完成不等于实施完成。F01—F05 各自依赖对应 P 任务；R01—R05 必须等全部 P/F 完成后由 00 派发；G00 依赖全部 R 任务并由 00 审核收敛；所有 I 任务依赖 G00。I03 另外依赖 I01/I02，I05 另外依赖 I01—I04。

| 逻辑任务 | Task ID | Dispatch ID | 状态 | 证据 |
| --- | --- | --- | --- | --- |
| P01 | task_cb50109a42a3 | ctx_69a8fa771177（identity changed 后 abandoned） | completed / 00 验证 PLAN 后手动结算 | evidence/P01-SETTLEMENT.md |
| P02 | task_e3068b23e0ec | ctx_437a1bece6ea（identity changed 后 abandoned） | completed / 00 验证 PLAN 后手动结算 | evidence/P02-SETTLEMENT.md |
| P03 | task_700f2ad88c2a | ctx_d6df8fe45947 | completed / `worker_done` 成功 | evidence/P03-PLAN-COMPLETION.md |
| P04 | task_50f08f4e3654 | ctx_29e62fe13483 | completed / `worker_done` 成功 | evidence/P04-PLAN-COMPLETION.md |
| P05 | task_32c57fe3be30 | ctx_7235be5b6c5f | completed / `worker_done` 成功 | evidence/P05-PLAN-COMPLETION.md |
| R01 | task_85042a9c3a19 | ctx_45ee83e9351e | completed / `worker_done` 成功；7 P1 + 6 P2 + 1 P3 | evidence/R01-REVIEW-COMPLETION.md |
| R02 | task_2471b02ebad1 | ctx_2a2cd0188bd5 | completed / `worker_done` 成功；5 P1 + 2 P2 | evidence/R02-REVIEW-COMPLETION.md |
| R03 | task_6a050790f209 | ctx_0463a68818e0 | completed / `worker_done` 成功；4 P1 + 7 P2 + 1 P3 | evidence/R03-REVIEW-COMPLETION.md |
| R04 | task_f599eead8bb9 | ctx_76c4bb8c5bb3 | completed / `worker_done` 成功；5 P1 + 7 P2 + 1 P3 | evidence/R04-REVIEW-COMPLETION.md |
| R05 | task_65523779744a | ctx_37a26cc4b700 | completed / `worker_done` 成功；5 P1 + 4 P2 | evidence/R05-REVIEW-COMPLETION.md |
| C01 | task_7570f2cc2734 | ctx_31c83783da6d | completed / `worker_done` 成功；14 簇契约修订完成，待 00 复核采纳 | evidence/C01-REVISION-COMPLETION.md |
| C02 | task_cf8639a1969b | ctx_2ef0a7a850fd | completed / `worker_done` 成功；14 簇契约修订完成，D10 待用户决策 | evidence/C02-REVISION-COMPLETION.md |
| G00 | task_42ded5238b08 | 无 | completed / `FROZEN-1.0` 冻结，实施许可开启 | evidence/G00-FREEZE.md |
| I01 | task_cf0661b66711 | ctx_76db58bdc644 | completed / `worker_done` 成功；核心实现与本地验证完成 | evidence/I01-IMPLEMENTATION-COMPLETION.md |
| I02 | task_aa96cfc6cf79 | ctx_5df4ade837f6 | completed / 依据 I02R 真实互通成果由 00 结算 | evidence/I02R-IMPLEMENTATION-COMPLETION.md |
| I02R | task_60fc0764101d | ctx_18b0dba055f3 | completed / `worker_done` 成功；真实 I01 Worker 互通 | evidence/I02R-IMPLEMENTATION-COMPLETION.md |
| I02R2 | task_86ecad99b2af | ctx_2abda5cc0b0d | completed / `worker_done` 成功；I03 元数据接口修订 | evidence/I02R2-IMPLEMENTATION-COMPLETION.md |
| I03 | task_8ea183def2cc | ctx_0f3d2eadd7cd | completed / 00 核验核心实现与 formal 证据后结算 | evidence/I03-IMPLEMENTATION-COMPLETION.md |
| I04 | task_001cc4346f29 | ctx_09fdf8167ef9 | completed / 00 核验真实 PostgreSQL 与 formal 证据后结算 | evidence/I04-IMPLEMENTATION-COMPLETION.md |
| I05 | task_0ccb45f5d2f5 | 无 | failed / 原 Compose 目标取消，待 I05R 替代 | 00 REVIEW |
| I05R | task_99e1d0eb6793 | ctx_a99eceeec30c | completed / 00 核验集成与隔离 K8s 验收后结算 | evidence/I05R-IMPLEMENTATION-COMPLETION.md |
| F01 | task_1d7f70ec05e1 | ctx_e6ebed729afe | completed / `worker_done` 成功 | evidence/F01-FORMAL-COMPLETION.md |
| F02 | task_ebcc62d86179 | ctx_b0d519ec6ab6 | completed / `worker_done` 成功 | evidence/F02-FORMAL-COMPLETION.md |
| F03 | task_434369177d46 | ctx_77786b24bb5c | completed / `worker_done` 成功 | evidence/F03-FORMAL-COMPLETION.md |
| F04 | task_0b1c574dbb33 | ctx_fa30bc24b8ce | completed / `worker_done` 成功 | evidence/F04-FORMAL-COMPLETION.md |
| F05 | task_d3406e8900c8 | ctx_e01343a422ee | completed / `worker_done` 成功 | evidence/F05-FORMAL-COMPLETION.md |

## 已核实与未完成

verified：既有六个分支/worktree/卡片存在且已映射到新 repo ID，原工作区 README 修改仍在，当前安装 Orca 可访问；P/F/R/C01/C02/G00/I01/I02/I02R/I02R2/I03/I04/I05R 完成状态均已核验。

partial：规划、形式化、交叉审核、契约冻结和实施集成均已完成。项目仍保持未提交状态；真实企业授权、生产部署和生产容量未验证。

未执行：真实企业授权、真实 JWT 调用、生产部署、commit/push。隔离 K8s/本地假上游链路已按 I05R 证据验证，不代表生产链路已通过。
