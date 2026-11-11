# I01 Shared Worker Verification Evidence

Date: 2026-11-09 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-shared-worker`. Branch: `feature/openapi-shared-worker`; baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Environment

- Host: macOS arm64.
- Local Python selected by `uv`: CPython 3.14.4.
- Image base: `python:3.12-slim` (digest `sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a3fa5b`).
- No real business API, historical JWT, production service, cluster, commit, or push was used.

## Commands And Results

| Time | Command / check | Result |
| --- | --- | --- |
| 2026-11-09 22:14:35 +0800 | `date` and image/working-tree inspection | Current image digest recorded; `git diff --check` clean. |
| 2026-11-09 22:14 | `rtk bash .codex/skills/verify-openapi-shared-worker/scripts/verify.sh` | Exit `0`; broken model exited `1` with the three expected violations; restored model exited `0` with all rules holding. |
| 2026-11-09 22:15–22:16 | `rtk docker build -t obot-openapi-worker:local .` | Exit `0`; image ID `sha256:aaee615dbc40cded87238f0fd07badcda048b5ecfd913d18890258561d72820c`, created `2026-11-09T22:16:29.207292321+08:00`. |
| 2026-11-09 22:19 | `cd workers/openapi && uv run pytest -q && uv lock --check && git diff --check` plus credential/artifact scan | Exit `0`; 22 tests passed in 0.45s with 2 upstream dependency deprecation warnings; lock resolved 76 packages; no fixed synthetic credential string found. |
| 2026-11-09 22:14 | `uv run python -m compileall -q openapi_worker tests` | Exit `0`. |

## Behavioral Evidence

- Snapshot vectors verify canonical execution digest input, schema digest binding, wrong digest rejection, non-canonical manifest rejection, and size-related envelope paths.
- Registry tests hold two snapshots concurrently and collapse 20 same-digest cold requests to one compile.
- ASGI tests verify four-route exclusivity, JSON `snapshot_required`, `workerInstanceID`, same-request one-shot fence, invalid-resupply rejection with zero upstream calls, and real random-port `healthz`/`readyz` responses.
- Executor tests verify typed lease preflight with zero upstream contact, top-level HTTP `operationKey` binding, raw query order/repetition, final credential isolation, original business 403/body/multi-value `Set-Cookie`, reserved platform header stripping, and upstream SSE rejection.
- Egress tests verify DNS answer enumeration, pinned IP connection construction, original SNI/Host, and mixed public/private DNS rejection before transport dispatch.

## Boundaries

- This is I01 local implementation verification, not I02 bridge integration, full cross-language canonical-vector completion, real mTLS handshake, Kubernetes deployment, or end-to-end acceptance.
- The production entrypoint refuses to start without server certificate, private key, and client CA; a real mTLS handshake is reserved for I05.
- Default egress rejects private/HTTP upstreams. The production file-based exact host/port/CIDR allowlist is an I05 integration item; current tests cover default rejection and pinned public transport.
- Business side effects are not claimed exactly-once. Verified behavior is one Worker upstream attempt per fenced request and no automatic retry after an uncertain result.
