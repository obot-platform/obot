# R04 集体交叉审核完成证据

时间：2026-11-09 20:09 Asia/Shanghai。结论：review complete；发现未解决前 G00 不得冻结。

## 交付核验

- Task：`task_f599eead8bb9`；Dispatch：`ctx_76c4bb8c5bb3`；Orca 显示 worker/dispatch 均 `succeeded/completed`，`worker_done` 成功。
- REVIEW：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/docs/plans/openapi-workers/REVIEW.md`。
- 独立复跑证据：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/docs/plans/openapi-workers/review-evidence/R04-FORMAL-RERUN.md`。
- 00 核验 REVIEW 与复跑证据存在、REVIEW 非空且无行尾空白；结论为 `Verdict：发现 P1，G00 不能冻结实施。`

## 阻断发现

| ID | 问题 | 责任 |
| --- | --- | --- |
| R04-01 | Worker 租约 fence 在 opaque metadata 与必检字段之间冲突 | 00/01/02/04 |
| R04-02 | 取消、deadline 与租约释放缺少跨层 typed outcome 协议 | 00/01/02/03/04 |
| R04-03 | 入队前凭证资格与请求体大小检查顺序不一致 | 00/02/03/04 |
| R04-04 | Go 侧 executionDigest 缺少 operation manifest 来源 | 00/01/02 |
| R04-05 | 生产 Worker 服务认证未收敛 | 00/01/02/05 |

另有 R04-06—R04-12 七个 P2，以及 R04-13 一个 P3 复跑命令问题。R04 对每项给出影响、依据、责任方、建议和可验证解决条件。

## 独立复跑

R04 在本 worktree 隔离副本中复跑 F01—F05：恢复模型全部通过，破坏模型均触发预期反例。复跑仅证明设计状态机，不证明业务实现、部署或 exactly-once。
