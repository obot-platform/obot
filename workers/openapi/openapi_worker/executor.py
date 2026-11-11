import asyncio
import base64
import time

import httpx

from .errors import WorkerError
from .httpclient import EgressPolicy, PinnedTransport
from .operations import build_request, match_operation, sanitize_headers
from .protocol import RESPONSE_BODY_LIMIT, RESPONSE_HEADER_LIMIT
from .snapshot import CompiledSnapshot, canonical_header


def validate_lease(lease: object, deadline: int) -> float:
    required = {"leaseID", "leaseIssuedAtUnixMs", "leaseDeadlineUnixMs", "fenceDeadlineUnixMs", "dispatchedAtUnixMs", "budgetKeyHash", "clockSkewLimitMs", "fencingMarginMs"}
    if not isinstance(lease, dict) or set(lease) != required:
        raise WorkerError("lease_invalid", "worker lease fields are invalid")
    try:
        issued = int(lease["leaseIssuedAtUnixMs"])
        lease_deadline = int(lease["leaseDeadlineUnixMs"])
        fence_deadline = int(lease["fenceDeadlineUnixMs"])
        dispatched = int(lease["dispatchedAtUnixMs"])
        skew = int(lease["clockSkewLimitMs"])
        margin = int(lease["fencingMarginMs"])
        if not isinstance(lease["leaseID"], str) or not isinstance(lease["budgetKeyHash"], str):
            raise ValueError
    except (KeyError, TypeError, ValueError) as exc:
        raise WorkerError("lease_invalid", "worker lease fields are invalid") from exc
    if margin <= skew or fence_deadline != lease_deadline - margin or not issued <= dispatched < fence_deadline:
        raise WorkerError("lease_invalid", "worker lease is expired or invalid")
    if fence_deadline < deadline:
        raise WorkerError("lease_invalid", f"worker lease deadline is before the request deadline: {fence_deadline=} {deadline=}")
    if abs(int(time.time() * 1000) - dispatched) > skew:
        raise WorkerError("lease_invalid", "worker clock skew exceeds lease limit")
    return time.monotonic() + max(0.0, (deadline - int(time.time() * 1000)) / 1000)


class Executor:
    def __init__(self, transport: httpx.AsyncBaseTransport | None = None, policy: EgressPolicy | None = None) -> None:
        self.transport = transport
        self.policy = policy
        self.upstream_attempts = 0

    async def execute(self, snapshot: CompiledSnapshot, request: dict) -> dict:
        deadline = int(request["deadlineUnixMs"])
        monotonic_deadline = validate_lease(request.get("lease"), deadline)
        input_value = request.get("input")
        if not isinstance(input_value, dict):
            raise WorkerError("invalid_request", "invalid execution input")
        kind = input_value.get("kind")
        if kind == "http":
            operation, matched_parameters = match_operation(snapshot, input_value["method"], input_value["rawPath"])
            if request.get("operationKey") != operation.operation_key:
                raise WorkerError("invalid_request", "operationKey does not match raw path")
            tool_input = {"rawQuery": input_value.get("rawQuery", "")}
            supplied_headers = input_value.get("headers", [])
            body = base64.b64decode(input_value.get("bodyBase64", ""), validate=True) if input_value.get("bodyBase64") else None
            media_type = input_value.get("bodyMediaType")
        elif kind == "tool":
            operation = snapshot.operations.get(request["operationKey"])
            if operation is None:
                raise WorkerError("operation_not_found", "operation is not published")
            tool_input = input_value.get("parameters", {})
            supplied_headers = []
            body = None
            media_type = None
        else:
            raise WorkerError("invalid_request", "unsupported input kind")
        if body is not None and len(body) > 1024 * 1024:
            raise WorkerError("request_too_large", "request body exceeds limit")
        method, path, parameter_headers, json_body = build_request(snapshot, operation, kind=kind, input_value=tool_input, body_media_type=media_type, matched_parameters=matched_parameters if kind == "http" else None)
        headers = sanitize_headers(snapshot, supplied_headers + parameter_headers)
        if json_body is not None:
            body = json_body
            media_type = media_type or "application/json"
        for credential in snapshot.policy.get("credentials", []):
            supplied = request.get("credentials", {})
            name = credential["name"]
            if not isinstance(supplied, dict) or name not in supplied:
                raise WorkerError("business_authorization_required", "business credential is required")
            value = str(supplied[name])
            prefix = credential.get("prefix")
            if prefix is not None and not value.startswith(prefix):
                raise WorkerError("business_authorization_required", "business credential prefix is invalid")
            headers = [header for header in headers if canonical_header(header["name"]) != canonical_header(name)]
            headers.append({"name": canonical_header(name), "values": [value]})
        http_headers: list[tuple[str, str]] = []
        for header in headers:
            for value in header["values"]:
                http_headers.append((header["name"], value))
        if body is not None and media_type:
            http_headers.append(("content-type", media_type))
        transport = self.transport or PinnedTransport(self.policy or EgressPolicy())
        remaining = max(0.001, monotonic_deadline - time.monotonic())
        timeout = httpx.Timeout(remaining, connect=remaining, read=remaining, write=remaining, pool=remaining)
        client = httpx.AsyncClient(transport=transport, follow_redirects=False, trust_env=False, timeout=timeout)
        try:
            self.upstream_attempts += 1
            upstream_request = client.build_request(method, snapshot.manifest["baseURL"] + path, headers=http_headers, content=body)
            response = await client.send(upstream_request, stream=True)
            if canonical_header(response.headers.get("content-type", "")).startswith("text/event-stream") or response.headers.get("trailer") or response.status_code == 101:
                await response.aclose()
                raise WorkerError("upstream_protocol_unsupported", "upstream protocol is unsupported", phase="upstream", upstream_attempted=True)
            body_bytes = bytearray()
            if response.is_stream_consumed:
                body_bytes.extend(response.content)
            else:
                async for chunk in response.aiter_raw():
                    body_bytes.extend(chunk)
                    if len(body_bytes) > RESPONSE_BODY_LIMIT:
                        await response.aclose()
                        raise WorkerError("response_too_large", "upstream response exceeds limit", phase="upstream", upstream_attempted=True)
            if len(body_bytes) > RESPONSE_BODY_LIMIT:
                await response.aclose()
                raise WorkerError("response_too_large", "upstream response exceeds limit", phase="upstream", upstream_attempted=True)
            grouped_headers: dict[str, list[str]] = {}
            serialized = 0
            for name, values in response.headers.multi_items():
                if name.lower() in {"content-length", "transfer-encoding", "connection", "trailer", "upgrade", "x-obot-error-code"}:
                    continue
                serialized += len(name) + sum(len(value) + 2 for value in values)
                if serialized > RESPONSE_HEADER_LIMIT:
                    raise WorkerError("response_too_large", "upstream headers exceed limit", phase="upstream", upstream_attempted=True)
                grouped_headers.setdefault(name, []).append(values)
            response_headers = [{"name": name, "values": values} for name, values in grouped_headers.items()]
            return {"kind": "business", "requestID": request.get("requestID"), "status": response.status_code, "headers": response_headers, "bodyBase64": base64.b64encode(body_bytes).decode("ascii"), "outcome": "business_result"}
        except WorkerError:
            raise
        except httpx.HTTPError as exc:
            raise WorkerError("transport_uncertain", "upstream transport outcome is uncertain", retryable=False, phase="upstream", upstream_attempted=True) from exc
        except (KeyError, TypeError, ValueError) as exc:
            raise WorkerError("invalid_request", "invalid execution request") from exc
        finally:
            await client.aclose()
