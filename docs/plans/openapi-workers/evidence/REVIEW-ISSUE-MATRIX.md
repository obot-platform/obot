# R01—R05 去重问题矩阵

更新时间：2026-11-09 20:09 Asia/Shanghai。输入为五份集体交叉审核 `REVIEW.md`。本矩阵只做去重、归属和修订拆分，不是问题关闭证据；G00 仍不得冻结。

原始发现共 55 项：26 个 P1、26 个 P2、3 个 P3。去重后形成 30 个问题簇，其中 11 个包含 P1。两个修订 worker 分别是：

- C01：身份、Resolver、HTTP 边界与跨语言身份契约修订。
- C02：租约、预算、调度恢复、Worker 生命周期与部署契约修订。

| 簇 | 最高级别 | 去重后问题 | 原始发现 | 影响/责任 | 修订归属 |
| --- | --- | --- | --- | --- | --- |
| D01 | P1 | Worker 租约字段可见性、margin 校验和 stale-start fence 未统一 | R01-01, R04-01, R05-05 | 旧 Worker 可在全局租约失效后发起业务调用 | C02 |
| D02 | P1 | 生产 Worker TLS/mTLS/bearer 必选与可选层次未冻结 | R01-02, R02-04, R04-05, R05-03 | Obot client、Worker server 和 Compose secrets 无法互操作，可能降级传输 | C01 |
| D03 | P1 | Principal-aware Executor 缺少平台身份、raw 输入、typed error 和收窄结果 | R01-03, R02-01, R03-02 | 03 被迫自填身份或复制授权，存在越权/隐藏 operation 风险 | C01 |
| D04 | P1 | canonical operation manifest、Header 策略与 executionDigest 的 Go/Python 来源未冻结 | R02-02, R03-02, R04-04 | 冷启动 digest 不一致、补送失败或缓存身份漂移 | C01 |
| D05 | P1 | 预算 source identity、URL 规范化、落库和 legacy 缺失策略未收敛 | R01-09, R02-06, R03-09, R04-08, R05-04 | 同源额度被拆分、异源误合并或 legacy 组件被静默拒绝 | C02 |
| D06 | P1 | 认证/资格、body 大小和 Acquire 的唯一 owner 与顺序冲突 | R01-04, R03-01, R04-03 | 409/413 是否占队、慢 body 占租约、双重 Acquire 语义分叉 | C02 |
| D07 | P1 | raw path 专用路由、业务响应 writer 和公共写集未划分 | R01-05, R01-07, R02-05, R03-03 | `%2F`/路径清洗重定向和平台 Header/Cookie 污染业务响应 | C01 |
| D08 | P1 | `/api-proxy` 不能复用管理 `vmcp_id` 授权，需专用调用资格链 | R01-06 | 管理可见性被误当作调用资格或泄漏资源存在性 | C01 |
| D09 | P1 | 取消、deadline、Worker lost 与租约释放缺少 typed outcome 状态矩阵 | R03-04, R04-02 | uncertain 业务执行期间提前释放额度导致跨副本超发 | C02 |
| D10 | P1 | 原 Namespace 目标改为 Compose-only 的需求变更未显式记录 | R05-01 | 目标范围漂移，缺少用户确认或替代验收 | C02 |
| D11 | P1 | 双 Worker 快照 miss/补送策略不能保证确定性恢复 | R05-02 | 随机路由、重启或缓存淘汰导致补送仍 miss | C02 |
| D12 | P2 | 多层预算、公平排队和等待取消缺少设计级反例覆盖 | R01-08, R04-06, R04-07 | 三级计数漂移、死锁、慢接口独占或断开票继续占队 | C02 |
| D13 | P2 | Compose/内网验收缺少窄化的 Worker egress allowlist 与负例 | R01-10, R05-06 | 验收默认拒绝失败，或被放宽为允许整个私网 | C02 |
| D14 | P2 | P02 `ensure` 是未定义协议名词 | R01-11 | 可能生成第三个 Worker endpoint 或不兼容客户端 | C01 |
| D15 | P2 | F01 模型未表达补送快照 digest 相等性 | R01-12 | 序列化/digest bug 可绕过模型并在实现中命中错误计划 | C01 |
| D16 | P2 | 平台保护 Header 名单、冲突覆盖和跨语言向量未冻结 | R01-13, R02-07, R03-08 | 请求方同名 Header 覆盖平台/业务凭证或平台语义穿透上游 | C01 |
| D17 | P2 | raw path percent-decode、静态段等价和歧义模板规则未冻结 | R03-06 | Go/Python matcher 对同一请求给出不同 operation 或错误码 | C01 |
| D18 | P1 | MCP component loopback 身份不足以独立复算完整 Profiles | R02-03 | 丢失 Obot/IdP groups，共享 Profile 放行/拒绝不正确 | C01 |
| D19 | P2 | API Key/vMCP scope 未进入形式化模型或显式边界 | R03-05 | 模型通过不能发现 Key 授权 A 却调用 B 的绕过 | C01 |
| D20 | P2 | HTTP mode 的最小 OpenAPI 参数校验边界未定义 | R03-07 | 坏参提前归平台 400 或透传成业务错误的行为分叉 | C01 |
| D21 | P2 | HTTP 响应协议子集、1xx/upgrade/trailer 和保留 Header 语义不完整 | R03-08 | 可能输出半截成功、丢 Trailer 或误删业务 Header | C01 |
| D22 | P2 | operation budget state 保留策略和 admission pump 扫描缺少长期上界 | R04-09 | 历史 operation 状态与锁范围无界增长 | C02 |
| D23 | P2 | describe/内部 JSON envelope 缺少独立硬上限和超限语义 | R04-10 | 编译计划合法但序列化后突破 02/03 内存边界 | C02 |
| D24 | P2 | MCP JSON 转换、审计截断与动态认证 Header 脱敏未接线 | R04-11 | MCP/HTTP 链路有界性不一致，token 可能进入普通日志 | C02 |
| D25 | P2 | Go transport 自动重试/连接重放和 Worker 重启后的 no-retry 语义未约束 | R03-10, R04-12 | 同一 requestID 可能二次触达业务系统 | C02 |
| D26 | P2 | Worker DNS 校验缺少 IP pinning 与 TOCTOU/重连策略 | R03-11 | 二次解析可把带凭证请求导向未授权私网地址 | C02 |
| D27 | P2 | Describe/冷编译与 Acquire 的先后顺序和独立资源预算未冻结 | R05-07 | 未授权请求可消耗编译资源，或破坏统一预算入口 | C02 |
| D28 | P2 | F02 缺少同一用户多实例与禁用工具/组件反例 | R05-08 | exact-instance 凭证隔离强于现模型的证明不足 | C01 |
| D29 | P3 | broken statechart 未清楚呈现被破坏的 gate/具体反例 | R01-14, R05-09 | 人审易误解模型反例，证据可读性不足 | G00 复核 |
| D30 | P3 | P02 verify skill 复跑命令未按 RTK 约定书写 | R01-14, R03-12, R04-13 | 后续复跑可能绕过本机统一命令约定 | G00 复核 |

## 修订边界

- C01/C02 只在各自 feature worktree 写 `docs/plans/openapi-workers/CONTRACT-REVISION.md`，不得直接修改 00 契约、他人 PLAN、formal 产物或业务代码。
- 每个修订建议必须给出可直接替换/新增到 `CONTRACTS.md` 的文本、影响 feature、测试向量和可验证关闭条件。
- 涉及用户需求取舍的 D10 不得自行改变目标，只能列 A/B 决策卡并标记需要用户确认；未确认前不得选择。
