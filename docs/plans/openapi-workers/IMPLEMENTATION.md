# I04 接口预算与租约实施

日期：2026-11-10 09:10 Asia/Shanghai。状态：partial——I04 预算/租约模块、迁移、有界组件、形式化模型和隔离真实 PostgreSQL 验证完成；业务入口接线和两 Obot/两 Worker E2E 按依赖留给 I02R/I03/I05。

## 基线与写集

- 分支 / HEAD：`feature/openapi-execution-budgets` / `2be04f025d1c86144e8670d9e9341b263edc28ea`。
- I04 写集：`pkg/openapi/executionbudget/**`、`pkg/gateway/types/openapi_execution_budget.go`、`pkg/gateway/db/openapi_budget_migration.go`、`pkg/gateway/db/openapi_budget_migration_test.go`、`pkg/gateway/db/db.go` 的三行接入、本目录实施/证据/formal/skill 产物。
- 未修改共享契约、业务入口、其他 feature PLAN、原工作区、全局配置或集群；未 commit/push。
- 除 `pkg/gateway/db/db.go` 为修改外，其余交付文件均为未跟踪文件；`docs/plans/openapi-workers/review-evidence/**` 是本任务开始前已存在的规划审核证据，本次未修改。

## 变更范围

- 配置校验与默认值：平台/池/接口、队列、deadline、fencing/reclaim、终态保留和请求/响应/Header/审计字节边界。
- Canonical budget identity：结构化 source URL/entryKey 规范化、RFC 百分号编码、dot-segment 处理和 SHA-256 source/operation/budget hash；缺失或多义引用 fail closed。
- PostgreSQL 状态：预算/operation 配置、三层运行态、执行请求行、唯一键、状态/时间索引、CHECK、默认行、platform/pool 种子和并发迁移 advisory lock。
- 生命周期：短事务入队、platform→pool→operation 锁序、公平 admission、waiting 即时取消、release token 哈希、typed outcome、snapshot miss 保留租约、不确定结果保守 grace 回收、计数重算、运行态删除和终态双上限清理。
- Typed Worker lease：协议字段全部为 Unix millisecond 数字，包含 issued/deadline/fence/dispatched、budget hash、clock skew 和 fencing margin。
- 有界组件：请求/上游/内部 envelope 读取、gzip/deflate 压缩与解压双限、HTTP 写出、MCP base64 内容、审计 Header/正文双限和动态脱敏。
- Formal model：完整模型和三个 chart 模型补入 waiting cancel、same-instance snapshot recovery，以及 `SnapshotRecoverySafety`；保留原有 early release / stale start 反例。

## 已验证结果

详见 `docs/plans/openapi-workers/evidence/I04-VERIFICATION.md` 和 `docs/plans/openapi-workers/formal/EVIDENCE.md`。

## 实施中修复的问题

- 并发 PostgreSQL DDL 迁移竞态、非法默认种子、operation pool CHECK、platform/pool runtime 缺行。
- Release/Cancel 原先的行锁在自动提交语句中无效；现已与状态更新、计数重算和清理放进同一短事务。
- runtime operation state 创建与空闲删除存在竞态；已移动到入队事务内。
- 同一 Manager 的 admission 事务现在串行化；所有状态事务默认 250ms deadline。
- typed lease JSON 时间从 Go time 字符串修正为冻结契约要求的 Unix millisecond 数字。
- 补齐 `snapshot_required` 保留 active 租约的 typed outcome 和模型行为。
- source root/default port/percent encoding 规范化、pool-platform 配置关系和审计 Header 硬上限。
- LimitedReader 现在越过上限立即失败；gzip/deflate 同时限制压缩输入和解压输出。

## 剩余工作与边界

- I02R 消费 typed lease/outcome；I03 将 admission coordinator 和有界响应/审计组件接入 HTTP/MCP 入口。
- I05 需要两 Obot/两 Worker/真实 PostgreSQL/假上游验证真实取消传播、Worker lost、重启恢复、扩缩容、全局峰值和部署行为。
- 本任务验证是代码、迁移、模型和隔离 PostgreSQL 流程；不是业务 API E2E、Kubernetes 部署或生产验收。
- SQLite 仅保留为仓库既有显式单进程迁移开发路径；`Manager` 对 SQLite/非 PostgreSQL 执行申请仍 fail closed，不得用于全局额度结论。
