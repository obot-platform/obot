<<<<<<< HEAD
# C01 身份、Resolver 与 HTTP 边界契约修订

日期：2026-11-09。修订范围：D02、D03、D04、D07、D08、D14、D15、D16、D17、D18、D19、D20、D21、D28。本文只修订契约，不实施业务代码，不修改 formal 产物，不声明任何实现或 E2E 已通过。

## 0. 总结论

唯一建议是：生产内部通信强制 TLS + mTLS，bearer 只能作为显式第二因子；Obot 侧 02 成为唯一 Principal-aware Resolver/Executor；显式发布时生成单源 canonical operation manifest，Go 生成权威字节、Python 只验证和消费；普通 HTTP 使用 raw-prefix 专用路由、专用调用资格和业务响应 writer；Header、路径匹配、参数校验、响应协议子集使用同一跨语言向量；loopback 身份不能直接代表完整平台身份，必须由 Obot 用权威身份源补全；F01/F02 现有模型缺口转为冻结契约中的强制实现前测试门槛。

该选择牺牲少量协议冗余和 HTTP 通用代理能力，换取可互操作、可测试、身份不可自填和业务响应不被平台污染。所有推荐方案均可由 00 按“替换/新增片段 + 关闭条件”独立复核，不依赖责任人后续再定义。

## 1. 逐簇决策

### D02 — Worker 服务认证与 mTLS/bearer 层次

**原始 finding：** R01-02, R02-04, R04-05, R05-03。

**唯一方案：** 生产 Obot→Worker 必须同时完成 TLS 服务端证书校验和 Obot 客户端证书校验。bearer 不是 mTLS 替代品；只在两侧都显式配置同名 secret 时作为第二因子，且使用内部专用 Header，避免与业务 `Authorization` 混淆。本地开发/测试可以用短期 CA + mTLS；仅允许测试进程显式配置 localhost HTTP，生产配置出现 HTTP、缺失 client CA、缺失 client cert 或缺失 server CA 校验时启动失败。

**备选取舍：** bearer + TLS 更容易接入既有网关，但无法证明调用方是 Obot 服务，且会把服务认证和可被复制的企业网关过渡策略耦合；明文“内网”直接违反业务 token 信任边界。mTLS-only 已满足最低要求，叠加 bearer 可满足已有网关的双因子要求，因此保留为可选而不是必选。

**可套用契约片段：**

```markdown
### Worker 服务认证

- 生产 `/internal/openapi/v1/*` 通信必须使用 HTTPS，并强制双向 TLS：
  - Obot 必须加载 Worker CA、Obot client certificate/private key，并验证 Worker 证书链与 SAN；
  - Worker 必须加载 Obot client CA、Worker server certificate/private key，并拒绝无 client certificate、错误 CA、错误身份、过期证书和不匹配 SAN；
  - TLS 至少使用 TLS 1.2，默认 TLS 1.3；不因部署网络位于内网、Compose 网络或企业网关后而豁免。
- bearer 仅可作为显式第二因子。两侧必须同时配置同一非空 secret；任何一侧未配置时不得发送/要求该 Header。该 Header 固定为 `X-Obot-Worker-Authentication: Bearer <token>`，不是业务 `Authorization`，不进入 executionDigest、snapshot、业务上游、审计正文或普通日志。
- 生产启动前必须完成配置互斥校验：HTTP、缺失任一 mTLS 材料、空 client CA、禁用证书校验、bearer 单侧配置均为致命配置错误。本地 HTTP 只允许测试进程对 `127.0.0.1` 或 `localhost` 显式开启，并不得出现在生产示例默认值。
- 证书轮换按“先信任新旧 CA/身份，逐实例刷新并验证健康，再撤销旧信任”执行；任何时刻至少一个身份可用。Compose secrets/config 只挂载文件路径，不把私钥或 token 写入环境、仓库或日志。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | Worker ASGI/TLS 配置、`/healthz`、`/readyz` | 错 CA、错 client、无 client、过期证书、SAN 不匹配、bearer 单侧缺失全部拒绝；合法 mTLS 通过 |
| 02 | Worker client 配置与 transport | 生产 HTTP/缺材料启动失败；正确 mTLS 通过；optional bearer 只作为第二因子且不泄漏 |
| 05 | Compose secrets、预检、轮换脚本 | 静态断言无明文 secret；真实握手和负例；新旧 CA 轮换保持可用 |

**Close condition：** 三方配置名、必选材料、可选 Header、TLS 最低版本、启动失败行为和轮换顺序完全一致；本地/Compose 证据包含一次真实 mTLS 成功与至少四类认证负例，且没有任何降级 HTTP 成功。

### D03 — Principal-aware Executor、raw 输入与 typed error

**原始 finding：** R01-03, R02-01, R03-02。

**唯一方案：** 02 独占 Resolver/Executor。`PrincipalContext` 只能从已通过平台认证的 request context 构造，类型不暴露可由调用方设置的 userID 字段；03 只提交 canonical reference、raw HTTP input 和调用上下文。`Describe` 返回按当前用户、实例、Profile、override 收窄后的 operation 清单；`Execute` 在 02 内重新执行同一解析并解密/注入业务凭证。业务结果与平台错误使用显式 `ResultKind` 和稳定错误码。

**备选取舍：** 让 03 复用现有 vMCP 授权代码会减少一个公共接口，但会把凭证解析和 operation 收窄复制到出口层，形成第二套授权实现；让 Worker 判断用户资格则缺少 Obot 数据库与 Profiles 上下文。因此唯一安全归属是 02。

**可套用契约片段：**

```markdown
### Principal-aware Executor 契约

`PrincipalContext` 是不可序列化的已认证主体句柄，只能由平台认证中间件或 MCP gateway 从 admitted principal/context 构造。它不得出现在 HTTP API、应用请求 body/Header、Worker 协议或调用方可写字段中。03 及其他入口不能构造、覆盖或伪造 userID、API Key scope、groups、instance、component 或 credential。

公共 Go 语义必须包含：

- `Describe(ctx, principal, reference) (ResolvedDescription, error)`；
- `Execute(ctx, principal, invocation) (ExecutionResult, error)`；
- `Invocation` 携带 `Reference`、`OperationKey`、`InputKind=tool|http`、raw/tool input、绝对 deadline 和 04 提供的不透明 lease metadata；
- `ResolvedDescription` 携带 canonical vMCP/component/instance identity、schema/execution digest、`BudgetSourceID`、当前用户已授权 operation 清单、Header 分类和参数描述；
- `HTTPInput` 保留 method、schema-relative raw path、raw query、允许的多值 Header、body media type 和原始 body 字节，不携带用户自填 userID、host、scheme或业务凭证值。

每次 `Describe` 和 `Execute` 都必须实时解析当前用户对应实例、legacy 发布快照、组件启用、原始工具授权、API Key vMCP scope 和业务凭证资格。`Execute` 在发送 Worker 前再次执行该解析；任何快照中的旧授权清单不能替代实时检查。

typed error 至少区分类别：`platform_unauthenticated`、`platform_forbidden`、`resource_hidden`、`operation_not_found`、`business_authorization_required`、`invalid_request`、`unsupported_media_type`、`request_too_large`、`budget_rejected`、`worker_unavailable`、`protocol_error`、`deadline_exceeded`、`upstream_protocol_unsupported`、`response_too_large`、`transport_uncertain`。上游业务 1xx 之外的最终 status/Header/body 属于 `business` result，不得映射为上述平台错误。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | `pkg/openapiworker/{resolver,executor,errors,types}.go` | 03 无法设置 userID；错误用户/实例、撤销组件/工具、缺凭证、隐藏资源和 raw input 不得触达上游 |
| 03 | `pkg/api/handlers/apiproxy/*` 与调用文档 | 只调用 Executor；管理可见但不可调用、错误 Key、跨实例、隐藏 operation 均拒绝 |
| 04 | `BudgetContext` 接线 | 只作为 04→Worker 的不透明执行元数据，不参与身份解析 |

**Close condition：** 02/03 编译使用同一接口；越权矩阵在进入 Worker/上游前全部失败；业务 403/404/429/5xx 保留为 business result；typed error 映射测试无双重归属。

### D04 — canonical manifest、Header 策略与跨语言 executionDigest

**原始 finding：** R02-02, R03-02, R04-04。

**唯一方案：** 显式发布时由 Obot 生成唯一 `CanonicalOperationManifest` 并持久化在组件发布快照中。它是 execution、describe、path matching 和参数绑定的权威语义源；Worker 不再从 schema 独立推导第二套 operation 清单来计算 digest。`executionDigest` 对 manifest 的 canonical bytes 与不可变执行 policy bytes 做定长长度前缀编码后 SHA-256；Go 生成权威字节，Python 对收到的同一字节重算并只消费 manifest。用户身份、requestID、凭证值、临时 Header、vMCP/component ID 和用户实例选择不参与 digest。

**备选取舍：** 让 Go/Python 各自从 schema 解析 operation 会保留两个实现，continue 存在冷启动 digest 漂移；让 Worker 首次返回 digest 会让 Obot 依赖运行时编译输出作为发布身份，滚动版本和双 Worker 冷 miss 更难收敛。单源 manifest 增加显式发布步骤，但能消除主要不兼容来源。

**可套用契约片段：**

````markdown
### Canonical operation manifest 与 executionDigest

显式发布必须生成并冻结 `CanonicalOperationManifest`。它包含：

- `manifestVersion=1`；
- `schemaDigest`，即保存的 normalized schema 原始字节的 SHA-256；
- 有效 BaseURL；
- 每个允许执行的 operation：`operationKey`、schema method/path template、`legacyToolName`、display metadata、参数绑定、媒体类型、JSON Schema 子集和启用状态；
- Header policy 定义，但不包含任何 Header 值；
- compiler semantics version、path matcher version、parameter binding version、protocol version。

manifest 采用 UTF-8、字段顺序固定、换行 LF、无 BOM、无 JSON 注释/尾随逗号的 canonical JSON 字节序列。`executionDigestV1` 输入为：

```text
domain="obot-openapi-worker-v1"
frame = len(domain)||domain
      || len(manifestBytes)||manifestBytes
      || len(policyBytes)||policyBytes
executionDigestV1 = hex(SHA-256(frame))
len 是 8-byte big-endian 字节数
```

`policyBytes` 只包含不可变共享执行 policy：Header 名、位置、required、prefix、style、是否 sensitive、是否允许端到端转发、URL/模板/参数语义版本。凭证值、用户身份、API Key、requestID、实例配置值和临时 Header 不得进入。用户实例只能为已声明 Header 提供值，不能新增 Header、改变 required/prefix/style、把非认证 Header 升级为业务认证或把管理员静态值作为个人业务认证 fallback。

Worker 收到 snapshot envelope 后必须验证 schemaDigest、manifest canonical bytes、executionDigest 和大小上限；任何不匹配返回 `snapshot_invalid`，不编译、不缓存、不触达上游。Worker 只按 manifest 编译和执行；原始 schema bytes 仅用于完整性验证与兼容诊断，不作为第二 operation 清单。
````

**跨语言向量：** 必须提交非敏感共享向量，至少覆盖：空 base、最小 GET、同 schema 不同 BaseURL、同 schema 不同 required/prefix/style/forwarding、operation 增删、path template 变化、同 schema 多 vMCP/component、非法 UTF-8/非 canonical JSON、错误 digest。Go 输出与 Python 验证必须逐字节一致；错误 digest 全部在编译/上游前拒绝。

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | 发布 manifest 生成、canonical serializer、digest | 向量生成、持久化不可变性、同来源多发布共享、旧 digest 不命中 |
| 01 | snapshot envelope、manifest validator/compiler | 错 digest、错 schemaDigest、非 canonical manifest、Header policy 漂移全部 `snapshot_invalid` |
| 03 | 文档/describe 消费 | 使用 manifest 的 operation/path/参数，不自行解析 schema |
| 05 | 验收 harness | 双 Worker 使用同一 manifest/digest，两份 schema 同时可用 |

**Close condition：** 冻结契约写入单源规则与向量目录；Go/Python 对全部向量输出相同 accept/reject 与 digest；任何 Python 重新序列化后得到的字节差异只导致明确拒绝，不会生成第二 digest。

### D07 — raw-prefix 路由、业务响应 writer 与公共写集

**原始 finding：** R01-05, R01-07, R02-05, R03-03。

**唯一方案：** `/api-proxy/` 在进入 `net/http.ServeMux` 前使用专用 raw-prefix branch，保留 `URL.RawPath/EscapedPath/RawQuery`、多值 Header 和 body 流，并设置固定 pattern。代理分支继续复用平台认证、限流和有界审计，但使用 dedicated business response writer，不经过默认 Header wrapper 或平台 Cookie replay。03 独占 server raw-prefix helper 与响应 writer 写集；02 的服务组装不得并行修改同一文件。

**备选取舍：** 自定义 `ServeMux` wildcard 最省代码，但会解码 `%2F`、清洗 `//`/`.`/`..` 并重定向，无法保留原始语义；为每个 schema 注册具体路由会造成路由表随发布变化并泄漏管理面细节。raw-prefix 分支是唯一能同时保留路径字节和固定外部路径的方式。

**可套用契约片段：**

```markdown
### `/api-proxy/` raw-prefix 与业务响应 writer

- APIServer 必须在进入通用 ServeMux 前识别 `/api-proxy/`，保留原始 escaped path、raw query、Header 多值顺序和请求 body 字节；不得触发 path clean、redirect、trailing-slash normalization 或 segment unescape 后再拼接。
- raw branch 解析出的路径固定记为 `pattern="/api-proxy/{vmcp_id}/{component_id}/{path...}"`，供 tracing、授权和审计使用。`vmcp_id` 只接受 canonical VMCP ID，`component_id` 只接受发布快照中的 immutable component ID；两者不接受实例 ID、legacy alias、显示名、通配符或空值。
- 平台认证、API Key scope、限流、deadline 和有界审计仍作用于该分支。业务响应必须使用 dedicated writer：写出上游最终 status、端到端 Header、多值 Header 顺序和 body 字节；不得默认补 `Content-Type`、`X-Content-Type-Options`、API no-cache、平台 session `Set-Cookie`或其他平台响应语义。
- 平台错误才使用平台 JSON envelope 与 `X-Obot-Error-Code`。认证器刷新产生的平台 Cookie 不得写入业务响应。`Content-Length` 按最终 body 重算；hop-by-hop、连接管理、代理、Worker 服务认证和平台保留 Header 在写回前剥离。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 03 | `pkg/api/server/server.go` raw helper、`pkg/api/handlers/apiproxy/*` writer、router | `%2F/%2E`、`//`、traversal、非法编码、空 path、query 顺序、HEAD、非 JSON、多值 Header、上游 Set-Cookie、平台 Cookie 刷新 |
| 02 | service 组装 | 只注入 Executor，不改动 raw branch；I02/I03 合并顺序由 00 记录 |
| 05 | E2E | 真实请求无 301/302，管理 API/UI/SCIM/`/mcp-connect` 响应不变 |

**Close condition：** raw path 回归和浏览器/E2E 证明业务 200/403/429/5xx、无 Content-Type、204/304、HEAD、多值 Header 与上游 Set-Cookie 均不被平台 writer 改写；平台错误才带错误 envelope。

### D08 — `/api-proxy` 专用调用资格链

**原始 finding：** R01-06。

**唯一方案：** 新增 `authorizeAPIProxy` 专用入口，不复用管理资源 `vmcp_id` 检查。入口先要求平台身份与 `GroupMCP`，解析 canonical vMCP 与当前用户连接，执行与 `/mcp-connect` 等价的 API Key `authorized_mcp_ids` scope；随后 02 resolver 实时校验 instance 归属/选择、组件、Profile、override 和 operation。资源不存在/无资格采用隐藏语义，不因管理可见性放行。

**备选取舍：** 复用 `vmcp_id` 管理检查实现最少，但它的读/管理可见性不等于当前用户调用资格，且可能在组件/operation 检查前放行并泄漏存在性；把所有授权下沉 Worker 则缺少 vMCP/API Key 数据。专用入口加 02 二次解析是唯一既保留平台语义又不复用管理权限的方案。

**可套用契约片段：**

```markdown
### API-proxy 调用资格

`/api-proxy/{vmcp_id}/{component_id}/{path...}` 必须使用专用资格链：

1. 平台认证得到 admitted principal；无效返回 401；
2. principal 必须具备 `GroupMCP`，API Key `authorized_mcp_ids` 必须匹配目标 canonical vMCP、其当前用户实例或 `*`；无 scope 返回隐藏/403 语义；
3. 解析当前用户对应 VMCP instance，不得采用管理端列表可见性、其他用户实例或调用方自选实例；
4. 02 resolver 校验 legacy 发布快照、immutable component、Profile、instance 组件选择、组件启用、ToolOverride 和原始工具名授权；
5. raw path/method 唯一匹配授权 operation 后才能进入预算和 Worker。

现有 `vmcp_id` 管理资源检查、read/manage visibility 和 catalog listing 权限不得作为以上任一步的替代。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 03 | API-proxy authz entry | 管理可见但不可调用、错误/窄 scope、canonical/instance/legacy alias、无 `GroupMCP`、错误用户实例 |
| 02 | resolver | 同一 operation 二次收窄；撤销组件/工具、跨组件同名工具、禁用 override、隐藏资源不触达上游 |
| 05 | E2E | 正确 Key 与实例可调用；错误目标 vMCP 在 Worker/假上游前失败 |

**Close condition：** 所有无资格负例的上游计数为 0；管理可见性测试明确不能放行；授权日志记录实际 principal、canonical target、instance、component 与 operation，不记录敏感值。

### D14 — 删除未定义 `ensure` 协议名词

**原始 finding：** R01-11。

**唯一方案：** 不新增 endpoint。协议只保留 `describe`、`execute`、`healthz`、`readyz`。Obot 侧可有内部 helper `EnsureSnapshotReady`，其唯一实现是对同一 immutable snapshot envelope 调 `describe`；缓存命中返回 ready，未命中返回 `snapshot_required` 或补送后的编译结果。`execute` 可在首次调用携带 snapshot envelope，但不称为 ensure，也不产生第三个 route。

**备选取舍：** 增加 `/ensure` 能把预热和描述分开，但会引入重复认证、补送和错误语义；保留当前 `describe/ensure/execute` 混称会导致客户端生成不一致。因此取消协议名词并保留 Go helper。

**可套用契约片段：**

```markdown
### Worker endpoint 与 snapshot readiness

Worker 只暴露 `POST /internal/openapi/v1/describe`、`POST /internal/openapi/v1/execute`、`GET /healthz`、`GET /readyz`。不存在 `/ensure` endpoint。

Obot 的 `EnsureSnapshotReady` 只是进程内 helper：它调用 `describe` 并可在该请求 envelope 中携带不可变 snapshot；命中返回 ready，未命中返回 `snapshot_required`，补送后仍由同一 `describe` 返回 ready 或 typed snapshot error。该 helper 不得改变 endpoint 列表、requestID 语义或 execute 一次上游尝试规则。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | HTTP route 表 | 只注册四个 endpoint；`/ensure` 404 |
| 02 | client/helper/生成测试 | helper 只发 describe；execute 不因 helper 产生第二次上游尝试 |

**Close condition：** 冻结契约、02 类型和生成协议测试中的 route 名单完全一致；文档不再出现 `ensure` endpoint 或 ambiguous helper 命名。

### D15 — F01 补送 digest 相等性强制测试

**原始 finding：** R01-12。

**唯一方案：** 不把现有 F01 抽象模型扩大为字节级解析器；把 D04 的 canonical bytes/digest 相等性列为 I01 前置阻断测试。Worker 必须在进入 compile/cache/fence/upstream 之前验证 supplied canonical manifest bytes、schemaDigest 与请求 `executionDigest` 一致。错误 digest 是 `snapshot_invalid`，不可作为 valid snapshot，也不得消耗一次补送后进入执行。

**备选取舍：** 扩展 TLA+ 为字节/digest 模型会重复描述 JSON/SHA 细节，模型复杂度增加却仍不能替代实现测试；保持完全抽象则会掩盖序列化 bug。选择“模型继续证明一次补送/fence，字节相等性由强制协议向量验证”。

**可套用契约片段：**

```markdown
### Snapshot digest equality gate

`snapshot_required` 后的一次补送必须携带与 request binding 相同的 canonical manifest bytes、schema bytes 和 `executionDigest`。Worker 在读取 envelope 后、编译前逐项重算并比较；不等、长度前缀不完整、manifest 非 canonical 或 schemaDigest 不匹配均返回 `snapshot_invalid`，并且 `upstream_attempted=false`。

该检查是 F01 模型的显式代码级边界：F01 证明补送次数与一次执行 fence，不证明序列化/SHA 字节。I01 不得在 digest equality 向量失败时结算；错误 digest 不得推进到 compiling/ready/executing。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | snapshot validator/compiler | correct vector ready；错 executionDigest、错 schemaDigest、manifest 字节变化、非 canonical JSON 均 reject 且上游计数 0 |
| 02 | snapshot producer/client | 不改变 envelope 字节；requestID/digest binding 不重绑 |

**Close condition：** 共享向量全部通过；F01/skill 边界明确声明 digest 字节不属模型；错误 digest 状态机测试证明不会消耗执行 fence 或触达上游。

### D16 — 平台保护 Header 公共契约

**原始 finding：** R01-13, R02-07, R03-08。

**唯一方案：** 定义三个不相交 Header 类别：无条件平台/传输保护、manifest 声明的端到端业务 Header、manifest 声明的凭证 Header。外部请求的平台保护 Header 全部丢弃；允许转发的业务 Header 保留多值；凭证 Header 的请求方值丢弃，02 解密值最后注入。若业务凭证名就是 `Authorization`，入口仍先剥离外部值用于平台认证，随后按 manifest 注入当前用户业务值；用户自填值永远不能覆盖。

**备选取舍：** 白名单保留上游 `Authorization` 会与平台认证冲突；完全禁止业务使用 `Authorization` 会破坏大量真实 API。折中是保留该名称作为 manifest credential，但值只能来自 02 精确 context，且最后应用。

**可套用契约片段：**

```markdown
### Header classes

外部 `/api-proxy` 和 MCP tool 输入先按 Header 名 canonical 化（ASCII 大小写不敏感，保留原 value 顺序），再分为：

1. **无条件剥离**：`Authorization`、`X-API-Key`、`X-Obot-Api-Key`、`X-Obot-Worker-Authentication`、`Cookie`、`Set-Cookie`、`Host`、`Content-Length`、`Transfer-Encoding`、`Connection`、`Keep-Alive`、`Proxy-Authenticate`、`Proxy-Authorization`、`Proxy-Connection`、`TE`、`Trailer`、`Upgrade`、所有 `Via`、`Forwarded`、`X-Forwarded-*`，以及 `X-Obot-*`。这些值用于平台认证/传输时不得再作为业务 Header 转发。
2. **端到端业务 Header**：仅 manifest 明确声明为 forwardable 且非 sensitive 的 Header，按多值保留。
3. **业务凭证 Header**：仅 manifest 明确声明为 credential 的 Header。请求方同名值丢弃；02 从当前用户请求实例的精确 context 解密并最后注入。缺失、解密失败或 prefix 不符为 `business_authorization_required`，不触达上游。

传输推导 Header 只允许 `Content-Type` 和 manifest media type 对应的 `Accept`。Worker 服务认证 Header 在进入上游执行前删除。OpenAPI 不得声明 cookie 参数、平台保留名作为 forwardable Header 或用户可覆盖 credential；声明冲突时发布校验失败。
```

**跨语言向量：** 覆盖大小写、多值、重复、`Authorization` 作为业务凭证、`authtoken` 自定义凭证、自填 `X-API-Key`/`X-Obot-*`、代理/转发 Header、OpenAPI Header 参数和 MCP tool Header 输入。假上游只观测最终 Header 集合。

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | executor Header builder | credential 最后应用；平台/服务认证不穿透；无默认 Cookie/token |
| 02 | resolver/credential injection | 精确实例值、prefix、缺失凭证；请求方覆盖无效 |
| 03 | raw 入站清理/文档 | 保留 forwardable 多值；平台保护全剥离；错误名不进入上游 |

**Close condition：** Go/Python/HTTP/MCP 共享向量结果一致；测试产物只记录 token 指纹或断言，不保存合成 token 原文；同名覆盖全部失败。

### D17 — raw path percent-decode、静态段等价与歧义规则

**原始 finding：** R03-06。

**唯一方案：** 冻结 strict raw-segment parser：先按 literal `/` 切分 raw escaped path；每个 segment 的 percent escape 必须合法且 hex 大写规范化；percent-decoded bytes 必须是合法 UTF-8 且不含 NUL/control。静态段用 percent-decoded UTF-8 字节比较，参数保留原始 escaped representation；参数内 `%2F` 是数据，重编码后仍是 `%2F`，不会变成路径分隔符。模板 specificity 使用从左到右 segment-wise 规则，static > parameter；完全同 specificity 的可匹配冲突拒绝发布，运行时不得尝试多个候选授权。

**备选取舍：** 直接比较 escaped bytes 最快，但 `/it%65ms` 与 `/items` 会被当成不同路由，违反常见 URI 语义；完全解码后重拼则会丢 `%2F`。按 segment decode/re-encode 是保留参数字节和静态语义的折中。

**可套用契约片段：**

```markdown
### Raw path matching V1

1. 以 literal `/` 切分 external schema-relative `RawPath`，不先执行 `PathUnescape`、clean、redirect 或 Unicode normalize。
2. 每个 segment 必须通过 strict percent syntax 检查；解码结果必须是 UTF-8，拒绝 NUL/control。percent hex 在内部表示中统一为大写。
3. 静态 template segment 与请求 segment 均先 percent-decode，再按 UTF-8 bytes 相等比较。解码后的 `/` 在静态段中不产生额外分隔符；不能相等即未匹配。
4. path parameter 保存原始 escaped segment bytes，按 manifest style 解码为参数值；执行时重新 percent-encode，确保 `%2F` 不变成分隔符、不引入新的 `/`，也不改变非保留字符的绑定语义。
5. 对同一 method/template 集合使用从左到右 specificity：segment 上 static 优先于 parameter，第一处差异决定优先级；完全等价或无法唯一判定为 ambiguous。发布时等价/重复模板必须失败；运行时 ambiguity 返回唯一平台错误，不逐个候选做授权尝试。
6. 空 path、空 segment、非法 escape、traversal 字节和模板 arity 不匹配按 strict parser/匹配结果返回 400/404/405，不重定向、不改写路径。
```

**跨语言向量：** `/items`、`/it%65ms`、`/items/a%2Fb`、`/%2E`、`/items/%2E%2E`、`//`、大小写、非 ASCII、非法 UTF-8、非法 escape、重复模板、`/a/{x}` vs `/{y}/b`、重复 query key/空值。Go/Python 选择同一 operation、参数 bytes、错误类别和 status。

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | Go manifest/matcher | 发布冲突拒绝；运行时唯一匹配 |
| 01 | Python matcher/binder | 同向量结果一致；`%2F` 不分隔回上游 path |
| 03 | raw-prefix parser/router | 无 redirect/clean；404/400/405 分类一致 |

**Close condition：** 全部向量在 Go/Python 中逐项一致；无授权候选尝试顺序；真实 HTTP 测试显示 escaped path/query 原样到达假上游。

### D18 — MCP component loopback 完整身份

**原始 finding：** R02-03。

**唯一方案：** loopback token 中的 UID 与三个内置组不得被当作完整 `kuser.Info`。OpenAPI component bridge 每次 `tools/list`/`tools/call` 先验证内部签名 loopback token，再以 UID 为键从权威用户/身份存储补全当前有效 Obot groups、IdP groups、用户状态和（如来自平台 Key 入口）API Key attribution/scope，构造 `PrincipalContext` 后复用同一 resolver。禁止只用 `GroupMCP/GroupCompositeMCP/GroupAuthenticated` 匹配共享 Profile。

**备选取舍：** 完全信任父 vMCP 已过滤结果实现最少，但组件 bridge 无法独立防父实现回归、 revoked tool 或实例变化；把全部 groups 写入 token 会引入过期和撤销窗口。权威存储按 UID hydrate 每次调用身份是安全且可撤销的方案。

**可套用契约片段：**

```markdown
### Component loopback identity

OpenAPI component loopback token 是短期内部凭证，只用于定位 admitted user 和调用归因，不是完整授权身份。bridge 必须验证签名/audience/过期时间，随后按 UID 从权威数据补全：

- 当前有效用户与禁用状态；
- Obot groups 与 auth provider groups；
- 目标 canonical vMCP、当前用户 instance 与组件选择；
- 平台 API Key attribution 和 `authorized_mcp_ids`（如入口为 API Key）；
- Profile/ToolOverride/operation 授权所需的完整 principal context。

补全后的身份才可进入 02 Resolver。用户、组、Key scope 或实例配置变化在下一次调用实时生效；不得缓存跨请求授权决定，不得把 token 内置三组当作 Profiles 全集，也不得因父 vMCP 已连接而跳过 component/operation 检查。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | loopback token claims、identity hydrator、bridge/resolver | Obot group/IdP group 放行与拒绝、组变更、用户禁用、Key scope、实例切换、跨组件同名工具 |
| 03 | MCP 与 HTTP 同 resolver | 两入口身份解析一致 |

**Close condition：** 真实 MCP `tools/list`/`tools/call` 覆盖完整 Profile 匹配和撤销；审计能区分身份来源为 hydrated principal 而非 token-only；伪造/过期 loopback token 拒绝且不触达上游。

### D19 — API Key/vMCP scope 授权边界

**原始 finding：** R03-05。

**唯一方案：** API Key scope 是 02 resolver 的强制输入，不是 03 的路由优化。目标 canonical vMCP 必须命中 key 的 canonical ID、当前用户实例对应授权或 `*`；legacy alias 只能作为已有授权兼容输入，解析后仍记录 canonical target。F02/F03 当前模型未覆盖该层，因此冻结为 G00 后、I02/I03 实施前的强制模型或 verifier 边界更新加上实现回归矩阵；未完成前不得宣称授权模型完整。

**备选取舍：** 只写代码测试能发现多数实现错误，但不能防止后续把 scope 从状态机中删掉；把完整 OAuth/API Key 存储建模进 TLA 会偏离 resolver 核心抽象。选择最小 scope state 扩展或明确 verifier boundary + mandatory tests。

**可套用契约片段：**

```markdown
### API Key and vMCP scope gate

OpenAPI HTTP/MCP 执行 authorization 必须同时满足：

1. admitted API Key 用户与目标 vMCP 当前用户实例一致；
2. key scope 为 `*`，或包含目标 canonical vMCP ID，或包含解析到该 canonical vMCP 的当前用户实例授权；
3. legacy alias scope 只能通过解析后的 canonical target 判定，不得绕过 instance owner/profile/tool 检查；
4. skills/device/API-only key 没有 `GroupMCP` 或目标 scope 时拒绝；
5. 管理可见性、catalog read、component 显示名和 tool alias 不参与 scope 判定。

F02/F03 verification artifacts must either add a key-scope/target-vMCP transition and wrong-scope counterexample, or explicitly list this gate as outside the formal model and require the implementation matrix below before I02/I03 settlement.
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | PrincipalContext/resolver/formal boundary | A-key→B-vMCP 拒绝；`*`、canonical、instance、legacy alias 分别验证 |
| 03 | API-proxy/MCP 入口 | 无 Key、错误 scope、无 `GroupMCP`、管理可见但不可调用 |

**Close condition：** 模型含目标 vMCP/scope 反例，或 skill/EVIDENCE 明确列为模型外且全部授权回归通过；上游计数为 0。

### D20 — HTTP mode 最小参数校验边界

**原始 finding：** R03-07。

**唯一方案：** HTTP mode 执行“结构绑定校验”，不做业务语义校验。只检查 manifest 声明的 required path/query/header/body、参数 style/repeat 支持、媒体类型、编码合法性和 binary base64；未知 query 可原样保留，未知/未声明 Header 剥离。不执行 enum/format/business rule、未知 JSON field 校验或业务错误预判。无效结构在平台层拒绝且不触达上游。

**备选取舍：** 完全透传最像反代，但与契约“缺 required 参数是平台 400”冲突；完整 JSON Schema 校验会把业务系统语义前移并造成不同版本分叉。最小结构校验是稳定子集。

**可套用契约片段：**

```markdown
### HTTP input structural validation

Worker 在发起业务请求前必须完成：

- matched operation 与 method/path template 唯一；
- required path/query/header/body 参数存在；
- manifest 声明 style 与重复参数语义可解析；V1 支持 simple/path/query form 及 explode/non-explode、space/pipe delimited、deepObject、JSON/form-urlencoded/multipart/base64 binary；cookie、脚本 transform、未声明 Header 参数和不支持 style 发布时拒绝；
- percent/control/UTF-8/base64 编码合法；
- required body 存在且 Content-Type 匹配声明 media type；不支持 media type 返回 `unsupported_media_type`；
- 外部 Header 已按 Header classes 清理。

不执行 enum/format/business rule、未知 JSON field、业务状态码预判或调用方自定义转换。未知 query 与 query 顺序/重复值保留；未声明 Header 不转发。所有结构失败为平台错误且 `upstream_attempted=false`；业务系统对合法透传参数返回的 4xx/5xx 保持 business result。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | parameter binder | 缺 required、非法编码、错误 media type、unsupported style、合法重复 query/多值 Header/binary |
| 02 | manifest/validation errors | typed error 映射一致 |
| 03 | HTTP 语义测试 | 400/415/422 与业务 4xx 区分；假上游计数 |

**Close condition：** 共享负例/正例全部通过；无平台替业务返回 enum/format 错误；合法未知 query 原样转发。

### D21 — HTTP 响应协议子集与保留响应 Header

**原始 finding：** R03-08。

**唯一方案：** 只支持单一最终 HTTP response。传输层可处理 `100 Continue`，但它不是业务结果；`101`/upgrade、连接切换、trailer 声明和事件流都是 unsupported protocol，返回平台错误并关闭连接，不输出半截成功。`X-Obot-Error-Code` 冻结为平台保留响应 Header：平台错误设置，上游业务同名值剥离；文档明确业务方不得依赖该 Header。多值端到端 Header、`Set-Cookie`、204/304/HEAD 按协议子集保留。

**备选取舍：** 透传同名 `X-Obot-Error-Code` 更符合“Header 原样”，但允许业务伪造平台错误标识；完全不用 Header 区分平台错误会让客户端必须解析 body，影响 HEAD/204。因此声明并剥离保留 Header 是显式契约例外。

**可套用契约片段：**

```markdown
### Supported response protocol subset

- Worker 只返回一个最终业务 response：status、有序多值端到端 Header、原始 body bytes 和 requestID。上游 2xx/3xx/4xx/5xx 均为 business result；重定向不自动携带认证跟随。
- `100 Continue` 只能由 transport 处理，不进入内部 business result。`101`、upgrade、protocol switch、请求或响应 trailer、`text/event-stream` 均返回 `upstream_protocol_unsupported` 类平台错误并关闭上游连接；不得返回部分 body 伪成功。
- hop-by-hop、连接管理、代理、Worker service auth、`Content-Length`、`Transfer-Encoding`、`Trailer`、`Upgrade` 和 `X-Obot-Error-Code` 不作为业务 Header 透出。最终 `Content-Length` 由出口按 body 重算。
- `Set-Cookie` 保留上游多值和顺序，但只属于本次业务响应，不进入平台 session、共享 Cookie jar 或后续请求。`204`/`304` 和 HEAD 不写 body；其他无 body 响应不得补 Content-Type 或空行变化。
- `X-Obot-Error-Code` 是平台保留响应 Header：仅平台错误设置；上游业务同名 Header 必须剥离，且发布文档必须说明业务方不可依赖该名称。
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 01 | response reader/executor | 101/upgrade/trailer/SSE 拒绝；100 continue 不当业务结果；多值/无 body/204/304 |
| 03 | business writer | 上游同名错误 Header 剥离；平台错误才设置；HEAD 无 body |
| 05 | E2E | 特殊响应不半截成功，业务错误原样返回 |

**Close condition：** 协议子集向量覆盖 100/101、upgrade、Trailer、SSE、204/304、HEAD、无 Content-Type、多值 Header 与同名错误 Header；实现无半截业务 body 或平台 Header 污染。

### D28 — F02 同一用户多实例与禁用工具/组件模型缺口

**原始 finding：** R05-08。

**唯一方案：** 将 F02 现有两用户/两实例一一对应模型扩展为：`u1` 同时拥有 `i1/i2` 且凭证不同、`i2` 缺必需凭证；`u2` 拥有 `i3`；每个用户均有可用与禁用 component/tool 组合。broken fallback 必须产生 `u1/i2` 借用 `u1/i1` 的反例；恢复模型必须证明同用户错误实例、跨用户实例、禁用 component/tool 和缺失凭证都拒绝。若 G00 在不修改 formal 产物的情况下冻结，上述场景至少必须成为 I02 前置阻断测试，并在 F02 skill/EVIDENCE 中列为模型边界。

**备选取舍：** 现有模型已证明跨用户 fallback，可能隐含同用户错误实例，但证据不可读且没有禁用工具/组件分支；只加实现测试可以挡实现，却不能防止 resolver 模型未来回归。优先扩展模型，代码矩阵仍保留。

**可套用契约片段：**

```markdown
### Exact-instance and disabled-resource boundary

Resolver authorization is bound to `(authenticated user, requested instance, immutable component, original tool)` and is not transitive over instances owned by the same user. V1 requires:

- `u1/i2` must not borrow a credential from `u1/i1`;
- a disabled component or original tool is rejected even when another component exposes the same display name or tool alias;
- missing/invalid required credential rejects before Worker dispatch;
- authorization is recomputed for every `tools/list` and `tools/call`.

The F02 verification boundary must be extended before I02 settlement (or explicitly recorded as a model gap with the mandatory implementation matrix) to include same-user multi-instance, missing credential, disabled component, and disabled tool counterexamples.
```

**影响与测试矩阵：**

| Feature | 文件/接口 | 必测 |
| --- | --- | --- |
| 02 | F02 model/view/skill、resolver/bridge | `u1/i2` 借 `u1/i1` 反例；恢复模型拒绝；禁用组件/工具不进入 describe/execute |
| 03 | HTTP 入口 | 同用户错误实例 URL/连接不能调用 |
| 05 | E2E | 同用户两实例并发使用不同合成 token，假上游断言无串用 |

**Close condition：** 扩展模型复跑或明确边界记录存在；实现测试覆盖同用户双实例和禁用资源；所有错误派发上游计数为 0。

## 2. 冻结用替换/新增汇总

00 复核后应将本文第 1 节的代码块按以下方式并入 `CONTRACTS.md`：

| CONTRACTS 位置 | 操作 | 覆盖簇 |
| --- | --- | --- |
| §1 信任、身份和产品边界 | 新增 Principal-aware Executor、API-proxy 资格链、loopback identity、Key scope | D03, D08, D18, D19 |
| §2 快照与 operation 身份 | 替换 executionDigest/operation 清单为 canonical manifest 与 policy bytes；新增 Header classes | D04, D16 |
| §3 Worker 内部协议 | 新增 endpoint 唯一性、digest equality gate、raw path matching V1、HTTP 结构校验、响应协议子集 | D14, D15, D17, D20, D21 |
| §4 普通 HTTP 与 Agent MCP | 替换路由/响应描述为 raw-prefix branch、business writer、调用资格链、Header/路径/参数规则 | D03, D07, D08, D16, D17, D20, D21 |
| §9 运行与部署 | 新增 Worker 服务认证与轮换 | D02 |
| §10 冻结记录 | 新增 F01/F02/F03 强制边界与测试向量 | D15, D19, D28 |

## 3. 受影响 feature 与精确矩阵

| Cluster | 01 Worker | 02 Bridge/Resolver | 03 HTTP proxy | 04 Budget | 05 Compose/E2E |
| --- | --- | --- | --- | --- | --- |
| D02 | TLS server/client CA/auth config | mTLS client + optional bearer | 不直接实现 | 不直接实现 | secrets、轮换、真实握手与负例 |
| D03 | typed result/error 消费 | Principal/resolver/executor 唯一 owner | 只传 principal ctx/raw input | 不透明 lease metadata | 双入口越权 E2E |
| D04 | manifest/digest 验证与编译 | manifest 生成/持久化/Go 权威字节 | 消费 description/docs | budget source 不用 executionDigest | 双 Worker digest 一致 |
| D07 | business result 字节 | 不并行改 server 写集 | raw branch + writer owner | 不直接实现 | 真实路径/响应 E2E |
| D08 | 无授权判断 | 组件/Profile/operation 二次解析 | 专用 API-proxy authz | Acquire 前拒绝 | 错 scope 上游计数 0 |
| D14 | 四 endpoint only | helper 只 describe | 不出现 ensure | 不直接实现 | route smoke |
| D15 | digest equality vector | snapshot producer | 不直接实现 | 不直接实现 | 冻结验收向量 |
| D16 | Header builder | credential 注入 | 入站清理/文档 | 审计脱敏 | 假上游 Header 断言 |
| D17 | Python matcher | Go matcher/manifest | raw parser/router | 不直接实现 | 跨语言向量 |
| D18 | 不解析平台身份 | loopback hydrator/resolver | 与 HTTP 同入口 | 不直接实现 | MCP 真实请求 |
| D19 | 不解析 Key scope | Key/vMCP gate | 入口 scope 测试 | 不替代资格 | wrong-scope E2E |
| D20 | 结构校验/binder | typed errors | 400/415/422 映射 | Acquire 前失败 | 假上游负例 |
| D21 | response subset | result 类型 | reserved writer | 不直接实现 | 特殊响应 E2E |
| D28 | 上游隔离 | F02/model/resolver tests | 同用户实例 URL | 不直接实现 | 并发 token 隔离 |

## 4. 每簇关闭条件总表

| Cluster | 00 可独立复核的关闭证据 |
| --- | --- |
| D02 | 契约/配置/Compose 三方一致；一次真实 mTLS 成功和错 CA/错身份/无 client/过期/降级 HTTP 负例 |
| D03 | 02/03 同接口编译；越权矩阵在 Worker/上游前失败；typed error 与 business result 测试通过 |
| D04 | 共享向量显示 Go 权威字节与 Python 验证一致；错误 digest/manifest 全部在编译前拒绝 |
| D07 | raw path 与 writer 回归显示无 redirect/clean/平台 Header/Cookie 污染；现有非代理响应不变 |
| D08 | 管理 `vmcp_id` 可见性不能调用；专用资格链和 02 二次收窄全部负例上游计数 0 |
| D14 | route/client/generated tests 只有 describe/execute/healthz/readyz；无 ensure route |
| D15 | F01 边界声明 + digest equality 向量和错误 digest 不进入 executing/upstream 的状态测试 |
| D16 | 跨语言 Header 向量一致；业务凭证最后注入，平台/服务/自填值不穿透 |
| D17 | Go/Python path vectors 的 operation、参数 bytes、错误/status 完全一致 |
| D18 | 真实 MCP Profile/组/实例/撤销测试通过；审计证明 token 被补全而非 token-only 授权 |
| D19 | 模型或 explicit boundary 更新，且 wrong-scope/wildcard/legacy alias/no-key 回归全部通过 |
| D20 | 结构负例不触达上游；合法未知 query/重复值/binary 透传；业务 4xx 保持原样 |
| D21 | 100/101/upgrade/trailer/SSE/204/304/HEAD/同名 Header 测试符合单一最终 response 子集 |
| D28 | 扩展 F02 或明确模型缺口记录；同用户双实例、缺凭证、禁用 component/tool 测试全部通过 |

## 5. 剩余风险与用户决策

- **真实企业授权交换仍是 GAP/UNVERIFIED。** 本文只冻结凭证存在、实例绑定、解密和注入语义；不宣称业务登录、刷新、撤销或真实企业 API 已闭环。
- **mTLS 依赖部署材料纪律。** 证书生成/挂载/轮换错误会使 OpenAPI 调用不可用；05 必须用真实 Compose 握手验证，不能用 `compose config` 替代。
- **canonical manifest 会改变显式发布产物。** 这是为了消除 Go/Python 双 parser 漂移；如果用户要求已发布历史组件零迁移，需要 00 单独批准 legacy 迁移策略，不能用旧 runtime 静默替代。
- **`Authorization` 作为业务凭证是显式例外。** 外部值先服务平台认证，业务值只能由 02 注入；若用户更倾向禁止业务 API 使用该名称，需改变本推荐并重新评审兼容性。
- **F01/F02 模型未在本任务修改。** D15/D28 按“冻结强制边界/测试”处理；若 00 要求 formal artifact 本身更新，需要另派受限任务，因为本任务唯一写集是本文。
- **路径/响应子集是刻意的 V1 有限协议。** 不支持 upgrade/trailer/SSE 和任意代理语义；这是防止无界流与平台/业务语义混合，不是通用 HTTP proxy 能力承诺。
=======
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
>>>>>>> feature/openapi-execution-budgets
