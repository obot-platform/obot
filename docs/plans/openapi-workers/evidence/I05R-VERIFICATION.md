# I05R Verification Evidence

Date: 2026-11-10 10:05 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance`. Baseline: `2be04f02`.

## Code and protocol

- `gofmt` on changed Go files: **PASS**.
- `go test -count=1 ./pkg/openapiworker/... ./pkg/openapi/executionbudget/... ./pkg/api/handlers/apiproxy/... ./pkg/api/handlers/mcpgateway/... ./pkg/api/server/... ./pkg/api/authz/... ./pkg/gateway/db/... ./pkg/mcp/... ./pkg/services/... ./pkg/api/router/...`
  - **PASS**: openapiworker, executionbudget, API proxy, MCP gateway/OAuth, server/rate limiter, authz, gateway DB, MCP, services.
- `go vet` over the same integrated packages: **PASS**.
- `git diff --check`: **PASS**.
- `cd workers/openapi && uv run pytest -q`
  - **PASS**: 23 tests; two non-failing uvicorn/websockets deprecation warnings.
- `I01_OPENAPI_WORKER_ROOT=workers/openapi go test ./pkg/openapiworker -run TestRealI01PythonWorkerDescribeAndExecute -count=1 -v`
  - **PASS** against a real Python Worker process and TLS fake upstream.
  - user-one receives injected `Bearer user-one`, HTTP 200, exact JSON body, forwarded business header, and no platform headers.
  - user-two receives injected `Bearer user-two`, HTTP 403, and the exact business body. The test proves exact-instance credential isolation and preserved business failures.

## UI and docs

- `cd ui/user && pnpm run i18n:compile && pnpm vitest run src/lib/components/vmcps/VMcpAPIDocumentationDialog.svelte.spec.ts`
  - **PASS**: 1 browser test.
- `cd ui/user && pnpm run check`
  - **PASS**: Svelte diagnostics 0 errors / 0 warnings.
- Required API documentation keys are present in `en`, `ja`, `ko`, and `zh-CN`: **PASS**.
- Real browser check at `http://127.0.0.1:5178/__openapi-ui-check`
  - **PASS**: actual SvelteKit page mounted the documentation dialog, showed Chinese copy, base path, GET operation, required business auth, path template, and parameter schema.
  - Screenshot: `evidence/I05R-UI-BROWSER.png`.
- Seeded real-vMCP walkthrough in the running Compose stack:
  - Created `Acceptance Items API` OpenAPI catalog entry.
  - Created and configured `Acceptance Items vMCP` and user instance.
  - `GET /api/vmcps/vmcp1btlfk/api-proxy` returned HTTP 200 with operation `GET /items/{id}`, path/query parameters, canonical base path, execution digest, and `businessAuthRequired=true`.
  - Real browser opened `/vmcps`, selected the vMCP, opened **HTTP API 文档**, and showed the actual operation and auth requirement.
  - Screenshot: `evidence/REAL-VMCP-API-DOCS.png`.
- `cd docs && pnpm run typecheck`: **PASS** after declaring direct `@types/react`, `@docusaurus/plugin-content-docs`, and `@docusaurus/theme-common` dependencies.
- `cd docs && pnpm run build`: **PASS**. `Direct OpenAPI HTTP calls` and `/api-proxy/...` are present in `docs/build/next/mcp-gateway/connect-clients/index.html`.

## Formal verification

- I02 restored resolver model: **PASS**, 18 distinct states.
- I02 broken model: **FAIL-AS-EXPECTED**, only `ExactCallerCredentialOnly` fails, 19 states.
- I04 full lease model: **PASS**, 928,529 distinct states.
- I04 early-release and stale-start mutations: each **FAIL-AS-EXPECTED** with exactly the intended invariant.
- All statechart reports complete with no unchecked rules.

## Isolated Kubernetes acceptance

- Runtime: Docker + `rancher/k3s:v1.33.3-k3s1`, synthetic namespace `openapi-acceptance`.
- PostgreSQL 16 ran in the isolated control plane and was initialized with the integrated migrations.
- Scale-up: **PASS**, `obot=3/3` and `openapi-worker=3/3`.
- Scale-down: **PASS**, both deployments successfully rolled out at `2/2`; final pods were all Running.
- Obot health inside Kubernetes: **PASS**, `/api/healthz` returned `ok`.
- Obot version inside Kubernetes: **PASS**, HTTP 200 reported `engine=openapi-worker`, `dynamicHosting=false`, `openAPIWorker=true`, DB session store.
- Real `/api-proxy` raw-prefix request through the Obot Service: **PASS**; an invalid canonical VMCP returned HTTP 403 instead of redirecting through the general mux.
- Worker mTLS:
  - valid client certificate: **PASS**, health JSON;
  - missing client certificate: **REJECTED**;
  - untrusted client certificate: **REJECTED**.
- Manifest inspection: no Namespace, ClusterRole, ClusterRoleBinding, Role, RoleBinding, ServiceAccount, Docker socket mount, or Worker database volume is created.

## Boundaries

- The isolated K8s cluster used synthetic DSNs, synthetic certificates, synthetic credentials, and a fake upstream only.
- Production ingress/cert-manager rotation, enterprise authorization exchange, production capacity, and real business systems were not accessed.
