# P03 普通应用 HTTP 代理计划

日期：2026-11-09。worktree：`/Users/echo/orca/workspaces/obot/openapi-http-proxy`；分支：`feature/openapi-http-proxy`；基线：`2be04f025d1c86144e8670d9e9341b263edc28ea`。状态：planning complete。本文仅规划，不表示业务代码已实施，也不表示本地或生产 E2E 已通过。

## 1. 问题与目标

当前 Obot 只有 `/mcp-connect/{mcp_id}` 的 MCP 出口；`pkg/openapi` 只负责导入和保存 OpenAPI 快照，Go 侧没有 operation 匹配、普通 HTTP 执行或应用代理路由。目标是新增 `/api-proxy/{vmcp_id}/{component_id}/{path...}`，让已构建应用用平台身份和平台 Key 直接调用已发布的 OpenAPI operation，并由 01/02 的统一执行核心使用当前用户绑定的业务凭证请求原始 API。

完成标准：

- 平台身份、API Key 范围、vMCP/用户实例资格、组件资格和 operation 白名单全部实时校验；管理端列表可见性不能替代调用资格。
- 原始 HTTP 方法、路径模板匹配、路径编码、重复 query、请求 body、允许的端到端 Header、业务状态码、业务 Header 和 body 保留；业务 403/404/5xx 不被包装成平台 500。
- 平台 `Authorization`、`X-API-Key`、Cookie、Worker 服务认证与用户业务认证彻底分离；请求方不能通过 Header/body 指定用户 ID 或覆盖平台绑定的业务凭证。
- HTTP 代理与 Agent MCP 共用 02 的 `Describe/Execute`、同一 snapshot、同一 operation 身份和同一预算入口；应用调用不依赖 Agent、MMMCP 会话或 `/tools/call`。
- 运营继续用 vMCP 组件快照、`ToolOverride` 和 OpenAPI schema 维护名称、描述、参数说明和启用清单；应用调用文档从同一发布定义生成，不新增 Endpoint 顶层对象。
- 入站请求、上游响应、内部编码和审计全部有界；取消和绝对截止时间贯穿 04 租约、02 Executor、01 Worker 和上游连接。

## 2. 已核查现有基础

以下 `FACT(local)` 均来自当前 worktree 源码。本任务尚未访问真实业务 API、生产服务、历史 JWT 或集群。

### 2.1 路由、平台身份与授权

- `pkg/api/router/router.go:617` 现有 MCP HTTP 出口是 `/mcp-connect/{mcp_id}` 和子路径；`/api/` 在 `router.go:611` 被管理 API 404 捕获，`/api-proxy` 尚不存在。
- `pkg/gateway/server/apikey_auth.go:31` 支持从 `Authorization: Bearer` 或 `X-API-Key` 识别 `ok1-...` 平台 Key，并只把 key 的 scoped groups 加进身份；`apikey_auth.go:76` 从数据库 key 得到用户 UID，并写入 `authorized_mcp_ids`、API Key 归因和用户角色组。
- `pkg/gateway/types/apikeys.go:35` 的 key scope 只有 `canAccessAPI/skills/deviceScans` 与 `mcpServerIds`；`apikeys.go:73` 有 `mcpServerIds` 时才给 `GroupMCP`。没有单独 component 或 operation scope。
- `pkg/api/authz/resources.go:109` 只给 `GroupMCP` 暴露 MCP connect 方法；`resources.go:176` 把路径变量转为资源检查，`authz.go:437` 在静态规则后执行资源级检查。
- `pkg/api/authz/mcpid.go:25` 复用 `vmcpaccess.ResolveConnectID` 解析当前用户连接；`mcpid.go:115` 校验 vMCP 归属/Profile，`mcpid.go:231` 再校验 API Key 的 vMCP/实例/legacy alias 范围。`vmcpScopeMatches` 支持 canonical vMCP、legacy slug 和实例名。
- `pkg/vmcp/profiles.go:44` 的授权单位是组件 ID + 原始工具名；`profiles.go:93` 的 `InstanceGrant` 把共享 vMCP Profile 与用户实例选择相交，个人 vMCP 只允许 owner 的组件选择。显示别名和前缀不参与该授权身份。
- 重要风险：`pkg/api/authz/resources.go` 已把 `vmcp_id` 绑定为管理 API 资源变量，`pkg/api/authz/vmcp.go:94` 的 `checkVMCP` 主要按管理读写/可见性判断。若直接把代理路径变量命名为 `vmcp_id`，GET 可能只因管理可见性在组件与 operation 检查前通过。因此代理不能复用该变量和检查顺序。
- `pkg/api/authz/mcpid.go:238` 在 key 直接授权 canonical vMCP 时可提前返回，且不一定把当前用户实例放入授权资源。代理 handler 不能信任中间资源快照，必须在 02 resolver 中重新解析用户实例和组件。

### 2.2 vMCP、用户实例与发布快照

- `apiclient/types/vmcp.go:41` 定义组件是 catalog entry 快照；`CatalogEntry`、`SourceDigest`、配置策略、`ToolPrefix` 和 `ToolOverrides` 都保存在 vMCP manifest。`ToolOverride` 在 `apiclient/types/mcpserver.go:183` 中保存原始工具名、显示名、描述覆盖和启用状态。
- `pkg/storage/apis/obot.obot.ai/v1/vmcp.go:32` 是持久化 vMCP manifest；`vmcpinstance.go:33` 的用户实例保存 `VMCPID`、用户 ID 和组件选择，`Status.Configured` 不等于业务凭证一定存在。
- `pkg/vmcp/resolve.go:15` 支持 canonical ID、实例 ID 和 legacy alias，并在实例属主不匹配时返回 NotFound；`resolve.go:99` 对 canonical vMCP 选择该用户最旧实例。
- `pkg/vmcp/legacy.go:11` 说明迁移连接可保留旧组件快照；`ComponentsForInstance` 必须用于读取用户实际发布定义，不能直接拿当前 vMCP 组件覆盖。
- `pkg/vmcp/credential.go:56` 有 vMCP 固定配置和实例配置 credential context，`ConfigurationKey` 用组件 ID 划分命名空间；`pkg/gateway/client/credential.go:76` 支持按 context/name 揭示加密凭证。现有 `serverConfigForAction` 存在 server/catalog 固定配置 fallback，02 已计划为 OpenAPI 用户业务认证改用无 fallback 的专用 resolver。

### 2.3 OpenAPI 快照、operation 与 Header

- `apiclient/types/openapi.go:60` 说明运行配置保存 normalized schema 与 baseURL，凭证值属于 manifest Header Config；`pkg/api/handlers/openapi.go:50` 每次保存目录条目重新导入 Source，已有部署保留旧快照直到显式更新。
- `pkg/openapi/import.go:148` 只做导入、规范化和 wrapper 支持子集检查，不生成工具、不执行业务 API；`MaxSchemaBytes` 为 1 MiB。
- `pkg/openapi/document.go:26` 接受 GET/PUT/POST/DELETE/OPTIONS/HEAD/PATCH/TRACE，`document.go:211` 拒绝 cookie 参数并检查 Header 参数。Go 导入器当前不输出 method/path/参数绑定到 operation 清单，运行时需要 01 Worker `describe` 提供该映射。
- `pkg/openapi/headers.go:9` 的保留 Header 校验包含 Host、Cookie、Content-Length、Connection、代理和转发类 Header，但没有保留 `authorization`、`x-api-key` 或 `x-obot-*`。这些名称可作为 OpenAPI Header 参数或凭证 Header 的候选，代理和 Worker 必须额外执行平台保护规则。
- `pkg/openapi/environment.go:21` 证明现有 wrapper 环境只包含 baseURL 与凭证 Header 名，凭证值留在请求链；这支持 01/02 的逐请求认证模型。

### 2.4 HTTP server、路径与审计现状

- 本地 Go 1.27 `net/http.ServeMux` 文档确认：`Request.PathValue` 返回已解码值，`%2F` 会变成 `/`；ServeMux 还会对重复斜杠、`.`/`..` 路径段做清洗重定向。普通 wildcard 注册无法满足原始路径字节语义。
- `pkg/api/server/server.go:186` 的统一 `Wrap` 负责认证、限流、活动记录、审计 writer、密码修改门禁和授权；`server.go:298` 的活动审计只覆盖 `/api/` 前缀，`server.go:358` 会为 `/api/` 添加 no-cache Header，`headersResponseWriter` 还会默认添加 `X-Content-Type-Options` 和 Content-Type。
- `server.go:308` 会在认证器刷新会话后向响应回放 `Set-Cookie`。这些平台响应语义不能混入业务原始响应。
- `pkg/gateway/types/mcpauditlog.go:22` 的 MCP audit 记录包含用户、API Key 归因、MCP ID、调用类型、调用名、状态、耗时、requestID 和有界请求/响应字段；`pkg/gateway/client/auditlogpersister.go:11` 提供异步持久化。普通 HTTP 代理应生成 `call_type=http_proxy` 类元数据，并复用 04 的有界截断规则，而不是完整缓冲业务正文。

### 2.5 UI 与文档现状

- vMCP 设计器已支持组件名称、工具前缀、原始工具名、显示名、描述覆盖和启用清单，相关类型与测试集中在 `ui/user/src/lib/components/vmcps/`。
- `ui/user/messages/vmcps/{en,ja,ko,zh-CN}.json` 是新增 UI 文案的四语言目录；`ui/user/src/AGENTS.md` 要求行为变化 colocated browser spec、可访问 locator、先跑受影响既有测试。
- 当前文档在 `docs/docs/mcp-gateway/connect-clients.md` 说明 API Key 和 MCP 连接，尚无普通应用 HTTP 调用说明。`docs/AGENTS.md` 要求文档内部相对链接；新增页面需更新 `docs/sidebars.ts`，修改既有页面则不需要。

## 3. 契约草案核查结论

以下发现需 00 在交叉审核和 G00 冻结时处理；前四项是实施阻断。

1. **P1：普通 ServeMux 路由不满足原始路径契约。** `{path...}` 会解码 `%2F`，且 `//`、`.`/`..` 会被清洗重定向。必须在 APIServer 进入 mux 前为 `/api-proxy/` 增加保留 RawPath 的专用分支，或由 00 明确放弃这些路径语义。默认方案是专用分支。
2. **P1：不能复用管理 `vmcp_id` 资源检查。** 代理要有专属授权入口，先以 canonical vMCP ID 走 `checkMCPID` 的 API Key/连接资格，再把组件、实例选择和原始工具名交给 02 resolver。不得让 `checkVMCP` 的管理可见性提前放行。
3. **P1：P02 当前接口草案不足以让 03 安全落地。** `Reference/Invocation/Description` 未明确平台 principal 来源、用户已收窄 operation 清单、raw HTTP 输入、Header 分类、budget key 和错误分类。本文第 6 节列出必须移交给 02/00 的具体字段。
4. **P1：统一响应 writer 会污染业务响应。** no-cache、默认 Content-Type、安全 Header 和会话 `Set-Cookie` 都是平台语义。代理分支必须保留上游端到端 Header 与 body，平台错误才使用平台错误格式；同时不能把认证器刷新的会话 Cookie 写回业务响应。
5. **P2：平台凭证 Header 保留名单不完整。** 03 需要无条件剥离入站 `Authorization`、`X-API-Key`、`X-Obot-Api-Key`、Cookie、代理和转发类 Header；即使 OpenAPI 把 `Authorization` 声明为普通 Header 参数，也不能由应用值覆盖 02 注入的业务身份。该规则需要 01 编译计划和 02 Header 分类同步。
6. **P2：API Key 直接授权 canonical vMCP 时不会自动携带用户实例。** 03 不能从授权层拿 instance，必须要求 02 resolver 在每次 Describe/Execute 中按用户重新查找并校验实例。
7. **P2：path 模板冲突策略未冻结。** 多个模板可匹配同一具体路径。安全默认是静态段优先且等价/歧义模板拒绝；不能为绕过禁用清单而尝试多个候选直到其中一个被授权。
8. **P2：文档中的“参数说明维护”需要明确边界。** 默认不新增参数级覆盖模型：名称/描述/启用来自现有 `ToolOverride`，参数说明来自已发布 OpenAPI schema，运营修改 schema 后必须显式发布新 vMCP 版本。若产品要求在 Obot 内覆盖参数说明，必须由 00 扩展共享 `ToolOverride` 类型与生成写集，不能在 03 私自加 Endpoint 或旁路对象。

## 4. 范围与非目标

### 范围

- 新增普通 HTTP 代理路由、raw path 解析、operation 白名单匹配、请求/响应转换和平台错误响应。
- 新增代理专属授权规则，复用平台认证、API Key `mcpServerIds`、vMCP/实例解析、Profiles、组件 ID 与原始工具名授权。
- 通过 02 `Executor` 调用 01 Worker，不直接访问 Worker 内部 HTTP 客户端或业务上游。
- 集成 04 预算、排队、取消、字节上限和有界审计接口。
- 提供应用调用契约 API/文档和 vMCP UI 的只读调用说明；现有 vMCP 管理能力全部保留。
- 增加授权、路径编码、HTTP 语义、错误归属、UI 行为和文档的回归测试。

### 非目标

- 不实现 Python Worker、固定 Worker 配置、MCP JSON bridge、全局预算状态机或 K8s 部署；分别由 01/02/04/05 负责。
- 不新增 Endpoint 顶层对象、远程 MCP 类型、业务权限引擎、任意脚本转换、默认 jobId 轮询、Redis 或新运行时。
- 不解析业务 JWT 推导业务角色/数据权限；业务 403 与 body 原样返回。
- 不移除或隐藏既有 MCP/vMCP、目录、组件快照、用户实例、凭证管理和 UI 功能。
- 不调用真实企业 API，不读取、复制或测试历史 JWT；实施验收只用本地假业务系统和运行期合成 token。
- 本规划阶段不修改 `pkg/openapiworker`、`pkg/services/config.go`、Worker、chart 或其他 feature 写集。

## 5. 候选设计

以下为 `CANDIDATE`，需 G00 冻结；与 DRAFT-0.1 冲突处已在第 3 节列出。

### 5.1 路由与请求阶段

外部固定路径：

```text
{METHOD} /api-proxy/{vmcp_id}/{component_id}/{path...}
```

- `vmcp_id` 只接受 canonical `VMCP.Name`，不接受实例 ID、legacy slug 或 `*`；应用调用文档只生成 canonical URL，避免同一个应用 URL 漂移到其他连接。
- `component_id` 必须精确等于 vMCP/用户实例快照中的 immutable component ID；组件显示名、别名和 tool prefix 不参与路由。
- `path...` 可为空时单独注册 `/api-proxy/{vmcp_id}/{component_id}`，避免 ServeMux trailing-slash 重定向；其余路径走 APIServer 专用 raw-prefix 分支。
- 专用分支在进入 ServeMux 前保留 `URL.RawPath/EscapedPath`、`URL.RawQuery`、Header 多值和 body 流，设置固定 `req.Pattern` 供 tracing/授权使用。解析函数只在 `/` 边界上切分 raw path，逐段执行严格 percent-decode，拒绝非法编码、控制字符、空 ID、路径穿越和歧义 segment。
- 认证仍完全复用 `pkg/api/authn` 现有链。API Key 需要包含目标 vMCP 的 `mcpServerIds` 或 `*`；仅 `canAccessAPI`、skills 或 device-scans key 不能调用代理。浏览器会话用户可调用，但业务身份仍从其 UID 和实例凭证解析。
- 新增 `authorizeAPIProxy` 专用入口：校验 `GroupMCP`，解析 canonical vMCP 与当前用户连接，复用 `checkMCPIDAccess` 和 `authorized_mcp_ids`，不调用 `checkVMCP` 管理可见性。组件和 operation 资格在 02 resolver 中二次校验。
- 限流沿用现有 server rate limiter；活动/审计不能因 `/api-proxy` 不在 `/api/` 前缀而静默缺失。

### 5.2 operation 白名单与快照

每次请求按以下顺序解析：

1. 从平台 principal 得到 userID、API Key 归因和角色组；用户 ID 不接受 Header/body 自填。
2. 02 resolver 解析 canonical vMCP、当前用户 instance、`ComponentsForInstance` 的组件快照和 immutable component ID。
3. 校验组件存在于该用户实例、共享 Profile/个人 owner 资格、instance 组件选择和组件启用状态。
4. `Describe` 使用该组件的 `OpenAPIConfig.Schema.Raw` 和有效 baseURL，返回 operation 清单、`schemaDigest/executionDigest`、旧工具名、method/path/参数位置和 Header 分类。
5. 用 01/02 共享的 OpenAPI path matcher 解析 raw relative path；静态段优先，模板参数保留原始编码语义；多个等价匹配或无法唯一确定时返回平台 404/400，不尝试候选绕过。
6. 将 HTTP method + matched path template 得到 `operationKey`，再映射到该组件内唯一 `legacyToolName`。
7. 应用 `ToolOverride`：无 overrides 表示该组件全部 discovered tools 可用；有 overrides 时仅 `Enabled=true` 且原始名匹配的 operation 可用。再用 Profile 与 instance `AllowedTools` 收窄；授权引用始终是 `{componentID, legacyToolName}`，不允许其他组件同名工具代授权。
8. 快照来自 vMCP/迁移连接保留定义，不读取 live catalog。目录说明、显示名或参数说明变化不静默替换已发布应用；显式发布后 docs 展示新 executionDigest，已消失 operation 返回平台 404。

`Describe` 不携带业务凭证、不发业务请求，可在 Worker 冷启动时返回 `snapshot_required`。应用执行仍由 02 按同 requestID 补送一次不可变快照，且不得在不确定是否触达上游后自动重试。

### 5.3 原始 HTTP 语义

请求：

- Method 原样传递，包括 HEAD/OPTIONS/PATCH/TRACE；路由自身不把 GET 伪造成 POST，不强制 JSON body。
- Relative path 使用 raw escaped segments 匹配模板；`%2F` 是参数数据，不是路径分隔符。模板参数按 01 执行计划重新编码，不使用 Go `PathValue` 的已解码字符串直接拼接。
- `URL.RawQuery` 原样传递，保留顺序、空值、重复 key 和编码；不由 `url.Values` 重新排序。
- Body 用 `http.MaxBytesReader` 按 04 上限读取，保留原始字节和声明的 body media type；超限返回 413，不把半截 body 发给上游。
- 入站 Header 分三类：平台认证/传输/安全 Header 全部剥离；OpenAPI 声明且非敏感的端到端业务 Header 按多值保留；凭证 Header 由 02 从当前用户实例解密并最后注入。请求方传入的同名凭证值被丢弃，不产生覆盖。
- 默认允许的传输类 Header 仅由 operation media type 推导，如 `Content-Type` 与必要的 `Accept`；`Host`、`Cookie`、`Authorization`、`X-API-Key`、代理、转发、连接管理和 `X-Obot-*` 不允许应用自填。
- 入站 `Accept` 只含 `text/event-stream` 时返回平台不支持错误；混合 JSON/事件流可继续，但 01 收到上游事件流仍必须拒绝，不建立长期流。

响应：

- 01 Worker 返回 `kind=business` 时，代理写出原始业务 status、body 字节和上游端到端 Header，多值 Header 保留顺序。
- 剥离 hop-by-hop Header、Worker 服务认证、内部地址、`Content-Length` 由 Go transport 按实际 body 重算；`Set-Cookie` 按上游多值保留，但入站 Cookie 永不进入共享 client/业务请求，认证器刷新的平台 Cookie 不写入代理响应。
- 上游 400/401/403/404/409/429/5xx 都是业务结果；不包装 JSON、不改状态码、不吞 body。
- HEAD 保留上游 status/Header，body 由标准 HTTP transport 语义不发送；平台错误同样遵守 HEAD 无 body。

### 5.4 平台错误与业务结果区分

平台错误统一为有界 JSON：`code`、安全 `message`、`requestID`、`retryable`，并加响应头 `X-Obot-Error-Code`。业务成功响应不加该错误头；上游若返回同名 Header，代理剥离后再写业务 Header，防止伪造平台错误标识。平台错误不回显业务 token、完整内部 URL、schema、原始敏感输入或业务 body。

| 阶段 | HTTP | 说明 |
| --- | ---: | --- |
| 平台身份无效/缺失 | 401 | 不触发 Describe/Execute |
| Key scope、vMCP、用户实例、组件或 operation 无资格 | 403 或现有隐藏语义 | 不因管理可见性放行 |
| canonical vMCP/组件不存在或路径无唯一授权匹配 | 404 | 不泄露未授权资源细节 |
| 路径存在但 method 未发布且同 path 有其他授权方法 | 405 + `Allow` | 仅在已授权同路径方法存在时返回 |
| 缺业务凭证/凭证解密失败 | 409 `business_authorization_required` | 不发起上游 |
| 请求体/Header/编码/模板参数无效 | 400 或 415/422 按错误类型 | 不发起上游 |
| 请求体超限 | 413 | 04/01 上限生效 |
| 队满/等待耗尽 | 429 + `Retry-After` | 04 平台错误 |
| Worker/预算存储/协议不可用 | 503 | 不回落 Docker/K8s |
| 总截止时间耗尽 | 504 | 取消上游并交还租约 |
| 上游 SSE、响应过大、传输失败 | 502 类平台错误 | 不返回半截成功 body |
| 上游业务响应 | 原状态/body | 不进入上表包装 |

### 5.5 预算、取消与审计

- 04 budget key 使用目录来源稳定身份 + `operationKey`，不包含用户、vMCP、component ID、副本或快照版本；02 `Description/ResolvedReference` 必须暴露该 key 所需 source identity。
- 在读取 body 前先完成 operation 匹配和 04 `Acquire`；排队等待受总 deadline 约束。获取租约后再按 1 MiB 请求上限读取 body，调用 02/01，收到结果或失败后幂等释放。
- `request.Context()` 断开、总 deadline、04 取消和 Worker 异常都要传递到上游连接。不能证明执行停止时按 04 保守租约策略处理，不得立即重复调用上游。
- 请求、响应、解压、base64、审计均使用 04/01 上限；内部 base64 上限由原始字节上限推导。代理不重复实现全局并发，只传递 lease metadata 和绝对截止时间。
- 审计记录 requestID、userID、API Key ID/name 掩码、canonical vMCP、component ID、operationKey、显示名、来源/结果状态、耗时、取消/错误阶段和大小。默认不记正文；启用正文时由 04 共通组件截断到 32 KiB 并写明确截断标记。认证 Header、Cookie、query 和业务 body 原文不进入普通日志或测试产物。

### 5.6 应用调用文档与 UI

- 新增只读契约 API，例如 `GET /api/vmcps/{vmcp_id}/api-proxy`，返回该当前用户可见的组件与已授权 operation 文档：canonical base path、component ID、显示名、method/path、`operationKey`、原始/显示工具名、描述、参数名/位置/类型/说明、body media type、必需业务授权状态、启用状态、snapshot digest 和调用示例。该 API 走现有 vMCP 管理读权限，不创建新资源。
- 文档数据由 02 `Describe` 与发布组件快照合成，不发业务请求、不携带凭证。Worker 不可用时文档接口返回 503；已构建应用的固定代理 URL 不因此改变，也不依赖 Agent 在线。
- vMCP 详情页/组件操作中新增“应用 HTTP 调用”只读入口，展示复制友好的 base URL、平台认证建议（需要业务 `Authorization` 时使用 `X-API-Key` 携带平台 Key）、operation 清单和参数说明。入口不得隐藏或替代现有 MCP connect、工具配置、Profile 或测试功能。
- 默认不新增参数级覆盖：参数说明来自发布 OpenAPI schema；名称/描述/启用继续用现有 `ToolOverride`。若 00 决定支持本地参数说明覆盖，必须单独分配共享类型和生成文件写集。
- `docs/docs/mcp-gateway/connect-clients.md` 增加“普通应用 HTTP 调用”章节，说明认证分离、路径构成、业务 403 保留、快照更新和 API Key 范围；使用相对 Markdown 链接，不新增 Endpoint 概念。

## 6. 跨 feature 接口与写集移交

### 6.1 需要 02 移交/冻结

`pkg/openapiworker` 的 Go 公共接口需要包含以下语义，不能让 03 直接解析内部 Worker JSON：

```go
type PrincipalContext struct { /* 从 admitted principal 提取，不接受调用方自填 userID */ }

type HTTPInput struct {
    Method       string
    RawPath      string // schema-relative escaped path，不含 query
    RawQuery     string
    Headers      []PreservedHeader
    Body         []byte
}

type ResolvedDescription struct {
    SnapshotDigest  string
    ExecutionDigest string
    BudgetSourceID  string
    ComponentID     string
    Operations      []AuthorizedOperation // 已按当前用户 Profile/instance/override 收窄
    CredentialHeaders []HeaderDefinition
    ForwardableHeaders []HeaderDefinition
}

type Executor interface {
    Describe(ctx context.Context, principal PrincipalContext, ref Reference) (ResolvedDescription, error)
    Execute(ctx context.Context, principal PrincipalContext, invocation Invocation) (Result, error)
}
```

必须由 02 提供或保证：

- 从 `context.Context`/admitted principal 得到用户、API Key 归因和角色组；03 不能传可自填 userID。
- canonical vMCP、用户实例、legacy 保留快照和组件解析结果。
- 已按当前用户收窄的 operation 清单，以及共享 path matcher 的 Go 语义；03 不自行用 operationId 猜测。
- 每个操作的 `operationKey`、legacy tool name、显示名、描述、参数说明、启用状态、method/path、参数位置和 media type。
- 业务凭证 Header 名、required、prefix、protected/incoming 分类；凭证值只在 Execute 内解密注入。
- `HTTPInput` 原始路径/RawQuery/多值 Header/body 的序列化语义和 `Result` 原始 status/body/多值 Header。
- typed error：平台无资格、业务授权缺失、operation 不存在、Worker/协议失败、预算失败、取消、deadline、上游过大/SSE，与 business result 分离。
- `BudgetSourceID` 和不透明 `BudgetContext`，供 04 计算稳定全局 key。
- `Services.OpenAPIWorkerExecutor` 的组装与构造参数。
- 若 03 需要 APIServer raw-prefix 分支，`pkg/api/server/server.go` 当前不在 02 移交写集；需 00 明确将该文件或专用 raw-prefix helper 划给 03，并要求 02 不并行修改。

### 6.2 需要 01 冻结

- OpenAPI path template matcher 的 raw percent-encoded segment 语义、静态段优先、歧义拒绝和参数重编码规则。
- describe 的 operation/参数 schema/Header 分类，不包含凭证值。
- execute `input.kind=http` 对 RawPath/RawQuery/body/多值 Header 的字节语义，以及业务 status/body/Header 返回。
- 平台保护 Header、业务认证 Header 最后应用、无重定向携带认证、上游 SSE/大小限制和取消行为。

### 6.3 需要 04 冻结

```go
type AcquireRequest struct {
    BudgetKey   string
    Deadline    time.Time
    QueuePolicy QueuePolicy
}

type Lease interface {
    Context() context.Context
    Metadata() BudgetContext
    Release(ctx context.Context, outcome Outcome) error
}
```

- `Acquire` 在读 body 前返回可取消 lease；队满/等待超时映射 429。
- 请求/响应/审计字节上限和解压计数组件；base64 上限由 raw 上限推导。
- 取消、Worker 失败、deadline 和客户端断开时的租约回收结果与幂等释放语义。
- 有界审计截断/脱敏公共组件，避免 03/04 各自实现。

### 6.4 P03 独占写集

实施 I03 仅允许写：

```text
docs/plans/openapi-workers/PLAN.md
pkg/api/handlers/apiproxy/*.go
pkg/api/handlers/apiproxy/*_test.go
pkg/api/authz/apiproxy.go
pkg/api/authz/apiproxy_test.go
pkg/api/authz/resources.go        // 仅新增 API proxy 专属入口/变量，不改变既有管理检查
pkg/api/router/router.go           // 仅注册 handler，不修改 Worker 配置
pkg/api/server/server.go           // 仅在 00 明确扩展写集后增加 raw-prefix 分支
ui/user/src/lib/components/vmcps/**  // 仅新增只读调用文档组件和接线
ui/user/src/lib/services/vmcps/types.ts
ui/user/messages/vmcps/en.json
ui/user/messages/vmcps/ja.json
ui/user/messages/vmcps/ko.json
ui/user/messages/vmcps/zh-CN.json
docs/docs/mcp-gateway/connect-clients.md
```

如新增独立 docs 页面，才允许 `docs/sidebars.ts`。禁止修改 `pkg/openapiworker`、`pkg/services/config.go`、Worker、预算状态表、生成文件或其他 feature 计划；接口不足时先报 00 回派 02/04。

### 6.5 公共写集冲突

- `pkg/api/server/server.go`：当前 02 计划未占用，但不是已移交给 03 的文件。raw-prefix 和业务响应 writer 语义必须由 00 单独划给 03，避免与 02 server 组装修改并行。
- `pkg/api/authz/resources.go`：03 只加专属 API proxy 检查，不改既有 `/api/vmcps` 管理行为。
- `pkg/api/router/router.go`：02 已移交注册权，03 只新增代理和文档 route。
- `apiclient/types` 与生成文件：默认不修改。若文档 API 响应必须公开到 apiclient，或增加参数说明覆盖，由 00 指定 02 后单独变更，03 不并行写。
- UI 共享 fixtures：只新增局部 mock/service，不改共享 `tests/mocks`；如必须改，先运行全部导入者测试并由 00 记录。

## 7. 分步实施

1. **冻结接口与写集**：向 00 提交第 3/6 节发现；等待 02 Executor/Header/BudgetSource、01 matcher 和 04 lease 接口冻结，取得 `pkg/api/server/server.go` 写集授权。
2. **raw-prefix 骨架**：实现 `/api-proxy` 专用进入路径、canonical/component/raw path 解析、固定 Pattern、限流和响应 writer；先写重复斜杠、`%2F`、`%2E`、非法编码、空路径和普通路径测试。
3. **代理授权**：实现 `authorizeAPIProxy`，接入 API Key scope、当前用户连接、canonical ID 限制和隐藏语义；补齐 key wildcard、具体 vMCP、错误 vMCP、另一用户实例、无 GroupMCP、管理可见但无调用资格的负例。
4. **operation 白名单**：接入 02 `Describe`，实现 component-scoped matcher、`operationKey`/legacy name 映射、`ToolOverride` 与 Profile/instance 收窄、同名工具跨组件负例、path 模板歧义负例和 method 405/404 规则。
5. **请求/响应语义**：实现 RawQuery、多值 Header、bounded body、平台 Header 清理、凭证注入由 02 完成、业务 status/body/Header 写出；补 GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS、重复 query、二进制/非 JSON、多值 Header、业务 403/429/5xx、上游 Set-Cookie 测试。
6. **预算与取消**：接入 04 Acquire/Release、绝对 deadline、body 前获取租约、取消传递、429/503/504 映射和有界审计；覆盖客户端断开、Worker kill、队满和慢上游。
7. **文档 API**：新增当前用户可见的 read-only 应用调用契约 API；用假 Worker describe 验证显示名、描述、参数、启用清单、snapshot digest 和不触上游。
8. **UI**：按 UI AGENTS 先运行受影响现有 vMCP specs；新增只读调用文档组件、四语言文案和 colocated browser spec；再启动真实本地页面检查复制、展开、权限为空、Worker 503 和既有功能入口。
9. **文档**：更新 connect-clients 文档与示例，明确平台/业务认证分离、业务错误保留、API Key 范围和已构建应用不依赖 Agent；运行 docs 链接构建。
10. **集成准备**：向 05 提供 P03 测试入口和证据要求；在 01/02/04 完成后运行本地假上游 E2E，不访问真实业务系统。

## 8. 验证证据要求

实施后按依赖逐层运行，不把静态检查写成 E2E：

```bash
go test ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server
go test ./pkg/api/router ./pkg/api/handlers/vmcps
cd ui/user && pnpm exec vitest run src/lib/components/vmcps/<新增或受影响 spec>
cd ui/user && pnpm run i18n:compile && pnpm run check
cd docs && npm run build
```

必须新增的 Go 回归：

- **A03-ROUTE**：raw path 不被重定向/解码破坏；`%2F` 仍为参数、`%2E` 不穿越、重复斜杠策略明确、query 重复和顺序保留。
- **A03-AUTHZ**：无身份、仅 API scope、错误 `mcpServerIds`、另一用户实例、Profile 禁组件/工具、instance 选择禁组件、管理可见但无调用资格均拒绝；正确 scoped key 和当前连接允许。
- **A03-WHITELIST**：禁用 ToolOverride、Profile 收窄、跨组件同名工具、别名/前缀变化、快照更新后消失 operation、路径模板歧义均不能绕过。
- **A03-HTTP**：所有已发布方法、body 位置、Header 位置、RawQuery、非 JSON 请求/响应、多值 Header、业务 403/404/429/5xx 原样保留。
- **A03-AUTH-HEADER**：应用自填 Authorization/authtoken/X-API-Key/Cookie 不能覆盖 02 注入凭证；上游只看到当前用户合成 token。
- **A03-ERROR**：平台错误与业务响应可用状态、Content-Type 和 `X-Obot-Error-Code` 区分；错误不含 token/内部 URL/body。
- **A03-BUDGET**：body 前获取租约、429/503/504、取消传递、lease 幂等释放、请求/响应/审计超限。
- **A03-AUDIT**：成功、业务失败、平台失败、取消都记录有界元数据；扫描测试产物不含合成 token。
- **A03-DOCS**：文档 API 不发业务请求、只返回当前用户授权 operation，UI 展示名称/描述/参数说明/启用清单和复制 URL。

UI 验证：

1. 先运行受影响既有 `VMcpDesigner.svelte.spec.ts`、组件详情/工具配置相关 specs，记录基线。
2. 新增 browser spec 使用可访问 locator 验证调用文档入口、复制、四语言文案、空授权、Worker 503，并断言既有工具配置/Profile/MCP connect 入口仍可见。
3. 实施时启动独立本地 UI/开发服务，在真实浏览器打开 vMCP 详情页面，保存 URL、console 错误和截图；不能用组件单测冒充真实页面验证。

本地 E2E（等待 01/02/04 后，由 05 汇总）：

- 独立临时数据库、随机端口、本地假业务 API、两用户两套运行期合成 token、平台 Key 只授权目标 vMCP。
- 停止测试 Agent 后调用 `/api-proxy/...`，证明不依赖 Agent 在线。
- 假上游断言每个请求的 Authorization/authtoken 只属于对应用户；403 body/status 原样返回。
- 覆盖两个组件同名 operation、两个用户实例、编码 path、重复 query、大 body/大响应、队满、取消、Worker 重启补快照。
- 不使用真实 JWT、不改用户服务、不部署生产；真实企业授权交换和企业 API 标记 `UNVERIFIED`。

## 9. 失败行为

| 失败 | 行为 |
| --- | --- |
| raw path/ID/query 编码无效 | 400，不进入 Describe/Execute |
| Worker describe 不可用或协议错误 | 503 平台错误，不发业务上游 |
| 快照缺失且可安全补送 | 02 仅补送一次；第二次缺失失败，不自动重试 |
| 快照 digest 不匹配 | 平台错误，拒绝执行，不使用 live catalog 覆盖 |
| 组件/operation 被撤权或禁用 | 403/404，不泄露未授权清单 |
| 当前用户缺业务凭证 | 409，不发起上游，不回落管理员/其他用户 token |
| 04 队满/不可用 | 429/503，不绕过全局预算 |
| 请求或响应超限 | 413/502 类平台错误，不返回半截业务成功 |
| 上游 SSE/重定向 | 明确平台错误/返回 3xx 业务结果，不自动携带认证跟随 |
| 客户端断开/deadline | 取消链路关闭上游，审计取消并按 04 回收 |
| Worker 崩溃或传输失败 | 结果不确定，不自动重试；保留平台错误和审计 |
| 业务上游失败 | 原状态/Header/body 返回，不算平台失败 |
| UI 文档 Worker 不可用 | 显示明确错误；已构建应用代理调用能力另行由服务状态决定 |

## 10. 风险与未验证项

- **raw-prefix 改造涉及公共 server**：必须与 02 写集协调，否则会并行冲突。实施前需要单元证明未影响 SCIM、管理 API、UI 和 MCP connect。
- **HTTP 语义子集复杂**：multipart、binary、压缩、多值 Header、编码和 HEAD 组合多。契约未显式支持的 OpenAPI style 必须拒绝，不能偶然执行。
- **业务 `Set-Cookie` 与平台同源**：入站 Cookie 全剥离且 Worker 无 Cookie jar；业务响应 Cookie 仅作为业务响应 Header 保留。需测试认证器会话 Cookie 不混入代理响应。
- **外部平台认证使用 `Authorization` 时无法同时携带业务 Authorization**：文档 API/UI 必须明确推荐 `X-API-Key` 携带平台 Key；02 注入业务 Authorization 前会丢弃请求方同名值。
- **快照与 docs 冷启动**：文档接口依赖 Worker describe；Worker 故障时文档 503，但已构建应用 URL 和授权身份不变。
- **真实 token 交换仍未存在**：本计划只定义缺失/失效语义；真实企业授权、刷新和撤销由 02/05 单列 `UNVERIFIED/GAP`。
- **公共 API/UI 类型冲突**：默认把文档响应限制在内部 UI service 类型，不改 `apiclient/types` 和生成文件；公开客户端会触发共享写集重新分配。
- **本规划未运行**：未启动 Obot、Worker、UI、假上游或真实浏览器；所有实施与 E2E 均为待执行验证。

## 11. 交付物

- 本计划：`/Users/echo/orca/workspaces/obot/openapi-http-proxy/docs/plans/openapi-workers/PLAN.md`。
- 实施阶段交付 `/api-proxy` handler、专属 authz、raw HTTP 请求/响应转换、04 接线、有界审计、应用调用文档 API/UI/文档及回归测试。
- 提供给 00 的契约分歧清单：raw path 专项分支、专用授权入口、P02 接口补充、Header 保留名单、path matcher 歧义策略、audit/server 写集。
- 提供给 05 的本地 E2E 断言和未验证项；不包含真实凭证、历史 JWT、生产部署或集群操作。

## 12. 形式化验证

### 性质

只有当一个请求按 raw escaped segments 唯一匹配目标 component/method 的 path template，且 operation 同时通过 ToolOverride、Profile allowlist 和 instance allowlist 后，代理才可标记为已执行；执行时必须保留原始 escaped path segments 与 RawQuery。违反该性质会把歧义路由、跨组件同名工具、被运营禁用或用户未授权的 operation 发到业务上游，造成越权调用和错误归属。

### 模型与结果

- 完整模型：`docs/plans/openapi-workers/formal/OpenAPIHTTPProxy.tla`；变量对应 PLAN 5.1 的 raw request/route、5.2 的 operation 白名单状态机和 5.3 的 raw 语义。`ResolveRoute` 模拟 component/method/template 匹配与 static precedence，`ResolveAuthorization` 模拟三层 allowlist，`ExecuteAuthorized` 才设置 `upstreamCalled` 并转发 raw path/query。
- 正常配置：`OpenAPIHTTPProxy.cfg`。TLA+ Tools v1.8.0（SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`）、Corretto 11.0.31 生成 613 states / 241 distinct states，四个不变量与死锁检查通过，无错误。
- 失败变体：`OpenAPIHTTPProxyBroken.tla` 在 `RouteMatched` 后跳过白名单；同一验证精确触发 `ExecutionRequiresUniqueAuthorizedMatch`，在 21 states / 21 distinct states 处停止，证明检查能失败。恢复完整模型后全部通过。
- 图表投影：`OpenAPIHTTPProxyChart*.tla/.cfg` 保持 raw parse → unique route → whitelist → execute/reject 决策序列，用严格 raw parser 之后的有限 route shape 替代序列匹配，避免 statechart 运行器与 TLA+ 1.8.0 标准模块 XML 输出不兼容；不放宽完整模型不变量。normal 图表 17 states / 29 steps；broken 图表 20 states / 35 steps，分别给出歧义路由回退与白名单跳过两条最短反例。
- 复跑：`rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh`。脚本校验固定 jar SHA、TLC 输出与退出码、完整 Broken 精确失败、normal/Broken statechart 关键状态和两条预期反例，并生成 `docs/plans/openapi-workers/formal/runs/latest/EVIDENCE.md`。

### 边界

该验证是 P03 planning-complete 设计模型，同步 commit `2be04f025d1c86144e8670d9e9341b263edc28ea`；当前 worktree 尚无业务代理实现。模型抽象了 strict URI parser（输入是校验后的 escaped segments）、上游网络、body、Header 分离、预算和审计；不宣称覆盖 E2E、部署或第 8 节实现回归。实现落地后必须按项目 skill 更新模型映射并复跑。
