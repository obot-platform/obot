# 契约修订 Worker 任务

当前是 G00 前置修订阶段，不是实施。用户只与 00 沟通；你通过 orchestration 报告，不派发其他 agent。

先读 `/Users/echo/.codex/RTK.md` 并使用 `rtk`；遵守适用 AGENTS.md。必读：

- 00 的 `ROADMAP.md`、`CONTRACTS.md`、`ACCEPTANCE.md`、`ORCA-INDEX.md`。
- `evidence/REVIEW-ISSUE-MATRIX.md`。
- 五个 feature worktree 的 `docs/plans/openapi-workers/PLAN.md` 与 `REVIEW.md`。
- 与 assigned matrix clusters 相关的 formal/EVIDENCE/skill。

唯一写集：你所在 feature worktree 的 `docs/plans/openapi-workers/CONTRACT-REVISION.md`。不得修改 00 契约、任何 PLAN、业务代码、formal 产物、全局配置或他人 worktree；不得 commit/push、部署、调用真实业务 API或访问历史 JWT。

交付必须包含：

1. 每个 assigned cluster 的唯一建议方案、备选方案取舍和原始 finding ID。
2. 可直接套用到 `CONTRACTS.md` 的完整替换/新增 Markdown 片段，不用“后续由责任人定义”逃避冻结。
3. 受影响 feature 与精确文件/接口/测试矩阵。
4. 每簇的 close condition，能由 00 独立复核。
5. 明确剩余风险与需要用户决策的事项。

如果证据不足以选择方案，输出最小决策卡（A/B、影响、默认推荐、关闭条件），仍不得留下两义协议。
