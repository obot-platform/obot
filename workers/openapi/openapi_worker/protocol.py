import json
from typing import Any

from .errors import WorkerError

KIB = 1024
MIB = 1024 * KIB
PROTOCOL_VERSION = 1
DESCRIBE_REQUEST_LIMIT = 2 * MIB
DESCRIBE_RESPONSE_LIMIT = 4 * MIB
EXECUTE_OVERHEAD_LIMIT = 256 * KIB
ERROR_LIMIT = 64 * KIB
HEALTH_LIMIT = 16 * KIB
REQUEST_BODY_LIMIT = MIB
RESPONSE_BODY_LIMIT = 8 * MIB
RESPONSE_HEADER_LIMIT = 64 * KIB
BASE64_BODY_LIMIT = 4 * ((RESPONSE_BODY_LIMIT + 2) // 3) + 64 * KIB


def bounded_json(body: bytes, limit: int) -> dict[str, Any]:
    if len(body) > limit:
        raise WorkerError("request_too_large", "request envelope exceeds limit")
    try:
        value = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise WorkerError("invalid_request", "invalid JSON") from exc
    if not isinstance(value, dict):
        raise WorkerError("invalid_request", "JSON object required")
    return value


def check_protocol(value: object) -> None:
    if value != PROTOCOL_VERSION:
        raise WorkerError("protocol_version_unsupported", "unsupported protocol version")


def safe_response(payload: dict, limit: int) -> dict:
    encoded = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    if len(encoded) > limit:
        raise WorkerError("response_limit_exceeded", "response envelope exceeds limit")
    return payload
