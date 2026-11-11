# MCP Audit End-to-End + /usage Layout Acceptance Evidence

Date: 2026-11-10 23:00–00:40 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance` (integrated), mirrored to 02/03/01. Obot image `3b5422df`, Worker image `bf5528dc`.

Covers the user-reported items: (1) actually call the new OpenAPI MCP chain and verify audit records; (2) the `/usage` page breaking out of the global layout.

## Real chain under test

Synthetic upstream (`host.docker.internal:18081`, hand-written v1/v2 schemas) → catalog entry `default-drift-acceptance-api-22dae316s6444` → vMCP `vmcp1f72ql` (component `Drift Acceptance API`) → launched instance → real MCP client session over `/mcp-connect/vmcp1f72ql` (initialize → notifications/initialized → tools/list → tools/call) → Obot aggregate → component loopback → OpenAPI bridge → mTLS Worker → upstream.

## Findings and fixes

| # | Finding (all verified live before fixing) | Fix | Verification |
| --- | --- | --- | --- |
| F1 | **OpenAPI MCP `tools/call` wrote NO audit records at any layer below the vMCP aggregate.** The aggregate layer records the JSON-RPC exchange (callType=tools/call, mcpID=vmcp), but the business execution (operation, business status code, bounded bodies) was invisible: component servers carry `AuditLogIgnore=true` by design and the bridge wrote nothing. | Added `openapi_bridge_audit.go`: per-tools/call entry with `CallIdentifier=tool name`, business `ResponseStatus`, bounded (32 KiB, marked) request/response excerpts, redacted API key, platform-derived identity; drops the ignore flag so the entry persists while the vMCP-level entry keeps protocol semantics. Adds component identity metadata (`mcpServerDisplayName`, `vmcpComponentID`, `userID`). | DB row id=27/28-class entries: `call_type=tools/call`, `mcp_server_display_name=Drift Acceptance API`, `response_status=200` matching the real business call |
| F2 | **JSON-RPC failures were audited as successes**: `tools/call` returning `error.code=-32602` (unknown tool) got `response_status=200` with an empty `error` column. Misleading for audit consumers. | `proxy_audit.go` now parses the JSON-RPC error object and records `error="jsonrpc error <code>: <bounded message>"`. | DB row: `call_identifier=nonexistent`, `err='jsonrpc error -32602: unknown tool "nonexistent"'` |
| F3 | **`/api-proxy` HTTP exit wrote no audit records either** (the I04 bounded audit capture helper had zero callers). | New `apiproxy/audit.go` AuditWriter wired into the proxy handler: `call_type=http/proxy`, `call_identifier=operationKey`, business status, bounded body excerpt (JSON only, marked truncation), error path, redacted API key; nil-collector safe. Unit tests cover success/failure/truncation/nil. | DB row id=28: `call_type=http/proxy`, `call_identifier=GET /items/{item_id}`, `response_status=200`, bounded JSON body, 30ms |
| F4 | **`/usage` page escaped the global layout**: `McpUsageView.svelte` wrapped content in `h-dvh` (viewport height) inside the app shell's `h-svh` scroll container, clipping content (docScrollH could not be reached; bottom panels cut off). | Removed `h-dvh`, kept `min-h-full` natural height. | `pnpm run check` 0/0; real browser: `docScrollH=1482 > viewportH=800` fully scrollable, all panels reachable; spec test passes; screenshot `USAGE-PAGE-FIXED.png` shows live data (the audit records from this very session feed the usage graphs) |
| F5 | (Discovered during E2E) **Worker rejected dev http base URLs unconditionally** (`_validate_manifest` hard-required https) while Obot dev mode allows them → vMCP launch failed with `snapshot_invalid: invalid base URL`. | `snapshot.py`: http base URL allowed only when `OPENAPI_WORKER_ALLOW_LOCAL_TEST_EGRESS=1` (same flag as http transports). | vMCP launch 200; subsequent real calls succeed |
| F6 | (Discovered during E2E) **compose fed `OPENAPI_WORKER_ALLOWED_EGRESS_HOSTS` but the Worker never read it** — allowed hosts were hardcoded `{127.0.0.1, localhost}`, so `host.docker.internal` upstreams failed with `transport_uncertain`. | `app.py` now parses `OPENAPI_WORKER_ALLOWED_EGRESS_HOSTS` (comma-separated, port-tolerant) into the egress policy; compose var name corrected. | Worker-2 metrics: `upstream_results 2xx completed=1`, `execute ok=1`; real business body returned through the whole chain |

## Live call matrix

| Call | Result | Audit row |
| --- | --- | --- |
| MCP `tools/call getItem {item_id:9}` | business 200 `{"item_id":"9","version":1}` | id=26 (aggregate, JSON-RPC) + id=27 (bridge, business: displayName=Drift Acceptance API, status 200) |
| MCP `tools/call nonexistent` | JSON-RPC error -32602 | id=23 with error text recorded (previously silent success-looking) |
| HTTP `GET /api-proxy/vmcp1f72ql/<comp>/items/5` | business 200 `{"item_id":"5","version":1}` | id=28 `http/proxy`, operationKey `GET /items/{item_id}`, 30ms |

Worker evidence (mTLS scrape of `/metrics` on the replica that executed): `requests_total{route="execute",outcome="ok"}=1`, `upstream_results_total{status_class="2xx",outcome="completed"}=1`, `snapshot_events_total{event="snapshot_required"}=2` (describe+execute cold resupply), `compiles_total=1`.

## Screenshots

- `evidence/USAGE-PAGE-FIXED.png` — live `/usage` page with real data from this session's audit records; full page scrollable within the app shell (layout fixed).
- `evidence/AUDIT-LOGS-PAGE.png` — audit-logs UI rendering the entries above.

## Test status

- 05 integrated: `go test -count=1` green for mcpgateway / apiproxy / api/server / controller-entry / openapi / openapiworker; `gofmt` clean; `pnpm run check` 0/0; usage page spec passes; Worker suite 28/28.
- Mirrored: 02 (bridge audit + proxy audit fix) green; 03 (apiproxy audit + adapted legacy test signatures) green; 01 (Worker egress env + http base URL) 27/27.

## Boundaries

- Aggregate-layer duplicate semantics: a vMCP `tools/call` now yields one protocol entry (vmcp-level) and one business entry (bridge-level) by design; they are distinguishable by `mcp_id`/`mcp_server_display_name`. UI grouping shows both (visible in the usage screenshot as `Audit Acceptance vMCP` and `Drift Acceptance API` rows).
- `CaptureBoundedAudit` (I04) remains unused; the new writers implement equivalent bounded capture inline. Consolidating onto it is follow-up work.
- No K8s rerun for this increment; Compose-level E2E only.
