# R03 集体交叉审核报告

日期：2026-11-09。审核 worktree：`/Users/echo/orca/workspaces/obot/openapi-http-proxy`。状态：complete（审核完成；下列 P1 未解决前 G00 不能冻结实施）。

## 1. 审核范围与方法

本次完整读取了 00 的 `ROADMAP.md`、`CONTRACTS.md`、`ACCEPTANCE.md`、`PLAN.md`，交接与架构说明，01—05 的 `PLAN.md`、formal 模型/配置、`EVIDENCE.md`（如存在）和项目本地 verify skill，并核对了 R02 报告与 R01 留下的五模型复跑摘要。

真实源码核对集中在普通 HTTP 出口、平台 Key/vMCP 授权、OpenAPI 快照、Header、原始响应 writer、凭证 fallback 和预算 source identity：

- 平台 Key：`pkg/gateway/server/apikey_auth.go:31-100`、`pkg/gateway/types/apikeys.go:35-90`。
- vMCP/组件/工具授权：`pkg/api/authz/mcpid.go:33-104`、`pkg/api/authz/mcpid.go:115-160`、`pkg/api/authz/mcpid.go:231-312`、`pkg/vmcp/profiles.go:25-103`、`pkg/vmcp/profiles.go:129-166`。
- 快照与 HTTP 子集：`apiclient/types/openapi.go:60-74`、`pkg/openapi/import.go:21-23`、`pkg/openapi/import.go:57-103`、`pkg/openapi/document.go:20-30`、`pkg/openapi/document.go:76-105`、`pkg/openapi/headers.go:9-31`。
- 路由与响应包装：`pkg/api/router/router.go:611-625`、`pkg/api/server/server.go:186-195`、`pkg/api/server/server.go:257-312`、`pkg/api/server/server.go:358-361`、`pkg/api/server/server.go:438-466`。
- 凭证绑定与 fallback 风险：`pkg/mcp/action.go:269-332`、`pkg/mcp/action.go:407-445`、`pkg/mcp/types.go:349-375`、`pkg/vmcp/credential.go:56-81`、`pkg/gateway/client/credential.go:76-96`。
- 预算来源引用：`pkg/storage/apis/obot.obot.ai/v1/vmcp.go:32-55`、`pkg/mcp/source.go:7-12`、`pkg/controller/handlers/mcpcatalog/mcpcatalog.go:449-465`、`pkg/controller/handlers/mcpcatalog/vmcp.go:176-229`。

另在当前 Go 1.27.0 环境读取 `net/http.ServeMux` 文档，确认其按段 unescape、保留 `%2F` 作为段内数据，但会清洗重复斜杠和 `.`/`..` 并重定向。没有访问真实业务 API、历史 JWT、生产服务、集群或用户现有 Docker 服务。

## 2. 总体结论

**Verdict：G00 不能冻结。**

00 的产品边界与当前源码事实基本相符：`/api-proxy` 尚不存在；API Key 只有 vMCP/MCP 级 `mcpServerIds`，没有 component/operation 独立 scope；现有授权身份是组件 ID + 原始工具名；OpenAPI 快照是 normalized JSON；统一响应 writer 会引入平台语义；固定 Worker、Go bridge、预算租约和代理实现均不存在。

阻断点主要是跨 feature 契约没有收敛：预算租约的获取归属和 deadline 释放语义在 02/03/04 间冲突；02 Executor 与 01 executionDigest 的 Go 侧生成来源不足；raw-prefix 与业务响应 writer 的公共写集尚未划出。若不冻结，会导致重复预算获取、全局额度超发、错误用户凭证绑定或原始 HTTP 语义破坏。

## 3. 发现

### R03-01 [P1] 预算获取归属与调用顺序在 02/03/04 间不兼容

- **状态**：CONFIRMED。
- **影响**：P04 要求平台认证、vMCP/component/operation 资格和业务凭证解析完成后才入队/获取租约，并把“Executor 调用前后接线”归属 02；P03 则计划由代理在读取 body 前 `Acquire`，再把 `BudgetContext` 传给 02；P02 的 `Invocation.Budget` 又没有说明由谁创建、释放或是否会在 Executor 内再次获取。并行实现会造成双重占额、03 绕过 02 授权预检，或缺失业务凭证的请求先消耗全局额度。
- **依据**：P04 PLAN 4.2 的固定顺序是凭证解析→source/operation 解析→预算→Worker，P04 9.3 将 `Manager.Acquire/Release` 接线交给 02；P03 PLAN 5.5 要求“在读取 body 前完成 operation 匹配和 04 Acquire”，6.3 直接定义 `Lease`；P02 PLAN 4.3 只有 `Invocation.Budget` 字段和“随请求传递”的语义。源码中 admitted principal 与 API Key 归因只在 `pkg/api/server/server.go:257` 之后进入 context，凭证解析又在现有 `pkg/mcp/action.go` 的 server/instance 配置链中，当前没有可复用的单一入口。
- **责任 feature**：00 冻结唯一归属；02/03/04 按同一接口实现。
- **建议**：明确一条路径。推荐 03 先通过 02 的无凭证 `Describe/ResolveExecution` 完成 principal、实例、组件、operation、Header 与业务凭证存在性检查；03 获取 lease 并包装 bounded body reader；02 `Execute` 只消费 lease、重做授权与凭证解析，不再二次 Acquire。也可以把 Acquire 全部收进 02，但必须把 body 传递从 `[]byte` 改为可取消 bounded reader，并说明 Content-Length/分块场景。
- **解决条件**：冻结版给出唯一 Acquire/Release owner、前置授权检查、body 读取时机、重复获取防护和 typed error；编译级测试证明错误 Key、无实例、无组件、禁用 operation、缺业务凭证均在 Acquire 前拒绝，且 02/03 不会各自创建租约。

### R03-02 [P1] 02 Executor 与 executionDigest 的 Go 侧来源不足以支撑 01/03

- **状态**：CONFIRMED。
- **影响**：01 的 `executionDigest` 包含排序 operation 清单、Header 定义、BaseURL 和编译协议版本；冷缓存 `describe/execute` 又必须携带该 digest。当前 Go 导入器只验证并保存 normalized schema，不输出 operation 清单。P02 接口也没有 `PrincipalContext`、raw `HTTPInput`、按用户收窄的 `ResolvedDescription`、Header 分类、typed error 或 `BudgetSourceID`。结果是无法安全生成冷启动请求，或 Go/Python 各自生成不同 digest、03 自行猜测授权与身份。
- **依据**：P01 PLAN 4.2—4.5；P02 PLAN 4.3 的 `Reference/Invocation/Executor` 与 4.4 的 digest 输入；P03 PLAN 6.1 要求的字段；源码 `pkg/openapi/import.go:148-165` 和 `pkg/openapi/document.go:76-105` 明确 importer 不生成工具/operation 清单。
- **责任 feature**：02 主责，01 提供跨语言测试向量，03 提供消费方契约，00 冻结。
- **建议**：在 `pkg/openapiworker` 冻结 canonical manifest/executionDigest 生成器，或明确 Obot 冷启动先携带 schemaDigest + raw snapshot 由 Worker 返回 canonical executionDigest 后再执行；不能让 03 解析内部 Worker JSON。接口必须从 admitted principal 取得用户与 API Key 归因，禁止调用方自填 userID。
- **解决条件**：00 契约包含字段矩阵、固定长度编码、Go/Python 同 schema/不同 Header policy 的相同与不同 digest 向量；02/01 互操作测试证明冷 miss 可补送、同 schema 多组件共享计划、旧 digest 不命中；03 编译测试证明其无法自选用户、组件、operation、上游 host 或凭证。

### R03-03 [P1] raw-prefix 分支与业务响应 writer 的公共写集未冻结

- **状态**：CONFIRMED，与 R02-05 相同结论。
- **影响**：普通 mux 会清洗或重定向部分路径；统一 `headersResponseWriter` 会默认添加 `X-Content-Type-Options`/`Content-Type`，认证刷新后可能回放平台 `Set-Cookie`，`/api/` 平台响应还会附加缓存控制语义。若直接复用，业务 status/Header/body 不再是原始响应。
- **依据**：P03 PLAN 3 第 1/4 项、5.1、5.3 和 6.4—6.5；源码 `pkg/api/router/router.go:611-625` 尚无 `/api-proxy`，`pkg/api/server/server.go:192-195` 总是包 `headersResponseWriter`，`server.go:308-312` 回放 refreshed cookies，`server.go:438-450` 写默认 Header，`server.go:358-361` 对 `/api/` 写 no-cache。`pkg/api/server/server.go` 当前不在 02 移交写集，P03 也只是“00 明确授权后”才可写。
- **责任 feature**：00 划分写集，03 实现，02 避免并行修改公共 server 组装。
- **建议**：G00 将 raw-prefix helper 或 server 专用分支独占划给 03，并规定平台错误 writer 与业务 writer 分离；认证、限流、活动审计仍复用平台链，但平台 Cookie/默认 Header 不得进入业务结果。
- **解决条件**：I03 前写集记录落盘；回归覆盖 `%2F`、`%2E`、重复斜杠、非法编码、无 Content-Type/空 body、上游多值 Header/Set-Cookie、平台认证刷新，并证明管理 API、SCIM、UI 和 `/mcp-connect` 响应不变。

### R03-04 [P1] 03 的 deadline “交还租约”与 04 保守回收语义冲突

- **状态**：CONFIRMED。
- **影响**：总截止时间到达时，调用方可能已把请求发送给 Worker/业务系统，无法证明上游停止。若 03 收到 504 后立即 `Release`，而 04 的安全性质要求 uncertain expired lease 占用到 `deadline + reclaim_grace`，全局并发会在旧业务请求仍在执行时放行新请求，突破跨副本额度。
- **依据**：P03 PLAN 5.4 错误表写“504：取消上游并交还租约”；P04 PLAN 7.1/7.3 明确无法确认 Worker 停止时不提前释放，回收器只在 `deadline + reclaim_grace` 后释放；F04 的 `ConfirmedReleaseInsideLease` 反例正是 deadline 处提前释放。P01 PLAN 4.7 也承认上游已发生的副作用不能回滚。
- **责任 feature**：03/04 语义对齐，00 冻结。
- **建议**：deadline 后只能把请求标记为 `expired`/结果不确定并关闭下游等待；仅当 Worker 在未触达业务上游前确认取消，或已返回可观察业务结果时才能立即幂等释放。否则进入 04 reclaim grace。
- **解决条件**：契约写明 deadline、客户端断开、Worker crash、取消确认、业务结果、传输失败六种状态的 release/reclaim 分支；用假上游长请求和两 Obot/两 Worker 验证 grace 前上游峰值不超限、grace 后可恢复且无重复 active。

### R03-05 [P2] 平台 Key/vMCP 准入未进入 02/03 形式化覆盖，也未列为边界

- **状态**：CONFIRMED coverage gap。
- **影响**：P03 要求 API Key scope、vMCP/实例资格、组件资格和 operation 白名单全部实时校验；但 F02 模型只覆盖 caller/instance/profile/tool/credential，没有 API Key authorized scope；F03 模型只覆盖 component/method/template 与 ToolOverride/Profile/instance allowlist，也没有 vMCP scope。模型通过不能发现“Key 只授权 vMCP A 却调用 vMCP B”的绕过。
- **依据**：P03 PLAN 第 1 节与 5.1/5.2；源码 `pkg/gateway/server/apikey_auth.go:76-100` 写入 `authorized_mcp_ids`，`pkg/api/authz/mcpid.go:238-264` 校验 canonical/legacy/instance scope；F02 `OpenAPIWorkerResolver.tla` 的变量只有 `caller/requestedInstance/component/operation/credential...`；F03 `OpenAPIHTTPProxy.tla` 的 whitelist 只有 override/profile/instance。
- **责任 feature**：02/03。
- **建议**：至少把 API Key scope 与目标 vMCP 变量加入其中一个模型，注入“用户可访问 A 的 Key 请求 B”和 wildcard/legacy alias 反例；若仍留在代码测试，则两个 skill/EVIDENCE 必须明确声明平台身份与 Key scope 不在模型覆盖内，不能宣称完整授权模型。
- **解决条件**：模型或验证 skill 边界更新并复跑；实现回归覆盖无 Key、错误 `mcpServerIds`、wildcard、canonical vMCP、legacy alias、另一用户实例和管理可见但无调用资格。

### R03-06 [P2] raw path matcher 的 percent-decode/静态段等价规则未冻结

- **状态**：CONFIRMED design ambiguity。
- **影响**：Go 1.27 `ServeMux` 对路径按段 unescape，`%2F` 不是分隔符，但静态 pattern `/a` 可匹配请求 `/%61`；P03 的专用 parser 虽避免清洗重定向，却未说明静态段应按 decoded octets、raw escaped bytes 还是规范化 UTF-8 比较。Go 与 Python 实现不一致时，同一请求可能一边 404、一边匹配另一个 operation，影响授权和 404/405 语义。
- **依据**：P03 PLAN 5.1/5.3 与 P01 PLAN 4.5 只写“raw escaped segments/template 精确匹配/参数重编码”；当前 `go doc net/http.ServeMux` 明确 segment-by-segment unescape 与 path sanitizing 行为；F03 模型输入已经是 parser 校验后的 raw segment，不覆盖 parser 等价规则。
- **责任 feature**：00 冻结，01/02 同算法，03 复用。
- **建议**：明确规定：先按 raw `/` 分段；静态段按 percent-decoded octets 比较，解码后出现 `/`、NUL/control 或非法 UTF-8 时按未匹配/400 处理；参数段保留原 escape 并在重编码后不得引入新的 `/`；大小写和非 ASCII 字节规则固定；等价/歧义模板拒绝。
- **解决条件**：共享测试向量覆盖 `/it%65ms`、`/items/a%2Fb`、`/%2E`、`//`、大小写、非 ASCII、非法 escape、重复模板；Go/Python matcher 对 404/400/405 和 selected operation 结果完全一致。

### R03-07 [P2] HTTP input 的 OpenAPI 参数校验边界未定义

- **状态**：CONFIRMED design ambiguity。
- **影响**：契约把无效 path/query/header/body 参数归为平台 400/415/422 且不得触达上游，但 P01 的 HTTP mode 只要求精确匹配 operation 和保留输入，没有说明是否执行 required 参数、类型、枚举、body media type、multipart、unknown field 或重复参数 schema 校验。若直接透传，缺参/坏参会变成业务 4xx，与平台错误语义冲突；若随意校验，又可能改变原始 HTTP 语义。
- **依据**：00 CONTRACTS 第 8 节；P03 PLAN 5.4 错误表；P01 PLAN 4.5 的 HTTP mode 和 4.7；源码 `pkg/openapi/document.go:211-228` 仅拒绝 cookie 参数和保留 Header 参数，不生成运行时校验器。
- **责任 feature**：00 冻结，01 实现，02/03 消费同一 typed error。
- **建议**：定义“HTTP mode 最小结构校验”：method/path template、必需 path/query/header/body/media type、二进制/base64、声明的重复语义；不支持 style 显式 `unsupported_parameter_style`。不要把业务系统自身的语义校验前移到平台。
- **解决条件**：假上游计数为 0 的负例覆盖缺 required、非法类型、错误 media type、非法编码、保留 Header、不支持 style；合法重复 query/多值 Header/binary body 原样透传并被上游观测。

### R03-08 [P2] 响应协议子集和平台保留 Header 语义不完整

- **状态**：CONFIRMED design ambiguity。
- **影响**：01 内部结果只建模一个最终 status、Header 列表和 body 字节，无法表达多个 1xx、`101 Switching Protocols`、upgrade、HTTP trailers 或连接层语义；P03 又规定剥离业务响应中的 `X-Obot-Error-Code`，这与“上游端到端 Header 原样保留”冲突。若不显式拒绝或保留，可能返回伪成功、丢失 Header 或误标平台错误。
- **依据**：00 CONTRACTS 第 4/7/8 节要求响应语义、多值 Header 和错误标识保留；P01 PLAN 4.5/4.7 的 business result 只有 `status/headers/bodyBase64`；P03 PLAN 5.3/5.4 剥离同名 `X-Obot-Error-Code`。
- **责任 feature**：00 冻结协议子集，01/03 实现相同行为。
- **建议**：明确只支持单一最终 HTTP response；上游 `101`、upgrade、trailer 声明和不支持协议返回 502/5xx 类平台错误。对 `X-Obot-Error-Code` 二选一：将其声明为平台保留 Header 并允许剥离（文档和 schema 测试说明业务方不能依赖），或业务响应原样保留、平台错误只用状态/JSON body 区分。
- **解决条件**：测试覆盖 `100/101`、upgrade 请求、带 Trailer 声明、业务同名 `X-Obot-Error-Code`、多值 Header、无 body/204/304；断言不会输出半截业务成功或误删未冻结的 Header。

### R03-09 [P2] P04 sourceRef 的现有规范化不足以保证同源共享额度

- **状态**：CONFIRMED source limitation；实际别名数据需实现时验证。
- **影响**：P04 用 `sourceRef + operationKey` 作为全局预算键，并要求同一目录来源经多个 vMCP/component 共享额度。现有 `SourceIDForURL` 只去掉 scheme 前缀和尾斜杠；host 大小写、默认端口、用户信息、多余路径分隔或等价来源别名会生成不同 `sourceID::entryKey`，把同一接口额度拆成多行，跨副本限额失效。
- **依据**：P04 PLAN 4.1；源码 `pkg/mcp/source.go:7-12`，`pkg/controller/handlers/mcpcatalog/vmcp.go:176-186` 直接使用该值，`mcpcatalog.go:449-465` 仅按 `::` 拼接/解析。
- **责任 feature**：04 主责，00 冻结来源身份，02 暴露。
- **建议**：为 catalog source URL 定义结构化 canonicalization（scheme 归一、host lowercase、移除默认端口和 credentials、path 规则、entryKey trim/case），并保留旧 reference 迁移/兼容策略；无法唯一确定时继续 `budget_identity_unavailable`，不得回退 component/vMCP/schema digest。
- **解决条件**：PostgreSQL 测试用同一 API 的等价 URL 别名、两个 vMCP、两个 component、legacy retained snapshot 和缺失 reference 场景，断言共享一行或明确拒绝；状态表无重复 budget key。

### R03-10 [P2] “不自动重试”未落实为 Go transport 行为和 Worker 重启语义

- **状态**：CONFIRMED plan gap，实际 transport 行为待实现验证。
- **影响**：Go HTTP transport 可在复用连接失效时对幂等请求做底层重试；P01 的 requestID fence 又是 Worker 内存态，Worker 崩溃/重启后可能丢失。若 Obot client 在不确定传输结果后重发同一 requestID，新 Worker 可能视为新请求并再次触达业务系统，违反 no-retry 契约。
- **依据**：P01 PLAN 4.5、9 和 F01 property；P02 PLAN 4.3/6 只写“之后失败不自动重试 execute”，没有禁用 transport retry、连接重放或 Worker 重启后的 requestID 持久化/拒绝策略。
- **责任 feature**：02 主责，01 提供 requestID 语义，04 确认结果不确定时租约回收。
- **建议**：02 client 明确禁用自动重试/重放（包括 transport 层），每次业务 execute 只能有一次 Worker attempt；Worker 重启后若 requestID 状态不可恢复，也必须由 Obot 侧禁止重试。连接池只复用未发送请求的连接，不把 requestID 幂等性外推为业务 exactly-once。
- **解决条件**：注入死连接/半发送/Worker kill 的测试断言假上游调用计数和 Obot 重试次数；Go client 在 `httptrace`/自定义 transport 下没有第二次写入；审计保留 uncertain outcome。

### R03-11 [P2] Worker DNS 校验缺少 IP pinning/TOCTOU 说明

- **状态**：NEEDS VERIFICATION。
- **影响**：P01 要求解析全部 DNS 答案并固定 origin，但如果校验后 `httpx` 再次解析 DNS，攻击者可在两次解析间切换公网记录到内网地址，导致业务凭证发向未授权地址；混合公私答案只覆盖第一层。
- **依据**：P01 PLAN 2.3 和 4.7 写“解析全部 DNS 答案/origin 固定”，但没有说明将校验通过的 IP 传给连接层、pinning 生命周期、TTL/重绑定处理或连接失败后的重新校验。
- **责任 feature**：01。
- **建议**：使用自定义 transport/resolver 将连接绑定到已验证地址，或等价实现 SNI/Host 与 pinned IP 的安全连接；关闭重定向；TTL 过期或连接重建时重新执行全答案策略。
- **解决条件**：本地 DNS/test transport 注入第一次公网、第二次私网答案，断言连接只到达允许地址；混合公私答案、TTL 过期、连接重建和证书/SNI 不匹配均拒绝。

### R03-12 [P3] P02 verify skill 的复跑命令未按 RTK 约定书写

- **状态**：CONFIRMED documentation issue。
- **影响**：项目约定所有 shell 命令经 `rtk` 执行。P02 skill 写的是 `bash .codex/skills/.../verify.sh`，而 P01/P03/P04/P05 均为 `rtk bash ...`。这不会改变模型结论，但会降低复跑入口一致性。
- **依据**：`/Users/echo/orca/workspaces/obot/openapi-worker-bridge/.codex/skills/verify-openapi-worker-bridge/SKILL.md` 的 Re-run 命令；R01 实际复跑已用 `rtk`。
- **责任 feature**：02。
- **建议**：改为 `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh`。
- **解决条件**：更新 skill 后执行一次命令并保持预期 pass/fail 结果。

## 4. 重点链路复核

### HTTP 方法、路径、参数与响应

- 源码接受 GET/PUT/POST/DELETE/OPTIONS/HEAD/PATCH/TRACE，拒绝 cookie 参数、外部 ref 和保留 Header 参数；但这只是导入校验，不生成 operation 清单或运行时参数校验。P03 依赖 01 describe 是合理的，R03-02/R03-07 是必须补齐的契约。
- P03 使用 canonical vMCP + immutable component ID + raw relative path 的方向正确。现有授权身份是组件 ID + 原始工具名，`ToolOverride.OverrideName/Description` 不改变身份，`Enabled` 才是 allowlist；P03 的三层收窄与源码一致。
- P03 formal model 覆盖 component/method、static precedence、等价模板拒绝、三层 allowlist、raw segments/query 保留和 rejection no-upstream。它不覆盖 strict parser、平台 Key/vMCP scope、Header/body、预算和网络，不能外推为 E2E。

### 平台 Key 与发布白名单

- API Key 从数据库得到用户 UID、API Key 归因和 `authorized_mcp_ids`；只有 `mcpServerIds` 非空才给 `GroupMCP`。`checkMCPID` 再解析当前用户连接并校验 vMCP/instance/legacy alias scope。P03 不能复用管理 `vmcp_id` 可见性检查的结论正确。
- Profiles 和 instance 选择按组件限定原始工具名相交，`GrantedToolOverrides` 证实无 overrides 时全部 discovered tools 可用，有 overrides 时仅 enabled 且授权的原始名可用。别名/前缀不会改变授权身份。
- 需要补 R03-05 的模型覆盖/边界声明，否则“平台 Key 范围实时校验”只停留在代码测试要求。

### 凭证与业务错误归属

- 现有凭证可以按 context 揭示和加密，但 `serverConfigForAction`/`configureHeaders` 存在固定配置 fallback；P02 专用 resolver 和精确 instance context 是必要的。业务 token 交换闭环仍不存在，不能宣称真实企业授权已完成。
- P03 将业务 4xx/5xx 定义为 business result、平台错误独立 JSON 的方向正确。必须同步 R03-04/R03-08，否则 deadline、租约和特殊响应会破坏错误归属。

### 预算与全局额度

- P04 使用 PostgreSQL、短事务、lease deadline/fencing/grace reclaim 的设计方向符合全局额度目标；F04 反例能证明提前释放和 stale start 的伤害。
- R03-01/R03-04/R03-09 显示 02/03 接线与 sourceRef 规范化仍未达到冻结条件；formal pass 不覆盖 PostgreSQL 事务、release token、公平队列和真实多副本拓扑。

## 5. 形式化审核与复跑

- 已完整读取五份模型/配置、证据和 verify skill；R01 的 own-worktree 复跑摘要显示 F01—F05 脚本均 exit 0，且包含预期 pass/fail 与 Mermaid chart 断言，未修改源 feature formal 产物。
- 本次在当前 worktree 独立复跑：

```bash
rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh
```

结果：

- normal：PASS，613 generated / 241 distinct，0 left；
- broken：预期 FAIL，仅 `ExecutionRequiresUniqueAuthorizedMatch` 违反，21 generated / 21 distinct；
- normal chart：PASS，17 states / 29 steps，7 states / 9 arrows；
- broken chart：预期两条反例（unique route fallthrough 与 whitelist bypass），20 states / 35 steps，7 states / 11 arrows。

证据由脚本刷新到 `docs/plans/openapi-workers/formal/runs/latest/EVIDENCE.md` 和相邻输出。该结果仅证明 P03 设计状态机，不证明 parser、Header/body、预算、网络、实现或部署。

## 6. 已验证 / 需要验证 / 剩余局限

### 已验证

- 读取 00 契约/验收、交接/架构、五份 PLAN/formal/skill 和相关真实源码；未以汇总替代源码。
- 当前源码确认 API Key scope、vMCP 授权身份、组件快照、OpenAPI 方法/参数边界、Header 保护缺口、响应 writer 污染和 sourceRef 生成方式。
- 复跑 P03 formal verifier，结果与记录一致；R01 五模型复跑摘要与原始证据一致。

### 需要后续验证

- Go/Python canonical executionDigest 与 matcher 的共享测试向量。
- strict raw parser 的百分号编码等价、静态段匹配、1xx/upgrade/trailer 和 reserved Header 行为。
- 02 client transport 是否存在底层重试，以及 DNS pinning/TOCTOU 防护。
- 预算 Acquire/Release 唯一归属、deadline grace、sourceRef alias、PostgreSQL 多副本全局额度。
- 所有实现、本地假上游 E2E、Compose 双栈、真实企业授权交换和生产部署均未执行。

### 剩余局限

本次是设计级审核，没有修改业务代码，没有调用业务接口，没有启动/停止服务，没有访问历史 JWT 或集群。R03-10/R03-11 是计划缺口与待实现验证条件，不声称当前实现已出现该故障。
