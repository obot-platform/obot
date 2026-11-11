
import socket
import secrets
import threading
import time

import httpx
import pytest
import uvicorn

from openapi_worker import app as app_module


@pytest.fixture
async def warm_state(snapshot):
    await app_module.state.registry.ensure(snapshot[0])
    yield app_module.state
    app_module.state.request_states.clear()


async def test_four_routes_json_and_cold_miss_fence(snapshot):
    transport = httpx.ASGITransport(app=app_module.app)
    async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
        missing = await client.post("/internal/openapi/v1/describe", json={"protocolVersion": 1, "executionDigest": snapshot[0]["executionDigest"]})
        assert missing.status_code == 404 and missing.headers["content-type"].startswith("application/json")
        assert missing.json()["code"] == "snapshot_required"
        health = await client.get("/healthz")
        assert health.status_code == 200 and health.json()["status"] == "ok"
        assert (await client.get("/mcp")).status_code == 404
        assert (await client.post("/ensure", json={})).status_code == 404


async def test_execute_cold_miss_then_one_fence(snapshot, lease, monkeypatch):
    credential = secrets.token_urlsafe(24)
    payload = {"protocolVersion": 1, "requestID": "request-fence", "executionDigest": snapshot[0]["executionDigest"], "operationKey": "GET /items/{id}", "input": {"kind": "tool", "parameters": {"id": "42"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1, "lease": lease, "credentials": {"authtoken": credential}}
    transport = httpx.ASGITransport(app=app_module.app)
    calls = []

    def upstream(request):
        calls.append(request)
        return httpx.Response(200, content=b"ok")

    app_module.state.executor = app_module.Executor(httpx.MockTransport(upstream))
    try:
        async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
            first = await client.post("/internal/openapi/v1/execute", json=payload)
            assert first.status_code == 409 and first.json()["upstream_attempted"] is False and not calls
            payload["snapshot"] = snapshot[0]
            second = await client.post("/internal/openapi/v1/execute", json=payload)
            assert second.status_code == 200 and second.json()["kind"] == "business" and len(calls) == 1
            third = await client.post("/internal/openapi/v1/execute", json=payload)
            assert third.status_code == 400 and third.json()["code"] == "invalid_request" and len(calls) == 1
    finally:
        app_module.state.request_states.clear()


async def test_invalid_resupply_consumes_recovery_attempt(snapshot, lease):
    app_module.state.registry.entries.clear()
    app_module.state.registry.bytes = 0
    app_module.state.request_states.clear()
    credential = secrets.token_urlsafe(24)
    base = {"protocolVersion": 1, "requestID": "request-invalid-resupply", "executionDigest": snapshot[0]["executionDigest"], "operationKey": "GET /items/{id}", "input": {"kind": "tool", "parameters": {"id": "42"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1, "lease": lease, "credentials": {"authtoken": credential}}
    invalid = dict(snapshot[0])
    invalid["executionDigest"] = "0" * 64
    calls = []

    def upstream(request):
        calls.append(request)
        return httpx.Response(200)

    app_module.state.executor = app_module.Executor(httpx.MockTransport(upstream))
    transport = httpx.ASGITransport(app=app_module.app)
    try:
        async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
            first = await client.post("/internal/openapi/v1/execute", json=base)
            assert first.status_code == 409 and first.json()["workerInstanceID"] == app_module.state.instance_id
            invalid_attempt = dict(base)
            invalid_attempt["snapshot"] = invalid
            rejected = await client.post("/internal/openapi/v1/execute", json=invalid_attempt)
            assert rejected.status_code == 422 and rejected.json()["code"] == "snapshot_invalid"
            valid_attempt = dict(base)
            valid_attempt["snapshot"] = snapshot[0]
            blocked = await client.post("/internal/openapi/v1/execute", json=valid_attempt)
            assert blocked.status_code == 422 and blocked.json()["code"] == "snapshot_recovery_failed"
            assert not calls
    finally:
        app_module.state.request_states.clear()


async def test_real_random_port_server_is_ready(snapshot):
    config = uvicorn.Config(app_module.app, host="127.0.0.1", port=0, lifespan="on", log_level="warning")
    server = uvicorn.Server(config)
    thread = threading.Thread(target=server.run, daemon=True)
    thread.start()
    for _ in range(100):
        if server.started:
            break
        time.sleep(0.02)
    port = server.servers[0].sockets[0].getsockname()[1]
    async with httpx.AsyncClient(base_url=f"http://127.0.0.1:{port}") as client:
        health = await client.get("/healthz")
        ready = await client.get("/readyz")
        assert health.status_code == 200 and health.headers["content-type"].startswith("application/json")
        assert ready.status_code == 200 and ready.json()["status"] == "ready"
    server.should_exit = True
    thread.join(3)
