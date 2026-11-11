# P05 两个 Docker Compose 部署单元与全链路验收计划

日期：2026-11-09。分支：`feature/openapi-k8s-acceptance`；基线：`2be04f025d1c86144e8670d9e9341b263edc28ea`。状态：planning。本文只规划，不代表任何业务代码、Compose 部署或 E2E 已实施或通过。

## 1. 问题与目标

当前 Obot 官方 Docker 路径是单容器评估形态：镜像内可选启动 PostgreSQL，文档的动态 MCP 托管路径挂载宿主 Docker socket；源码运行后端也默认在 Docker/Kubernetes 动态工作负载中选择。OpenAPI Workers 项目需要改为两个可独立生命周期管理的固定 Compose 单元：

```text
[Obot Compose project]                 [OpenAPI Workers Compose project]
 edge/TLS -> obot-1, obot-2            worker-1, worker-2
 postgres                                fixed /healthz,/readyz + internal API
      \                                  /
       \------- explicit shared network --/
                 mTLS + service auth only

 Worker -> fake business API only in acceptance harness
 Worker does not receive Docker socket, DB credentials, or per-user secrets
```

完成标准：

- 一个 Obot 栈包含 Obot 服务及其 PostgreSQL 依赖；一个 OpenAPI Workers 栈只包含固定 Worker 服务。两个 Compose project 可分别启动、停止、升级和清理，通过显式外部共享网络通信。
- 生产 Obot→Worker 通信使用 TLS 和固定服务认证，默认候选为 mTLS（可叠加 bearer）；不使用逐用户 Secret、Docker socket、Kubernetes 管理员或动态工作负载控制器。
- 数据库、加密、TLS、服务认证、资源、日志、health/readiness、SIGTERM 排空、逐实例升级和重启恢复均有明确文件与操作路径。
- 本地验收使用独立 Compose project、随机/独立宿主端口、独立卷和网络、临时 PostgreSQL、假业务系统、假 Agent 和运行期合成凭证；不停止或访问用户现有服务，不访问真实集群或历史 JWT。
- 最终验收覆盖 `ACCEPTANCE.md` 全部场景，特别验证两用户、两份 schema、两 Obot/两 Worker 的全局额度、取消/崩溃回收、大响应、无 SSE 和应用脱离 Agent。
- 在无提交授权条件下，通过限定路径补丁和未跟踪清单把 01—04 的允许实现集成到本 worktree，不覆盖其他 worktree，不删除本 PLAN。

真实集群部署不再是本项目目标；本文不规划 Kubernetes、kubectl 或 Helm 操作。

## 2. 已核查现有基础

证据标签说明：`FACT` 为本 worktree 源码/文件的直接事实；`PLAN` 为其他 feature 计划内容；`REAL` 为本机只读检查结果；`CANDIDATE` 为待冻结设计；`UNVERIFIED` 为尚未运行验证；`GAP` 为明确缺口。

### 2.1 源码与仓库事实

- `FACT`：`Dockerfile` 构建单一 Obot 镜像；`run.sh` 在 `OBOT_SERVER_DSN` 为空时才在容器内启动 PostgreSQL。设置外部 DSN 后可直接运行 `obot server`，因此 Obot 栈可以使用独立 PostgreSQL 服务，而不是每个 Obot 容器内嵌数据库。
- `FACT`：Git 中只有 `tools/jaeger-compose.yaml`，没有 Obot 主服务 Compose 或 OpenAPI Workers Compose。官方 `docs/docs/installation/docker-deployment.md` 的评估路径挂载宿主 Docker socket 以启动 sibling MCP 容器，不满足本项目运行边界。
- `FACT`：`pkg/server/server.go` 监听 `0.0.0.0:${HTTPListenPort}`，默认端口由 `services.Config` 定义为 8080；进程本身没有公开 TLS 监听。Compose 需要外部 TLS 终端，或将 TLS 交给既有企业入口。
- `FACT`：`pkg/gateway/server/router.go` 只有 `/api/healthz`，同时检查数据库和 controller healthy；没有独立公开 readiness endpoint。Worker 契约则有 `/healthz` 与 `/readyz`。
- `FACT`：`pkg/cli/server.go` 捕获 SIGTERM；`pkg/server/server.go` 关闭时先等 router/controller，再用 20 秒窗口关闭 OTel、HTTP server、router、审计、MCP session 和 gateway。关闭排空存在基础，但没有 Compose stop grace、readiness 摘除和逐实例升级编排。
- `FACT`：`pkg/storage/services/config.go` 默认 SQLite，`OBOT_SERVER_DSN` 可切 PostgreSQL；`pkg/storage/server.go` 对 PostgreSQL 并发 schema 迁移使用固定 advisory lock。两个 Obot 副本共享一个外部 PostgreSQL 有源码基础。
- `FACT`：`pkg/mcp/manager.go` 的 `MCPRuntimeBackend` 默认 `docker`，只接受 Docker、Kubernetes/K8s。Docker backend 初始化 Docker client、网络和事件监听；Kubernetes backend 创建/依赖集群 client、cache 和 router。当前源码没有 P02 规划的 `openapi-worker` 固定模式。
- `FACT`：`pkg/services/config.go` 仅在 Kubernetes backend 下构建 local Kubernetes 配置和 router；PostgreSQL、身份、存储、vMCP controller 等通用服务不要求集群管理员。这是固定模式可以不读取 kubeconfig 的源码基础，但必须由 P02 显式实现。
- `FACT`：当前 OpenAPI runtime 仍是每个配置生成一份 `openapi-mcp` wrapper 部署；共享 Worker、服务认证、预算租约和 HTTP 代理均不存在，本文不能预设其已可用。
- `FACT`：`chart/templates/mcp.yaml` 包含 Namespace、Role、ClusterRole/ClusterRoleBinding 等动态托管资源。更新后的目标放弃 K8s 路线，该 chart 不作为本任务交付或验收基础。

### 2.2 其他 feature 计划输入

- `PLAN(P01)`：新增 `workers/openapi/`、固定依赖锁与 Dockerfile；Worker 暴露 `/internal/openapi/v1/describe|execute`、`/healthz`、`/readyz`；无本地持久缓存，重启后由 Obot 补送不可变快照。
- `PLAN(P02)`：新增 `openapi-worker` 后端模式、`pkg/openapiworker`、固定 URL/服务认证/TLS 配置、每调用凭证 resolver、JSON-only MCP bridge，并保留 Docker/K8s 后端。
- `PLAN(P03)`：新增 `/api-proxy/{vmcp_id}/{component_id}/{path...}`，复用 02 executor 和 04 预算；应用调用不依赖 Agent 在线。
- `PLAN(P04)`：使用 Obot PostgreSQL 存放跨副本预算和租约；两 Obot/两 Worker/真实 PostgreSQL 是全局额度与崩溃回收的唯一有效验收拓扑，SQLite 不能替代。

### 2.3 本机工具条件

本阶段只做版本/占用检查，不安装、不启动、不重试连接 Docker daemon：

| 工具 | 结果 |
| --- | --- |
| Go | `REAL go1.27.0 darwin/arm64` |
| uv | `REAL uv 0.12.19` |
| PostgreSQL client/server binaries | `REAL PostgreSQL 16.15` |
| Docker CLI | `REAL Docker 29.8.2` |
| Docker Compose | `REAL Compose v5.5.0` |
| Python / OpenSSL | `REAL Python 3.15.0` / `OpenSSL 4.1.0` |
| Docker daemon | `GAP/UNVERIFIED`：Compose read-only 查询报告无法连接 `~/.orbstack/run/docker.sock`；未启动 daemon，未执行 build/up |
| 宿主端口 | `REAL`：18080 已被占用；18081、18082、18443、18181、18182、18281、18282 当前空闲。验收必须动态选端口，不能固定 18080 |

因此，“本机具备二进制条件”不等于“Compose E2E 可执行”。最终实施若 daemon 仍不可用，只能报告 Compose 验收 blocked，不得用 `compose config` 冒充部署通过。

## 3. 契约核查结论与分歧

以下发现需 G00 冻结；P05 不私自改共享契约。

1. **服务认证方式不一致。** P01 推荐 mTLS，P02 候选为 bearer token + TLS。P05 建议冻结为：生产默认 mTLS，可配置叠加 bearer；本地验收必须实际验证 mTLS，不允许降级为 HTTP。bearer 仅作为服务认证第二因子或明确的企业现有网关方案，不作为明文传输替代。
2. **“Worker 可访问 Obot 数据库”不应理解为持有数据库凭证。** P01 Worker 无持久状态，P04 租约由 Obot 侧访问 PostgreSQL；Worker 只需要共享网络上的 Obot 内部服务和自身认证材料。P05 采用最小权限：PostgreSQL 只在 Obot 栈私有网络，Worker 不挂载 DB secret，不连接 PostgreSQL。若 G00 坚持 Worker 直连数据库，必须重开 P01/P04 状态边界，不能由部署计划自行扩大权限。
3. **Obot 缺少独立 readiness。** 现有 `/api/healthz` 是数据库+controller 复合状态。建议 P05 增加 `/api/readyz`，保留 `/api/healthz` 兼容语义；readiness 不等待 Worker 栈，因为两个 Compose 单元要独立升级，Worker 状态应通过 OpenAPI 能力/调用错误单独暴露。
4. **Compose 的独立升级要求不能依赖 `scale` 造成的同批重建。** 为支持逐实例排空，Worker 栈定义固定 `worker-1`、`worker-2` 服务，并给两者同一个网络别名；Obot 栈定义 `obot-1`、`obot-2`。副本数固定，不按 schema、接口或用户变化。
5. **Docker Compose 项目间没有 `depends_on`。** 启动顺序必须由部署/验收脚本显式等待：Obot 自身 ready 后启动或升级 Worker，随后执行 OpenAPI 集成 readiness；任一栈单独停止时另一栈应保持进程健康，并把调用失败映射为 503。
6. **镜像/依赖版本必须在集成时冻结。** Compose 文件不得默认 `latest`。验收记录记录镜像 digest、P01 lock 摘要、Git 基线和补丁清单；否则无法复现全局额度与崩溃结果。

## 4. 范围与非目标

### 范围

- 新增两个生产 Compose 文件与示例配置：Obot 栈、OpenAPI Workers 栈。
- 新增 Compose secrets/config 布局、TLS 证书 SAN/有效期要求、服务认证与轮换说明。
- 新增两 Obot/两 Worker 的固定副本、资源/日志限制、health/readiness、SIGTERM 排空、逐实例升级和重启恢复流程。
- 新增本地验收 harness：第三个只用于测试的临时 Compose project，包含假业务系统与 Agent stub；生产两个部署单元仍不变。
- 新增 Compose 配置静态断言、生命周期模型检查和全链路验收测试/脚本与证据格式。
- 在实施许可后审查并集成 01—04 的允许路径补丁，保留本 worktree 的 PLAN。

### 非目标

- 不实施 Kubernetes、Helm、Namespace/RBAC、真实集群部署或集群验收。
- 不实现共享 Worker 本体、Obot 固定后端、HTTP 代理或预算算法；这些分别属于 01—04。
- 不挂载 Docker socket，不引入 Docker-in-Docker，不按用户/schema/接口创建容器或进程。
- 不把用户业务 token、平台 Key、数据库密码或服务认证写入仓库、Compose 示例、日志、计划或验收产物。
- 不访问真实业务系统、企业凭证、历史 JWT、原工作区服务或生产配置。
- 不提交、不推送、不创建分支/worktree/PR，不在规划阶段启动 Docker 服务。

## 5. 候选部署设计

### 5.1 项目、网络与名称

生产示例使用两个 project：

```bash
docker network create obot-openapi-shared

docker compose -p obot -f deploy/compose/obot.yaml up -d
docker compose -p openapi-workers \
  -f deploy/compose/openapi-workers.yaml up -d
```

- `obot-openapi-shared` 必须是 `external: true`，由部署脚本显式创建/删除；Compose down 不应误删共享网络。
- Obot 栈内有 `obot_internal` 私有网络，PostgreSQL 和 Obot 副本位于该网络；只有 Obot 副本额外接入共享网络。
- Worker 栈只接入 `obot-openapi-shared`。两个 Worker 服务均使用网络别名 `openapi-worker`，Obot 固定 URL 指向 `https://openapi-worker:8443`。
- 项目名、网络名、卷名和容器名可由 `.env` 覆盖，但默认不含用户凭证。验收生成 `obot-oad-e2e-<runid>-obot`、`...-workers`、`...-harness` 及 `obot-oad-e2e-<runid>-net`，避免碰撞。
- 静态断言必须解析最终 Compose config，确认不存在 `/var/run/docker.sock` bind、没有 Docker socket secret、没有 `privileged: true`、没有逐用户 secret、没有 Postgres 暴露到共享网络或宿主。

### 5.2 Obot Compose 栈

固定服务：

| 服务 | 数量 | 端口 | 职责 |
| --- | ---: | --- | --- |
| `postgres` | 1 | 私有 5432 | Obot 主库与 LLM gateway 数据库 |
| `obot-1` | 1 | 私有 8080 | Obot 副本 1 |
| `obot-2` | 1 | 私有 8080 | Obot 副本 2 |
| `edge` | 1 | 宿主 `${OBOT_HTTPS_PORT:-8443}` | TLS 终端与两副本反代 |

要求：

- PostgreSQL 镜像固定 digest，使用 `POSTGRES_PASSWORD_FILE`、`POSTGRES_USER`、`POSTGRES_DB` 和初始化 SQL；初始化同时创建 `obot` 与 `obot_llm_gateway`，避免首启竞态依赖。数据库密码、Obot 加密材料和 bootstrap/初始 owner 配置均走 Compose secrets。
- `obot-1`/`obot-2` 使用同一镜像、同一配置和同一个持久加密材料；共享 PostgreSQL 卷。禁止容器内嵌 PostgreSQL 路径，必须显式设置外部 DSN。
- `OBOT_SERVER_HOSTNAME` 使用外部 HTTPS 地址；`MCPRuntimeBackend` 使用 P02 冻结的 `openapi-worker` 值。Worker URL、TLS CA/client cert/key、可选 bearer token 通过文件路径配置，不从应用请求读取。
- `edge` 使用固定摘要的轻量 TLS 反代镜像（候选 Caddy）或在文档中明确由既有企业 ingress 终止 TLS。若使用内置 edge，TLS cert/key/CA 是只读 secrets，upstream 只走栈内私有网络。
- 健康检查：
  - PostgreSQL：`pg_isready` 后再执行 `SELECT 1`，避免只依赖启动中间态。
  - Obot liveness：保持 `/api/healthz` 兼容。
  - Obot readiness：新增 `/api/readyz`，检查 DB、controller 和必需本地配置；不把 Worker 栈可用性作为全局 ready 条件。
  - edge：TLS 证书有效且 upstream 至少一个 ready。
- 资源候选：每个 Obot request `250m/512Mi`、limit `2/2Gi`；PostgreSQL request `250m/512Mi`、limit `2/2Gi`；edge request `50m/64Mi`、limit `200m/128Mi`。这些是部署安全边界，不是容量承诺。
- 日志 JSON 输出，Docker logging driver 限制 `max-size=10m`、`max-file=3`；敏感 Header 由 01—04 实现脱敏，Compose 不记录 secret 文件内容。

### 5.3 OpenAPI Workers Compose 栈

固定服务：

| 服务 | 数量 | 端口 | 职责 |
| --- | ---: | --- | --- |
| `worker-1` | 1 | TLS 8443 | 固定共享 Worker |
| `worker-2` | 1 | TLS 8443 | 固定共享 Worker |

要求：

- 两个 Worker 使用同一 P01 镜像摘要、固定 CPU/内存边界和同一网络别名；服务名稳定，便于逐实例排空、杀进程和升级。不得按 OpenAPI 文档、operation 或用户扩缩。
- Worker 监听 TLS 8443；服务端证书 SAN 包含 `openapi-worker` 和实际 Worker 服务名。客户端 CA 只签 Obot 服务身份。TLS key/cert/CA 通过 Compose secrets 只读挂载。
- `/healthz` 表示进程/事件循环存活；`/readyz` 表示编译 helper、transport、认证配置和协议版本可用。SIGTERM 后 readiness 立即 503，health 继续有界响应，既有请求排空到总 deadline。
- Worker 不挂载 PostgreSQL secret，不连接数据库，不挂载 Docker socket，不拥有宿主 Docker 控制权。共享客户端只复用连接池，不保存用户业务 Header/Cookie/token。
- 资源候选：每个 Worker request `250m/512Mi`、limit `1/1Gi`，必须覆盖 P01 的 16 MiB schema 缓存、8 MiB 响应、base64 envelope 和固定编译 helper 的峰值；实施时用实际内存采样校准。
- `stop_signal: SIGTERM`，`stop_grace_period: 35s` 或按 P01/P04 总 deadline + fencing margin 冻结值加 5s。重启策略生产 `unless-stopped`；验收由 harness 显式控制并在结束后 `down -v --remove-orphans`。

### 5.4 服务认证、TLS 与轮换

候选配置接口（最终名称由 P02/P01 冻结，P05 Compose 只使用文件路径，不使用长期明文 env）：

| 侧 | 材料 | 用途 |
| --- | --- | --- |
| Obot | Worker CA cert | 验证 Worker 服务证书 |
| Obot | Obot client cert/key | mTLS 调用 Worker |
| Obot | optional Worker bearer token file | 叠加认证或过渡方案 |
| Worker | Worker server cert/key | TLS 服务端 |
| Worker | Obot client CA cert | 仅接受受信 Obot client |
| Worker | optional bearer token file | 与 Obot 一致时启用 |
| edge | public cert/key | 外部 HTTPS |
| PostgreSQL | password file | 仅 Obot 栈私有网络 |

- 生产同一身份不得长期不可轮换。P05 交付轮换流程：先添加新 CA/证书，逐实例刷新 Obot/Worker，验证健康，再移除旧材料；任何时刻保留一个可用身份。
- 本地验收由 harness 用 OpenSSL 生成短期 CA 和叶子证书，SAN 固定为 Compose 服务名，密钥放在运行临时目录 `0600`，结束后删除；证书不是业务凭证，可记录指纹/有效期，不记录私钥。
- 服务认证 Header 与用户业务认证完全隔离，不进入 executionDigest、snapshot、业务上游、审计正文或日志。

### 5.5 启动、升级、排空与恢复

启动/升级规则：

1. 预检：Docker daemon、Compose config、镜像 digest、secrets 权限、证书链/SAN/有效期、端口占用和共享网络存在。
2. Obot 栈：先 PostgreSQL ready，再逐个启动/替换 `obot-1`、`obot-2`，每次等待 `/api/readyz`；至少一个副本健康才继续。
3. Worker 栈：逐个启动/替换 `worker-1`、`worker-2`，每次等待 `/readyz`；确认两个容器都存在且固定副本数不变。
4. 集成检查：Obot 调用 Worker describe/ready，补送两份快照，再执行最小业务调用。
5. 升级反向同理：先摘 readiness、等既有请求关闭或 deadline、替换一个实例、健康后再替换下一个。PostgreSQL 不随 Obot 应用镜像升级自动重建。

恢复规则：

- Worker 崩溃/重启后无本地快照缓存；Obot 在 describe/execute 收到 `snapshot_required` 时最多补送一次不可变快照，并用假上游证明补送前未触达业务系统。
- PostgreSQL 重启后，Obot `/api/readyz` 变为 503，新 OpenAPI 调用按预算契约返回 503；恢复后并发迁移/计数重算不产生重复 active 租约。
- Obot 单副本重启不影响另一个副本；Worker 单副本重启不创建额外 schema/user 进程，剩余 Worker 继续服务热快照。
- 配置变更必须显式 `docker compose up -d` 或 secret 轮换；不允许运行时从请求指定 Worker URL、上游 host、TLS CA 或认证身份。

## 6. 本地验收拓扑

生产双栈之外，验收脚本额外创建一个临时 harness Compose project。它不是第三个生产部署单元，只属于测试：

```text
project A: obot-oad-e2e-<runid>-obot
  postgres + obot-1 + obot-2 + edge

project B: obot-oad-e2e-<runid>-workers
  worker-1 + worker-2

project C: obot-oad-e2e-<runid>-harness
  fake-business + agent-stub + evidence exporter

network: obot-oad-e2e-<runid>-net (external to all three projects)
```

隔离要求：

- 使用动态选择的空闲宿主端口，绑定 `127.0.0.1`；预检发现占用即换端口，不杀占用进程。18080 当前已占用，不能作为默认验收端口。
- 三个 project 名、卷名和容器名都含 run ID。脚本只 `down` 自己记录的 project/network，不调用全局 Docker 清理，不停止 `docker ps` 中已有服务。
- 临时 PostgreSQL 使用随机密码和项目专属卷；LLM gateway 数据库同建。测试结束后删除卷与临时 secrets/certs。
- 假业务系统只接入测试共享网络，记录请求标签、用户 token 指纹/对号断言、峰值在途、Header 集合、响应类型、延迟和取消，不保存合成 token 原文。
- Agent stub 是独立容器，用于形成/记录一个测试 Agent 会话；验收代理调用前显式停止它，并断言应用代理仍可用。
- 平台用户、API Key、业务 token 全部由 harness 在运行期合成；不读取环境中的历史 JWT、真实企业凭证或备份。
- 验收命令失败即输出退出码并停止；预期失败场景必须断言错误码/状态/无上游调用，不能把异常吞掉后标记通过。

## 7. 文件归属与写集

### 7.1 本规划阶段

只允许新增/更新：

```text
docs/plans/openapi-workers/PLAN.md
```

### 7.2 I05 计划独占实施写集

在 G00 和 I01—I04 完成后，P05 只新增/修改：

```text
deploy/compose/obot.yaml
deploy/compose/obot.acceptance.override.yaml
deploy/compose/openapi-workers.yaml
deploy/compose/openapi-workers.acceptance.override.yaml
deploy/compose/env.example
deploy/compose/README.md
deploy/acceptance/compose.yaml
deploy/acceptance/fake_business.py
deploy/acceptance/run_e2e.sh
deploy/acceptance/validate_compose.py
tests/e2e/openapiworkers/*
docs/docs/installation/openapi-workers-compose.md
docs/sidebars.ts
pkg/gateway/server/router.go
pkg/gateway/server/router_test.go
docs/plans/openapi-workers/PLAN.md
docs/plans/openapi-workers/ACCEPTANCE-EVIDENCE.md
```

说明：

- `pkg/gateway/server/router.go` 仅允许新增 `/api/readyz`，不改现有 `/api/healthz` 和业务路由；若 00 将该文件划给 P02/P03，则 P05 必须改为消费对方 endpoint，不并行写。
- `docs/sidebars.ts` 只有新增正式安装文档页时才修改；必须遵守 docs 目录相对链接和 tabs 约定。
- `ACCEPTANCE-EVIDENCE.md` 只保存脱敏证据：命令、时间、配置摘要、状态、断言、容器 ID/名称、摘要指纹和产物路径；不保存 token 原文、私钥、完整业务 body 或内部地址。
- 如需 Makefile 入口，先向 00 申请根 `Makefile` 单行写集；默认使用脚本命令，避免 P05 私占公共构建文件。

### 7.3 明确不写

- 不修改 01—04 的 PLAN、REVIEW、formal、`.codex` skill 或其他 worktree 文件。
- 不修改 `workers/openapi/**`、`pkg/openapiworker/**`、`pkg/openapi/executionbudget/**`、`pkg/api/handlers/apiproxy/**` 的实现语义；P05 只集成和验收。
- 不修改 chart/K8s 资源，不把旧 K8s 目标带入交付。

## 8. 无提交集成方案

目标：把 01—04 已完成的允许实现并入本 worktree，同时保留 P05 PLAN、避免未审查文件和覆盖他人 worktree。

### 8.1 集成前检查

对每个源 worktree 执行并记录：

```bash
git status --short --branch
git diff --name-only
git ls-files --others --exclude-standard
git diff --check
```

- 只有状态与对应 PLAN 的允许写集一致才可生成补丁；未知文件、敏感路径、原工作区 README 或其他用户修改一律排除并上报 00。
- `.codex/**`、`docs/plans/openapi-workers/PLAN.md`、REVIEW/formal 文件默认不进入 I05 实现补丁；本 worktree 保留自己的 P05 PLAN，后续 F05 交付自己的验证 skill。
- 生成每个源 worktree 的补丁清单 SHA-256 和路径列表，作为交付来源记录。

### 8.2 补丁生成与应用

推荐顺序：I01 → I02 → I04 → I03。

1. **tracked 变更**：按对应 PLAN 允许路径执行 `git diff --binary -- <paths>`。
2. **untracked 新文件**：先取得 `git ls-files --others --exclude-standard` 清单；对每个允许文件生成 `/dev/null -> file` 的 binary patch，或临时 `git add -N <paths>` 后生成 diff 并立即恢复 intent-to-add 状态。源 worktree 不 commit、不 stash、不覆盖。
3. **目标预检**：在本 worktree 对每个补丁执行 `git apply --binary --check --3way`；路径冲突时停止并报告冲突文件，不手工强行覆盖。
4. **应用**：`git apply --binary --3way`，不执行 `git add`、不提交、不推送。
5. **来源审计**：每轮后 `git status --short`，把每个 modified/untracked 文件映射到 I01/I02/I03/I04/I05；未知来源立即回滚该补丁或请求 00 处理。
6. **目标测试**：按 8.3 执行对应 targeted checks，再进入下一轮。

### 8.3 每轮最小验证

- I01：`uv sync --locked`、P01 单元/本地 Worker endpoint 测试、Worker 镜像 build（Docker 可用时）。
- I02：`go test ./pkg/openapiworker ./pkg/mcp ./pkg/services ./pkg/api/authz ./pkg/api/handlers/mcpgateway`。
- I04：带独立 PostgreSQL DSN 的 `go test -race ./pkg/openapi/executionbudget ./pkg/gateway/db -run 'TestOpenAPIBudget|TestBounded|TestLimit|TestGatewayAutoMigrateConcurrentlyOnFreshPostgresSchema'`。
- I03：`go test ./pkg/api/authz ./pkg/api/handlers/apiproxy ./pkg/api/server ./pkg/api/router`，再按 P03 运行受影响 UI/i18n/docs 检查。
- I05：Compose config 静态断言、build、双栈部署、健康/生命周期和下文全链路矩阵。

## 9. 实施步骤

1. **等待门槛**：确认 G00 冻结契约、I01—I04 完成、每个源 worktree 允许写集和测试结果可审查；否则只保持本 PLAN，不实施。
2. **集成补丁**：按第 8 节生成、检查并按顺序应用，记录来源和 SHA-256。
3. **Compose 静态交付**：新增双栈 YAML、override、env 示例、README 和配置断言脚本；先运行 `docker compose config` 与安全断言，不启动。
4. **生命周期接入**：新增 Obot `/api/readyz`、Worker health/readiness 编排、stop grace、日志/资源限制和逐实例升级脚本。
5. **验收 harness**：新增临时 project、短期证书/secret 生成、假业务系统、Agent stub、动态端口、清理 trap 和脱敏证据输出。
6. **冒烟链路**：启动双栈，导入两份 OpenAPI、两用户凭证与两个组件，验证 describe、MCP tools/list/call 和 HTTP proxy 最小成功/业务 403。
7. **完整矩阵**：按第 10 节执行功能、无 SSE、预算、失败、部署和原工作区保护场景，沉淀 `ACCEPTANCE-EVIDENCE.md`。
8. **F05 形式化**：为 Compose 生命周期建立模型，实际运行模型检查，输出 Mermaid 状态图、反例修复证明和项目本地验证 skill；不把设计模型写成部署结果。
9. **最终复核**：检查全部未提交文件来源、无敏感值、无 Docker socket、无 K8s 操作、无真实凭证；报告 verified/partial/blocked 与剩余缺口。

## 10. 验收矩阵

所有最终证据必须包含场景 ID、状态、命令、拓扑、开始/结束时间、预期/实际、非敏感产物路径和未验证项。下表“责任”表示最终执行/汇总归属，不表示其他 feature 的单元测试可省略。

### 10.1 规划与协作

| ID | P05 验收路径 | 责任 |
| --- | --- | --- |
| P-01 | 本文件存在且只改本路径；`git status` 与 `git diff --check` 记录 | 05 |
| P-02 | 覆盖目标、基础、范围、契约、写集、依赖、步骤、证据、失败、风险、交付 | 05/00 |
| P-03 | 阅读 01—04 PLAN 与 00 CONTRACTS/ACCEPTANCE；本文第 3 节列出分歧和解决条件 | 05/00 |
| P-04 | 未实施业务代码；等待 G00 冻结和 I01—I04 完成后才应用补丁 | 05/00 |
| P-05 | 只通过既有 Orca Task/Dispatch 汇报；不新建 Run、不派发 helper、不接触用户聊天 | 05/00 |
| P-06 | F05 实际运行 Compose 生命周期模型检查，生成 skill、结果、Mermaid 和反例证明 | 05 |

### 10.2 功能与隔离

| ID | 验收 | 责任 |
| --- | --- | --- |
| F-01 | 两份 schema 发布到同一 Worker 栈，`worker-1/2` 均可执行；容器/进程数不随 schema 增加 | 01/02/05 |
| F-02 | 显式发布更新生成新 executionDigest；旧发布仍可调用至撤版；重启两个 Worker 后由 Obot 补送快照恢复 | 01/02/05 |
| F-03 | 两用户并发同一路径，假业务系统断言每个请求只含对应用户合成 token，服务认证/平台 Header 不穿透 | 01/02/03/05 |
| F-04 | 假业务系统返回 403 与业务 body，应用代理原样返回；平台不添加业务权限判断 | 03/05 |
| F-05 | 无身份、错误 Key scope、无 vMCP/component/operation 资格、另一用户实例均拒绝且不触达假上游 | 02/03/05 |
| F-06 | GET/POST/PUT/PATCH/DELETE、path/query/body/header、多值参数、非 JSON 响应按契约保留 | 01/03/05 |
| F-07 | 别名/描述/前缀变化后，授权仍绑定组件 ID+原始工具名+operationKey；跨组件同名不混淆 | 01/02/03 |
| F-08 | 停止 `agent-stub` 后，已配置应用仍通过 `/api-proxy/...` 完成调用 | 03/05 |
| F-09 | 模拟授权写入/读取、缺失/失效凭证返回明确 409；真实企业交换标 `UNVERIFIED/GAP` | 02/05 |

### 10.3 无 SSE 与固定执行

| ID | 验收 | 责任 |
| --- | --- | --- |
| T-01 | MCP initialize、tools/list、tools/call 实际响应 `application/json`，无事件流和长期 session | 01/02/05 |
| T-02 | Worker 连接失败、非 200 readiness、异常 Content-Type 下不发起 SSE GET、不访问旧 `/messages` | 02/05 |
| T-03 | SSE-only 请求和上游 `text/event-stream` 明确拒绝；混合 Accept 仍返回 JSON | 01/02/03/05 |
| T-04 | Compose config、容器 mounts、进程/容器清单证明无 Docker socket、无 K8s 凭证、无逐接口工作负载 | 02/05 |

### 10.4 资源与失败

| ID | 验收 | 责任 |
| --- | --- | --- |
| B-01 | 两 Obot/两 Worker 共用临时 PostgreSQL；接口限额 2 时假上游峰值不大于 2，请求标注实际 Obot/Worker | 04/05 |
| B-02 | 未配专属限制接口同时受接口、默认池和平台总预算约束 | 04/05 |
| B-03 | 慢接口独立排队，快接口在等待预算内继续；两个 runnable operation 轮转可观察 | 04/05 |
| B-04 | 打满总/接口队列返回 429 与 Retry-After；票据释放后可恢复 | 04/05 |
| B-05 | 客户端断开/截止时间传递到 Obot、Worker、假上游；lease 幂等释放且无持续泄漏 | 01/02/04/05 |
| B-06 | 杀死一个 Worker，不确定执行保守占用到 deadline+grace 后回收；后续额度不超限 | 04/05 |
| B-07 | 停用 PostgreSQL 后新调用 503、不回退内存限额；恢复后无重复 active 名额 | 04/05 |
| B-08 | 请求、分块、压缩膨胀、base64/JSON、HTTP 返回和审计均有界；超限无伪成功半截 body | 01/02/03/04/05 |
| B-09 | 两份 schema 冷编译并发去重、缓存份数/字节受限、活跃引用保护；慢编译不阻塞热调用 | 01/05 |
| B-10 | `Authorization`、`authtoken`、服务认证和声明自定义 Header 全脱敏；证据扫描不到合成 token 原文 | 01/02/04/05 |

### 10.5 部署与交付

| ID | 验收 | 责任 |
| --- | --- | --- |
| D-01 | 两个 Compose project 分别 up/down/upgraded；共享网络和服务认证连通；harness 是测试专用第三 project | 05 |
| D-02 | 静态与运行态证明无 Kubernetes 管理员、无动态工作负载管理、无 Docker socket、无逐用户 Secret | 02/05 |
| D-03 | PostgreSQL/auth/TLS secrets、资源、日志、health/readiness、SIGTERM 排空、重启和恢复均有真实观测 | 01/02/05 |
| D-04 | 限定路径补丁、来源清单、未跟踪清单、目标 status 和保留 PLAN 可审查；无 commit/push/覆盖 | 00/05 |
| D-05 | 分列静态 config、镜像 build、Compose up、本地 E2E；未执行 up 不能写部署成功 | 00/05 |
| D-06 | 记录原服务/容器清单仅作只读对比；验收前后用户服务不变；未访问原工作区 README、历史 JWT 或真实集群 | 00—05 |

## 11. 失败行为

| 场景 | 要求 |
| --- | --- |
| Docker daemon 不可用 | 二进制检查可记录，Compose build/up/E2E 标 blocked；不启动服务或改用假结果 |
| secrets/证书缺失或不匹配 | `compose config`/预检失败，不启动应用；错误不回显密钥内容 |
| 共享网络缺失 | 拒绝启动需通信的服务；不自动使用 bridge/host 网络 |
| PostgreSQL 未 ready | Obot readiness 503，新调用 503；不回退 SQLite 或独立内存预算 |
| Worker 未 ready | Obot 进程可 ready，OpenAPI 调用 503；不启动 Docker/K8s 动态工作负载 |
| Worker TLS/服务认证失败 | 明确 503/传输错误；不降级 HTTP，不重试业务上游 |
| 单副本升级/崩溃 | 摘 readiness 并排空；剩余副本承接热调用；崩溃租约保守回收 |
| 端口占用 | 动态换端口；不杀占用进程，不报告“选择端口”为服务已启动 |
| 清理失败 | 输出残留 project/network/volume 名称并标 partial；不得清理非本项目资源 |
| 验收产物含敏感值 | 立即停止发布/上报，删除或重跑生成脱敏产物，并记录泄漏范围；不得仅手工编辑后冒充原证据 |

## 12. 风险与未验证项

- `GAP`：共享 Worker、固定后端、HTTP 代理、预算租约均未实施；本文所有全链路通过状态为 pending。
- `GAP`：Docker daemon 当前不可可靠访问，未执行 Compose config/build/up；只完成工具二进制与端口只读检查。
- `UNVERIFIED`：Compose v5.5.0 对具体 `deploy.resources`、restart/stop 和多服务 alias 的实际行为需在实施时验证；文件不能只依赖 schema 校验。
- `UNVERIFIED`：P01 Worker 内存峰值、TLS 握手开销和 P04 轮询数据库负载未测；资源初值只能作为安全上限，不能外推生产吞吐。
- `UNVERIFIED`：真实企业授权交换、真实业务 API、业务 token 刷新和生产 TLS 入口不在本地验收范围。
- `RISK`：两个 Compose project 跨网络升级存在窗口期；必须保留单副本可用和 503 语义，不能为了“全局 ready”把两个生命周期耦合。
- `RISK`：Docker Desktop/OrbStack 的 socket 和网络行为可能随宿主配置变化；验收以解析后的 mounts/networks/container 清单为准，不以镜像文档为准。
- `RISK`：无提交集成依赖各源 worktree 保持稳定；生成补丁前必须再次检查状态，若源在期间变化则重新生成并记录新摘要。

## 13. 交付物

- 本计划：`/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance/docs/plans/openapi-workers/PLAN.md`。
- I05 实施交付：两个生产 Compose 单元、安全配置示例、验收 harness、静态断言、生命周期/升级文档、全链路证据和 F05 验证 skill。
- 集成交付：01—04 限定补丁来源清单、目标 worktree 未提交状态说明、每轮 targeted 测试与最终 Compose E2E 结果。
- 不交付：业务功能提前实现、K8s/Helm 清单、真实集群部署、生产发布、提交/推送、真实凭证处理结论。

## 14. 形式化验证（F05）

### 14.1 高代价性质

**OpenAPI 流量门槛**：任何 OpenAPI 调用被放行时，显式共享网络、PostgreSQL ready、受信服务认证、至少一个 Obot 副本 ready、至少一个 Worker 副本 ready 必须同时有效。违反时，应用流量可能在未认证路径、数据库不可用或 Worker 未 ready 时被放行，造成服务凭证暴露、串栈连接或大面积 503。

**计划内滚动升级容量**：未发生额外基础设施故障时，Obot 或 Worker 的逐实例升级任一步都必须保留至少一个可服务 Obot 副本和一个可服务 Worker 副本。违反时，一次正常发布即造成 OpenAPI 全停。

### 14.2 模型与结果

- 模型：`docs/plans/openapi-workers/formal/ComposeLifecycle.tla`；变量对应第 5.1/5.5 节的 shared network、数据库 readiness、服务认证、`obot-1/2`、`worker-1/2`、活动调用和 rollout target。
- 完整基线：`ComposeLifecycle.cfg`；`TypeOK`、`NoDeadlock`、`OpenAPITrafficGated`、`PlannedRollRetainsCapacity` 全部通过。
- 正常流转图：`ComposeLifecycleChart.cfg` 与 `statechart-chart/chart.mmd`；覆盖共享网络、数据库、认证、两栈 ready、流量开启和 capacity-retained rollout，仍检查全部规则。
- 故意破坏：`ComposeLifecycleBrokenChart.cfg` 在同一模型中注入路由门槛缺失和滚动保护缺失；精确只触发上述两个目标反例，`TypeOK`、`NoDeadlock`、`CHECK_DEADLOCK` 通过且无未检查规则。
- 工具命令、版本、checksum、开始/结束时间、状态数、最短反例和未覆盖边界记录在 `docs/plans/openapi-workers/formal/EVIDENCE.md`；复跑命令为 `rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh`。

### 14.3 边界

本验证只覆盖设计状态机，不执行 Docker/Compose、不解析 YAML、不验证 TLS 握手字节或证书轮换、不覆盖预算租约/请求体/上游语义，也不代表本地 E2E 或部署通过。完整模型中的 `infrastructureOutage` 只把计划升级期间叠加的额外故障移出“正常发布不全停”性质；其他网络、数据库、认证和单副本失败交错仍由完整状态空间检查目标门槛。
