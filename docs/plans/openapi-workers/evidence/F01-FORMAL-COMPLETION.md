# F01 共享 Worker 形式化验证完成证据

时间：2026-11-09 16:24 Asia/Shanghai。结论：formal planning complete；这是设计级模型检查，不表示实现、E2E、部署或业务 exactly-once 已完成。

## 交付核验

- Task：`task_1d7f70ec05e1`；Dispatch：`ctx_e6ebed729afe`；`worker_done` 成功。
- 关键性质：冷缓存请求只有在恰好补送一份有效快照且 requestID 获得一次性执行 fence 后，才能发起业务上游调用。
- 项目本地 skill：`/Users/echo/orca/workspaces/obot/openapi-shared-worker/.codex/skills/verify-openapi-shared-worker/SKILL.md`。
- 复跑脚本：`/Users/echo/orca/workspaces/obot/openapi-shared-worker/.codex/skills/verify-openapi-shared-worker/scripts/verify.sh`；00 核验 Bash 语法通过。
- 模型/配置/视图/输出：`docs/plans/openapi-workers/formal/SnapshotFencing.tla`、`SnapshotFencing.cfg`、`SnapshotFencingBroken.cfg`、`snapshot-fencing.view.mjs`、`EVIDENCE.md` 与 `evidence/{base,broken}/`。

## 验证结果

- 恢复/受保护配置：TLC 65 generated / 33 distinct，0 left on queue，全部不变量通过且无死锁。
- 破坏配置：TLC/statechart 完整探索 57 distinct / 120 generated steps；精确触发 `ExecutionEnteredAtMostOnce`、`UpstreamRequiresOneSnapshotAndOneFence`，并在重复执行后不确定传输失败时触发 `NoRetryAfterUncertainOutcome`。
- 复跑脚本固定 TLA+ Tools 1.8.0 与 SHA-256，运行 Broken→恢复顺序，并断言必需反例、允许的第三个关联反例、禁止其他反例、Broken 不得有未检查规则、恢复配置所有规则均持有。
- `statechart` 生成基线正常流转与 Broken 最短踩坑路径两份 Mermaid 图。
- 00 于 2026-11-09 16:40 Asia/Shanghai 在 `/Users/echo/orca/workspaces/obot/openapi-shared-worker` 独立执行 `bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh`，退出码 0；Broken 报告 57 states/120 steps 且包含三个预期反例，恢复配置 33 states/56 steps 且全部规则持有。

## 边界

未建模热缓存、LRU、并发同 digest 编译去重、helper 取消、进程崩溃/重启/关闭、HTTP 序列化/DNS/body 细节和业务上游 exactly-once。上述边界已写入 PLAN/EVIDENCE，不能外推为全系统证明。
