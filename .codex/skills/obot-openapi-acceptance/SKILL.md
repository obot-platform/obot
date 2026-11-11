---
name: obot-openapi-acceptance
description: Run the standard Obot OpenAPI Workers acceptance pass — local Compose stack, real synthetic upstream, end-to-end verification with screenshots, and evidence packaging. Use when asked to 验收/test-and-accept OpenAPI Workers changes, regenerate acceptance evidence, or produce the screenshot review package.
triggers:
  - 验收 / acceptance / test-and-accept the OpenAPI Workers stack or an increment on it
  - Regenerate ACCEPTANCE evidence after Worker / bridge / proxy / drift / metrics changes
  - Produce the screenshot-based review package (验收截图包) for user review
---

# Obot OpenAPI Workers Acceptance

The standard acceptance for this project is always the same shape: **run the real thing, prove each claim with a number or a screenshot, never claim what was not run.** This skill encodes the v1 acceptance pass (I05R) and the operations increments pass (`/metrics`, drift detection) into one repeatable procedure.

## Non-negotiable rules

1. **Real stack only.** Claims come from the running Compose stack (or isolated K3s for scale tests), never from unit tests alone. Unit/formal results are cited as *additional* evidence, not as acceptance.
2. **Synthetic data only.** Fake upstreams with switchable schemas, synthetic certs via `generate-local-certs.sh`, generated passwords. No real business APIs, no real credentials, no historical JWTs.
3. **Isolation.** Local dev-mode flags (`OBOT_DEV_MODE=true`) are allowed *inside the disposable stack* and must be listed as a boundary in the evidence. Never touch user services or the production cluster.
4. **Evidence triple.** Every check gets: result (PASS/FAIL), the exact command or request that produced it, and a pointer (screenshot path or transcript excerpt). A check without evidence is a finding, not a pass.
5. **Boundaries section is mandatory.** State what was *not* covered (no real Prometheus, accelerated intervals, auth-disabled local user comparisons, etc.). Overclaiming is the only unacceptable outcome.
6. **Detect-vs-adopt separation.** Any drift/telemetry acceptance must prove both directions: the change is *detected/reported*, and it *does not take effect* until the explicit administrator flow runs.

## Stack bring-up (deploy/compose)

```bash
cd <worktree>/deploy/compose
openssl rand -hex 16 > /tmp/openapi-compose-password
./generate-local-certs.sh          # if runtime/ is stale: chmod -R u+w runtime && rm -rf runtime first
OBOT_POSTGRES_PASSWORD="$(cat /tmp/openapi-compose-password)" docker compose up -d --build
```

Critical gotchas learned the hard way:

- **Prebuilt images do not rebuild.** `obot:openapi-workers-local` and `obot-openapi-worker:local` are referenced by tag; `compose up --build` only rebuilds services with a `build:` stanza. After changing Go or Worker code you MUST rebuild both images explicitly:
  ```bash
  docker build -q -t obot-openapi-worker:local <worktree>/workers/openapi
  docker build -q -t obot:openapi-workers-local --build-arg MODEL_DATA_URL= <worktree>
  # then: OBOT_POSTGRES_PASSWORD=... docker compose up -d --force-recreate
  ```
  Symptom of skipping this: new endpoints 404, old behavior persists, "unsupported runtime" from stale validators.
- **Verify the binary actually shipped**: `docker exec compose-obot-1 sh -c 'strings $(which obot) | grep -m1 "<new-symbol>"'`.
- Required env in compose for OpenAPI acceptance: `MCPRuntimeBackend: openapi-worker`, `OBOT_DEV_MODE: "true"` (http sources), `MCPOpenAPIDriftProbeInterval: 30s` (accelerated probing; production default 6h).
- Worker egress allowlist: `OPENAPI_WORKER_ALLOWED_EGRESS_HOSTS=host.docker.internal:<port>` must include the synthetic upstream port.
- Readiness: `curl http://127.0.0.1:18080/api/version` → `"engine": "openapi-worker"`, `hostingCapabilities.openAPIWorker: true`.

## Synthetic drift upstream

Use the hand-written schema server (no framework magic, deterministic paths): serve `/openapi.json` returning v1 or v2 based on in-process state, with `GET /__set_version?v=1|2` and `GET /__version` toggles. v1/v2 must differ in all three drift classes: added path, removed path, changed parameter type. Serve `servers: [{"url": "http://host.docker.internal:<port>"}]`. Keep the toggle endpoints OUT of both schemas.

## Standard check matrix

### Worker /metrics (or any new ops endpoint)
1. Fetch over mTLS in-container with the prober cert (`ssl.create_default_context(cafile=server-ca)` + `load_cert_chain(prober)`); assert 200 + `text/plain; version=0.0.4`.
2. Generate known traffic (N× healthz/readyz, one describe miss) and assert counters show exactly N (numeric equality, not "nonzero").
3. mTLS negative: same fetch without client cert must fail at handshake.
4. Inspect rendered output: no requestIDs, digests, URLs, header names, or credential-looking strings anywhere.
5. Screenshot the real output in a dark terminal-style page (`evidence/WORKER-METRICS.png` style).

### Drift detection
1. Save entry from v1 (or v2) via `POST /api/mcp-catalogs/default/entries` — body is the manifest at top level (NOT wrapped in `{"manifest": ...}`; that yields "unsupported runtime" with runtime swallowed).
2. First probe must be clean: equal digests, no `detected`.
3. Flip upstream version; within one interval assert `openAPISourceDrift.detected=true` AND exact `addedOperations`/`removedOperations`/`changedOperations` lists (string-array equality).
4. **No-auto-apply proof**: after detection, snapshot digest unchanged. Record both digests.
5. Adoption proof: re-save entry (PUT), assert snapshot digest switched to the new schema and next probe is clean.
6. Screenshot the detected=true API response (`DRIFT-DETECTED-API.png` style).

### Regression floor (before declaring acceptance)
```bash
go build ./pkg/... && gofmt -l <changed files>
go test -count=1 ./pkg/openapi/ ./pkg/openapiworker/ ./pkg/controller/handlers/mcpservercatalogentry/ ./pkg/api/handlers/
cd workers/openapi && .venv/bin/python -m pytest -q        # 3× for flake isolation (helper-loop fixture)
cd ui/user && pnpm run check                                # if UI touched
```

## Screenshot procedure (sandbox browser)

1. Write raw evidence (API JSON, metrics text) to a temp file first — screenshots render *captured real output*, never hand-typed data.
2. Use Playwright from `ui/user` (`node` script importing 'playwright', chromium is cached at `~/Library/Caches/ms-playwright`).
3. Render a dark evidence page (`setContent`) that embeds the captured output verbatim with syntax highlighting; title states what it is and where it came from; footer states the boundary.
4. Save to `docs/plans/openapi-workers/evidence/<NAME>.png`, then **view the PNG yourself** and verify the content matches the captured data (this step once caught a rendering bug injecting `v">` garbage).
5. UI-context shots: `page.goto('http://127.0.0.1:18080/...')`, `networkidle`, full-page screenshot.

## Evidence packaging

Write `docs/plans/openapi-workers/evidence/<TOPIC>-ACCEPTANCE.md`:

- Header: date, worktree, image digests (short), env flags.
- Environment section: topology + synthetic upstream description + accelerated intervals.
- Check matrix table: `# | Check | Result | Evidence pointer` — every row PASS/FAIL, no "looks good".
- Screenshots section: each path + one line on what it proves.
- Boundaries section (mandatory): what was NOT covered and why it is not claimed.
- For user-facing review, optionally assemble an HTML index page following `ACCEPTANCE-REVIEW-v1.html` conventions (11-step main line, one screenshot per step, architecture before/after, matrix, gaps).

## Cleanup

```bash
cd deploy/compose && docker compose down -v
pkill -f "uvicorn main:app"; pkill -f "drift-upstream/main.py"   # synthetic upstreams
rm -f /tmp/openapi-compose-password /tmp/worker-metrics*.txt
chmod -R u+w runtime 2>/dev/null && rm -rf runtime               # regenerable certs
```

## Failure interpretation

- "unsupported runtime" on entry save with correct body → stale Obot image or `MCPRuntimeBackend` not `openapi-worker`; check `/api/version`.
- New endpoint 404 in-container → stale image; rebuild both tags (see gotchas).
- Probe never fires → status already holds a future `nextProbeAt` from a longer interval; recreate the entry (delete+create) rather than editing status.
- Metrics render artifacts in screenshots → regex highlight bug in the render script, not in the endpoint; fix the script and re-shoot (verify by viewing the PNG).
- Worker pytest flaky across files → compile-helper loop leakage across asyncio loops; the `test_metrics.py` autouse fixture is the fix pattern; do not delete it to "unflake".
