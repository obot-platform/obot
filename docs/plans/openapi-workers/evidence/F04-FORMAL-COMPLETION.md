# F04 预算租约形式化验证完成证据

时间：2026-11-09 18:42 Asia/Shanghai。结论：formal planning complete；这是设计级模型检查，不表示 PostgreSQL 实现、跨副本集成、E2E 或部署已完成。

## 交付核验

- Task：`task_0b1c574dbb33`；Dispatch：`ctx_fa30bc24b8ce`；`worker_done` 成功。
- 关键性质：跨副本共享租约必须未过期、唯一归属一个请求并计入全局限额；确认完成/取消可在租约内释放，不确定过期保守占用至回收 grace；过期许可不得启动上游调用。
- 项目本地 skill：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/.codex/skills/verify-openapi-execution-budgets/SKILL.md`。
- 复跑脚本：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/.codex/skills/verify-openapi-execution-budgets/scripts/verify.sh`；00 核验 Bash 语法、图表非空断言和临时清理通过。
- 完整模型、两个破坏配置、三个标量图表模型、`EVIDENCE.md`、三组 Mermaid/report/TLC 输出均已交付。

## 验证结果

- 完整恢复模型：TLC 233,082 generated / 181,137 distinct，0 left on queue；`TypeOK`、`ActiveLeaseSafety`、`ConfirmedReleaseInsideLease` 全部通过。
- Early-release 变体：只触发 `ConfirmedReleaseInsideLease`，在 deadline=3 时从 `expiredPending` 释放，早于 grace。
- Stale-start 变体：只触发 `ActiveLeaseSafety`，在 now=2、deadline=3 且 fencing margin 不足时启动上游。
- 图表模型：正常 115 states 全规则通过；early 139 states 只触发 early-release；stale 136 states 只触发 stale-start；三个 `chart.mmd` 均非空。
- 00 于 2026-11-09 18:41 Asia/Shanghai 独立执行 `rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh`，退出码 0；完整模型与三个图表全部符合预期，`TTrace/states` 计数为 0。

## 边界

模型覆盖两个请求、两个 Obot 副本、一个全局 active slot、一个等待 slot、队列准入、派发、一次上游、确认释放/取消、过期与 grace 回收。不覆盖 PostgreSQL 锁/事务、release-token 哈希、跨操作公平性、HTTP 取消、重试、请求/响应上限或真实部署。
