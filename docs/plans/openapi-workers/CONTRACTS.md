# 共享契约

版本：FROZEN-1.0。日期：2026-11-09。阶段：实施许可已开启。

本契约由 DRAFT-0.2、R01—R05、C01/C02 契约修订和用户 2026-11-09 最终决策收敛。C01/C02 的推荐方案是冻结附录；其中 D10 的 Compose-only 备选被用户最终决策覆盖，改为无状态 Worker 的 Kubernetes Deployment 交付。除该覆盖外，实施不得降低 C01/C02 中的认证、身份、路由、预算、租约、恢复和有界性要求。

## 1. 信任、身份和产品边界

- 外部应用使用 Obot 平台身份和 Key。用户 ID 只由平台认证结果得出，不能从调用方自填 Header/body 指定。
- 复用现有 vMCP、组件快照、Profiles、用户实例选择与 Key 授权范围；这些是平台资源准入，不是业务角色或数据权限引擎。
- 用户业务 token 来自现有加密凭证存储中该用户对应的上下文。个人业务认证不得回退到其他用户或管理员固定 token。
- 业务系统执行权限和数据权限拦截，平台不解析业务 JWT 推导权限。
- 业务授权交换尚未确认闭环；02 定义适配边界与失败语义，05 用假业务系统和合成凭证验收，真实企业交换单列缺口。
- Worker 仅接受受信 Obot 服务调用；服务认证与业务认证完全分开。生产通信使用 TLS，不能把“集群内”当作可以随意暴露业务凭证的依据。

## 2. 快照与 operation 身份

复用已持久化的 OpenAPI schema 与 vMCP 组件快照，不在执行时重新抓取 Source URL。

- `schemaDigest`：对现有规范化 schema 字节做 SHA-256，不跨语言重新序列化后计算。
- `executionDigest`：包含 schemaDigest、有效 BaseURL、非敏感参数/认证 Header 定义、operation 清单、编译协议版本等执行因素的版本摘要。
- 凭证值、平台 Key、用户身份和临时 Header 值均不属于共享配置摘要或缓存内容。
- `operationKey`：大写 HTTP 方法、一个空格、schema 路径模板。原始 operationId 保留作元数据，不作为唯一定位依据。
- 稳定原始工具名、operationKey 和显示别名有显式映射；兼容已有工具授权身份，不通过重命名绕过授权。
- 参数位置限于 OpenAPI path/query/body/header 必需绑定，不能引入脚本变换、业务字段改名或复杂工作流。
- 原始工具名、显示名称、描述、参数说明、启用清单分别维护；别名不能改变 HTTP operation 或权限身份。

已发布 vMCP 组件使用其快照；目录或说明更新不静默替换应用定义。显式更新发布生成新版本；运行时平台撤权和当前用户资格仍实时生效。

## 3. Worker 内部协议

候选内部路径：

- `POST /internal/openapi/v1/describe`
- `POST /internal/openapi/v1/execute`
- `GET /healthz`
- `GET /readyz`

内部业务接口只返回有界 JSON。describe 不接收用户业务凭证、不发业务请求；返回 operation 到工具的映射及参数 schema。

execute 最小上下文包括协议版本、requestID、executionDigest、operationKey、输入模式、绝对截止时间、预算/租约上下文，以及逐请求临时认证 Header。HTTP 输入保留原始方法、匹配后的路径、RawQuery、允许的端到端 Header 和原始 body 字节；JSON 内字节采用 base64。工具输入使用同一 operation 定义进行位置绑定。

执行结果包含原始业务状态码、多值 Header、原始 body 字节及关联 requestID。业务 4xx/5xx 属于执行结果；平台、协议、预算和传输失败使用独立错误，不把业务失败统一包装为平台 500。

Worker 缓存未命中时，在任何上游业务调用前返回 `snapshot_required`。Obot 最多携带对应不可变快照补送一次；除此之外不得对不确定是否已执行的请求自动重试。这个行为必须有证明未触发上游的测试。

共享客户端只复用连接池，不保存用户默认 Header、业务 Cookie 或用户 token。认证 Header 名称与前缀按配置传入；示例 Authorization/authtoken 不默认强加 Bearer，也不宣称已实测两个 Header 都必需。

Worker 上游地址来自已验证的快照，不能由应用请求指定任意 host。保留现有 HTTPS、地址校验和网络访问限制，运行时也需执行，不仅在保存时检查。重定向不自动携带认证跟随。

## 4. 普通 HTTP 与 Agent MCP

应用出口固定候选为 `/api-proxy/{vmcp_id}/{component_id}/{path...}`，沿用已有 vMCP 和组件，不建立 Endpoint 顶层对象。

解析当前用户对应实例及已发布 operation 白名单，保持方法、路径编码、重复参数、body 与业务响应语义。不使用其他组件的同名工具授权当前组件，不允许路径匹配绕过白名单。

平台 Authorization/Cookie/Key 和 Worker 服务认证不能穿透至业务 API；只有声明的业务凭证 Header 由凭证解析器注入。用户自填认证 Header 不得覆盖平台绑定的业务身份。

HTTP 代理与 MCP 发现/调用均复用同一执行定义和预算入口。MCP 出口保持现有 vMCP 能力，使用 JSON-only、stateless；不能仅保留 HTTP 而隐藏既有 MCP 功能。

## 5. 无 SSE 与同步时限

OpenAPI 新执行链及其关联的 MCP/vMCP 请求禁用：

- 旧 HTTP+SSE、SSE GET 和消息端点。
- POST 请求的事件流响应。
- 客户端独立 SSE、健康探测/连接失败后的 SSE 自动回退。
- 业务上游 `text/event-stream` 持续响应。

对明确要求只接受事件流的请求给出不支持响应；常见 MCP Accept 同时列出 JSON 和事件流时仍只选择 JSON。不能只删除 Accept 中的文本却保留后端流。

健康检查使用专用有界 HTTP 端点。普通业务接口默认同步请求—响应，入站时建立截止时间，排队、冷加载、执行和返回均不得突破总预算，不默认返回 jobId。

## 6. 预算、队列和回收

使用现有 PostgreSQL 作为跨副本状态基础，不新增 Redis 或调度集群。04 核查现有迁移/事务方式并给出具体状态模型。SQLite 只能用于显式单进程开发，不可冒充多副本全局限额。

预算键由目录来源稳定身份与 operationKey 构成，排除用户、Worker 副本和快照版本；同一目录来源通过多个 vMCP 发布时仍共享该接口额度。

初始候选值：

| 项目 | 默认 |
| --- | --- |
| 平台总在途 | 32 |
| 默认池总在途 | 16 |
| 专属受控池总在途 | 16 |
| 未配专属限制的接口并发 | 4 |
| 队列总票据数 | 64 |
| 单接口队列数 | 8 |
| 最长队列等待 | 2 秒 |
| 单调用总截止时间 | 30 秒 |
| 原始请求体 | 1 MiB |
| 原始业务响应 | 8 MiB |
| schema 单份 | 沿用导入器 1 MiB |
| 缓存份数 / 累计 schema 字节 | 32 / 16 MiB |

这些值供评审，是可配置的有限初值，不是吞吐或海量容量承诺。专属接口上限受池预算约束，不得变相无限制。

接口独立计数与有限排队，按可运行接口公平放行，避免一个接口队首阻塞所有接口。全局、池与接口名额在一个短状态转换中确认，不跨上游请求持有数据库事务或连接。

租约只绑定一次调用，有截止时间、幂等释放和过期回收。取消须传递到 Worker 与上游连接；断开或 Worker 退出后，不在无法证明执行停止时立即释放全部额度。超时回收不能允许旧执行继续使用过期许可发起业务请求。

默认预算存储不可用时关闭执行并返回明确错误，不回退为各副本独立无限制。运行中下调限制不取消既有业务副作用，阻止新请求直至回到限额内。

## 7. 大响应与审计

请求读取、上游解压、响应读取、base64、JSON 转换、HTTP 返回和审计均有界。内部编码上限必须由原始字节上限推导，不能用同一个数字导致合法结果经 base64 后被误拒；也不能只检查 Content-Length 而漏掉分块/压缩响应。

审计默认只记元数据。启用正文时最多 32 KiB，使用明确截断标记，不把截断 JSON 当成完整业务对象。认证 Header 脱敏包含声明的自定义名称，例如 authtoken。

缓存同时受份数、schema 字节及编译展开限制保护；相同摘要的并发加载去重，活跃引用不能被关闭，淘汰关闭无凭证的客户端资源。冷编译不得沿用主线程 SIGALRM 请求处理方式，不得无限创建线程或进程。

## 8. 失败与 HTTP 状态

候选统一错误：`code`、安全的 `message`、`requestID`、`retryable`；普通应用响应通过明确平台错误标识区分上游原始响应。

| 情况 | 平台状态 |
| --- | --- |
| 无效平台身份 | 401 |
| 平台 Key / vMCP / operation 无资格 | 403 或现有隐藏资源语义 |
| 缺少用户业务授权 | 409，明确需要业务授权 |
| 无效方法、路径或参数 | 400 / 404 / 405，按匹配阶段确定 |
| 请求体超限 | 413 |
| 队满或排队等待耗尽 | 429，带 Retry-After |
| Worker / 预算状态不可用 | 503 |
| 截止时间耗尽 | 504 |
| 上游响应过大或协议不支持 | 502 |
| 上游业务 4xx/5xx | 保留原始状态及业务响应 |

错误不能回显凭证、完整内部地址或原始敏感输入。连接取消时仍记录有界审计和回收结果。

## 9. 运行与部署

Obot 与共享 Workers 均部署为 Kubernetes 无状态工作负载：Obot 使用 `Deployment`，OpenAPI Worker 使用可水平扩容的 `Deployment`。Worker Pod 不保存业务状态、租约、用户凭证、operation 编译缓存或请求去重状态；跨副本状态只进入现有 PostgreSQL。服务启动不按用户、schema 或接口创建额外工作负载。

部署目标为用户已存在的 Kubernetes Namespace。项目只创建或更新固定 `Deployment`、`Service`、`Secret`、`ConfigMap` 和必要的 namespaced 最小权限对象；不创建 Namespace、ClusterRole、ClusterRoleBinding，不要求集群管理员权限，不挂载 Docker socket，不为用户 token 创建逐用户 Secret。Obot 与 Worker 可各自扩缩容；正确性不得依赖 Pod 亲和、单实例内存状态或粘性会话。

数据库决策：Obot 原始实现已集成 PostgreSQL，因此继续使用现有 PostgreSQL，不改 MySQL、不新增 Redis 或独立调度集群。预算、队列、租约、审计、幂等请求状态和恢复状态均为 PostgreSQL 状态。Worker 崩溃、重建、扩缩容后不得依赖本地内存恢复语义。

生产 Obot→Worker 内部通信使用 TLS + mTLS；bearer 只能作为显式第二因子，详见 C01。证书、数据库连接和服务认证材料通过 Kubernetes Secret 挂载或外部受信注入，不写入镜像、环境变量明文、仓库或普通日志。滚动升级必须保持至少一个 Obot 副本和一个 Worker 副本可用，并验证 readiness、graceful shutdown、旧 Worker fencing 与全局预算不超发。

本地验收优先使用隔离 Kubernetes 测试环境或等价控制平面；不得操作用户现有服务或生产集群。部署验收必须覆盖多 Obot / 多 Worker、重启、扩缩容、快照恢复、认证负例和资源边界。未实际完成部署与健康/流量验收，不得写部署成功。

## 10. 冻结记录

- 状态：已冻结，实施许可开启。
- 冻结版本 / 摘要：`FROZEN-1.0`。C01 负责 Principal/Executor、mTLS、canonical manifest、raw HTTP、路径/Header/响应和跨语言身份边界；C02 负责 typed lease、PostgreSQL 全局预算、公平排队、恢复、DNS/no-retry、有界 envelope 和审计。用户最终覆盖 D10：无状态 Worker + 已有 Namespace 的 Kubernetes Deployment，保留现有 PostgreSQL，不切换 MySQL。
- 输入产物：`evidence/REVIEW-ISSUE-MATRIX.md`、R01—R05 `REVIEW.md` 与完成证据、C01/C02 `CONTRACT-REVISION.md`、五份 feature PLAN 与 formal evidence。
- 阻断问题处理：C01/C02 覆盖全部 30 个问题簇。P1 协议冲突按冻结附录作为实现验收条件关闭；实现中必须以测试和真实流程验证，不能用计划或模型通过替代。
- 最终写集：I01 独占 Python Worker 与测试；I02 独占 Go 协议/Resolver/Executor/接入组装与测试；I04 独占预算/租约/迁移/有界 IO；I03 等 I01/I02 后实施 HTTP 出口；I05 最后负责 K8s 部署、多副本集成和全链路验收。跨写集变更必须由 00 协调。
- 实施调度：立即同时启动 I01/I02，活跃实施 Worker 不超过两个；I03 依赖 I01/I02；I05 依赖 I01—I04。全程不自行 commit/push、不部署生产、不访问历史 JWT 或真实业务系统。
