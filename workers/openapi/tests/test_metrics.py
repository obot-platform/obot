import re

import httpx
import pytest

from openapi_worker import app as app_module
from openapi_worker.metrics import WorkerMetrics


@pytest.fixture(autouse=True)
def reset_metrics():
    app_module.state.metrics = WorkerMetrics()
    yield
    # Restore the process-global registry so later tests observe a cold, open
    # state. Subprocess compile helpers bind to one asyncio loop, and
    # pytest-asyncio gives every test a fresh loop, so stale helpers from an
    # earlier test (or the real-server test's lifespan teardown) must be
    # reaped here; CompileHelperPool.compile lazily restarts them in the
    # current loop.
    for process in app_module.state.registry.helper._processes:
        if process.returncode is None:
            process.kill()
    app_module.state.registry.helper._processes.clear()
    app_module.state.registry.closed = False
    app_module.state.registry.entries.clear()
    app_module.state.registry.bytes = 0
    app_module.state.registry.active.clear()
    app_module.state.registry.pending.clear()
    app_module.state.request_states.clear()


async def scrape(client) -> str:
    response = await client.get("/metrics")
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/plain")
    return response.text


def parse_counters(body: str) -> dict[str, float]:
    values: dict[str, float] = {}
    for line in body.splitlines():
        if line.startswith("#") or not line.strip():
            continue
        name, _, value = line.rpartition(" ")
        values[name] = float(value)
    return values


async def test_metrics_exposes_gauges_and_help(snapshot):
    transport = httpx.ASGITransport(app=app_module.app)
    async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
        body = await scrape(client)
    assert "openapi_worker_up 1" in body
    assert "# TYPE openapi_worker_snapshot_cache_entries gauge" in body
    assert "# TYPE openapi_worker_request_seconds histogram" in body
    assert "openapi_worker_snapshot_cache_entries 0" in body


async def test_metrics_counters_track_describe_and_execute(snapshot, lease, monkeypatch):
    credential = "synthetic-credential-value"
    digest = snapshot[0]["executionDigest"]
    calls = []

    def upstream(request):
        calls.append(request)
        return httpx.Response(200, content=b"ok")

    app_module.state.executor = app_module.Executor(httpx.MockTransport(upstream))
    transport = httpx.ASGITransport(app=app_module.app)
    try:
        async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
            await client.post("/internal/openapi/v1/describe", json={"protocolVersion": 1, "executionDigest": digest})
            payload = {"protocolVersion": 1, "requestID": "metrics-1", "executionDigest": digest, "operationKey": "GET /items/{id}", "input": {"kind": "tool", "parameters": {"id": "42"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1, "lease": lease, "credentials": {"authtoken": credential}, "snapshot": snapshot[0]}
            executed = await client.post("/internal/openapi/v1/execute", json=payload)
            assert executed.status_code == 200
            body = await scrape(client)
    finally:
        app_module.state.request_states.clear()

    counters = parse_counters(body)
    assert counters['openapi_worker_snapshot_events_total{event="snapshot_required"}'] == 2
    assert counters['openapi_worker_errors_total{route="describe",code="snapshot_required"}'] == 1
    assert counters['openapi_worker_requests_total{route="describe",outcome="error"}'] == 1
    assert counters['openapi_worker_requests_total{route="execute",outcome="ok"}'] == 1
    assert counters['openapi_worker_upstream_results_total{status_class="2xx",outcome="completed"}'] == 1
    assert "openapi_worker_request_seconds_count{route=\"execute\"} 1" in body


async def test_metrics_never_leak_request_ids_or_credentials(snapshot, lease):
    digest = snapshot[0]["executionDigest"]
    transport = httpx.ASGITransport(app=app_module.app)
    async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
        await client.post("/internal/openapi/v1/execute", json={"protocolVersion": 1, "requestID": "super-secret-request", "executionDigest": digest, "operationKey": "GET /items/{id}", "input": {"kind": "tool", "parameters": {"id": "42"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1, "lease": lease, "credentials": {"authtoken": "secret-token-value"}})
        body = await scrape(client)
    assert "super-secret-request" not in body
    assert "secret-token-value" not in body
    assert digest not in body
    # Every label set uses only fixed-vocabulary values.
    for label_value in re.findall(r'="([^"]+)"', body):
        assert not label_value.startswith("request-")


async def test_metrics_upstream_attempted_and_uncertain_classification(snapshot, lease):
    import httpx as _httpx

    digest = snapshot[0]["executionDigest"]
    credential = "synthetic-credential-value"

    def upstream(request):
        raise _httpx.ConnectError("boom")

    app_module.state.executor = app_module.Executor(httpx.MockTransport(upstream))
    transport = httpx.ASGITransport(app=app_module.app)
    try:
        async with httpx.AsyncClient(transport=transport, base_url="http://worker.test") as client:
            payload = {"protocolVersion": 1, "requestID": "metrics-2", "executionDigest": digest, "operationKey": "GET /items/{id}", "input": {"kind": "tool", "parameters": {"id": "42"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1, "lease": lease, "credentials": {"authtoken": credential}, "snapshot": snapshot[0]}
            failed = await client.post("/internal/openapi/v1/execute", json=payload)
            assert failed.status_code == 502
            body = await scrape(client)
    finally:
        app_module.state.request_states.clear()

    counters = parse_counters(body)
    assert counters.get('openapi_worker_upstream_results_total{status_class="0xx",outcome="uncertain"}') == 1
    assert counters.get('openapi_worker_errors_total{route="execute",code="transport_uncertain"}') == 1
