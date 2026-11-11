# C02 契约修订：租约、预算与运行恢复

日期：2026-11-09。状态：proposed for G00。写集：本文件唯一。本文只修订共享契约，不实施业务代码、不改其他 PLAN/formal 产物、不运行业务 API。

## 1. 结论

- D01、D05、D06、D09、D11、D12、D13、D22、D23、D24、D25、D26、D27 均给出唯一建议方案；第 3 节是可直接替换/新增到 `CONTRACTS.md` 的整合片段。
- D10 是用户需求取舍，不能由 C02 选择。第 4 节给出 A/B 决策卡；用户明确选择前 G00 必须继续阻断。
- 建议将“Obot 侧 02 admission coordinator”作为唯一 Acquire/Release owner，Worker 只消费 typed lease 字段；凭证资格与声明大小在入队前检查，实际 chunked body 在租约内流式读取。
- 建议 execution 采用命名 Worker 端点按 `executionDigest` 粘路由；`snapshot_required` 只允许向返回 miss 的同一 Worker 实例补送一次。控制面 describe 独立限流， execute 冷编译计入执行租约。
- 预算身份只保存 canonical source hash、operation hash 与预算键 hash；legacy 缺失时固定模式明确拒绝，不回退 component/vMCP/schema digest。

## 2. 簇决策索引

| 簇 | 原始 finding | 唯一建议 | 主要备选与不采纳原因 |
| --- | --- | --- | --- |
| D01 | R01-01, R04-01, R05-05 | Worker 可见 typed lease/fence 字段，Obot 同时收缩 HTTP deadline；双重防线 | 完全 opaque 且只靠 Obot deadline：少一次 Worker 防线，时钟/调度错误更难阻止 |
| D05 | R01-09, R02-06, R03-09, R04-08, R05-04 | 结构化 canonical source identity + SHA-256 落库 + legacy 显式拒绝/保留旧后端 | 继续 raw `sourceID::entryKey`：等价 URL 拆额度并泄漏内部 URL；component/digest fallback 会错误合并或拆分 |
| D06 | R01-04, R03-01, R04-03 | 02 admission coordinator 唯一 Acquire/Release；资格与声明大小前置，实际 body 租约内读取 | 03 直接持有 lease：MCP/HTTP 双入口会分叉；Worker 持有 release token：扩大信任面 |
| D09 | R03-04, R04-02 | typed outcome；仅 `upstream_attempted=false` 或完整业务结果可立即释放，其余进 grace | 所有 cancel/504 立即释放：旧上游仍执行时全局超发；全部等 grace：确认无上游的失败也占额 |
| D10 | R05-01 | 用户 A/B 决策，未选择 | 不允许默认选择；否则静默删除原 Namespace 验收 |
| D11 | R05-02 | 命名 Worker + digest 粘路由 + 同实例一次快照补送 + 双实例预热 | 共享 alias 随机重试：补送可能落到另一 cold Worker；Worker 回拉快照：新增反向信任与回调协议 |
| D12 | R01-08, R04-06, R04-07 | 三层同一短事务、最久未放行 runnable operation 优先、waiting 即时取消 | 每请求轮询 DB 或池内队首阻塞：锁范围和慢接口隔离均不可控 |
| D13 | R01-10, R05-06 | 默认拒私网；私网只允许“精确域名 + 端口 + 成对 CIDR”文件化 allowlist | 允许整段私网：验收方便但扩大 SSRF/DNS rebinding 面 |
| D22 | R04-09 | 只为有 active/waiting 的 operation 维护临时 runtime state，空后原子删除；终态请求双上限保留 | 永久 operation state：历史 schema 无界增长并放大 pump 锁/扫描 |
| D23 | R04-10 | describe 强制 cursor 分页，4 MiB envelope、1 MiB 单 operation、2 MiB 请求 | 放大单个 describe 上限：合法 schema 仍可放大 02/03 内存边界 |
| D24 | R04-11 | 04 提供 `ToBoundedMCPContent` 与动态脱敏审计，01/02/03 复用；MCP 任意 body 用 base64 保留 | 各入口自行 buffer/截断：HTTP/MCP 边界不一致且可伪造完整响应 |
| D25 | R03-10, R04-12 | execute 使用 SingleAttemptTransport，无 `GetBody`、无 redirect、无底层重放；requestID 不承诺跨重启 exactly-once | 仅 Worker requestID 内存去重：重启丢失且无法约束 Go transport |
| D26 | R03-11 | 每次业务 attempt 校验全部 DNS 答案并连接 pinned IP，TLS/SNI/Host 保持原域名；同一 attempt 不重连 | 校验后再交给普通 transport：二次解析存在 TOCTOU/rebinding |
| D27 | R05-07 | 本地 canonical manifest 匹配；describe 独立预算；execute 必须先 Acquire，冷编译在租约内 | 执行前 Worker describe：未入队请求可耗编译资源且破坏统一预算 |

## 3. 可直接套用的共享契约片段

本节是建议替换/新增文本。G00 复核时不要把 D10 的两个备选同时套用；D10 未决策前保持阻断。

### 3.1 替换 `CONTRACTS.md` 第 3 节中 execute 最小上下文段落

> execute 最小上下文包括协议版本、requestID、executionDigest、operationKey、输入模式、绝对截止时间、typed Worker lease、逐请求临时认证 Header，以及冷启动提示所需的不可变快照 envelope。HTTP 输入保留原始方法、匹配后的路径、RawQuery、允许的端到端 Header 和原始 body 字节；JSON 内字节采用 base64。工具输入使用同一 operation 定义进行位置绑定。
>
> Worker lease 对 Worker 不是 opaque。必需字段为：
>
> ```text
> leaseID: string
> leaseIssuedAtUnixMs: number
> leaseDeadlineUnixMs: number
> fenceDeadlineUnixMs = leaseDeadlineUnixMs - fencingMarginMs
> dispatchedAtUnixMs: number
> budgetKeyHash: string
> clockSkewLimitMs: number
> fencingMarginMs: number
> ```
>
> release token 只存在于 Obot 进程和数据库哈希中，不出现在 Worker 协议、日志或审计。Worker 必须在 DNS 解析、上游连接和任何业务请求前完成全部检查：字段完整；`leaseIssuedAt <= dispatchedAt < fenceDeadline`；`fencingMarginMs > clockSkewLimitMs`；Worker 本地 UTC 时间与 `dispatchedAtUnixMs` 的偏差不超过 `clockSkewLimitMs`；最终执行 deadline 不晚于 `fenceDeadlineUnixMs`。任一检查失败返回 `lease_expired` 或 `lease_invalid`，`upstream_attempted=false`，不解析业务 DNS。Obot 仍必须把 HTTP context deadline 设为不晚于 fence deadline；这是第二道防线，不能替代 Worker 检查。
>
> Worker 将墙钟时间转换为单调时钟后执行剩余 deadline；本地时钟回拨不得延长已转换的 deadline。生产必须同步 Obot/Worker/PostgreSQL 时钟并监控三者偏差；偏差告警阈值必须小于 fencing margin。任意时钟攻击、宿主长时间冻结和非 NTP 可控环境不属于平台硬实时保证。

### 3.2 替换 `CONTRACTS.md` 第 6 节

> ## 6. 预算、队列和恢复
>
> 使用现有 PostgreSQL 作为跨副本状态基础，不新增 Redis 或调度集群。SQLite 只能用于显式单进程开发，不可冒充多副本全局限额。预算存储不可用时新执行关闭并返回 503，不回退为副本内存限额。
>
> ### 6.1 BudgetSourceID 与预算键
>
> `BudgetSourceID` 只能来自发布组件的不可变 catalog reference，或同一 retained component snapshot 中唯一可推导的 catalog entry 元数据。优先级为：发布组件 `ComponentCatalogReferences`；快照内唯一 catalog source 引用；两者均缺失或多义时返回 `budget_identity_unavailable`。禁止回退 vMCP ID、component ID、schema digest、executionDigest、用户或实例。
>
> 新 source URL 先做结构化 canonicalization：仅允许小写 `http/https`；禁止 userinfo；host 使用 IDNA 小写并去尾点；删除默认端口；path 使用 RFC 3986 百分号规范编码、移除 dot-segment、合并重复斜杠并将尾斜杠规范为一个；丢弃 query/fragment；entryKey 做 Unicode NFC、去除首尾空白并禁止空值、换行和 `::`。无法唯一规范化的 legacy reference 固定模式拒绝；选择固定 Worker 模式不迁移、不回退、不隐藏旧 Docker/K8s 后端能力。
>
> 数据库和日志不保存原始 source URL。可持久化字段为 `budget_source_hash = SHA-256("openapi-budget-source-v1\0" + canonicalSourceTuple)`、`operation_key_hash = SHA-256(operationKey)` 和 `budget_key_hash = SHA-256("openapi-budget-key-v1\0" + budget_source_hash + "\n" + operation_key_hash)`。管理观测只显示完整 `budget_key_hash`、`budget_source_hash` 前 16 个十六进制字符、pool、operation 指纹和计数；不得显示完整内部 URL 或凭证。operation 配置唯一键为 `(budget_source_hash, operation_key_hash)`。
>
> `operationKey` 仍为大写 HTTP 方法、空格、schema path template。用户、实例、vMCP、component、Worker、快照版本、别名、旧工具名和 executionDigest 均不参与预算键。同一来源经 HTTP/MCP、多个 vMCP/component、多个 Obot/Worker 共享额度；快照更新不拆分额度。
>
> ### 6.2 唯一 admission owner 与调用顺序
>
> 02 的 Obot admission coordinator 是唯一 Acquire/Release owner。03 和 MCP bridge 只调用 02 Executor，不直接调用 04 Manager；Worker 永不 Acquire/Release。02 先用本地 canonical manifest 和 admitted principal 完成平台认证、vMCP/当前实例/component/operation/Profile/override 资格、业务凭证存在性与可解密资格、canonical source 身份、方法/路径匹配和声明 body 大小检查；这些检查失败在入队前返回 401/403/404/409/413 等对应错误，不创建等待票据。凭证值在全部检查和 body 读取完成后、Worker 发送前才揭示。
>
> 有 `Content-Length` 且超过原始 body 上限时，必须在 Acquire 前 413。chunked 或长度不可知 body 只能在 Acquire 后用 1 MiB bounded reader 流式读取；读取时间计入 lease deadline，实际超限、超时或客户端断开则关闭输入并按 `upstream_attempted=false` typed outcome 幂等释放。请求 body 不得在入队前无界缓冲，也不得因慢上传在无租约状态下占用全局执行资源。
>
> execute 冷编译、快照补送、上游执行、响应读取和下游有界转换均发生在同一执行租约内。业务 4xx/5xx 是 business result，不是平台失败；完整业务结果取得后正常释放。
>
> ### 6.3 typed outcome 与释放矩阵
>
> 所有 Worker 最终响应、取消回执和失败路径必须输出 `outcome`、`phase`、`upstream_attempted`、安全 `message` 和 requestID。只有两类证明允许立即幂等释放：Worker 证明 `upstream_attempted=false`，或 Worker 已返回完整、有界的最终 business result。其余已派发或已发送结果一律保持 active 到 `leaseDeadline + reclaimGrace` 后由 reclaimer 回收。
>
> | 状态 | upstream 事实 | 释放时机 | outcome |
> | --- | --- | --- | --- |
> | 等待中断开/取消 | 未派发 | 条件更新 waiting→cancelled，立即 | `waiting_cancelled` |
> | 派发前取消/资格后置失败 | false | 立即 | `pre_upstream_cancelled` / `pre_upstream_rejected` |
> | 首次 snapshot miss | false | 租约保持，同实例补送一次 | `snapshot_required` |
> | 第二次 miss/快照无效/编译失败 | false | 立即 | `snapshot_recovery_failed` / `compile_failed` |
> | Worker 确认取消且上游未连接 | false | 立即 | `cancelled_before_upstream` |
> | 完整 business result | 已返回最终响应 | 读取并完成有界内部转换后立即；审计失败不占额 | `business_result` |
> | deadline 后、取消后或传输失败时无法证明未发送 | unknown | grace 后回收 | `uncertain_deadline` / `uncertain_cancel` / `transport_unknown` |
> | Worker 连接丢失/崩溃/分区 | unknown | grace 后回收 | `worker_lost` |
> | 内部响应读取或转换超限 | 已开始读业务响应但未取得完整结果 | grace 后回收 | `response_limit_uncertain` |
> | Obot crash | unknown | reclaimer 在 grace 后回收 | `obot_lost` |
> | Release/DB 暂时失败 | 按已取得 outcome | 保持名额并幂等重试；Obot crash 交回回收器 | 原 outcome + `release_retry` |
>
> `snapshot_required` 是同一 requestID 的安全快照补送状态，不是业务重试；补送后的任何不确定结果禁止重发。调用方新 requestID 的外部重试可能造成业务系统自然重复副作用，平台不承诺业务 exactly-once。
>
> ### 6.4 三层计数、公平排队与等待取消
>
> platform、pool、operation 三层 active 名额必须在同一 PostgreSQL 短事务中确认，锁序固定为 platform→pool→operation，事务中禁止 DNS、Worker HTTP、上游 HTTP、审计外呼或日志外呼。每个 operation 只属于一个 pool：无专属配置为 default，专属配置为 dedicated。放行条件为 operation、pool、platform 均有余量；三层 active 计数同事务递增。
>
> 队列只保留 operation 等待票据和平台总票据；默认池/专属池不另设独立队首。入队原子校验平台总队列、operation 队列和 wait deadline。Admission pump 每 Obot 副本最多一个，默认 20ms 加最多 10ms 抖动；只在存在 waiting 的 runtime operation state 中按 `last_admitted_at` 最旧优先、同值按最老票据 `id` 选择 runnable operation。慢接口满载时不可 runnable 并被跳过，不能阻塞其他接口。
>
> waiting 票据支持条件 `waiting→cancelled` 短事务，立即递减 waiting 并释放队列容量；与 Admit 并发时以数据库状态条件更新决胜负。重复取消幂等，错误 owner/release token 不生效。Obot crash 后由 reclaimer 按同一规则兜底。配置下调不取消已 active 业务副作用，只阻止新放行直至计数低于新限制。
>
> ### 6.5 有界运行状态
>
> 不保留永久 operation budget state。`operation_runtime_state` 仅在首个 waiting/active 请求出现时创建，字段包含 budget hashes、pool、active/waiting、last_admitted_at 和版本；最后一个非终态请求离开时同事务删除。默认运行行上界为 platform 最大 active 128 + 最大队列 1024 = 1152；配置更大时上界按两者之和推导。pump 只扫描 `waiting_count>0` 的 runtime state，扫描行数不得超过当前平台队列上限。
>
> 专属 operation 配置最多 4096 行，管理查询必须按 hash/时间分页且每页不超过 500。终态 request 行保留不超过 15 分钟且最多 50,000 行；达到任一上限时在 Release/Reclaim 事务中按索引删除最老终态批。回收器始终从非终态 request 行重算三层计数，不信任漂移计数。
>
> ### 6.6 Worker 选择与快照恢复
>
> Obot 配置有序命名 Worker 端点（初始 `worker-1`、`worker-2`），不用共享 alias 执行业务 attempt。同一 `executionDigest` 用 rendezvous hash 选择命名端点；目标 readiness epoch 变化或 Obot 重启时清除 warm hint。首次或冷 hint 的 execute 请求可直接携带不可变 snapshot envelope；Worker 热缓存时可忽略。返回 `snapshot_required` 时必须在响应中携带 `workerInstanceID` 和 `upstream_attempted=false`，Obot 只向该实例用同一 requestID 补送一次快照，不改路由、不换 requestID。
>
> 发布更新、Worker readiness epoch 变化或运维预热命令必须按命名实例逐个上传快照；预热属于 control-plane describe/ensure 资源，不消耗接口执行租约。若目标实例在安全 miss 后不可用，当前请求失败并立即释放；下一 requestID 可带快照选择可用实例。两 Obot、两 Worker、重启、缓存淘汰和随机 published digest 集合下都必须证明补送前业务上游计数为 0。
>
> ### 6.7 Describe 与 Acquire 顺序
>
> operation 匹配使用 Obot 本地 canonical snapshot manifest，不在 execute 前调用 Worker describe。describe 只服务文档、预热和管理面，不携带业务凭证、不发业务请求、不消耗执行预算，但必须先通过平台身份与 operation/文档资格检查，并受独立 control-plane 预算限制：每 Obot 并发 2、队列 16、等待 2s；Worker 冷编译继续受固定 helper、编译队列和总编译时间限制。未认证或无资格请求不得触发 Worker 编译。
>
> execute 的顺序固定为：本地 resolve/资格/声明大小→Acquire→bounded body→凭证值揭示→命名 Worker execute→冷编译（如需）→唯一上游 attempt→typed outcome→Release/Reclaim。队满、等待耗尽、编译失败和 Worker 不可用均不得触达业务上游。
>
> ### 6.8 Worker 出口 allowlist 与 DNS pinning
>
> Worker 默认拒绝 loopback、private、link-local、multicast、unspecified 和非 HTTPS 业务上游（测试模式可显式允许本地 HTTP）。私网例外必须来自只读配置文件，条目为精确主机名、允许端口列表和成对允许 CIDR；不支持通配域名、整段私网或“域名允许即任意 IP”。DNS 的每一个 A/AAAA 答案都必须落在该主机条目的 CIDR 内，混合公私答案全拒。公网上游只允许公网答案。
>
> 每个业务 attempt 先解析全部答案并校验，再把连接 pin 到其中一个允许 IP；TLS SNI/证书主机名和 HTTP Host 保持原域名，不使用 IP 代替域名验证。禁止重定向携带认证；同一业务 attempt 连接失败后不得重建或二次解析。连接池 key 必须包含 origin、pinned IP、端口和 egress policy epoch；租约 fence 检查必须在复用连接写入前执行。
>
> 验收 override 只允许 fake-business 精确服务名/CIDR/端口。负例必须覆盖 loopback、另一个私网主机、混合 DNS、TTL/连接重建、第一次公网第二次私网、origin 漂移、SNI/证书不匹配。
>
> ### 6.9 no-retry 传输契约
>
> Obot→Worker execute 必须使用 SingleAttemptTransport：一次 RoundTrip、无自动 redirect、无 transport retry/replay、无 `GetBody`、请求 body 只能消费一次。可为 health/describe 复用连接池，但 execute 禁止在已开始写入后重放；实现若不能证明连接复用安全，必须对 execute 独立 `DisableKeepAlives`。Worker 重启后的内存 requestID fence 丢失不能提供 exactly-once；数据库 request 行中的 uncertain requestID 禁止自动补发。
>
> 唯一允许的应用层同 requestID 重试是 D11 的 `snapshot_required` 且 `upstream_attempted=false`，且最多一次、目标实例不变。死连接、半发送、连接重建、Worker kill、错误 Content-Type 和超时测试必须断言假 Worker/假上游写入次数不超过一次，并保留 uncertain outcome。

### 3.3 替换 `CONTRACTS.md` 第 7 节

> ## 7. 大响应、describe 与审计
>
> 请求读取、上游解压、响应读取、base64、JSON 转换、HTTP 返回、MCP 工具内容和审计均有界。内部 base64 上限必须由原始字节上限推导：`4 * ceil(rawLimit/3) + 64 KiB`。请求、分块、无 Content-Length、Content-Length 谎报、gzip/deflate 膨胀、Header、JSON 转换和下游返回都必须先限制再分配，不得先 `ReadAll`。
>
> describe 请求 envelope 硬上限 2 MiB，响应 envelope 硬上限 4 MiB；单个 operation 序列化描述上限 1 MiB，超过返回 `description_too_large`。operation 清单必须 cursor 分页：默认每页 32、请求最大 64，但服务器以响应字节预算先止页，返回 `nextCursor`、`totalOperations` 和 `complete=true/false`；禁止一次性全量 describe。02/03 文档 API 复用同一分页和 4 MiB 上限，UI 懒加载。内部 execute 请求上限按 schema base64 + body base64 + Header + 256 KiB 推导；业务响应 envelope 按原始 8 MiB 推导；健康响应不超过 16 KiB，普通错误响应不超过 64 KiB。
>
> 04 提供唯一 `ToBoundedMCPContent` 与 `CaptureBoundedAudit`，02 MCP bridge 和 03 HTTP proxy 必须复用。MCP 任意业务 body 以 base64 内容返回并保留原始 Content-Type、字节长度、业务状态码和 `truncated=false`；仅允许附加有限预览。完整 8 MiB 原始 body 必须可返回，内部上限为 base64 公式 + 1 MiB；超出推导上限返回 `mcp_content_too_large` 平台错误，不得输出看似完整的截断 JSON/text。
>
> 审计默认仅元数据；启用正文时最多采样 32 KiB，保存原始采样字节数和 `truncated=true`。动态脱敏集合包含平台认证、Cookie、代理/连接、Worker 服务认证、当前发布快照与实例声明的一切业务认证 Header、`Sensitive=true` Header；未知名只记名称和长度。审计队列有界；正文必需模式下队列满须执行前拒绝，元数据模式下可丢弃正文但记录计数。审计失败不得造成租约永久占用。

## 4. D10 用户决策卡

**状态：未选择；G00 不得冻结。**

| 选项 | 内容 | 影响 | 默认推荐 | 关闭条件 |
| --- | --- | --- | --- | --- |
| A | 恢复原目标：公司已有 Namespace 中固定 Deployment/Service | 05 重写为 K8s/RBAC/挂载/滚动验收；仍需集群管理员或预置 Namespace 权限，与当前 Compose-only 设计冲突 | 不推荐，除非用户明确恢复集群交付要求 | 用户书面选择 A；00 替换 ROADMAP/ACCEPTANCE 的部署目标、权限、验收和“不创建 Namespace”约束，并撤销 Compose-only 结论 |
| B | 明确废弃原 Namespace/K8s 目标，仅交付两个 Compose 部署单元 | 满足无 Docker socket、无集群管理员、独立生命周期目标；公司 Kubernetes 部署成为非目标，需清晰迁移说明 | 推荐，但仅在用户明确接受范围变更后生效 | 用户书面选择 B；00 在 ROADMAP/ACCEPTANCE 记录原需求被显式废弃及日期，保留 Compose-only 替代验收，不再引用 K8s 交付 |

### A 选择后的替换片段

> OpenAPI Workers 交付到用户指定且已存在的 Kubernetes Namespace。只创建固定 Deployment/Service 和最小 Role/RoleBinding，不创建 Namespace、ClusterRole、ClusterRoleBinding或逐接口工作负载；集群管理员仅用于一次性 namespace/角色准备。验收必须在该 Namespace 中完成两 Obot/两 Worker、无 Docker socket、最小 RBAC、滚动升级、mTLS 和全局额度测试。Compose 栈仅作为本地开发/验收替代，不冒充生产交付。

### B 选择后的替换片段

> 原公司 Namespace/Kubernetes 交付目标已由用户于 G00 决策日明确废弃。本项目生产交付物仅为一个 Obot Compose 栈和一个 OpenAPI Workers Compose 栈，通过显式外部共享网络与 mTLS 通信；不创建 Kubernetes Namespace、RBAC、Deployment、Service，不要求集群管理员，不挂载 Docker socket。本地和部署验收均以两个 Compose project 为准；Kubernetes 只保留为 Obot 既有其他能力，不属于 OpenAPI Workers 目标。

## 5. 逐簇修订卡

### D01 — Worker lease fence

- **建议**：采用 typed Worker lease 与 Obot deadline 双防线；release token 永不下发。字段见第 3.1 节。
- **备选**：只传 opaque context 并让 Obot 提前截止。实现更简单，但 Worker 无法自查近过期/时钟偏差，F04 stale-start 性质缺少协议边界。
- **影响矩阵**：01 `workers/openapi/openapi_worker/{protocol,app,executor,errors}` 增字段与 DNS/上游前置检查；02 `pkg/openapiworker/{types,client,executor}` 序列化并收缩 deadline；04 lease admission 生成 DB 时间、fence 和 release token hash；05 验收采集三时钟偏差。
- **测试**：缺失字段、过期、剩余等于/小于 margin、`dispatchedAt>=fence`、本地偏差超限、HTTP deadline 晚于 fence、DNS 前失败；断言 DNS 和假上游均为 0。
- **关闭条件**：00 套用第 3.1 节；后续 P01/P02/P04 PLAN、F04 模型名词和 skill 同步，新增上述负例。设计级关闭只证明契约一致，不代表实现通过。

### D05 — BudgetSourceID canonicalization 与 legacy

- **建议**：结构化 canonical source tuple、全 hash 存储、快照唯一引用 fallback、缺失多义显式拒绝；固定模式失败不隐藏旧后端。
- **备选**：继续 raw `sourceID::entryKey` 便于排障，但会因别名拆额度且泄漏内部 URL；自动 component/sourceDigest fallback 会错误合并语义不同来源。
- **影响矩阵**：02 `resolver/types` 输出 `BudgetSourceID` hash 与错误；04 migration/state/query 只存 hash；03 只消费 resolver 结果；05 用两个 vMCP/component 与 legacy snapshot 验收。
- **测试**：host 大小写/默认端口/重复斜杠/dot-segment、entryKey NFC/空值、两个发布共享一行、不同 source 不合并、legacy reference 可推导/缺失/多义、日志无 URL。
- **关闭条件**：契约片段套用后，00 复核 hash 输入、fallback 优先级和拒绝路径唯一；实现期 PostgreSQL 测试按矩阵通过。

### D06 — 认证资格、body 与 Acquire owner

- **建议**：02 admission coordinator 唯一持有 04 Manager lease；03/MCP 只给 BodyProvider。资格、credential eligibility 与声明长度前置；chunked 实际 body 在租约内流式读取。
- **备选**：03 直接 Acquire 会让 MCP 与 HTTP 两套 owner；02 每入口分别 Acquire 易双重占额；Worker Acquire/Release 扩大信任面并要求下发 token。
- **影响矩阵**：02 executor/resolver/bridge 为主；03 `apiproxy` 不 import 04 Manager，只提供 bounded stream；04 Manager owner token API；05 慢 body E2E。
- **测试**：错误 Key、无实例、无组件、禁用 operation、缺凭证、声明超限均在入队前拒绝且无队列行；慢/chunked/谎报长度在租约内 413 或取消；02/03/Worker 不出现第二次 Acquire。
- **关闭条件**：接口编译与 HTTP/MCP 回归证明唯一 owner 与顺序；契约错误码不含两义。

### D09 — typed outcome 与 reclaim

- **建议**：采用第 6.3 状态矩阵；立即释放只依赖 `upstream_attempted=false` 或完整 business result 证明。
- **备选**：所有失败等 grace 最安全但浪费名额；把 504/cancel 当确认停止会恢复 F04 已证反例。
- **影响矩阵**：01 Worker outcome；02 client/admission 映射；03 外部错误；04 request state/reclaimer；05 两 Obot/两 Worker 故障拓扑。
- **测试**：等待取消、派发前取消、两次 snapshot miss、业务 400/500、deadline 后 sent、客户端断开、Worker kill、分区、响应超限、Release DB 失败、Obot crash；grace 前峰值不超限，grace 后计数恢复。
- **关闭条件**：00 确认矩阵覆盖全部原始分支；后续 F04 outcome 名词同步并复跑，E2E 按矩阵取证。

### D10 — Namespace/Compose 目标

- **建议**：未选择。用户必须显式选 A 或 B；第 4 节两段文本均为条件可用，不得同时进入契约。
- **备选**：无默认选择；静默采用任一侧都违反需求变更可审计要求。
- **影响矩阵**：00 ROADMAP/CONTRACTS/ACCEPTANCE；05 全部部署写集与验收；02 仅受运行配置影响；01/03/04 主体不变。
- **测试**：A 需要 real Namespace/RBAC/Deployment/Service 验收；B 需要 real Compose 双栈、独立生命周期、无 socket/集群依赖验收。均不得用静态 config 冒充。
- **关闭条件**：用户明确选择且 00 记录范围废弃/替代；对应验收全部列为实施门槛。

### D11 — 双 Worker 快照恢复

- **建议**：命名端点 + `executionDigest` rendezourse 粘路由 + cold hint 主动携带快照 + miss 后同实例一次补送 + readiness epoch 触发双实例预热。
- **备选**：随机 alias 重试已被 R05-02 证伪；Worker 回拉快照需要反向内部 API 和额外信任；每次 execute 恒带快照简单但放大常态带宽。
- **影响矩阵**：02 client 配置从单 URL 改命名端点列表并维护 warm hint；01 execute/describe snapshot envelope 与 `workerInstanceID`；05 Compose 暴露命名服务；04 不改变预算键。
- **测试**：两 Obot 随机发布多 digest、两 Worker 重启、单 Worker 缓存淘汰、目标 miss 后不可用、预热失败；断言同 requestID 补送目标不变、补送前上游 0、最终两 Worker 均可恢复。
- **关闭条件**：00 套用第 6.6 节；P01/P02/P05 后续 PLAN 移除随机 alias 执行假设，05 E2E 通过恢复矩阵。

### D12 — 多层公平与等待取消

- **建议**：三层名额同一事务、runnable operation 最久未放行优先、waiting 条件取消、runtime state 只保留活跃操作。
- **备选**：真实 PostgreSQL 测试单独兜底可以补充锁/事务，但不能关闭设计级计数和公平状态歧义；池内独立队列会重新引入队首阻塞。
- **影响矩阵**：04 `manager/admission/postgres/cleanup` 与状态表；02 只消费等待/取消错误；05 两 Obot/两 Worker 与多 operation 验收。
- **测试**：两个 runnable operation 轮转、慢接口满载不阻塞快接口、三层上限同时打满、Admit/CancelWaiting 竞态、配置下调、计数重算、锁序死锁负例。
- **关闭条件**：契约片段冻结后，P04 必须新增小型 multi-operation/multi-scope 模型或把该矩阵列为强制 PostgreSQL gate 并同步 skill 边界；实现前不得宣称 F04 已覆盖。

### D13 — Worker egress allowlist

- **建议**：默认拒私网；精确域名+端口+成对 CIDR 文件化 allowlist；全部 DNS 答案落入 CIDR 才放行。
- **备选**：测试时放开整个私网便于联调，但无法证明没有引入 SSRF/rebinding 通道；仅域名 allowlist 可被恶意解析到任意私网 IP。
- **影响矩阵**：01 `httpclient/executor` 与配置；05 acceptance override、Compose secret/config 与负例；02 不解析业务上游；03 不控制 Worker egress。
- **测试**：允许 fake business 成功；loopback、未列私网、混合公私答案、错误端口、域名漂移、策略 epoch 变化均连接前拒绝且无凭证发出。
- **关闭条件**：00 套用第 6.8 节，05 静态配置与真实连接正负例均通过。

### D22 — operation state 与 admission 扫描上界

- **建议**：runtime operation state 生命周期绑定 active/waiting，空后删除；terminal request 双上限；配置和查询分页。
- **备选**：仅加 TTL 可控制最老数据但 pump 仍可能扫描大量冷行；全动态聚合可减少行数但并发锁和公平更新更复杂。
- **影响矩阵**：04 migration/admission/cleanup/query；05 大量历史/冷操作压测；管理 API/UI 只显示 hash 与分页。
- **测试**：1152 默认运行行上界、空 operation 原子删除、4096 配置上限、50k/15m 终态清理、pump 扫描不超过队列上限、热公平不受清理影响。
- **关闭条件**：表结构、查询计划与 PostgreSQL 测试均显示扫描/行数/内存有硬上界；无全表管理查询。

### D23 — describe 与内部 envelope 上限

- **建议**：cursor 分页 + 4 MiB describe envelope + 1 MiB 单 operation + 2 MiB describe 请求；execute/health/error 上限分别推导。
- **备选**：把 describe 上限提高到 16 MiB 会掩盖编译计划与序列化差异，并迫使 02/03 分配更大内存；一次性清单对 512 operations 不可扩展。
- **影响矩阵**：01 `protocol/app/registry` 流式有界序列化；02 client/docs API；03 文档 API/UI 分页；04 LimitWorkerEnvelope；05 边界 schema 验收。
- **测试**：最大合法页、单 operation 1 MiB+1、512 operations 全 cursor 遍历、请求/响应 envelope +1、03 docs +1、ASGI 序列化前后内存、超限不发业务上游。
- **关闭条件**：01/02/03 读取同一常量与测试向量；无无全量 describe 调用。

### D24 — MCP 转换与有界审计

- **建议**：04 公共 `ToBoundedMCPContent` 与 `CaptureBoundedAudit`；MCP 任意 body 用 base64 完整保留，审计动态脱敏并显式截断。
- **备选**：按文本/JSON 原样转换会遇到转义放大；静默截断会伪装完整业务响应；各入口独立实现会造成 HTTP/MCP 上限和脱敏不一致。
- **影响矩阵**：04 boundedio/audit；02 MCP bridge；03 HTTP audit；01 Worker 内部 envelope；05 对同一 8 MiB/超限响应跑双入口。
- **测试**：binary、UTF-8、JSON 含大量转义、8 MiB 合法、8 MiB+1、Header 超限；HTTP/MCP 输出等价元数据；审计 `Authorization/authtoken/custom` 均脱敏且产物无 token。
- **关闭条件**：02/03 编译依赖公共组件，双入口同一测试矩阵通过，审计失败不泄漏租约。

### D25 — transport no-retry

- **建议**：SingleAttemptTransport + 无 GetBody + 无 redirect + requestID uncertain 持久拒绝；仅安全 snapshot miss 允许一次应用层补送。
- **备选**：依赖 Go 默认非幂等 body 不重放不足以给出可验收契约；Worker 持久 requestID 表会误导 exactly-once 并引入新状态。
- **影响矩阵**：02 `client/transport`；01 requestID/fence/outcome；04 uncertain request 状态；05 死连接/半发送/重启注入。
- **测试**：复用连接断开、请求半写入、Worker kill、重定向、连接重建、第二次自动补送；假 Worker 写入和假上游 handler 均不超过一次，审计为 uncertain。
- **关闭条件**：transport 配置与 `httptrace`/自定义 RoundTripper 证据显示一次 attempt；契约明确外部重试不承诺业务 exactly-once。

### D26 — DNS pinning

- **建议**：全答案策略校验后 pin IP，TLS/SNI/Host 保留原域名，同一 attempt 不重连，连接池记录 policy epoch。
- **备选**：普通 `httpx` transport 配 allowlist 仍有二次解析 TOCTOU；只做保存时 URL 校验不覆盖运行时 rebinding。
- **影响矩阵**：01 `httpclient/executor`；05 DNS/test transport 注入；02/03 不接触上游；04 无需变化但依赖 upstream proof。
- **测试**：第一次公网第二次私网、混合答案、TTL 过期、连接重建、IPv6/IPv4、自签/错误 SAN、redirect、pinned 连接复用；凭证只到达允许地址。
- **关闭条件**：实现显示 resolver→pinned connection 是同一 attempt 的不可绕过路径，全部负例连接前失败。

### D27 — Describe/冷编译与 Acquire 顺序

- **建议**：本地 canonical manifest 匹配；describe 只作授权文档/预热并独立限流；execute 先 Acquire，冷编译和补送在租约内。
- **备选**：执行前 Worker describe 会让未入队请求消耗编译资源；把编译全部放 Acquire 前会拉长队列外资源占用并破坏统一预算入口。
- **影响矩阵**：02 resolver/executor/client；01 describe/execute 编译预算；03不再依赖请求路径 Worker describe；04 control-plane 与 execution 预算分离；05 慢编译/队满顺序断言。
- **测试**：无资格 Describe 不触 Worker；慢编译受 2 并发/16 队列；execute 队满不调 Worker；Acquire 后冷编译 miss/补送/compile busy 均不触上游；文档 503 不影响已发布 URL。
- **关闭条件**：00 套用第 6.7 节；P02/P03/P04 后续计划与接口顺序一致，日志/trace 可证明调用次序。

## 6. 集中影响矩阵

| Feature | 必改接口/文件 | 必补测试 |
| --- | --- | --- |
| 01 | `workers/openapi/openapi_worker/{protocol,app,executor,httpclient,registry,errors}` | typed lease、snapshot recovery、no-retry、DNS pin、describe 分页、bounded MCP envelope |
| 02 | `pkg/openapiworker/{types,client,executor,resolver,bridge}`、admission coordinator | 唯一 Acquire owner、credential eligibility、BodyProvider、typed outcome、命名 Worker 路由、SingleAttemptTransport |
| 03 | `pkg/api/handlers/apiproxy/**`、audit/docs 接线 | 不直接 Acquire、声明大小前置、chunked body、业务结果/outcome 映射、分页文档、公共审计 |
| 04 | `pkg/openapi/executionbudget/**`、gateway migration/types | source hash、三层公平、waiting cancel、runtime state TTL、release/reclaim、boundedio/audit |
| 05 | `deploy/compose/**`、`deploy/acceptance/**`、E2E | 双命名 Worker 恢复、egress 正负例、两 Obot/两 Worker grace/公平/上限、no-retry、时钟偏差、D10 对应部署验收 |
| 00 | `CONTRACTS.md`、`ACCEPTANCE.md`、ROADMAP 决策记录 | G00 独立复核每簇 close condition；D10 用户决策记录 |

## 7. 剩余风险

- F04 当前模型仍不覆盖 PostgreSQL 锁、多 operation 公平、HTTP 取消、transport、DNS 和大小边界；D12/D22/D25/D26 的关闭依赖后续模型或实现测试，不能用现有 pass 外推。
- 等价 source URL 的语义别名无法全部由句法规范化识别；本方案只合并明确 RFC/部署规范等价，不同业务路径仍可能被拆分，需以真实目录数据验证。
- 1s fencing/reclaim 只覆盖小于 margin 的受控时钟偏差与调度误差，不承诺任意时钟攻击或宿主冻结下的硬实时安全。
- requestID 只阻止平台自动重试，不提供业务 exactly-once；调用方外部重试仍可能触发业务副作用。
- D10 未决策前，所有部署写集和验收范围均不能冻结；即使用户选择 B，也必须显式记录原 Namespace 目标废弃。
