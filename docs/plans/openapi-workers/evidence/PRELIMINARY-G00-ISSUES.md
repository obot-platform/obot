# G00 预审问题矩阵

时间：2026-11-09 16:28 Asia/Shanghai。状态：preliminary。本文件只汇总各 feature PLAN 已声明的分歧和缺口，不构成问题关闭证据；R01—R05 仍必须独立审核，G00 只能以复核后的 REVIEW 与实际产物为准。

| ID | 问题 | 初步责任 | 交叉复核重点 | 状态 |
| --- | --- | --- | --- | --- |
| G-I01 | v0.27.2 只设置 `stateless_http=True` 不足以保证 POST JSON-only；草案无 SSE 与现状冲突 | 01/02 | 02 bridge 不暴露 `/mcp`，实际请求/响应负例验证；不得只看配置名 | open |
| G-I02 | Worker 运行时 public-only egress 与公司内网业务 API 冲突 | 01/00/05 | 冻结默认拒绝私网 + 显式部署允许 CIDR/域名，并验证 DNS 全答案与 origin 固定 | open |
| G-I03 | Header policy 来源多层，快照摘要与授权身份可能漂移 | 02/01/03 | 冻结为发布组件 snapshot + 用户 instance userAllowed 策略；固定 Header 不得作为个人业务凭证 fallback | open |
| G-I04 | 现有 server/catalog 固定凭证 context 可能造成用户/管理员 token 串用 | 02 | 每调用 resolver 只读精确用户实例 context；F02 模型必须证明派发条件 | open，F02 进行中 |
| G-I05 | 普通 ServeMux 清洗/解码 RawPath，破坏原始 HTTP 语义 | 03/02 | `/api-proxy/` 需要 mux 前专用 RawPath 分支；测试 `%2F`、`//`、`.`/`..` | open |
| G-I06 | 管理端 `vmcp_id` 授权变量/可见性不能替代应用代理资格检查 | 03/02 | 专属授权顺序：API Key/vMCP 资格、组件/实例、原始工具名、operation 白名单 | open |
| G-I07 | P02 `Reference/Invocation/Description` 草案缺少安全落地字段 | 02/03/04 | 平台 principal、用户收窄 operation、raw 输入、Header 分类、预算键和错误归属必须在冻结接口中落齐 | open |
| G-I08 | 统一响应 writer 会把平台 Header/Cookie/Content-Type 污染业务响应 | 03/02 | 业务响应保留端到端 Header/body；平台错误才使用平台 envelope | open |
| G-I09 | 多 path template 匹配同一具体路径时可能绕过白名单 | 03 | 静态段优先，等价/歧义拒绝，不允许多候选试授权 | open |
| G-I10 | Worker 是否需要访问 PostgreSQL 存在表述差异 | 01/04/05 | 默认 Worker 不持 DB secret、不连 DB；预算/租约由 Obot 侧访问 PostgreSQL | open，倾向 P05 最小权限 |
| G-I11 | 生产服务认证候选不一致：mTLS 与 bearer+TLS | 01/02/05 | 冻结生产 mTLS，可叠加 bearer；本地验收必须实测 mTLS，不允许 HTTP 降级 | open |
| G-I12 | Obot 缺少独立 readiness，双 Compose 栈升级会耦合故障域 | 05/02/03 | 新增 `/api/readyz`，保留 `/api/healthz`；Obot ready 不等待 Worker 栈 | open |
| G-I13 | 现有代理审计无界缓冲且脱敏清单固定 | 04/03/02 | OpenAPI 新链路使用 bounded capture，按声明认证 Header 脱敏；F04 需覆盖租约/预算边界 | open，F04 未开始 |
| G-I14 | 公共 Go 类型、router、migration 和生成文件存在并行写冲突 | 02/03/04/00 | G00 必须冻结写集和串行接线顺序；默认避免 `apiclient/types` 生成 | open |
| G-I15 | Docker daemon 当前不可访问，不能执行 Compose build/up/E2E | 05/00 | 保留 GAP；`compose config` 不能冒充部署通过；实施时 daemon 可用则实测，否则 blocked | open |

## R02 复核增量

- R02-01（P1）：P02 Executor 缺少平台 principal 与 P03 所需安全字段。
- R02-02（P1）：Header 策略与 executionDigest 语义未收敛。
- R02-03（P1）：MCP component loopback 身份缺少 Profiles 所需完整用户组。
- R02-04（P1）：生产 Worker 服务认证默认值不兼容。
- R02-05（P1）：raw path 与原始响应 writer 公共写集未划分。
- R02-06/R02-07（P2）：预算 source identity、平台保护 Header 名单未统一。

## R01 复核增量

- R01-01（P1）：Worker 租约 fencing/deadline 协议在 01/02/04 间不兼容。
- R01-02（P1）：生产 Worker 服务认证未收敛。
- R01-03（P1）：P02 Executor 不足以承载 P03 安全边界。
- R01-04（P1）：凭证/body/预算获取顺序冲突。
- R01-05/R01-06/R01-07（P1）：raw path、专用 vmcp 授权和响应 writer 均必须由 00 划定写集并修订契约。
- R01-08—R01-13（P2）与 R01-14（P3）：覆盖预算多作用域、legacy source reference、Compose 私网 egress、`ensure` 命名、F01 digest 绑定、保护 Header 和图表可读性。

## R03 复核增量

- R03-01（P1）：预算获取归属与调用顺序在 02/03/04 间不兼容。
- R03-02（P1）：P02 Executor 与 executionDigest 的 Go 侧来源不足以支撑 01/03。
- R03-03（P1）：raw-prefix 分支与业务响应 writer 公共写集未冻结。
- R03-04（P1）：deadline 提前交还租约与保守回收语义冲突。
- R03-05—R03-11（P2）与 R03-12（P3）：覆盖平台 Key/vMCP scope、raw path 等价、参数校验、响应协议、sourceRef 规范化、transport retry、DNS TOCTOU 和 RTK 复跑命令一致性。

## 已有形式化进展

- F01 已证明冷缓存请求必须“一份有效快照 + 一次 requestID fence”才能触达上游；破坏变体能触发重复执行/前置条件失败/不确定后重试。热缓存、LRU、并发编译去重、重启和业务 exactly-once 仍在边界外。
- F02 完成：恢复配置 TLC 22 generated / 18 distinct，`ExactCallerCredentialOnly`、`TypeOK`、`NoDeadlock`、`CHECK_DEADLOCK` 均持有；Broken 21 generated / 19 distinct，仅 `ExactCallerCredentialOnly` 失败，最短路径显示 u2 请求 i2 时 fallback 取得 u1/i1 凭证并派发。项目本地 skill 与 EVIDENCE 已交付。
- F05 完成。完整基线 TLC 实际通过：563,869 generated / 65,706 distinct、深度 29，`TypeOK`、`NoDeadlock`、`OpenAPITrafficGated`、`PlannedRollRetainsCapacity` 均持有；Broken 精确复现流量门槛与滚动容量丢失。37-state 正常图覆盖共享网络、依赖、服务认证、Obot/Worker ready、流量开启和滚动升级；64-state Broken 图同时展示 unsafe traffic 与 capacity lost。项目本地 skill、复跑脚本与 EVIDENCE 已交付。
- F03 完成：完整模型 613 generated / 241 distinct 全部不变量通过；Broken 精确触发 `ExecutionRequiresUniqueAuthorizedMatch`。图表模型 17 states 生成正常流转，20 states 生成 unique-route fallthrough 与 whitelist bypass 两个反例；项目本地 skill、严格复跑脚本、EVIDENCE 和 Mermaid 已交付。
- F04 完成：完整模型 233,082 generated / 181,137 distinct 全部不变量通过；early-release 只触发 `ConfirmedReleaseInsideLease`，stale-start 只触发 `ActiveLeaseSafety`。正常/early/stale 三个图表模型分别为 115/139/136 states，均生成 Mermaid、无未检查规则；项目本地 skill 与 EVIDENCE 已交付。
