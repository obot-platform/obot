# Obot OpenAPI Shared Worker

This package implements the frozen I01 internal Worker protocol: only bounded JSON `describe`/`execute` plus dedicated health endpoints. It accepts an Obot-produced canonical manifest and immutable schema, validates digests before compilation, keeps per-request credentials outside snapshots, and performs one upstream attempt per fenced request.

## Local verification

```sh
cd workers/openapi
uv sync --locked
uv run pytest -q
uv lock --check
docker build -t obot-openapi-worker:local .
```

Unit tests inject synthetic transports and generate credentials in process. Loopback HTTP requires an explicit test environment flag; production requires TLS/mTLS configuration and rejects private/HTTP upstreams unless I05 supplies the frozen narrow allowlist. The ASGI application exposes no `/mcp` or `/ensure` route.
