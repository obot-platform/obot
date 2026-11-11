# Feature Codex 交叉审核任务

当前是集体审核阶段，不是实施。用户只与 00 沟通；你通过 Orca orchestration 向 00 报告，不自行派发其他 agent。

先读 `/Users/echo/.codex/RTK.md` 并使用 `rtk`；遵守适用 AGENTS.md。读取完整交接文件和架构说明（绝对路径见同目录 PLANNING.md），以及 00 的 ROADMAP、CONTRACTS、ACCEPTANCE。

读取 `/Users/echo/orca/workspaces/obot/` 下五个 feature 的 `docs/plans/openapi-workers/PLAN.md`，以及每个 feature 的 `docs/plans/openapi-workers/formal/`、`EVIDENCE.md`（如存在）和 `.codex/skills/verify-openapi-*/SKILL.md`。对照真实源码、当前版本依赖、TLC/statechart 输出和复跑命令核查，重点按 ROADMAP 的 reviewer 分配，不只审查自己的计划。

形式化审核至少确认：性质与业务伤害对应设计、模型抽象没有隐藏关键失败模式、失败注入确实能触发预期反例、恢复配置通过、Mermaid 图能被人读懂、项目本地 skill 可按同一命令复跑、同步提交与覆盖范围有效。不能把模型通过外推为 E2E、部署或业务 exactly-once。

唯一写集：你自己的 worktree 中 `docs/plans/openapi-workers/REVIEW.md` 及必要的非敏感审核证据。不得修改其他 PLAN、00 契约、业务代码、生成文件或全局配置。

每项发现记录：ID、严重程度、影响、具体 PLAN/契约/源码依据、责任 feature、建议、可验证解决条件。区分已确认问题和需要验证的问题；无发现时也写清检查范围、证据和剩余局限。

- P0：凭证跨用户泄漏、未授权业务执行等严重隔离破坏。
- P1：关键需求无法达成、跨 feature 协议不兼容、跨副本限额失效或无界资源风险。
- P2：可恢复缺陷、覆盖不足或非核心行为不明确。
- P3：文档和维护性问题。

P0/P1 和协议不兼容问题阻断实施冻结。报告发现是审核任务的正常交付，不要为了让 Task 显示成功而删除发现或宣称契约已冻结。审核完成通过 worker_done 交付结果；由 00 安排责任人修订和复核。

不自行 commit/push、创建分支/worktree/PR、访问历史 JWT、调用真实业务接口、部署或停止用户服务。
