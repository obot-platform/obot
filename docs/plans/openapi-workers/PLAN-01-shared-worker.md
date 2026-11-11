# P01 共享 OpenAPI Worker 计划

日期：2026-11-09。分支：`feature/openapi-shared-worker`；基线：`2be04f025d1c86144e8670d9e9341b263edc28ea`。状态：planning；本文件只规划，不表示实现或 E2E 通过。

## 1. 问题与目标

当前每个 OpenAPI 配置由 Obot 动态创建一个 `openapi-mcp:v0.27.2` wrapper 工作负载，并在启动时同步转换一份快照。目标是在固定的一组 Python Worker 副本内加载多份不可变快照，为普通 HTTP 出口和 MCP `tools/call` 提供同一个有界、逐请求认证隔离的执行核心，并支持冷编译隔离、缓存去重、取消、健康检查与重启后的按需恢复。

完成标准：

- 两份及以上快照可在同一 Worker 副本发现和执行，不按文档、接口或用户创建持久进程/容器。
- `operationKey`、旧工具名和显示别名分离；已发布 vMCP 授权不因说明或别名变化漂移。
- 原始 HTTP 输入和工具参数输入收敛到同一执行计划，业务 4xx/5xx 保持原始响应。
- Worker 无 `/mcp` SSE 出口；内部接口只返回有界 JSON。
- 缓存、编译、请求体、响应体、头部和内部 base64 均有硬上限；取消和截止时间传递到上游连接。
- Worker 重启后由 Obot 通过 `describe`/首次 `execute` 补送快照恢复，不依赖本地持久缓存。

## 2. 已核查现有基础

以下均按证据类型标注；本任务尚未运行真实业务 API、历史 JWT、生产服务或集群。

### 2.1 Obot 本地源码事实

`FACT(local)`：

- `pkg/mcp/manager.go:46` 默认 OpenAPI 镜像为 `ghcr.io/obot-platform/mcp-images/openapi-mcp:v0.27.2`；运行后端只有 Docker/Kubernetes 方向，没有固定 Worker 服务模式。
- `pkg/mcp/openapi.go:57` 为每个配置生成端口 `8080`、路径 `/mcp`、readiness `/readyz`，并把规范化 schema 作为 `OPENAPI_SPEC_FILE` 注入。
- `pkg/openapi/import.go:21` 限制单份 schema 为 1 MiB；`pkg/api/handlers/openapi.go:50` 每次保存重新导入 Source，已有部署保留旧快照直到显式更新。
- `pkg/openapi/environment.go:21` 只把 `OPENAPI_BASE_URL` 和凭证 Header 名写入 wrapper 环境，凭证值留在 Obot 请求链。
- `pkg/openapi/headers.go:14` 禁止凭证 Header 覆盖 HTTP/MCP 传输控制头；`pkg/openapi/document.go:210` 拒绝 cookie 参数和保留 header 参数。
- `pkg/mcp/types.go:349` 固定凭证可进入共享 HTTP client 默认 Header；`pkg/mcp/types.go:428` 用户凭证作为逐请求 Header 名。`pkg/mcp/manager.go:491` 已把用户 ID、审计元数据和 Header 值从部署身份排除。
- `pkg/mcp/openapi_test.go:367` 已验证 OpenAPI 用户 Header 是请求级隔离，且共享部署身份不随用户值变化。
- `apiclient/types/vmcp.go:41` 的组件是 catalog entry 快照；`pkg/vmcp/profiles.go:44` 授权使用组件 ID + 原始工具名；`apiclient/types/mcpserver.go:183` 的 `ToolOverride` 只改显示名/描述/启用，不应改变执行身份。
- `pkg/mcp/backend.go:147` 在无专用健康路径时会自动尝试 SSE；OpenAPI 当前有 `/readyz`，但这条公共回退仍是 02/05 必须处理的链路风险。

### 2.2 官方 v0.27.2 与 FastMCP 3.4.7 事实

`FACT(upstream-tag)`：读取官方 `obot-platform/mcp-images` tag `v0.27.2`，提交 `64b62ab7c8862b15bc29b5a5805f3999e486136f` 的 `openapi-mcp/{server.py,network.py,config.py,pyproject.toml}`；`pyproject.toml` 固定 `fastmcp==3.4.7`、`httpx==0.28.1`、`httpcore==1.0.9`。

`FACT(pinned wheel)`：按 `uv.lock` 解析到 `fastmcp-slim==3.4.7` wheel，SHA-256 `6c931a0089705f3f2935428ef9b2bc74ad94140adc64aab84d116d103e694b3a`，并直接读取该 wheel 源码，不用新版文档推断。

关键事实：

- `FastMCP.from_openapi()` 存在于 3.4.7，接受 `client`、`mcp_names`、`mcp_component_fn`、`validate_output` 等；`OpenAPIProvider` 会把所有 route 默认转换为 `MCPType.TOOL`。
- 3.4.7 的 `http_app()`/`run_http_async()` 均接受 `json_response` 与 `stateless_http`。MCP SDK 1.30.0 中 `json_response=False` 时 POST 返回 `text/event-stream`；`stateless_http=True` 只是移除 GET 独立流路由，并不能让 POST 自动 JSON-only。
- v0.27.2 `server.py` 调用 `run_http_async(... stateless_http=True)` 但没有传 `json_response=True`。因此 `SOURCE-INFERENCE`：当前 `/mcp` POST 在常见双 Accept 下会返回 SSE；本任务尚未实际启动镜像，计划中的本地真实端点测试必须验证这个结论。
- `network.py` 的 `IsolatedDirector` 捕获 FastMCP 生成 Header，再从当前 MCP 请求提取声明的凭证 Header，重置请求 Header 后注入；共享连接池没有用户默认 Header/Cookie。这个请求级隔离思路可保留。
- `network.py` 将响应完整读入 `bytearray`，README 明确“无大小限制”；`OpenAPITool.run()` 也调用 `response.json()`/`response.text` 并包装错误。它不能直接满足原始 HTTP 字节语义、8 MiB 硬限制和业务 4xx/5xx 原样返回。
- `server.py` 的 30 秒 `SIGALRM` 只用于启动转换，因为同步解析可能占住事件循环。该机制不能复制到多请求异步路径。
- `config.py` 运行时拒绝非公网 IP，并要求凭证转发使用 HTTPS。它与保存时 Obot `safehttp` 策略方向一致，但会阻断仅公司内网可达的业务 API；新 Worker 需要服务级策略而不是硬编码 public-only。
- FastMCP 默认工具名来自 operationId 前缀/summary/path，slugify 后截断 56 字符，同名追加 `_2`。这能复刻旧名，但不足以作为唯一身份；必须另用 `operationKey`。

### 2.3 契约草案核查结论

`CANDIDATE/finding`，需 G00 冻结：

1. **无 SSE 是当前草案与 v0.27.2 的直接冲突。** 共享 Worker 不应暴露 `/mcp`；02 的 MCP 网关用 `describe`/`execute` 适配。若 G00 要求 Worker 直接暴露 MCP，则必须显式 `json_response=True, stateless_http=True` 并用真实端点测试，不能只设 stateless。
2. **FastMCP `OpenAPITool.run()` 不适合作为共用执行核心。** 它完整缓冲、JSON 化响应并把业务错误变成工具异常。推荐保留 FastMCP 3.4.7 的 OpenAPI IR/兼容性测试，但由 Worker 统一执行规范化后的 HTTP 计划。
3. **`snapshot_required` 只能证明缓存未命中阶段未触达上游。** 该响应必须在进入执行状态机前返回，并用假上游计数证明。快照已补送后的传输失败、Worker 崩溃或截止时间耗尽不能保证上游未执行，必须禁止自动重试。
4. **内部 base64 上限必须由原始字节上限推导。** 原始响应 8 MiB 时，内部编码上限不能仍写 8 MiB。建议 envelope 上限为 `ceil(raw_limit/3)*4 + 64 KiB`，另设头部/元数据上限。
5. **缓存同时受 32 份和 16 MiB schema 字节约束。** 若每份接近 1 MiB，实际最多 16 份；这是设计结果，不是异常。
6. **public-only 运行时策略与公司内网 API 可能冲突。** 默认仍应拒绝 loopback/private/link-local，除非部署显式配置允许的私有 CIDR/域名；无论如何继续做 DNS 全答案校验和 origin 固定。
7. **凭证前缀应在 Obot/02 解析完成。** Worker 只接收本次调用的最终 Header 值，不保存默认值、Cookie 或业务 token；快照摘要只包含 Header 名、required、prefix 等非敏感定义。

## 3. 范围与非目标

### 范围

- 新增 `workers/openapi/` Python Worker、其测试、固定依赖锁和镜像构建定义。
- 内部 `describe`、`execute`、`healthz`、`readyz` 协议实现与无凭证测试样例。
- 多快照计划编译、缓存、并发去重、活跃引用、失效和关闭。
- 原始 HTTP 与工具参数两种输入的共用执行器。
- 逐请求认证隔离、请求/响应/头部限制、取消、无 SSE、错误分类。

### 非目标

- 不修改现有 Go 动态 Docker/Kubernetes 后端，不新增远程 MCP 类型或 Endpoint 顶层对象。
- 不实现 02 的 Obot 服务组装、凭证解析、vMCP 发现桥接或固定 Worker URL 配置。
- 不实现 03 的外部 `/api-proxy/...` 路由、平台身份和发布白名单。
- 不实现 04 的 PostgreSQL 跨副本租约、接口并发和公平排队；Worker 只消费绝对截止时间和调用元数据。
- 不实现 05 的 Deployment/Service/Chart；不改公共 Go 类型或根 `Makefile`。
- 不引入 Eunomia、任意脚本转换、业务权限引擎、默认 jobId 轮询、Redis 或新 Go 框架。
- 不运行真实企业 API、不读取/复制历史 JWT、不把任何业务凭证写入计划、日志或测试产物。

## 4. 推荐架构与内部契约

### 4.1 服务形态

`CANDIDATE`：

```text
Obot core / 02 bridge
  POST /internal/openapi/v1/describe
  POST /internal/openapi/v1/execute
  GET  /healthz
  GET  /readyz
        |
        v
Snapshot Registry + Fixed Compile Helper Pool
        |
        v
Canonical Execution Plan LRU
        |
        v
One Common Bounded HTTP Executor
        |
        v
Original business API
```

- Worker 不监听 `/mcp`，因此不依赖 FastMCP Streamable HTTP transport，也没有独立 SSE GET/POST 流。
- FastMCP 3.4.7 继续作为 OpenAPI IR 和旧工具名兼容性的 pinned 依赖；执行器不调用 `OpenAPITool.run()`。
- ASGI 层复用 FastMCP 依赖中的 Starlette/Uvicorn，不新增 Web 框架。
- 每个 Worker 副本启动固定数量的编译 helper 进程（初始 1，可配置最高 2），不按快照启动持久进程。慢/失控编译只杀死并重建固定 helper，主服务继续处理热调用。

### 4.2 快照与摘要

`schemaDigestV1`：对现有 `OpenAPIConfig.Schema.Raw` 的原始字节做 SHA-256，不在 Go/Python 间重新序列化后计算。

`executionDigestV1` 由不可变 manifest 的固定顺序、长度前缀编码生成，双方用同一算法实现，避免 JSON 转义差异：

- domain tag：`obot-openapi-worker-v1`；
- `schemaDigest`；
- 规范化后的 `baseURL`；
- 非敏感认证定义数组：小写 Header 名、required、prefix，按名排序，值 excluded；
- operation 清单：排序后的 `operationKey`；
- compiler/parameter semantics 版本；
- 内部协议版本。

Worker 在收到快照后重算两个摘要；不匹配返回 `snapshot_invalid`，不进入缓存。相同 executionDigest 的多个 vMCP 组件共享一份计划。

快照 envelope 只包含 schema bytes、baseURL、认证定义和摘要；不包含用户 ID、平台 Key、业务 token、Cookie 或固定凭证值。schema bytes 使用 base64，接收上限按 1 MiB 原始字节推导。

### 4.3 operation 与工具映射

- `operationKey = uppercase(method) + " " + schemaPathTemplate`。
- `legacyToolName` 按 FastMCP 3.4.7 provider 规则生成：优先 operationId `__` 前段，否则 summary，否则 method/path；slugify、最长 56、冲突追加 `_2`。用实际 3.4.7 `tools/list` 兼容样例锁定。
- `operationId` 仅保留元数据；缺失、重复或改名不改变 operationKey。
- `describe` 返回每个 operation 的 `operationKey`、`legacyToolName`、`operationId`、summary/description、输入 schema、HTTP method、path template、参数位置和媒体类型。
- 02/03 负责把 vMCP 显示别名、前缀和授权映射到原始名；Worker 不理解组件授权，也不允许别名改变 operation。

### 4.4 `describe`

请求：`protocolVersion`、`executionDigest`、可选不可变快照 envelope。描述请求永远不携带用户业务凭证，也不请求业务 API。

结果：

- 命中：`status=ready`、digest、有界 operation 清单和输入 schema。
- 未命中：`status=snapshot_required`、`retryable=true`、`upstream_attempted=false`。Obot 只能补送一次快照。
- 快照无效/超限/摘要不符：`snapshot_invalid` 或 `snapshot_too_large`，不可作为业务结果。

并发 `describe` 未命中时按 digest 合并编译；同 digest 同时到达只编译一次。编译失败移除 pending entry，所有等待者得到同一安全错误；不缓存失败对象。

### 4.5 `execute`

请求最小字段：

- `protocolVersion`、`requestID`、`executionDigest`、`operationKey`；
- `input.kind=http`：原始 method 校验值、schema 相对 path、RawQuery、允许的端到端 Header、body base64；或
- `input.kind=tool`：按 describe 输入 schema 生成的参数对象；
- 绝对截止时间（Unix 毫秒）和 04/02 传来的不透明预算元数据；
- 逐请求最终认证 Header 名/值；
- 首次调用可选快照 envelope。

执行规则：

- 缓存未命中在任何业务连接前返回 `snapshot_required`；补送后同 `requestID` 只允许进入执行状态机一次。
- 快照补送后的传输错误、取消、Worker 崩溃或超时不自动重试；调用方只能把结果标记为不确定，除非上游本身提供幂等键。
- HTTP 模式必须用 path template 精确匹配 operation，不允许应用指定 host、absolute URL、scheme 覆盖或另一 operation。
- 工具模式按计划绑定 path/query/header/body。初始支持 path simple、query form/explode 与不 explode、space/pipe delimited、deepObject、header simple、JSON/JSON-subtype、form-urlencoded、multipart 和 base64 binary；cookie 与保留传输 Header 拒绝。
- RawQuery 保留顺序和重复参数；path 参数做百分号编码并拒绝路径穿越。
- 平台 Authorization/Cookie/代理/Header 与 Worker 服务认证不穿透；认证 Header 最后应用，用户或工具参数不能覆盖。
- 共享连接池只按 origin 复用，不保存默认业务 Header、Cookie 或用户 token。

业务结果：

```json
{
  "kind": "business",
  "requestID": "...",
  "status": 403,
  "headers": [{"name":"...","values":["..."]}],
  "bodyBase64": "..."
}
```

业务 4xx/5xx 是成功取得的 business result，不是 Worker 错误。内部错误单独给出 `code/message/requestID/retryable/phase`，message 不包含 URL、Header 值、schema 内容或响应体。

### 4.6 编译隔离与缓存

固定 helper 使用 `spawn`，通过 stdin/stdout 交换有限 JSON。helper 内使用 FastMCP 3.4.7/OpenAPI 解析结果生成 JSON-safe plan，不回传可执行对象、pickle 或凭证。主进程按以下顺序执行：

1. 摘要与大小检查；
2. 编译队列票据；
3. helper 编译；
4. plan 字节数/operation/参数/节点数上限检查；
5. `executionDigest` 重算；
6. 原子插入 LRU。

初始限制（均可配置，作为硬上限而非目标值）：

| 项目 | 默认 |
| --- | ---: |
| schema 原始字节 | 1 MiB |
| 缓存份数 | 32 |
| schema 累计字节 | 16 MiB |
| 编译后 plan 序列化字节 | 16 MiB |
| operations / parameters | 512 / 4096 |
| JSON Schema 节点 | 20,000 |
| 同时冷编译 | helper 数，默认 1 |
| 冷编译等待队列 | 4 |
| 单次冷编译 | 30 秒，且小于总截止时间 |
| 请求 body 原始字节 | 1 MiB |
| 响应原始字节 | 8 MiB |
| 响应解压后字节 | 8 MiB |
| 响应 Header 序列化 | 64 KiB |

行为：

- 到达缓存份数/字节/plan 上限时先淘汰 LRU 中非活跃计划；没有可淘汰项则返回 `snapshot_cache_full`，不突破硬上限。
- 执行开始前对计划加活跃引用；响应、错误或取消后释放；活跃计划不可淘汰。
- 相同 digest 并发加载共享同一个 future；后到请求不能触发第二次编译。
- 编译 helper 超时被 supervisor 终止并替换，固定池回到健康数后才 readiness 成功。
- 关闭时先拒绝新请求，等待既有请求到各自绝对截止时间，关闭编译池与连接池；超时请求按取消处理并记录无凭证元数据。
- Worker 无本地持久缓存；重启后由 Obot `describe` 或首次 `execute` 补送快照。

### 4.7 HTTP 执行、限制与取消

- 使用共享 `httpcore.AsyncConnectionPool` 或等价无 Cookie transport；每次构造全新 `httpx.Request`，`follow_redirects=false`、`trust_env=false`。
- 连接前解析全部 DNS 答案并套用服务级 egress policy；拒绝默认 localhost/private/link-local/multicast，显式生产配置只允许指定私有 CIDR/域名。连接的 origin 必须等于快照 baseURL origin。
- 不自动跟随重定向；3xx 作为业务结果返回，不携带认证发起第二个请求。
- 上游 `Content-Type: text/event-stream` 在读 body 前返回 `upstream_sse_unsupported` 并关闭连接。
- 广告 `Accept-Encoding: gzip, deflate` 时必须流式解压计数；同时限制压缩字节和解压后字节。返回原始压缩 body 与原始端到端 Header 以保留语义，不把截断数据伪装为完整业务响应。
- 无可靠 `Content-Length`、分块和压缩响应同样按读取字节流硬限制，超过即关闭并返回 `response_too_large`。
- 只保留端到端响应 Header；`Connection`、`Transfer-Encoding` 等 hop-by-hop Header 不返回，`Set-Cookie` 多值保留但不会进入共享 Cookie jar。
- 绝对截止时间派生 connect/read/write/pool 超时。入站断开、截止时间或 04 取消触发 transport close；上游已完成的副作用不回滚，租约回收语义由 04 保守处理。

### 4.8 健康、认证与日志

- `/healthz` 只证明进程和事件循环可用，返回有界 JSON。
- `/readyz` 检查 ASGI、缓存、固定编译池、HTTP transport、服务认证配置和协议版本；无快照时也应 ready，因为支持冷加载。编译 helper 不健康时返回 503。
- 生产服务认证默认 mTLS；本地验收可用测试进程生成的短期 bearer。业务凭证不参与 `/healthz` 或 `/readyz`。
- 请求日志只记录 requestID、digest、operationKey、阶段、状态、限制和耗时；`Authorization`、声明认证 Header、Cookie、body、schema 与完整内部地址脱敏或不记录。
- readiness 响应可暴露非敏感版本、缓存份数/字节、活跃引用和上限，不暴露 baseURL、Header 定义详情或租约内容。

## 5. 文件归属与依赖

### 5.1 本任务独占写集

计划中的实施只新增/修改：

```text
workers/openapi/pyproject.toml
workers/openapi/uv.lock
workers/openapi/Dockerfile
workers/openapi/README.md
workers/openapi/openapi_worker/__init__.py
workers/openapi/openapi_worker/app.py
workers/openapi/openapi_worker/protocol.py
workers/openapi/openapi_worker/snapshot.py
workers/openapi/openapi_worker/compiler.py
workers/openapi/openapi_worker/compiler_helper.py
workers/openapi/openapi_worker/registry.py
workers/openapi/openapi_worker/operations.py
workers/openapi/openapi_worker/executor.py
workers/openapi/openapi_worker/httpclient.py
workers/openapi/openapi_worker/errors.py
workers/openapi/tests/**
workers/openapi/testdata/**
```

不修改 `pkg/`、`apiclient/`、根 `go.mod`、根 `Makefile`、UI 或 chart。若最终需要根级构建入口，由 00 在 G00 后明确分配，不在 P01/I01 私自改公共写集。

### 5.2 依赖

- Python `>=3.12`。
- `fastmcp==3.4.7`（实际解析到 `fastmcp-slim==3.4.7`），只使用其 OpenAPI IR/工具名兼容接口，不使用 `OpenAPITool.run()` 作为执行器。
- `httpx==0.28.1`、`httpcore==1.0.9`、Starlette/Uvicorn/Pydantic 按 v0.27.2 lock 的兼容版本锁定。
- 测试：`pytest`、`pytest-asyncio`；上游请求用注入 transport 或本地临时 `127.0.0.1` 假服务，不访问互联网业务 API。
- 编译 helper 只用标准库进程通信；不引入 Redis、Celery、Docker SDK、Kubernetes client 或任意脚本引擎。

### 5.3 跨 feature 接口

| Feature | P01 需要的接口 | P01 提供 |
| --- | --- | --- |
| 02 | 固定 Worker URL、服务认证、Obot 侧 canonical digest、凭证解析后的最终 Header、MCP `tools/list`/`tools/call` 桥接 | `describe`/`execute` JSON 契约、operation 映射、错误码 |
| 03 | 外部 HTTP path/query/body/header 的匹配与允许端到端 Header | 同一 `input.kind=http` 执行结果和字节语义 |
| 04 | 绝对截止时间、取消信号、预算/租约元数据 | 执行阶段、取消/完成结果、无 Worker 内部自动重试 |
| 05 | 固定副本数、资源限制、探针、优雅关闭时长、镜像构建上下文 | health/readiness 语义、端口、日志和可观测指标 |
| 00 | 冻结协议字段、错误码和摘要算法 | 本文件发现的契约分歧和测试样例 |

## 6. 实施步骤

1. 建立 `workers/openapi` 包、协议模型、错误分类和 ASGI 骨架；只落测试和实现所需最小文件。
2. 实现 digest、快照 envelope 解码、1 MiB 检查、固定长度前缀 executionDigest 和失败测试。
3. 实现固定编译 helper 与 JSON-safe plan；用 FastMCP 3.4.7 真实 IR 生成 operation、输入 schema、参数位置和旧工具名。
4. 实现多快照 LRU、并发 future、活跃引用、hard limits、缓存满和关闭测试。
5. 实现共用执行器和安全 transport：HTTP/tool 两种输入、逐请求认证、origin/DNS policy、重定向/SSE/响应限制。
6. 实现 `describe`/`execute`、`snapshot_required` 状态机和 requestID 单次执行 fence。
7. 补齐 health/readiness、服务认证、无凭证日志、ASGI 生命周期和优雅关闭。
8. 增加本地真实端点集成测试：随机端口启动 Uvicorn，验证 JSON-only、取消、恢复和两份快照并发。
9. 锁定依赖、镜像构建和测试命令；把仍未覆盖的 02/03/04/05 集成点列为未验证，不写成通过。

## 7. 精确测试场景

所有上游均为本地假服务或注入 transport；凭证是每次测试进程生成的合成值，断言后不落盘。测试输出不得包含 token 原文。

### 协议与快照

- `T-PROTO-01`：协议版本不支持、请求 JSON 超限、重复字段/未知大字段、base64 非法和 digest 不匹配分别返回安全错误。
- `T-PROTO-02`：两份不同 schema/baseURL/认证定义生成不同 executionDigest；同 schema 多 vMCP 组件命中同 digest。
- `T-PROTO-03`：1 MiB schema 通过，1 MiB+1 字节在解码前拒绝；base64 envelope 按 raw limit 推导。
- `T-PROTO-04`：快照含凭证值、Cookie、用户 ID、绝对 URL 覆盖或外部 ref 时拒绝。

### operation 与 FastMCP 兼容

- `T-MAP-01`：operationId、operationId 含 `__`、无 operationId 用 summary、无 summary 用 method/path、超过 56 字符、同名冲突，分别与 FastMCP 3.4.7 实际 `tools/list` 名称一致。
- `T-MAP-02`：说明、别名、前缀、operationId 改名不改变 `operationKey`；旧工具名变化会生成新 describe 结果但不跨组件授权。
- `T-MAP-03`：OpenAPI 3.0.x 与 3.1.x、本地 `$ref`、path/query/header/body 参数生成计划；cookie/外部 ref/动态 ref/保留 Header 拒绝。
- `T-MAP-04`：path/query/header/body 绑定及重复 query、编码 path、binary body、JSON subtype、form 和 multipart 的工具输入执行请求与假上游断言一致。

### 缓存与冷编译

- `T-CACHE-01`：两份快照同时 `describe`/`execute` 均可用，进程数不随文档数增加。
- `T-CACHE-02`：相同 digest 100 个并发未命中只编译一次；一个失败等待者不能让其他同 digest 请求得到半初始化计划。
- `T-CACHE-03`：分别打满份数、schema 字节和 plan 字节；只有非活跃 LRU 被淘汰，全部活跃且满时返回 `snapshot_cache_full`。
- `T-CACHE-04`：慢/递归/失控 schema 在冷编译 deadline 被 helper 隔离终止；主事件循环继续响应已热 operation，helper 被替换。
- `T-CACHE-05`：编译队列满、编译失败、摘要不符和缓存关闭时 pending 状态清理，无 future/引用泄漏。
- `T-CACHE-06`：Worker 重启后本地无快照也可 readiness；Obot 补送两份快照后恢复执行。

### 共用执行与认证隔离

- `T-EXEC-01`：同一无参数 operation 分别以 HTTP 模式和 tool 模式调用，假上游观测到完全相同 method/path/query/header/body。
- `T-EXEC-02`：两个用户并发同一路径，各自合成 token 只出现在对应请求；请求后共享 client 无默认 Header/Cookie，下一请求无串用。
- `T-EXEC-03`：用户自填 Authorization、Cookie、代理 Header 和工具 header 参数不能覆盖平台注入认证；保留 Header 拒绝。
- `T-EXEC-04`：业务 400/401/403/500、非 JSON、空 body、多值 Header 和压缩 body 原样返回；平台错误与业务结果分类不同。
- `T-EXEC-05`：RawQuery 顺序、编码、重复 key、PATCH/PUT/HEAD/OPTIONS 和原始 body 字节保留。
- `T-EXEC-06`：上游 302/307 不自动跟随，不第二次携带认证；DNS 混合公私答案、origin 漂移和非允许私有地址拒绝。

### snapshot_required 与不重复执行

- `T-SNAP-01`：缓存未命中返回 `snapshot_required`，假上游计数为 0；同 requestID 补送快照后业务执行计数为 1。
- `T-SNAP-02`：补送快照后的连接失败、读取失败和 Worker 进程被杀死不触发 Worker/测试客户端自动重试。
- `T-SNAP-03`：第二次自动补送、requestID 重用但缺 `snapshot_supplied_once` fence、digest 与 requestID 不一致均拒绝。
- `T-SNAP-04`：崩溃发生在上游发送前/后的两种测试只断言可观察事实：前者计数 0，后者计数 1 且调用结果不确定；不宣称通用 exactly-once。

### 无 SSE、限制与生命周期

- `T-NOSSE-01`：真实 Uvicorn 随机端口端点上 `initialize`、`tools/list`、`tools/call` 语义请求不适用；内部 `describe`/`execute` 响应必须是 `application/json` 且有界。
- `T-NOSSE-02`：`GET/POST /mcp` 不存在，不返回 SSE；未带 JSON Accept 的内部 POST、上游 `text/event-stream` 和混合 Accept 均不会打开事件流。
- `T-LIMIT-01`：请求 body 1 MiB+1、响应 `Content-Length` 谎报、分块超限、gzip/deflate膨胀超限和 Header 超限均明确失败并关闭连接。
- `T-LIMIT-02`：8 MiB 响应、多值 Header 和合法压缩响应通过，内部 base64 envelope 不因推导上限误拒。
- `T-CANCEL-01`：入站断开、绝对截止时间和 04 取消传递到假上游；上游 handler 观测连接关闭，Worker 请求槽位最终归零。
- `T-CANCEL-02`：慢上游、慢编译、Worker kill 和 shutdown 排空互不阻塞热调用超过配置预算。
- `T-LIFE-01`：无快照 ready、编译 helper 异常 ready=false、helper 恢复后 ready=true；`/healthz` 与 `/readyz` 故障语义不同。
- `T-LIFE-02`：优雅关闭等待活跃请求，超限取消并关闭连接/编译池；再次 readiness 前不接受新请求。
- `T-SEC-01`：无效服务认证、无认证、过期测试 token、跨租户 digest 请求拒绝；日志和测试产物扫描不到合成 token。

## 8. 验证证据要求

实施后本 feature 至少运行：

```bash
cd workers/openapi
uv sync --locked
uv run pytest -q
uv lock --check
docker build -t obot-openapi-worker:local .
```

其中 Docker build 只证明镜像可构建；不表示 K8s 部署或真实业务 API 通过。若本机不能运行 Docker，将该项标为未验证，不能用静态检查替代。

本地真实端点测试必须记录：命令、随机端口、请求/响应 Content-Type、开始/结束时间、假上游观测值、缓存计数、进程数和清理结果。不得记录凭证原文。P01 完成只代表 Worker 单元/本地集成，通过 02/03/04/05 之前不得写全链路通过。

## 9. 失败行为

| 阶段 | 行为 |
| --- | --- |
| 请求/协议无效 | `invalid_request`，不读业务上游 |
| 快照缺失 | `snapshot_required`，`upstream_attempted=false`，允许 Obot 补送一次 |
| 快照无效/超限/digest 不符 | `snapshot_invalid`/`snapshot_too_large`，不缓存 |
| 缓存满且无可淘汰 | `snapshot_cache_full`，不新增对象 |
| 编译排队满/超时 | `compile_busy`/`compile_timeout`，主服务继续可用 |
| operation/path/header/body 不匹配 | `invalid_request` 或 `operation_not_found`，不发业务请求 |
| 缺认证或服务认证失败 | `unauthenticated`，无业务请求 |
| 业务上游 4xx/5xx | business result，保留状态、端到端 Header 和 body |
| 上游 SSE/过大/编码不支持 | Worker 平台错误并关闭上游连接，不返回半截成功 |
| 取消/截止时间 | 关闭上游，返回或记录 cancelled/deadline_exceeded；04 负责租约 |
| 传输失败或 Worker 崩溃 | 结果不确定，禁止自动重试 |
| 内部异常 | `internal_error`，日志 requestID 和阶段，不泄露凭证/URL/body |

## 10. 风险与未验证项

- **FastMCP 私有 IR 依赖**：虽版本完全锁定，升级仍可能变化。用 pinned wheel 兼容测试和 compiler version digest 阻止静默漂移；重大升级需 G00 重新评审。
- **编译 helper 复杂度**：固定进程池必须处理 macOS/Linux spawn 差异、僵尸进程和信号；先实现单 helper，压测后再决定是否配置 2。
- **HTTP 语义完整度**：multipart、二进制、压缩、多值 Header 和编码组合多。测试先覆盖契约声明的子集；未覆盖的 OpenAPI style 必须显式拒绝而不是偶然执行。
- **exactly-once 不可普遍证明**：只能证明 snapshot miss 未触达上游和没有自动重试；上游副作用幂等性由业务系统决定。
- **内网 egress 策略**：默认拒绝私网可能影响公司内 API；允许私网必须是显式部署策略并继续防 DNS rebinding。
- **当前 v0.27.2 SSE 行为**：源码和 MCP SDK 已证明默认分支，但尚未启动官方镜像实测；实施测试不得把这个静态结论写成已运行验证。
- **跨副本预算**：Worker 自身限制连接/请求体/响应体，不能替代 04 的全局接口并发和租约。
- **真实部署**：尚未访问集群、RBAC、镜像仓库或生产网络；05 单独验证。

## 11. 交付物

- 本计划：`/Users/echo/orca/workspaces/obot/openapi-shared-worker/docs/plans/openapi-workers/PLAN.md`。
- 实施阶段交付 `workers/openapi/`、固定 lock、Dockerfile、单元/本地集成测试和测试证据。
- 提供给 00 冻结的协议样例、错误码表、digest 测试向量和 FastMCP 3.4.7 兼容性发现。
- 明确移交给 02/03/04/05 的集成缺口，不在 P01 中宣称完成。

## 12. 形式化验证

日期：2026-11-09。状态：verified（设计级 TLA+，非实现/E2E/部署）。

### 性质与伤害

**关键性质**：冷缓存请求只有在恰好补送一份有效快照、且同一 `requestID` 恰好获取一次执行 fence 后，才能开始业务上游调用。违反时，Obot 可能在未加载已发布不可变快照时执行，或在传输结果不确定后重放有副作用的业务请求。

### 模型与工具

- 模型：`docs/plans/openapi-workers/formal/SnapshotFencing.tla`。
- 通过配置：`docs/plans/openapi-workers/formal/SnapshotFencing.cfg`。
- 故意破坏配置：`docs/plans/openapi-workers/formal/SnapshotFencingBroken.cfg`，只移除 `requestID` fence，并把故障注入限定为一次复用；不变量未削弱。
- `/statechart` 视图：`docs/plans/openapi-workers/formal/snapshot-fencing.view.mjs`。
- 工具：TLA+ Tools `v1.8.0` asset，TLC build `2026.11.06.222357`；选择 TLA+ 是因为该性质是事件顺序与生命周期状态机，不适合用输入样例或 SMT 代替。

模型变量对应 `request.phase`、digest 绑定、快照补送次数、执行状态机进入次数和上游尝试次数；动作覆盖冷 miss、`snapshot_required`、有效/无效快照、编译成功/失败、fence、上游开始、业务完成/传输不确定、同 request 复用和 digest 重绑。模型抽象排除热缓存、LRU/活跃引用、并发编译去重、helper 取消、重启/关闭、HTTP 细节和业务 exactly-once；这些不得由通过结果推导。

### 结果

- 基线：TLC 无错误、无死锁，65 states generated / 33 distinct，深度 10；全部不变量通过。
- Broken：TLC 预期失败；完整 `/statechart` 重跑 57 distinct states / 120 steps。`ExecutionEnteredAtMostOnce` 与 `UpstreamRequiresOneSnapshotAndOneFence` 在“同一 request 再次进入执行”处失败；若其后传输失败，`NoRetryAfterUncertainOutcome` 也失败。
- 正常图：`docs/plans/openapi-workers/formal/evidence/base/chart.mmd`。
- 反例图：`docs/plans/openapi-workers/formal/evidence/broken/chart.mmd`。
- 复跑：`rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh`。
- 详细命令、时间、版本和边界：`docs/plans/openapi-workers/formal/EVIDENCE.md`。

项目本地复跑技能：`.codex/skills/verify-openapi-shared-worker/SKILL.md`。该技能要求模型与设计同步、读取 `/statechart` 报告，并在 Broken 变体保持可失败后才推进同步 commit。
