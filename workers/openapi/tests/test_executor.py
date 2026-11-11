
import base64
import json

import httpx
import pytest

from openapi_worker.errors import WorkerError
from openapi_worker.executor import Executor, validate_lease
from openapi_worker.snapshot import compile_snapshot, parse_snapshot


@pytest.fixture
def compiled(snapshot):
    return compile_snapshot(parse_snapshot(snapshot[0]))


def request_payload(snapshot, lease, credential, **overrides):
    payload = {
        "requestID": "request-1", "executionDigest": snapshot[0]["executionDigest"], "operationKey": "GET /items/{id}",
        "input": {"kind": "tool", "parameters": {"id": "42", "tag": "blue"}}, "deadlineUnixMs": lease["fenceDeadlineUnixMs"] - 1,
        "lease": lease, "credentials": {"authtoken": credential},
    }
    payload.update(overrides)
    return payload


async def test_lease_rejection_prevents_upstream(compiled, snapshot, lease, credential):
    lease["fenceDeadlineUnixMs"] = lease["dispatchedAtUnixMs"]
    calls = []

    def handler(request):
        calls.append(request)
        return httpx.Response(200)

    with pytest.raises(WorkerError) as caught:
        await Executor(httpx.MockTransport(handler)).execute(compiled, request_payload(snapshot, lease, credential))
    assert caught.value.code == "lease_invalid" and caught.value.upstream_attempted is False and not calls


async def test_business_result_and_credential_areolated(compiled, snapshot, lease, credential):
    observed = []

    def handler(request):
        observed.append(request)
        return httpx.Response(403, headers=[("set-cookie", "a"), ("set-cookie", "b"), ("x-obot-error-code", "fake")], content=b"business-denied")

    result = await Executor(httpx.MockTransport(handler)).execute(compiled, request_payload(snapshot, lease, credential))
    request = observed[0]
    assert request.url.path == "/items/42"
    assert request.url.query == b"tag=blue"
    assert request.headers["authtoken"] == credential
    assert result["kind"] == "business" and result["status"] == 403
    assert base64.b64decode(result["bodyBase64"]) == b"business-denied"
    assert next(header for header in result["headers"] if header["name"] == "set-cookie") == {"name": "set-cookie", "values": ["a", "b"]}
    assert all(header["name"] != "x-obot-error-code" for header in result["headers"])


async def test_http_input_uses_top_level_operation_binding(compiled, snapshot, lease, credential):
    observed = []

    def handler(request):
        observed.append(request)
        return httpx.Response(204)

    payload = request_payload(snapshot, lease, credential, input={"kind": "http", "method": "GET", "rawPath": "/items/42", "rawQuery": "tag=blue&tag=red"}, bodyBase64=None)
    result = await Executor(httpx.MockTransport(handler)).execute(compiled, payload)
    assert result["status"] == 204
    assert observed[0].method == "GET" and observed[0].url.path == "/items/42"
    assert observed[0].url.query == b"tag=blue&tag=red"


async def test_upstream_sse_is_rejected_without_success_body(compiled, snapshot, lease, credential):
    def handler(request):
        return httpx.Response(200, headers={"content-type": "text/event-stream"}, content=b"data: x\n\n")

    with pytest.raises(WorkerError) as caught:
        await Executor(httpx.MockTransport(handler)).execute(compiled, request_payload(snapshot, lease, credential))
    assert caught.value.code == "upstream_protocol_unsupported" and caught.value.upstream_attempted is True
