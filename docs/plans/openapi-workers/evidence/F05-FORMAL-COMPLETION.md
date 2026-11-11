# F05 Compose 生命周期形式化验证完成证据

时间：2026-11-09 16:44 Asia/Shanghai。结论：formal planning complete；这是设计级模型检查，不表示 Compose 文件、Docker 部署、TLS 实测或 E2E 已实施。

## 交付核验

- Task：`task_d3406e8900c8`；Dispatch：`ctx_e01343a422ee`；`worker_done` 成功。
- 关键性质：OpenAPI 流量只有在共享网络、PostgreSQL、服务认证、至少一个 Obot 与至少一个 Worker 均满足门槛后才允许准入；计划内逐实例升级期间不得失去全部服务容量。
- 项目本地 skill：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/.codex/skills/verify-openapi-compose/SKILL.md`。
- 复跑脚本：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/.codex/skills/verify-openapi-compose/scripts/verify.sh`；00 核验 Bash 语法通过。
- 模型/配置/视图/输出：`ComposeLifecycle.tla`、完整基线/正常图/Broken 图配置、`statechart-view.mjs`、`EVIDENCE.md` 与三组 statechart 输出。

## 验证结果

- 完整基线：TLC 563,869 generated / 65,706 distinct，深度 29；`TypeOK`、`NoDeadlock`、`OpenAPITrafficGated`、`PlannedRollRetainsCapacity` 与 `CHECK_DEADLOCK` 全部通过。
- 正常图配置：37 distinct states，8 个可见状态 / 13 条箭头，覆盖共享网络、PostgreSQL、服务认证、Obot/Worker ready、流量开启和滚动升级。
- Broken 图配置：64 distinct states，精确触发 `OpenAPITrafficGated` 与 `PlannedRollRetainsCapacity`；`TypeOK`、`NoDeadlock`、`CHECK_DEADLOCK` 继续持有，无未检查规则。
- 复跑脚本固定 TLA+ Tools 1.8.0 SHA-256，断言完整基线与正常图全部规则、图表关键状态、Broken 精确反例集合、无 `Rules not checked`，并生成 EVIDENCE。
- 00 于 2026-11-09 16:48 Asia/Shanghai 在 `/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance` 独立执行 `bash .codex/skills/verify-openapi-compose/scripts/verify.sh`，退出码 0；完整基线 65,706 states、正常图 37 states、Broken 图 64 states，均符合预期。

## 边界

模型不执行 Docker/Compose、不解析 YAML、不验证 TLS 字节/轮换、不覆盖请求载荷限制、租约、上游行为或双故障；通过不代表 Compose build/up、部署或 E2E 成功。
