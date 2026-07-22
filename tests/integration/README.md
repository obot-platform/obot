# Integration tests

These tests exercise obot over HTTP. The test process starts an isolated obot server with a temporary SQLite database, runs the tests, and shuts the server down. They are not part of `make test` — they live behind the `integration` build tag and are run via `make test-integration`.

## Prerequisites

1. **Docker is reachable** for the MCP-runtime tests. The test suite builds and caches a small local MCP fixture image, uses the Docker MCP runtime backend, and removes its containers afterward. When `DOCKER_HOST` is unset, it discovers the endpoint from the active Docker context.

## Running

From the repo root:

```sh
make test-integration
```

The server starts on dynamically selected local ports, uses authentication-free dev mode, and removes its temporary data after the run.

To run a single test:

```sh
go test -count=1 -v -tags=integration -run TestMCPServerLifecycle_Containerized ./tests/integration/...
```

## Layout

```
tests/integration/
  main_test.go          isolated obot startup, readiness, and shutdown
  testdata/mcpserver/   minimal MCP HTTP server and Dockerfile for the cached fixture image
  harness/
    harness.go    server URL and per-run resource tagging
    client.go     thin HTTP helpers (GET/POST/PUT/DELETE) using apiclient/types
    fixtures.go   higher-level helpers (CreateMCPServer, WaitForMCPServerAvailable, etc.)
  mcp_lifecycle_test.go   the first end-to-end test (multi-step MCP server flow)
```

## Conventions

- Every test file starts with `//go:build integration` so the default `make test` ignores it.
- `TestMain` starts one isolated obot server for the package.
- `harness.New(t)` is the per-test entry point. It registers a `t.Cleanup` that deletes every resource created via the harness, so partially-failed tests do not leave junk behind.
- Resource names are prefixed with `test-<runID>-` (6-byte random ID) so concurrent runs don't collide.
- The harness uses production types from `apiclient/types` — if a type changes, the tests break at compile time.

## Adding a test

1. New file under `tests/integration/`, prefixed with `//go:build integration`.
2. Call `harness.New(t)` at the top. Use `h.Get / h.Post / ...` for raw HTTP, or extend `fixtures.go` if a helper would be reused.
3. Register cleanups for anything you create that the existing fixtures don't already register.
4. Keep each test focused on one flow. Slowness adds up.
