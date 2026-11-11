# F02 Obot 接入形式化验证完成证据

时间：2026-11-09 16:37 Asia/Shanghai。结论：formal planning complete；这是设计级模型检查，不表示实现、E2E、部署或真实 token 交换已完成。

## 交付核验

- Task：`task_ebcc62d86179`；Dispatch：`ctx_b0d519ec6ab6`；`worker_done` 成功。
- 关键性质：OpenAPI 请求只有通过平台身份、实例归属、Profiles、原始工具授权，并取得调用者本人请求实例的精确有效业务凭证后，才允许派发到 Worker。
- 项目本地 skill：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/SKILL.md`。
- 复跑脚本：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/verify.sh`；00 核验 Bash 语法与 statechart 入口有效。
- 模型/配置/视图/输出：`OpenAPIWorkerResolver.tla`、恢复/Broken 配置、`view.mjs`、`EVIDENCE.md`、`statechart-pass/`、`statechart-broken/`。

## 验证结果

- 恢复配置：TLC 22 generated / 18 distinct，深度 8，0 left on queue；`ExactCallerCredentialOnly`、`TypeOK`、`NoDeadlock`、`CHECK_DEADLOCK` 全部通过。
- Broken 配置：TLC 21 generated / 19 distinct；仅 `ExactCallerCredentialOnly` 失败，基础规则仍通过且无未检查规则。
- 预期反例：`u2` 请求 `i2`，跨实例 fallback 选择 `i1` 中属于 `u1` 的有效凭证，并把请求派发给 Worker。
- 复跑脚本固定 TLA+ Tools 1.8.0 SHA-256，断言恢复规则、唯一预期失败、具体反例值、无意外不变量失败和状态数解析。
- 00 于 2026-11-09 16:40 Asia/Shanghai 在 `/Users/echo/orca/workspaces/obot/openapi-worker-bridge` 独立执行 `bash .codex/skills/verify-openapi-worker-bridge/verify.sh`，退出码 0；脚本报告恢复 18 states、Broken 19 states，并生成 `docs/plans/openapi-workers/formal/RUN.md`。

## 边界

模型覆盖两个用户、两个实例、一个组件、一个工具、身份/实例/授权/凭证解析与 fallback。未覆盖 HTTP 传输、snapshot digest、snapshot_required 重试、JSON-only SSE、加密、token 刷新、预算、审计和上游语义；不能外推为实现或 E2E 通过。
