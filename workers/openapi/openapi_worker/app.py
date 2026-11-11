import json
import os
import ssl
import time
from contextlib import asynccontextmanager

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, PlainTextResponse
from starlette.routing import Route

from .errors import WorkerError
from .executor import Executor
from .httpclient import EgressPolicy
from .metrics import WorkerMetrics, record_executor_error
from .protocol import DESCRIBE_REQUEST_LIMIT, ERROR_LIMIT, HEALTH_LIMIT, PROTOCOL_VERSION, check_protocol, safe_response
from .registry import SnapshotRegistry


class WorkerState:
    def __init__(self, policy: EgressPolicy | None = None) -> None:
        self.registry = SnapshotRegistry()
        self.executor = Executor(policy=policy)
        self.request_states: dict[str, tuple[str, bool, bool]] = {}
        self.instance_id = os.environ.get("OPENAPI_WORKER_INSTANCE_ID", "worker-1")
        self.auth_token = os.environ.get("OPENAPI_WORKER_TEST_BEARER")
        self.closed = False
        self.metrics = WorkerMetrics()

    def check_auth(self, request: Request) -> None:
        if self.auth_token and request.headers.get("x-obot-worker-authentication") != f"Bearer {self.auth_token}":
            raise WorkerError("platform_unauthenticated", "worker service authentication failed")


def _egress_policy() -> EgressPolicy:
    local_test = os.environ.get("OPENAPI_WORKER_ALLOW_LOCAL_TEST_EGRESS") == "1"
    allowed = {"127.0.0.1", "localhost"}
    for entry in os.environ.get("OPENAPI_WORKER_ALLOWED_EGRESS_HOSTS", "").split(","):
        entry = entry.strip()
        if entry:
            allowed.add(entry.rsplit(":", 1)[0] if ":" in entry else entry)
    return EgressPolicy(allow_private=local_test, allow_http=local_test, allowed_hosts=allowed)


state = WorkerState(_egress_policy())


@asynccontextmanager
async def lifespan(_app):
    await state.registry.helper.start()
    yield
    state.closed = True
    await state.registry.close()


def error_response(exc: WorkerError, status: int = 400, request_id: str | None = None) -> JSONResponse:
    try:
        payload = safe_response(exc.payload(request_id), ERROR_LIMIT)
    except WorkerError:
        payload = {"kind": "error", "code": "response_limit_exceeded", "requestID": request_id, "retryable": False}
    if exc.code == "snapshot_required":
        payload["workerInstanceID"] = state.instance_id
    return JSONResponse(payload, status_code=status, media_type="application/json")


async def read_json(request: Request, limit: int) -> dict:
    if int(request.headers.get("content-length", 0)) > limit:
        raise WorkerError("request_too_large", "request envelope exceeds limit")
    body = bytearray()
    async for chunk in request.stream():
        body.extend(chunk)
        if len(body) > limit:
            raise WorkerError("request_too_large", "request envelope exceeds limit")
    try:
        value = json.loads(bytes(body).decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise WorkerError("invalid_request", "invalid JSON") from exc
    if not isinstance(value, dict):
        raise WorkerError("invalid_request", "JSON object required")
    return value


async def health(request: Request):
    state.metrics.record_route("healthz", "ok")
    return JSONResponse(safe_response({"kind": "health", "status": "ok", "protocolVersion": PROTOCOL_VERSION, "workerInstanceID": state.instance_id}, HEALTH_LIMIT))


async def ready(request: Request):
    state.metrics.record_route("readyz", "ok")
    helper_ready = len(state.registry.helper._processes) >= state.registry.helper.helpers and not state.closed
    payload = {"kind": "ready", "status": "ready" if helper_ready else "unavailable", "protocolVersion": PROTOCOL_VERSION, "workerInstanceID": state.instance_id, "cacheEntries": len(state.registry.entries), "cacheBytes": state.registry.bytes, "activeReferences": sum(state.registry.active.values())}
    return JSONResponse(safe_response(payload, HEALTH_LIMIT), status_code=200 if helper_ready else 503)


async def metrics(_request: Request):
    # Same authenticated mTLS listener as the internal protocol; no secrets,
    # bounded cardinality, and no per-request identifiers in the payload.
    return PlainTextResponse(state.metrics.render(state.registry, sum(state.registry.active.values())), media_type="text/plain; version=0.0.4; charset=utf-8")


async def describe(request: Request):
    started = time.monotonic()
    outcome = "ok"
    try:
        state.check_auth(request)
        payload = await read_json(request, DESCRIBE_REQUEST_LIMIT)
        check_protocol(payload.get("protocolVersion"))
        digest = payload.get("executionDigest")
        if not isinstance(digest, str):
            raise WorkerError("invalid_request", "executionDigest is required")
        snapshot = await state.registry.get(digest)
        if snapshot is None:
            if "snapshot" not in payload:
                state.metrics.record_snapshot_event("snapshot_required")
                raise WorkerError("snapshot_required", "snapshot is required", retryable=True)
            snapshot = await state.registry.ensure(payload["snapshot"])
        page_size = min(int(payload.get("limit", 32)), 64)
        cursor = max(0, int(payload.get("cursor", 0)))
        values = sorted(snapshot.operations.values(), key=lambda operation: operation.operation_key)
        selected = values[cursor:cursor + page_size]
        complete = cursor + len(selected) >= len(values)
        operations = [{"operationKey": operation.operation_key, "legacyToolName": operation.legacy_tool_name, "method": operation.method, "pathTemplate": operation.path_template, "inputSchema": operation.input_schema, "parameters": operation.parameters} for operation in selected]
        response = {"kind": "describe", "status": "ready", "protocolVersion": PROTOCOL_VERSION, "executionDigest": digest, "operations": operations, "nextCursor": None if complete else cursor + page_size, "totalOperations": len(values), "complete": complete}
        return JSONResponse(safe_response(response, 4 * 1024 * 1024))
    except WorkerError as exc:
        outcome = "error"
        state.metrics.record_error("describe", exc.code)
        record_executor_error(state.metrics, exc)
        return error_response(exc, 404 if exc.code == "snapshot_required" else 400)
    except Exception:
        outcome = "error"
        state.metrics.record_error("describe", "internal_error")
        return error_response(WorkerError("internal_error", "internal worker error"), 500)
    finally:
        state.metrics.record_route("describe", outcome)
        state.metrics.record_route_duration("describe", time.monotonic() - started)


async def execute(request: Request):
    request_id = None
    started = time.monotonic()
    outcome = "ok"
    try:
        state.check_auth(request)
        payload = await read_json(request, 6 * 1024 * 1024 + 256 * 1024)
        check_protocol(payload.get("protocolVersion"))
        request_id = payload.get("requestID")
        digest = payload.get("executionDigest")
        if not isinstance(request_id, str) or not isinstance(digest, str):
            raise WorkerError("invalid_request", "requestID and executionDigest are required")
        previous = state.request_states.get(request_id)
        if previous is not None and previous[0] != digest:
            raise WorkerError("invalid_request", "requestID is bound to another executionDigest")
        snapshot = await state.registry.get(digest)
        if snapshot is None:
            if previous is not None and previous[2]:
                state.metrics.record_snapshot_event("snapshot_recovery_failed")
                raise WorkerError("snapshot_recovery_failed", "snapshot was already supplied once")
            if "snapshot" not in payload:
                state.request_states[request_id] = (digest, False, False)
                state.metrics.record_snapshot_event("snapshot_required")
                raise WorkerError("snapshot_required", "snapshot is required", retryable=True)
            if previous is not None:
                state.request_states[request_id] = (digest, False, True)
            snapshot = await state.registry.ensure(payload["snapshot"])
        if previous is not None and previous[1]:
            raise WorkerError("invalid_request", "requestID already entered execution")
        state.request_states[request_id] = (digest, True, previous is not None)
        snapshot = await state.registry.acquire(digest)
        try:
            result = await state.executor.execute(snapshot, payload)
            if result.get("kind") == "business" and isinstance(result.get("status"), int):
                state.metrics.record_upstream(result["status"], "completed")
            return JSONResponse(safe_response(result, 12 * 1024 * 1024))
        finally:
            await state.registry.release(digest)
    except WorkerError as exc:
        outcome = "error"
        status = {"snapshot_required": 409, "snapshot_invalid": 422, "snapshot_recovery_failed": 422, "snapshot_too_large": 413, "snapshot_cache_full": 503, "compile_busy": 503, "compile_timeout": 504, "lease_invalid": 422, "transport_uncertain": 502, "response_too_large": 502, "upstream_protocol_unsupported": 502}.get(exc.code, 400)
        if exc.upstream_attempted:
            state.metrics.record_upstream(0, "uncertain" if exc.code == "transport_uncertain" else "failed")
        state.metrics.record_error("execute", exc.code)
        record_executor_error(state.metrics, exc)
        return error_response(exc, status, request_id)
    except Exception:
        outcome = "error"
        state.metrics.record_error("execute", "internal_error")
        return error_response(WorkerError("internal_error", "internal worker error", phase="internal"), 500, request_id)
    finally:
        state.metrics.record_route("execute", outcome)
        state.metrics.record_route_duration("execute", time.monotonic() - started)


app = Starlette(routes=[Route("/internal/openapi/v1/describe", describe, methods=["POST"]), Route("/internal/openapi/v1/execute", execute, methods=["POST"]), Route("/healthz", health, methods=["GET"]), Route("/readyz", ready, methods=["GET"]), Route("/metrics", metrics, methods=["GET"])], lifespan=lifespan)


def serve() -> None:
    import uvicorn

    test_http = os.environ.get("OPENAPI_WORKER_TEST_HTTP") == "1"
    cert_file = os.environ.get("OPENAPI_WORKER_TLS_CERT_FILE")
    key_file = os.environ.get("OPENAPI_WORKER_TLS_KEY_FILE")
    client_ca_file = os.environ.get("OPENAPI_WORKER_CLIENT_CA_FILE")
    if test_http:
        if cert_file or key_file or client_ca_file:
            raise RuntimeError("test HTTP cannot be combined with TLS configuration")
        uvicorn.run(app, host="127.0.0.1", port=int(os.environ.get("OPENAPI_WORKER_PORT", "8080")))
        return
    if not cert_file or not key_file or not client_ca_file:
        raise RuntimeError("production Worker requires server certificate, key, and client CA")
    uvicorn.run(
        app, host="0.0.0.0", port=int(os.environ.get("OPENAPI_WORKER_PORT", "8080")),
        ssl_certfile=cert_file, ssl_keyfile=key_file, ssl_ca_certs=client_ca_file,
        ssl_cert_reqs=ssl.CERT_REQUIRED,
    )


if __name__ == "__main__":
    serve()
