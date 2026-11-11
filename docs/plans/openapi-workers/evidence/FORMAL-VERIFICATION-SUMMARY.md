# F01—F05 形式化验证汇总

时间：2026-11-09 18:44 Asia/Shanghai。结论：verified for design-level formal checks；不表示业务实现、E2E、Compose 部署或真实企业授权完成。

| Feature | 关键性质 | 00 复跑结果 | 项目本地复跑入口 | 设计边界 |
| --- | --- | --- | --- | --- |
| F01 | 冷缓存请求必须恰好补送一份有效快照并取得一次 requestID fence，才能触达上游 | Broken 57 distinct / 120 steps 出现预期三反例；恢复 65 generated / 33 distinct 全规则通过 | `rtk bash /Users/echo/orca/workspaces/obot/openapi-shared-worker/.codex/skills/verify-openapi-shared-worker/scripts/verify.sh` | 单请求冷缓存；不覆盖热缓存、LRU、并发编译去重、重启和业务 exactly-once |
| F02 | 只有调用者本人请求实例的精确有效业务凭证可派发 Worker | 恢复 18 distinct 全规则通过；Broken 19 distinct 仅凭证隔离失败，u2/i2 借用 u1/i1 | `rtk bash /Users/echo/orca/workspaces/obot/openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/verify.sh` | 两用户/两实例/一组件/一工具；不覆盖传输、digest、重试、SSE、加密和刷新 |
| F03 | 唯一 raw 模板匹配且三层工具白名单全通过后才可保留 raw path/query 执行 | 完整恢复 241 distinct 全规则通过；完整 Broken 精确触发唯一授权匹配失败；图表 17/20 states 覆盖正常流和双反例 | `rtk bash /Users/echo/orca/workspaces/obot/openapi-http-proxy/.codex/skills/verify-openapi-http-proxy/scripts/verify.sh` | 有限组件/方法/segment/query；不覆盖 Header/body、预算、网络、实现和 E2E |
| F04 | 跨副本租约未过期且唯一归属，过期保守占用，过期许可不得启动上游 | 完整恢复 233,082 generated / 181,137 distinct 全规则通过；early/stale 各只触发预期反例；图表 115/139/136 states | `rtk bash /Users/echo/orca/workspaces/obot/openapi-execution-budgets/.codex/skills/verify-openapi-execution-budgets/scripts/verify.sh` | 两请求/两副本/单 active slot；不覆盖 PostgreSQL 锁、token 哈希、跨操作公平性和真实部署 |
| F05 | 双 Compose 栈共享网络、DB、服务认证、Obot/Worker 全 ready 后才放行流量；滚动升级保留容量 | 完整基线 65,706 distinct 全规则通过；Broken 图表 64 states 精确触发 traffic/capacity 反例；正常图 37 states | `rtk bash /Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/.codex/skills/verify-openapi-compose/scripts/verify.sh` | 设计生命周期；不执行 Docker/Compose、不解析 YAML、不验证 TLS 字节/轮换或双故障 |

所有复跑均固定 TLA+ Tools v1.8.0，SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`。R01—R05 必须审查本表对应的 PLAN、formal 输出、skill、反例与边界，不能把任何一行外推为实现或 E2E 通过。
