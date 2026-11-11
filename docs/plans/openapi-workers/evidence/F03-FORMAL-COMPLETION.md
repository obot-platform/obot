# F03 HTTP 代理形式化验证完成证据

时间：2026-11-09 18:29 Asia/Shanghai。结论：formal planning complete；这是设计级模型检查，不表示代理实现、E2E、部署或真实业务 API 已完成。

## 交付核验

- Task：`task_434369177d46`；Dispatch：`ctx_77786b24bb5c`；`worker_done` 成功。
- 关键性质：代理请求只有在目标组件/方法下唯一 raw 模板匹配，且 ToolOverride、Profile 与实例白名单全部通过后，才允许保留原始 path/query 并执行；拒绝路径不得触达上游。
- 项目本地 skill：`/Users/echo/orca/workspaces/obot/openapi-http-proxy/.codex/skills/verify-openapi-http-proxy/SKILL.md`。
- 复跑脚本：`/Users/echo/orca/workspaces/obot/openapi-http-proxy/.codex/skills/verify-openapi-http-proxy/scripts/verify.sh`；00 核验 Bash 语法、产物和临时清理通过。
- 模型/配置/图表/证据：`OpenAPIHTTPProxy.tla`、完整恢复/Broken 配置、图表专用模型、`statechart.mjs`、`runs/latest/EVIDENCE.md` 与 statechart 输出。

## 验证结果

- 完整恢复配置：TLC 613 generated / 241 distinct，0 left on queue，四条不变量全部通过。
- 完整 Broken 配置：21 generated / 21 distinct，精确违反 `ExecutionRequiresUniqueAuthorizedMatch`，其他规则不被隐藏。
- 正常图表：17 distinct states / 7 个可见状态 / 9 条箭头，覆盖 raw 解析、唯一路由、四层白名单、执行与两类拒绝。
- Broken 图表：20 distinct states / 7 个可见状态 / 11 条箭头，展示 ambiguous route 执行和 whitelist bypass 两个最短反例。
- 00 于 2026-11-09 18:29 Asia/Shanghai 独立执行 `rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh`，退出码 0；TLC 与 statechart 全部按预期，临时 `TTrace/states` 文件计数为 0。

## 边界

模型覆盖有限组件、方法、raw segment/query、模板歧义和三层工具白名单，不覆盖 Header/body 细节、预算、租约、网络、实现代码或 E2E。图表模型是完整模型的受限投影，不能替代完整 TLC 基线。
