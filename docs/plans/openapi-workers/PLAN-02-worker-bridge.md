# P02 Obot Workers 接入计划

日期：2026-11-09。worktree：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge`。状态：planning complete；本文只是实施计划，尚未修改业务实现，也没有运行端到端功能。

## 1. 问题与目标

OpenAPI 目录条目当前仍按配置启动独立 `openapi-mcp` 工作负载。目标是给 Obot 增加一个显式的固定共享 Worker 执行模式：OpenAPI 工具发现和调用改为访问运营预部署的固定 Worker URL；vMCP 目录、组件快照、用户实例、Profiles、平台 Key 授权和加密凭证存储继续使用；Agent MCP 与后续普通 HTTP 代理共用同一 operation 定义和统一执行入口。

完成标准：

- 新模式不初始化 Docker client，不读取 kubeconfig / in-cluster 配置，不创建 Namespace、Deployment、Pod、Service、RBAC 或逐接口工作负载。
- Docker 与 Kubernetes 后端原样保留；选择新模式时，动态托管的 NPX/UVX/containerized 能力明确不可用，OpenAPI、remote/vMCP 聚合和无托管能力继续可用。
- 每次工具发现或调用都从当前平台身份重新解析 vMCP 实例、组件快照和该用户业务凭证；不把用户 token 放进 pooled MCP session、共享 HTTP client 默认 Header、配置快照、Worker 缓存或日志。
- Agent MCP 出口仍暴露同一工具定义，实际响应为有界 JSON、stateless，无 SSE 请求和探测回退；验证看实际方法和响应类型，不只检查 `DisableStandaloneSSE`。
- 为 01/03/04 提供稳定的 Go 公共类型与 `Executor` 入口，并明确生成产物和路由写集移交。

## 2. 已核查现有基础

以下结论来自当前 worktree 源码，属于源码事实；未标注为已运行的行为。

### 2.1 启动与后端

- `pkg/services/config.go` 只把 `MCPRuntimeBackend` 为 Kubernetes 时才构建 local Kubernetes config、client、cache 和 local router；数据库、存储 router、身份、凭证、vMCP 控制器仍由通用路径组装。`NewSessionManager` 在 `pkg/mcp/manager.go` 只接受 Docker 和 Kubernetes/K8s。
- Docker backend 初始化即创建 Docker client、检测网络、清理历史容器并启动事件 watcher；Kubernetes backend 会尝试创建 MCP Namespace，并依赖 client/cache/router。二者都不能直接满足“无 Docker daemon、无 kubeconfig、无工作负载控制权”的运行目标。
- `pkg/controller/routes.go` 中的 vMCP、MCPServer、MCPServerInstance 控制器主要操作数据库资源，不能整体关闭；`pkg/controller/controller.go` 中 local K8s router 已按 nil 跳过。新模式应保留数据库控制器，只让动态工作负载控制路径返回明确 unsupported。
- OpenAPI 现有配置在 `pkg/mcp/openapi.go` 写入 schema 文件、`/mcp` 路径和 `/readyz`；`pkg/openapi/environment.go` 证明执行使用保存的 normalized JSON snapshot，不重新读取 Source，schema 上限 1 MiB，业务凭证值不在环境变量中。

### 2.2 vMCP 与调用链

- `pkg/mcp/vmcp.go` 将 vMCP 解析为组件连接；共享组件使用用户 `MCPServerInstance`，个人组件使用实例所属 server。`pkg/mcp/action.go` 在每次解析配置时读取 server 配置和实例凭证，并把 `UserAllowed` Header 变成 `PassthroughHeaderValues`。
- `pkg/api/handlers/mcpgateway/handler.go` 对 vMCP 创建进程内 MMMCP；组件 URL 指回 `/mcp-connect/{connection_id}`。`composite_auth.go` 已意识到 pooled component session 会重放过期请求头，因此 loopback token 会按同一身份重签。
- 依赖的 `obot-platform/mmmcp@e619b4f7dc01` 源码核查结果：普通 component `CallTool/Discover` 每次新建下游 session，`DisableStandaloneSSE` 为 true；composite POST 使用 `Stateless:true`、`JSONResponse:true`。但 `OpenRuntime` 仍可打开 persistent standalone SSE session，Obot 自身 `loadSession` 也缓存 `ClientSession`。因此不能只用配置名宣称全部链路无 SSE。
- `pkg/mcp/backend.go` 的 `ensureServerReady` 在没有专用 health path 时先 POST Streamable HTTP，失败后自动 GET SSE。当前 OpenAPI 已配置 `/readyz`，实际会走专用 GET；实现仍必须删除 OpenAPI 新链路的 MCP 探测和回退分支，防止配置丢失时回归。

### 2.3 授权与凭证

- `pkg/vmcp/profiles.go` 以组件 ID + 原始工具名授权；`pkg/api/authz/mcpid.go` 对 `/mcp-connect` 重新解析当前用户、vMCP 访问和 API Key `authorized_mcp_ids`，不依赖管理端列表可见性。
- `pkg/gateway/client/credential.go` 支持按 context/name 揭示和加密凭证。`pkg/mcp/action.go` 中用户 Header 使用 `userID + "-" + instanceName` 的精确 context；`pkg/vmcp/credential.go` 另有 vMCP fixed 和 instance 配置 context。
- 风险：`serverConfigForAction` 会读取 server/catalog 级 credential context，`configureHeaders` 可把非 `UserAllowed` Header 固化为 `ServerConfig.Headers`；`serverFromMCPServerInstance` 同时合并 server 固定配置和实例配置。该机制适合运行时静态配置，但不能作为个人业务 token 的 fallback。
- 现有代码没有“业务系统登录后交换用户业务 token”的完整闭环。凭证存储、按实例保存和 reveal 不等于授权交换已完成；真实企业对接仍是缺口。

### 2.4 可复用测试

规划后的实现优先扩展这些既有测试，而不是另起孤立测试：

- `pkg/mcp/openapi_test.go`：snapshot、Header 前缀、缺失凭证、共享 Header 请求作用域、镜像选择和 destination policy。
- `pkg/mcp/backend_test.go`：专用 health path 不探测 MCP；需新增“无 SSE GET 回退”的负例。
- `pkg/mcp/action_test.go`、`pkg/mcp/vmcp_action_test.go`、`pkg/mcp/client_vmcp_test.go`：connect ID、共享组件连接和工具列表。
- `pkg/api/authz/mcpid_test.go`、`mcpconnect_test.go`、`vmcp_*_test.go`：当前用户、实例归属、API Key scope 与 vMCP Profiles。
- `pkg/api/handlers/mcpgateway/handler_test.go`、`composite_auth_test.go`、`generic_tool_call_test.go`：MMMCP 身份、loopback token、工具调用和审计。
- `pkg/gateway/client/credential*_test.go`、`pkg/vmcp/credential_test.go`：加密存储、context 与实例配置。
- `pkg/services/config_test.go`：后端选择和配置解析边界。

## 3. 范围与非目标

### 范围

- 新增 opt-in `MCPRuntimeBackend=openapi-worker` 固定执行模式及配置解析。
- 新增共享 Worker 客户端、公共请求/响应类型、snapshot digest 计算和统一 `Executor`。
- OpenAPI 组件在固定模式下的 JSON-only stateless MCP bridge，复用现有 `/mcp-connect/{id}` 授权和 vMCP 聚合。
- 每调用身份、实例、工具授权、业务 Header 与固定服务认证解析。
- 后端能力边界、显式错误、审计脱敏和无 SSE 行为测试。
- 为 03 预接线和移交路由写集；为 04 预留预算上下文。

### 非目标

- 不实现 Python Worker 本体、全局预算状态机、应用 `/api-proxy` 路由、K8s 清单或真实集群部署；分别属于 01/04/03/05。
- 不移除 Docker/Kubernetes backend，不把所有 MCP 切到固定 Worker，不新增远程 MCP 类型或 Endpoint 顶层对象。
- 不实现业务角色/数据权限引擎，不解析业务 JWT 推导权限，不引入任意脚本转换或默认异步轮询。
- 不在 02 中完成真实企业 token 交换。只定义缺失/失效/未配置语义和模拟凭证验收边界。
- 不访问、复制或使用历史真实 JWT；测试只用运行时合成 token。

## 4. 候选设计与接口契约

以下为 02 的候选设计，需 00 冻结；其中与 DRAFT-0.1 不冲突的部分按 DRAFT 执行，分歧在本节列出。

### 4.1 后端与配置

新增配置属于 `services.Config` / `mcp.Options`：

- `MCPOpenAPIWorkerURL`：唯一固定 Worker base URL，必须 HTTPS；dev mode 显式允许 localhost HTTP。
- `MCPOpenAPIWorkerAuthTokenFile`：推荐的服务认证 token 文件路径。
- `MCPOpenAPIWorkerAuthToken`：仅本地开发/测试可用的显式 token；生产优先文件挂载，避免长期环境明文。
- `MCPOpenAPIWorkerRequestTimeout`、连接池、TLS CA 等传输配置。

约束：

- `openapi-worker` 模式必须同时配置 Worker URL 和服务认证；OpenAPI 调用前做一次有界 readiness 检查，但不在启动时阻塞 Obot 全局就绪。
- 服务认证只用于 Obot→Worker `/internal/openapi/v1/*`，建议独立 `Authorization: Bearer`；它不进入 executionDigest、snapshot、业务上游、审计和日志。
- 固定 Worker URL 不套用 `DisallowLocalhostMCP` 的远程 MCP 地址策略；它是受信内部执行器配置，仍强制 scheme、TLS 和固定 host 校验，且不能由应用请求覆盖。
- Worker 客户端只复用连接池和 TLS 状态，不保存用户业务 Header、Cookie、token 或 `http.Client` 默认认证。

### 4.2 后端能力语义

新增 `fixedOpenAPIWorkerBackend` 或等价实现：

- OpenAPI：不部署工作负载，返回可调用的固定 Worker 执行引用；readiness 使用 `/readyz`，健康使用 `/healthz`。
- remote：保持现有直连能力。
- vMCP：作为逻辑聚合，不需要部署。
- NPX/UVX/containerized：`ensure/deploy/restart/logs/details/capacity` 返回 `ErrNotSupportedByBackend`，错误文案明确“该模式未启用动态托管”。
- MCP Network Policy、SecretBinding、image pull secret 等 K8s 相关管理能力在固定模式不可用；含必需 SecretBinding 的 OpenAPI 配置应保存/启动前拒绝，不能把必需凭证静默变成缺失。

`/api/version` 增加机器可读的 hosting capability 字段，而不是让 UI 猜 engine 字符串；动态托管操作以该能力禁用或返回明确错误。此改动如涉及 UI 文案，实施时另读 `ui/user/src/AGENTS.md` 并完成四语言目录，默认优先 API 能力与后端错误。

### 4.3 公共协议与类型

新增包 `pkg/openapiworker`，由 02 独占：

- `types.go`：冻结协议版本、`schemaDigest`、`executionDigest`、`operationKey`、describe/ensure/execute 请求响应、错误码、预算租约上下文和有界 body 编码。
- `client.go`：服务认证、TLS、连接池、绝对截止时间、一次 `snapshot_required` 补送逻辑和错误映射。
- `executor.go`：面向 03/04 的稳定 Go 接口。
- `resolver.go`：vMCP/组件/实例/工具授权/凭证的每调用解析。
- `bridge.go`：JSON-only stateless MCP component adapter。

统一入口候选：

```go
type Reference struct {
    VMCPID      string
    ComponentID string
    ConnectID   string
}

type Invocation struct {
    Reference    Reference
    OperationKey string
    Input        Input
    Deadline     time.Time
    Budget       BudgetContext
}

type Executor interface {
    Describe(ctx context.Context, ref Reference) (Description, error)
    Execute(ctx context.Context, invocation Invocation) (Result, error)
}
```

接口语义：

- 03 只传平台认证已得到的调用上下文、operation 和原始输入；不能自选用户 ID、上游 host 或业务 token。
- 02 在 Executor 内重新解析当前用户对应 vMCP instance、组件快照、Profiles 授权和业务凭证。
- 04 的 lease 对 02 是不透明上下文，只随请求传给 Worker 并参与取消/回收；02 不实现全局预算算法。
- Worker 返回业务状态码/body 时是执行结果；协议、认证、传输、预算、上游 SSE/超大响应是平台错误，二者不可混淆。
- `snapshot_required` 只允许在尚未发起业务上游调用的证明下补送不可变快照一次；之后失败不自动重试 execute。

### 4.4 Snapshot 与 operation 身份

- `schemaDigest = SHA-256(component.CatalogEntry.Manifest.OpenAPIConfig.Schema.Raw)`，使用已保存的 normalized JSON 原始字节，不重新跨语言序列化。
- `executionDigest` 输入为协议版本、schemaDigest、有效 BaseURL、Header 名称/位置/前缀/必需性/作用域、operation 清单与 Go 侧桥接协议版本。凭证值、平台身份、requestID、服务 token 和临时 Header 不参与。
- Header 定义来自 vMCP 组件快照及实例配置策略，而不是当前目录可变说明；目录或别名更新不静默替换已发布组件。
- `operationKey = strings.ToUpper(method) + " " + pathTemplate`。原始工具名保持授权身份；显示别名、前缀和描述仅影响展示。
- describe 返回原始工具名、显示名、参数 schema、method/path/参数位置和启用状态。Go bridge 和 03 使用同一 Description 生成 MCP tool 与 HTTP 白名单。

与 DRAFT 的补充分歧：DRAFT 只说“非敏感参数/认证 Header 定义”，但当前 Header policy 可来自 catalog/server/vMCP instance 多层。02 建议冻结为“以发布组件 snapshot 的 manifest.Config 为基础，叠加该用户 instance 的 userAllowed policy；不读取 live catalog Header 定义”。固定非用户 Header 不作为个人业务认证 fallback。

### 4.5 每调用凭证与授权语义

新增 OpenAPI 专用 resolver，不复用含 fallback 的 `serverConfigForAction`：

1. 从平台认证结果取得 userID；API Key scope 只作为准入，不能替代当前用户解析。
2. 解析 connect ID/vMCP instance，校验归属、vMCP Profiles、组件启用和原始工具名授权。
3. 读取组件 snapshot 与有效 executionDigest。
4. `UserAllowed` 业务 Header 只从精确 instance credential context 读取：`userID + "-" + MCPServerInstanceName`，vMCP instance 配置使用 `vmcp-instance/{instanceID}` 的组件限定 key。
5. 对业务认证 Header 禁止 fallback 到 catalog/server/admin 固定 credential 或其他用户实例。缺必需值、值无效或凭证过期时返回 `business_authorization_required`（普通 HTTP 建议映射 409）。
6. 非认证类服务固定配置未来必须显式标记 service scope 才进入 Worker；现有 OpenAPI Header 定义本质上都是业务认证，02 保守地不把非 userAllowed Header 当作用户 token 转发。需要真正服务级 Header 时由 00 增加显式作用域字段，不能借用管理员配置。
7. SecretBinding 在固定模式不可解析为用户业务 token；必需绑定直接失败，可选绑定记录配置未满足。

真实 token 交换边界：

- 02 只提供“凭证存在、可解密、可按实例读取、缺失/失效明确失败”的接口和测试。
- 业务授权 URL、交换协议、刷新和撤销属于后续企业适配，不能因凭证表可用而标记完成。
- 05 使用模拟业务系统与合成 token 验证双用户隔离；真实企业交换永远单列为 UNVERIFIED/GAP。

### 4.6 Agent MCP JSON-only bridge

固定模式下，OpenAPI 组件继续通过现有 `/mcp-connect/{connect_id}` 暴露：

- authorization 完全复用 `checkMCPID` / API Key scope / vMCP Profiles。
- handler 先识别 OpenAPI runtime，不再 `LaunchServer`，改为 Go bridge。
- bridge 使用 Worker `describe` 动态生成工具 schema；`tools/list` 受组件 tool overrides、prefix、启用清单和 instance 授权收窄。
- `tools/call` 先解析 operationKey，再调用统一 `Executor.Execute`，每次调用重新执行 resolver。
- Go SDK Streamable HTTP handler 显式配置 `Stateless:true`、`JSONResponse:true`，并在外层拒绝 SSE-only GET、旧 `/messages` 依赖和 `Accept` 不含 JSON 的请求；混合 JSON+SSE Accept 时只返回 JSON。
- 不创建持久 component runtime，不把 Header 放进 mmmcp server config；loopback 身份仅用于回到 Obot 后再次解析当前用户。
- OpenAPI readiness 只 GET `/readyz`；删除/绕过 `ensureServerReady` 的 POST MCP 探测和 SSE fallback。

验证必须断言实际请求方法、Content-Type、状态码和 body，不允许只断言 transport 配置字段。

## 5. 文件归属与写集

### 02 独占写集

实施 I02 只允许写：

- `pkg/openapiworker/*.go` 与对应 `*_test.go`
- `pkg/mcp/worker_backend.go`、`pkg/mcp/worker_backend_test.go`
- `pkg/mcp/openapi_bridge.go`、`pkg/mcp/openapi_bridge_test.go`
- `pkg/mcp/backend_test.go` 中新增无 SSE 回退测试
- `pkg/services/config.go`、`pkg/services/config_test.go`
- `pkg/api/handlers/mcpgateway/handler.go`、`handler_test.go`
- `pkg/api/handlers/version.go` 及相应测试（如新增 capability）
- `docs/plans/openapi-workers/PLAN.md`

如实际实现需要小幅调整 `pkg/mcp/manager.go` / `backend.go` 常量与接口，必须先报 00 扩展写集；不得同时让 01/03/04 编辑这些公共文件。

### 生成产物

优先把跨 feature Go 类型放在 `pkg/openapiworker`，不触发生成。若交叉审核决定把公共响应放入 `apiclient/types`，则 02 独占修改源类型，并在同一变更中运行：

- `go generate`
- 提交 `apiclient/types/zz_generated.deepcopy.go`
- 提交 `pkg/storage/openapi/generated/openapi_generated.go`
- 提交 `pkg/storage/openapi/generated/zz_generated.openapi_modelname.go`

这些生成文件不得由 01/03/04 并行修改。默认方案是不放入 `apiclient/types`，减少共享生成冲突。

### 移交 03 的路由写集

02 预接线 `Services.OpenAPIWorkerExecutor` 和构造参数后，I03 独占：

- `pkg/api/router/router.go`
- `pkg/api/handlers/apiproxy/*.go` 与测试
- `pkg/api/authz` 中 `/api-proxy` 资源识别与测试（若需要）
- 应用调用文档与 UI 文档入口

03 不得改 `pkg/services/config.go` 或 `pkg/openapiworker`；如接口不足，先由 00 回派 02。

### 04 接口边界

04 独占预算包和状态表。02 在公共 `BudgetContext` 中保留不透明 lease 字段；I04 若需要修改 `pkg/openapiworker/types.go`，必须在 I02 settlement 后由 00 单独授权，避免并行写公共类型。

## 6. 分步实施

1. 冻结协议类型：新增 `pkg/openapiworker/types.go`，实现 digest、错误和边界常量；先写协议序列化测试。
2. 实现 Worker client：服务认证、TLS、readiness、describe/ensure/execute、一次 snapshot 补送、绝对 deadline 和无默认用户 Header 测试。
3. 增加固定 backend：解析 `openapi-worker` 配置，禁止 Docker/K8s 初始化，显式 unsupported 动态托管，保留 remote/vMCP；扩展 services/config 测试。
4. 实现每调用 resolver：vMCP instance、组件 snapshot、Profiles、原始工具名、精确用户凭证 context、缺授权错误和脱敏测试。
5. 实现统一 Executor：组装 executionDigest、operationKey、输入位置绑定和 Worker 调用；用 httptest Worker 验证双用户 token 不串用。
6. 接入 MCP gateway：OpenAPI 组件改走 Go bridge，不 `LaunchServer`；完成 tools/list、tools/call、覆盖/前缀、授权和审计测试。
7. 删除 OpenAPI 新链路探测回退：专用 readiness；新增非 200、连接失败和异常 Content-Type 时不发 SSE GET 的测试。
8. 暴露 capability 与文档边界：version/metadata 显示动态托管不可用，错误说明可用 runtimes；不隐藏既有功能或伪装成功。
9. 移交 03/04：确认 Executor/BudgetContext 编译可用，记录路由写集和接口快照；不实施应用代理或预算算法。

## 7. 验证证据要求

### 单元与集成

- `go test ./pkg/openapiworker ./pkg/mcp ./pkg/services ./pkg/api/authz ./pkg/api/handlers/mcpgateway`
- 新增测试必须覆盖：
  - Worker URL/token 缺失、错误 scheme、过期 token、TLS 失败。
  - `openapi-worker` 模式不构造 Docker/K8s client，动态托管 API 返回 unsupported。
  - 两用户并发调用同 component，上游只观察到各自合成 token；无管理员 fallback。
  - 必需业务凭证缺失/无效返回 `business_authorization_required`，不发起上游调用。
  - Profiles 禁止组件/工具、另一用户实例、API Key scope 不匹配均拒绝。
  - snapshot 更新生成新 executionDigest；旧发布不被目录说明静默替换。
  - describe 不带凭证且不发业务请求；`snapshot_required` 补送一次且不重复执行。
  - MCP initialize/tools/list/tools/call 响应实际为 JSON、stateless；GET SSE、SSE-only Accept、旧 message endpoint 不被调用。
  - 审计与日志只含脱敏 Header 名，不含 token、服务认证或完整敏感输入。

### 无 Docker/K8s 本地真实验证

在 I01 提供 Worker 后执行：

1. 使用独立临时 SQLite 数据库、独立端口、本地 auth，不触碰用户现有 Obot 服务。
2. 启动本地 01 Worker、合成业务 API 和两套合成用户凭证。
3. 显式设置 `DOCKER_HOST=unix:///definitely-unavailable/docker.sock`、`KUBECONFIG=/definitely/unavailable/kubeconfig`，并确认不在 Kubernetes pod service account 环境。
4. 以 `MCPRuntimeBackend=openapi-worker` 启动 Obot，先验证 version capability，再导入两份 OpenAPI、构建 vMCP、并发调用 MCP tools/call。
5. 记录进程列表和测试断言，证明没有 Docker client、K8s client、容器、Pod、逐接口 Service 或工作负载创建；固定 Worker 副本数保持不变。
6. 用合成上游返回 403、429、5xx、非 JSON、分块和大 body，验证业务响应语义和有界失败不被包装成伪成功。

如果 01 Worker 尚未可用，02 只能标记单进程 Go bridge/client 测试通过，不能标记本地 E2E 通过。

### 规划期检查

本次规划只要求：

- `PLAN.md` 落盘且包含任务要求的所有章节。
- `git diff --check` 通过。
- `git status --short` 只出现本文件。
- 不运行无关全量测试，不把源码静态核查写成 E2E 通过。

## 8. 失败行为

- Worker URL/token 未配置、TLS 失败、readiness 失败：OpenAPI 调用返回 503，Obot 其他功能不因此启动 Docker/K8s。
- 动态托管操作：返回 backend unsupported，不隐式回落 Docker 或 Kubernetes。
- 用户/实例/组件/工具无资格：403 或现有隐藏资源语义；不泄露资源存在性。
- 业务凭证缺失/失效：409 + `business_authorization_required`；不使用管理员或其他用户 token。
- Worker 返回 `snapshot_required`：仅补送不可变快照一次；无法证明未触上游或第二次缺失则失败，不自动重试。
- Worker/协议/预算/传输错误：独立错误码和 requestID；业务 4xx/5xx 原样作为 Result 返回。
- SSE-only 或上游事件流：明确 unsupported/502 类错误，不建立长流，不回退 SSE。
- 请求取消或 deadline：传递到 Worker 与上游连接；审计记录取消，凭证仍不落盘。
- 服务认证泄漏风险：任何日志/审计测试发现 token 原文即阻断合并。

## 9. 风险与契约分歧

| 风险/分歧 | 影响 | 处理 |
| --- | --- | --- |
| DRAFT 未定义 Header policy 来自 snapshot 还是 live catalog | 可能导致授权身份或 digest 漂移 | 00 冻结为发布组件 snapshot + 用户 instance 叠加 |
| 现有固定 Header 可被解析为服务默认 Header | 有用户 token 串用/管理员 token 冒用风险 | OpenAPI 个人业务认证只用精确 instance context；服务级 Header 需显式 scope |
| 真实业务 token 交换不存在 | 不能宣称授权闭环 | 02 只做缺失/读取/隔离语义，真实企业交换标 GAP |
| mmmcp `OpenRuntime` 支持 persistent standalone SSE | 单看普通 CallTool 配置会漏风险 | OpenAPI bridge 不走 OpenRuntime，并用实际请求负例验证 |
| `DisableStandaloneSSE` 只是配置名 | 不能证明响应无 SSE | 验收检查方法、路径、Accept、Content-Type 和响应 body |
| API Key scope 可能只覆盖 vMCP ID | 03 若按 component 绕路会越权 | 03 必须复用 02 resolver 与 `/mcp-connect` 等价授权 |
| SecretBinding 依赖 K8s client | 固定模式无法解析 | 必需绑定显式失败；未来服务 scope 需 00 冻结 |
| 公共类型并行修改 | 01/03/04 合并冲突 | 默认避免生成文件；`pkg/openapiworker` 由 02 独占，后续变更经 00 调度 |
| Worker readiness 影响启动 | 可能把固定 Worker 故障放大为 Obot 不可用 | 启动不阻塞全局，OpenAPI 调用时返回 503 |

## 10. 交付物

- 固定 Worker backend 与服务组装配置。
- `pkg/openapiworker` 公共协议、client、resolver、Executor 和 MCP JSON bridge。
- 每调用身份/凭证/授权与无 SSE 回归测试。
- 动态托管能力边界 API 与错误语义。
- 给 03 的路由写集、给 04 的 `BudgetContext` 边界、给 01 的协议分歧清单。
- 本 PLAN 与后续实施证据记录；不包含真实凭证、历史 JWT、生产部署或集群操作。

## 11. 形式化验证

### 11.1 已验证性质

模型 `OpenAPIWorkerResolver` 覆盖 P02 的每调用身份、实例、组件 snapshot、原始工具授权和业务凭证解析链路。关键性质是：只有调用者本人拥有目标实例、Profiles 与原始工具授权全部通过，并且精确 instance credential context 中存在该调用者的有效业务凭证时，请求才允许派发到 Worker。若违反，一个用户的 OpenAPI 请求可能携带另一用户或管理员凭证访问业务系统，导致跨租户数据泄露、越权写入和审计归属错误。

TLA+ 变量与设计对应如下：`caller` 是平台认证得到的 userID；`requestedInstance` 是 connect ID 解析出的 vMCP instance；`component/operation` 来自发布 snapshot 与原始工具名；`credentialSource` 是被读取的 credential context；`credential` 是业务 Header 的合成值；`phase` 表示解析状态。`AuthenticateCaller`、`SelectInstance`、`AuthorizeInstanceAndProfiles`、`ResolveSnapshotAndOperation`、`AuthorizeOperation`、`ResolveCredential` 和 `DispatchToWorker` 分别对应 PLAN 4.5 与 4.6 的 resolver 顺序。

### 11.2 模型抽象与边界

模型使用两个用户、两个实例、一个组件、一个工具和一个必需业务 Header；`u1:i1` 有合成凭证，`u2:i2` 故意缺失凭证。Profiles 与工具授权以有限布尔矩阵表示，snapshot 视为不可变并省略 schema/execution digest 的字节级计算。凭证值只用于区分身份，不建模加密实现、业务 token 刷新、真实企业授权交换、HTTP 传输、重试、预算、审计、日志脱敏或上游业务语义。模型也未证明 snapshot_required 单次补送或 JSON-only 无 SSE；这些性质由 I02 的代码级回归测试覆盖。

### 11.3 结果与复跑

通过配置 `formal/OpenAPIWorkerResolver.cfg` 检查不变量 `ExactCallerCredentialOnly`、`TypeOK` 与 `NoDeadlock`；破坏配置 `formal/OpenAPIWorkerResolverBroken.cfg` 将 `ResolveExactCredential` 替换为可从任意实例借用有效凭证的 fallback，并用同一组规则复跑。2026-11-09 最终复跑中，恢复模型 18 个 distinct states 全部通过，破坏模型 19 个 distinct states 且仅 `ExactCallerCredentialOnly` 精确失败，反例为 `u2` 请求 `i2` 时 fallback 选择 `u1/i1` 凭证。证据、状态数、时间、工具版本和 Mermaid 状态图存放在 `docs/plans/openapi-workers/formal/`；项目本地复跑入口是 `.codex/skills/verify-openapi-worker-bridge/SKILL.md`。形式化通过只覆盖上述设计模型，不代表 Go 实现、E2E 或部署验收已经通过。
