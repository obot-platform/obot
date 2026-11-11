# R01 source and upstream review evidence

Date: 2026-11-09 Asia/Shanghai. All checks are read-only except for the copied formal rerun outputs under `r01-formal-rerun-20261109T184839/`. No credential, JWT, snapshot, business URL, or business response was stored.

## Repository baseline

All five feature worktrees and the reviewer worktree point at:

```text
2be04f025d1c86144e8670d9e9341b263edc28ea
```

Commands used:

```bash
rtk git rev-parse HEAD
rtk git status --short --branch
```

## Local source facts checked

- Current runtime selection only supports Docker/Kubernetes in `pkg/mcp/manager.go:149`; the OpenAPI image remains `openapi-mcp:v0.27.2` at `pkg/mcp/manager.go:46`.
- Current OpenAPI deployment still injects one schema file and uses `/mcp` plus `/readyz` in `pkg/mcp/openapi.go:57-79`.
- Saved schema and destination policy are snapshot-based and bounded to 1 MiB in `pkg/openapi/import.go:21-23`, `pkg/openapi/import.go:57-100`, and `pkg/openapi/environment.go:17-60`.
- Existing credential resolution has ordered-context fallback in `pkg/gateway/client/credential.go:76-96`; server-owned fixed headers can enter shared `ServerConfig.Headers` in `pkg/mcp/types.go:349-375`, while user headers are per-request values in `pkg/mcp/action.go:407-438`. This supports P02's dedicated no-fallback resolver requirement.
- Existing vMCP authorization is component ID plus original tool name in `pkg/vmcp/profiles.go:44-103`.
- Current `ensureServerReady` has a POST-then-SSE fallback when no dedicated health path exists in `pkg/mcp/backend.go:89-185`.
- Current audit buffers a response in `bytes.Buffer` (`pkg/api/handlers/mcpgateway/proxy_audit.go:68-74`) and redacts only a fixed header set (`proxy_audit.go:688-698`); the hook body limit is 10 MiB (`proxy_hooks.go:23-25`).
- Current Go `net/http.ServeMux` documentation confirms segment-by-segment unescaping and sanitizing redirects; `go version go1.27.0 darwin/arm64` was the local toolchain.
- Current `APIServer.Wrap` unconditionally installs `headersResponseWriter` (`pkg/api/server/server.go:186-195`), whose default `Content-Type` and `X-Content-Type-Options` are set in `server.go:438-459`; refreshed provider cookies are replayed in `server.go:308-312`.
- Current authz binds `vmcp_id` as a management resource and runs `checkVMCP` before component/operation checks in `pkg/api/authz/resources.go:176-225`; `checkVMCP` uses read/manage visibility in `pkg/api/authz/vmcp.go:94-118`.
- Existing vMCP `ComponentCatalogReferences` is optional (`omitempty`) in `pkg/storage/apis/obot.obot.ai/v1/vmcp.go:54-55`; this matters to P04's no-fallback budget identity.

## Upstream dependency facts

Read directly from official GitHub tag `obot-platform/mcp-images@v0.27.2`, commit `64b62ab7c8862b15bc29b5a5805f3999e486136f`:

- `openapi-mcp/pyproject.toml` pins `fastmcp==3.4.7`, `httpx==0.28.1`, and `httpcore==1.0.9`.
- `server.py` calls `run_http_async(... stateless_http=True ...)` without `json_response=True`.
- `network.py` rewrites generated request headers to only declared per-request credentials, disables redirects and cookies, uses a shared pool with origin/DNS checks, and fully buffers responses.
- `config.py` rejects non-public destinations at runtime.

PyPI metadata for `fastmcp-slim==3.4.7` reports wheel SHA-256:

```text
6c931a0089705f3f2935428ef9b2bc74ad94140adc64aab84d116d103e694b3a
```

The locally cached 3.4.7 source exposes `json_response` and `stateless_http` parameters on both `http_app()`/`run_http_async()` paths. This confirms P01's version/API claims; it does not execute the upstream image or prove its runtime Content-Type.

## Explicitly not performed

- No historical JWT or real business credential access.
- No real business API call.
- No service deploy, stop, Compose up, Docker daemon start, or production cluster operation.
- No change to any source feature PLAN, formal model, implementation, or global configuration.
