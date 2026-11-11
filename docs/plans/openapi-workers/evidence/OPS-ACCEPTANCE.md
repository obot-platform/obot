# Operations Increment Acceptance Evidence — /metrics + Source Drift Detection

Date: 2026-11-10 22:00–23:10 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-k8s-acceptance` (integrated). Baseline: `2be04f02` + uncommitted I05R + operations increments.

This acceptance covers the two user-requested increments after v1 acceptance:

1. Worker operations-dashboard endpoint (`GET /metrics`).
2. Automatic upstream OpenAPI schema drift detection with no auto-adoption.

## Environment

Real Docker Compose stack from `deploy/compose/`:

- 1 Obot (rebuilt image `obot:openapi-workers-local` @ `87b838d1a41e`, includes drift probe + API status exposure + `MCPRuntimeBackend=openapi-worker`)
- 2 OpenAPI Worker replicas (rebuilt `obot-openapi-worker:local` @ `e3065bb13cea`, includes `/metrics`)
- 1 PostgreSQL; synthetic mTLS material via `generate-local-certs.sh`
- `OBOT_DEV_MODE=true` (http schema source allowed), `MCPOpenAPIDriftProbeInterval=30s` (accelerated probe for acceptance; production default 6h)
- Synthetic drift upstream: hand-written OpenAPI server on `host.docker.internal:18081`, toggled v1↔v2 via `/__set_version` (v1: `/items/{item_id}` string id + `POST /removed`; v2: integer id + new summary + `GET /added`, no `/removed`)

## A. Worker /metrics (operations dashboard)

| # | Check | Result | Evidence |
| --- | --- | --- | --- |
| A1 | `GET /metrics` on the mTLS listener returns 200 with `content-type: text/plain; version=0.0.4; charset=utf-8` | PASS (in-container prober-cert fetch) | terminal transcript |
| A2 | Exposition includes gauges (`up`, `start_time_seconds`, cache entries/bytes/limits, active references, pending compilations, `compiles_total`) | PASS | `WORKER-METRICS.png` |
| A3 | Request counters track real traffic: 5× `healthz` + 5× `readyz` produce `openapi_worker_requests_total{route="healthz",outcome="ok"} 5` and `{route="readyz",outcome="ok"} 5` | PASS | `WORKER-METRICS.png` (values 5/5) |
| A4 | mTLS negative: same fetch **without client certificate** is rejected at TLS handshake (`RemoteDisconnected`) | PASS | terminal transcript |
| A5 | No secrets / requestIDs / digests / URLs appear as labels; label vocabularies are fixed (routes, known error codes, status classes) | PASS (rendered output inspected; enforced by `metrics.py` + `tests/test_metrics.py` no-leak test) | `WORKER-METRICS.png` |
| A6 | Worker unit suites pass with metrics: 27/27 (01 worktree, 3× stable), 28/28 (05 integrated) | PASS | prior section transcripts |

## B. Upstream source drift detection (detect, never auto-apply)

| # | Check | Result | Evidence |
| --- | --- | --- | --- |
| B1 | Administrator creates OpenAPI catalog entry from live upstream (`POST /api/mcp-catalogs/default/entries`, runtime `openapi`, http source under dev mode) | PASS: entry `default-drift-acceptance-api-22dae316lnprc`, snapshot saved from v2 | terminal transcript |
| B2 | First probe at save time reports clean (snapshot digest == source digest) | PASS | status JSON, `detected` empty, digests equal |
| B3 | Upstream switched v2→v1; within one probe interval (30s) status flips to `detected=true` with exact three-way classification: `addedOperations=["POST /removed"]`, `removedOperations=["GET /added"]`, `changedOperations=["GET /items/{item_id}"]` | PASS | `DRIFT-DETECTED-API.png` |
| B4 | Drift **does not auto-apply**: after detection the stored snapshot digest is unchanged (still the v2 digest `a219f7757dbb51c4…`); only `sourceDigest` differs | PASS | status JSON |
| B5 | Probe failure path: (validated in unit suite) fetch/parse failures persist bounded `probeError` and still reschedule; no crash, no snapshot mutation | PASS | `openapidrift_test.go` `FetchFailureIsRecordedNotFatal` |
| B6 | Adoption stays explicit: administrator re-saves the entry → snapshot digest becomes v1 (`c7c8064f…`), next probe returns clean (`detected` empty) | PASS | terminal transcript |
| B7 | Non-probeable entries skipped: non-OpenAPI runtime, inline content source, snapshotless | PASS (unit) | `openapidrift_test.go` skip tests |
| B8 | Handler is leader-only, credential-free importer (same SSRF-protected client as save flow), 30s probe fetch bound | PASS by construction + code review | `openapidrift.go` |
| B9 | Go suites green: `pkg/openapi`, `pkg/controller/handlers/mcpservercatalogentry`, `pkg/api/handlers` (`-count=1`) in both 02 and 05 worktrees | PASS | transcripts |

## Screenshots

1. `evidence/WORKER-METRICS.png` — real `/metrics` output scraped over mTLS from `compose-openapi-worker-1`, after 5× healthz/readyz traffic, with counters showing `5`/`5`.
2. `evidence/DRIFT-DETECTED-API.png` — real Obot API response for the catalog entry showing `openAPISourceDrift.detected=true` with the exact added/removed/changed operation lists and both digests.
3. `evidence/DRIFT-OBOT-UI.png` — Obot local stack UI context.

## Boundaries (not claimed)

- No real Prometheus/Grafana server was deployed; the endpoint's scrape compatibility is asserted from the exposition format and content-type. Wiring a real dashboard is deployment work.
- No non-admin user market-visibility comparison (local auth disabled; same boundary as v1 acceptance).
- Production 6h interval not waited out; accelerated 30s env override used and itself is the shipped configuration mechanism.
- Drift detection does not cover upstream auth/behavioral changes invisible in the schema (e.g. response field semantic drift).
- K8s deployment acceptance for these increments not rerun; Compose-level acceptance only. The K8s example manifest needs no change (`/metrics` rides the existing TLS port; prober cert already exists).
