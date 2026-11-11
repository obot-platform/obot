# G00 共享契约冻结记录

时间：2026-11-09 21:41 Asia/Shanghai。结论：`FROZEN-1.0` 冻结完成，实施许可开启。

## 决策

- C01/C02 均已 `worker_done` 成功，00 已核验交付覆盖与格式。
- R01—R05 的 55 个原始发现已去重为 30 簇，并由 C01/C02 全覆盖。
- 用户最终决策覆盖 D10：Worker 为无状态 Kubernetes `Deployment` 副本，部署到已有 Namespace，可横向扩容；不创建 Namespace / ClusterRole / ClusterRoleBinding，不要求集群管理员。
- Obot 原始实现已集成 PostgreSQL，因此保留现有 PostgreSQL；Worker 不新建 MySQL/Redis/本地数据库，跨副本状态进入 PostgreSQL。
- 生产 Obot→Worker 使用 TLS + mTLS，bearer 仅可选第二因子；详细协议以 C01/C02 冻结附录为准。

## 产物

- 主契约：`docs/plans/openapi-workers/CONTRACTS.md`
- 问题矩阵：`docs/plans/openapi-workers/evidence/REVIEW-ISSUE-MATRIX.md`
- C01 冻结附录：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge/docs/plans/openapi-workers/CONTRACT-REVISION.md`
- C02 冻结附录：`/Users/echo/orca/workspaces/obot/openapi-execution-budgets/docs/plans/openapi-workers/CONTRACT-REVISION.md`

## 实施许可

- 立即并行启动 I01/I02，最多两个活跃实施 Worker。
- I03 等待 I01/I02 完成；I04 可在有空位后启动；I05 等待 I01—I04。
- 所有实现必须保留既有功能、使用合成凭证和假业务系统、按写集交付，并运行受影响验证；不得自行 commit/push、部署生产或访问真实 JWT。
