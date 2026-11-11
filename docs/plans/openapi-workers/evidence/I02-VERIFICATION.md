# I02R Verification Evidence

Date: 2026-11-09 Asia/Shanghai. Worktree: `/Users/echo/orca/workspaces/obot/openapi-worker-bridge`. Baseline: `2be04f025d1c86144e8670d9e9341b263edc28ea`.

## Environment and approvals

- Go: `go1.27.0 darwin/arm64`; commands ran from the I02 worktree and were prefixed with RTK as required.
- I01 checkout: `/Users/echo/orca/workspaces/obot/openapi-shared-worker/workers/openapi`.
- 00 reply `msg_52f88cb590a0` approved the minimal I01 `Authorization` credential-exception fix in `snapshot.py` plus positive/negative coverage in `test_snapshot.py`.
- Credentials were synthetic in-memory values. No real JWT, historical credential, production service, Kubernetes cluster, Docker daemon, or real business API was accessed.

## Commands and results

| ID | Command / scenario | Result | Boundary |
| --- | --- | --- | --- |
| V01 | `rtk gofmt -w ...` across all I02 Go files | PASS | Formatting only |
| V02 | `rtk proxy go test ./pkg/openapiworker ./pkg/mcp ./pkg/api/handlers/mcpgateway ./pkg/services ./pkg/api/handlers ./pkg/api/authz` | PASS | Required affected Go packages |
| V03 | `rtk proxy env I01_OPENAPI_WORKER_ROOT=... go test ./pkg/openapiworker -run TestRealI01PythonWorkerDescribeAndExecute -count=1 -v` | PASS | Actual I01 Python process on a random localhost port; TLS fake business upstream |
| V04 | `rtk uv run pytest -q` in I01 `workers/openapi` | PASS: 23 passed, 2 warnings | I01 targeted suite |
| V05 | `rtk proxy go vet ./pkg/openapiworker ./pkg/mcp ./pkg/api/handlers/mcpgateway ./pkg/services ./pkg/api/handlers` | PASS | Static vet |
| V06 | `rtk git diff --check` | PASS | Diff hygiene |
| V07 | `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh` | PASS / expected broken failure | TLA+ design model |
| V08 | `rtk proxy go test ./pkg/openapiworker -count=1` after the I03 interface unblock | PASS | All OpenAPI Worker Go regressions |
| V09 | `rtk proxy go vet ./pkg/openapiworker` after the I03 interface unblock | PASS | Static vet |
| V10 | `rtk proxy env I01_OPENAPI_WORKER_ROOT=... go test ./pkg/openapiworker -run TestRealI01PythonWorkerDescribeAndExecute -count=1 -v` after the I03 interface unblock | PASS | Real I01 describe/execute remains interoperable with synthetic credentials and fake upstream |
| V11 | `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh` rerun | PASS / expected broken failure | TLA+ design model after metadata-only resolver change |

Formal reruns completed 2026-11-09 15:09:29–15:09:47 UTC and, after the I03 metadata-only interface change, 15:26:56–15:27:10 UTC with TLA+ Tools v1.8.0 SHA-256 `e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f`. Restored model: 18 distinct states; `ExactCallerCredentialOnly`, `TypeOK`, `NoDeadlock`, and `CHECK_DEADLOCK` hold. Broken model: 19 distinct states; only `ExactCallerCredentialOnly` fails with caller `u2` / requested `i2` falling back to `u1/i1`.

## Real I01 interop evidence

- I02 starts the real I01 `uvicorn` process, not a Go fake Worker, on a randomly selected free localhost port in explicit test-HTTP mode.
- The fake business upstream is a separate random-port HTTPS test server using a generated test certificate and synthetic response.
- Cold `describe` supplies the Go-generated canonical snapshot containing exactly `manifestBase64`, `policyBase64`, `schemaBase64`, `protocolVersion`, and `executionDigest`; I01 rechecks and accepts the digest.
- Describe pagination follows `nextCursor` until `complete`; the wire response uses `kind`, `status`, `protocolVersion`, and paged operation fields.
- Execute uses nested `input.kind=tool`, `parameters`, `deadlineUnixMs`, exact typed lease fields, top-level `credentials`, and returns I01's list-shaped business headers/body envelope.
- Describe metadata now includes each authorized operation's `method`, `pathTemplate`, and parameters (name, location, required, style, explode, and schema), enabling I03 to strict-match a raw path locally and pass the already-resolved `operationKey` to the unchanged Executor.
- The shared path vector binds `/items/42`; static percent-equivalent paths and `%2F` path data remain covered by Go/I01 regressions.
- The shared header vector forwards `X-Business`, drops caller-supplied `Authorization`, and sends only the exact synthetic instance credential as the final `Authorization` value. I01 tests also reject forwardable `Authorization`.
- Execute returns HTTP 200 and the exact fake-upstream JSON body to Go; no execute retry is introduced.

## 2026-11-10 continuation rerun

- `rtk bash .codex/skills/verify-openapi-worker-bridge/verify.sh`: PASS; restored model held all required invariants across 18 states, while the intentionally broken model failed only `ExactCallerCredentialOnly` across 19 states.
- `rtk proxy go test ./pkg/openapiworker ./pkg/mcp ./pkg/api/handlers/mcpgateway ./pkg/services ./pkg/api/handlers ./pkg/api/authz`: PASS.
- `rtk proxy go vet ./pkg/openapiworker ./pkg/mcp ./pkg/api/handlers/mcpgateway ./pkg/services ./pkg/api/handlers`: PASS.
- `rtk proxy env I01_OPENAPI_WORKER_ROOT=/Users/echo/orca/workspaces/obot/openapi-shared-worker/workers/openapi go test ./pkg/openapiworker -run TestRealI01PythonWorkerDescribeAndExecute -count=1 -v`: PASS in 0.61s with the real I01 Python Worker and synthetic TLS fake business upstream.
- `rtk git diff --check`: PASS.

## Not verified

- No full Obot HTTP/MCP process E2E, PostgreSQL lease/budget flow, two-user concurrent process test, multi-replica test, business-error matrix, chunked/non-JSON/large-response matrix, or cancellation matrix was run in this I02R pass; those remain I04/I05 or later integration work.
- No real enterprise authorization exchange, certificate rotation, Kubernetes deployment, production deployment, commit, or push was performed.
