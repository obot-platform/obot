import base64
import binascii
import hashlib
import json
import re
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlsplit

from .errors import WorkerError
from .protocol import MIB, PROTOCOL_VERSION

DIGEST_DOMAIN = b"obot-openapi-worker-v1"
HEX64 = re.compile(r"^[0-9a-f]{64}$")
OPERATION_KEY = re.compile(r"^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) /[^?#]*$")


@dataclass(slots=True)
class Operation:
    operation_key: str
    method: str
    path_template: str
    legacy_tool_name: str
    parameters: list[dict]
    input_schema: dict
    request_media_types: list[str]


@dataclass(slots=True)
class CompiledSnapshot:
    execution_digest: str
    schema_digest: str
    manifest: dict[str, Any]
    policy: dict[str, Any]
    operations: dict[str, Operation]
    schema_size: int
    plan_size: int


@dataclass(slots=True)
class SnapshotEnvelope:
    execution_digest: str
    schema_digest: str
    manifest: dict[str, Any]
    policy: dict[str, Any]
    schema_bytes: bytes


def length_frame(*parts: bytes) -> bytes:
    result = bytearray()
    for part in parts:
        result += len(part).to_bytes(8, "big") + part
    return bytes(result)


def execution_digest(manifest_bytes: bytes, policy_bytes: bytes) -> str:
    return hashlib.sha256(length_frame(DIGEST_DOMAIN, manifest_bytes, policy_bytes)).hexdigest()


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8")


def decode_b64(value: object, name: str, raw_limit: int) -> bytes:
    if not isinstance(value, str):
        raise WorkerError("snapshot_invalid", f"{name} must be base64")
    if len(value) > 4 * ((raw_limit + 2) // 3) + KIB_LIMIT:
        raise WorkerError("snapshot_too_large", f"{name} exceeds limit")
    try:
        raw = base64.b64decode(value, validate=True)
    except (binascii.Error, ValueError) as exc:
        raise WorkerError("snapshot_invalid", f"invalid {name}") from exc
    if len(raw) > raw_limit:
        raise WorkerError("snapshot_too_large", f"{name} exceeds limit")
    return raw


KIB_LIMIT = 1024


def _json_bytes(raw: bytes, name: str, raw_limit: int) -> tuple[dict[str, Any], bytes]:
    if len(raw) > raw_limit:
        raise WorkerError("snapshot_too_large", f"{name} exceeds limit")
    try:
        value = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise WorkerError("snapshot_invalid", f"invalid {name}") from exc
    if not isinstance(value, dict):
        raise WorkerError("snapshot_invalid", f"{name} must be an object")
    try:
        encoded = canonical_json(value)
    except (TypeError, ValueError) as exc:
        raise WorkerError("snapshot_invalid", f"invalid {name}") from exc
    if encoded != raw or b"\n" in raw or raw.startswith(b"\xef\xbb\xbf"):
        raise WorkerError("snapshot_invalid", f"{name} is not canonical JSON")
    return value, encoded


def _validate_no_external_ref(value: Any) -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            if key == "$ref" and isinstance(child, str) and (not child.startswith("#/") or ".." in child):
                raise WorkerError("snapshot_invalid", "external schema reference is not allowed")
            _validate_no_external_ref(child)
    elif isinstance(value, list):
        for child in value:
            _validate_no_external_ref(child)


def parse_snapshot(value: object) -> SnapshotEnvelope:
    if not isinstance(value, dict):
        raise WorkerError("snapshot_invalid", "snapshot must be an object")
    allowed = {"protocolVersion", "executionDigest", "manifestBase64", "policyBase64", "schemaBase64"}
    if set(value) != allowed:
        raise WorkerError("snapshot_invalid", "unexpected snapshot fields")
    if value.get("protocolVersion") != PROTOCOL_VERSION:
        raise WorkerError("protocol_version_unsupported", "unsupported protocol version")
    supplied_digest = value.get("executionDigest")
    if not isinstance(supplied_digest, str) or not HEX64.fullmatch(supplied_digest):
        raise WorkerError("snapshot_invalid", "invalid executionDigest")
    manifest_raw = decode_b64(value.get("manifestBase64"), "manifest", 2 * MIB)
    policy_raw = decode_b64(value.get("policyBase64"), "policy", MIB)
    schema_raw = decode_b64(value.get("schemaBase64"), "schema", MIB)
    manifest, manifest_bytes = _json_bytes(manifest_raw, "manifest", 2 * MIB)
    policy, policy_bytes = _json_bytes(policy_raw, "policy", MIB)
    actual_digest = execution_digest(manifest_bytes, policy_bytes)
    actual_schema_digest = hashlib.sha256(schema_raw).hexdigest()
    if supplied_digest != actual_digest:
        raise WorkerError("snapshot_invalid", "executionDigest mismatch")
    _validate_manifest(manifest, actual_schema_digest, policy, allow_http_base_url=_allow_http_base_url())
    return SnapshotEnvelope(actual_digest, actual_schema_digest, manifest, policy, schema_raw)


def _validate_manifest(manifest: dict, schema_digest: str, policy: dict, allow_http_base_url: bool = False) -> None:
    required = {"manifestVersion", "schemaDigest", "baseURL", "operations", "compilerSemanticsVersion", "pathMatcherVersion", "parameterBindingVersion", "protocolVersion"}
    if set(manifest) != required or manifest["manifestVersion"] != 1 or manifest["protocolVersion"] != PROTOCOL_VERSION:
        raise WorkerError("snapshot_invalid", "invalid manifest shape")
    if manifest["schemaDigest"] != schema_digest:
        raise WorkerError("snapshot_invalid", "schemaDigest mismatch")
    url = urlsplit(str(manifest["baseURL"]))
    scheme_ok = url.scheme == "https" or (allow_http_base_url and url.scheme == "http")
    if not scheme_ok or not url.hostname or url.username or url.query or url.fragment:
        raise WorkerError("snapshot_invalid", "invalid base URL")
    operations = manifest["operations"]
    if not isinstance(operations, list) or not operations or len(operations) > 512:
        raise WorkerError("snapshot_invalid", "invalid operations")
    keys: set[str] = set()
    for operation in operations:
        if not isinstance(operation, dict) or not isinstance(operation.get("operationKey"), str):
            raise WorkerError("snapshot_invalid", "invalid operation")
        key = operation["operationKey"]
        if not OPERATION_KEY.fullmatch(key) or key in keys or operation.get("method") != key.split(" ", 1)[0]:
            raise WorkerError("snapshot_invalid", "invalid operationKey")
        keys.add(key)
        if not isinstance(operation.get("pathTemplate"), str) or not operation["pathTemplate"].startswith("/"):
            raise WorkerError("snapshot_invalid", "invalid path template")
        if not isinstance(operation.get("legacyToolName"), str) or not operation["legacyToolName"]:
            raise WorkerError("snapshot_invalid", "invalid legacy tool name")
        if not isinstance(operation.get("parameters", []), list) or len(operation.get("parameters", [])) > 4096:
            raise WorkerError("snapshot_invalid", "invalid parameters")
    _validate_header_policy(policy)
    _validate_no_external_ref(manifest)


def _validate_header_policy(policy: dict) -> None:
    if set(policy) != {"forwardable", "credentials"}:
        raise WorkerError("snapshot_invalid", "invalid header policy")
    protected = PROTECTED_HEADERS
    for group in ("forwardable", "credentials"):
        if not isinstance(policy[group], list):
            raise WorkerError("snapshot_invalid", "invalid header policy")
        for definition in policy[group]:
            if not isinstance(definition, dict) or not isinstance(definition.get("name"), str):
                raise WorkerError("snapshot_invalid", "invalid header definition")
            name = definition["name"]
            if canonical_header(name) in protected or name.lower() == "cookie" or canonical_header(name).startswith("x-forwarded-"):
                if group == "credentials" and canonical_header(name) == "authorization":
                    continue
                raise WorkerError("snapshot_invalid", "protected header cannot be declared")
    credential_names = {canonical_header(item["name"]) for item in policy["credentials"]}
    forward_names = {canonical_header(item["name"]) for item in policy["forwardable"]}
    if credential_names & forward_names:
        raise WorkerError("snapshot_invalid", "header has conflicting classes")


PROTECTED_HEADERS = {
    "authorization", "x-api-key", "x-obot-api-key", "x-obot-worker-authentication", "cookie", "set-cookie",
    "host", "content-length", "transfer-encoding", "connection", "keep-alive", "proxy-authenticate",
    "proxy-authorization", "proxy-connection", "te", "trailer", "upgrade", "via", "forwarded",
}


def canonical_header(name: str) -> str:
    return name.lower().strip()


def compile_snapshot(envelope: SnapshotEnvelope) -> CompiledSnapshot:
    operations: dict[str, Operation] = {}
    for item in envelope.manifest["operations"]:
        operations[item["operationKey"]] = Operation(
            item["operationKey"], item["method"], item["pathTemplate"], item["legacyToolName"],
            item.get("parameters", []), item.get("inputSchema", {}), item.get("requestMediaTypes", []),
        )
    plan_size = len(canonical_json(envelope.manifest)) + len(canonical_json(envelope.policy))
    if plan_size > 16 * MIB:
        raise WorkerError("snapshot_invalid", "compiled plan exceeds limit")
    return CompiledSnapshot(envelope.execution_digest, envelope.schema_digest, envelope.manifest, envelope.policy, operations, len(envelope.schema_bytes), plan_size)


def encode_snapshot(manifest: dict, policy: dict, schema: bytes) -> dict:
    manifest_bytes = canonical_json(manifest)
    policy_bytes = canonical_json(policy)
    return {
        "protocolVersion": PROTOCOL_VERSION,
        "executionDigest": execution_digest(manifest_bytes, policy_bytes),
        "manifestBase64": base64.b64encode(manifest_bytes).decode("ascii"),
        "policyBase64": base64.b64encode(policy_bytes).decode("ascii"),
        "schemaBase64": base64.b64encode(schema).decode("ascii"),
    }


def _allow_http_base_url() -> bool:
    # Production rejects http base URLs unconditionally. The explicit local
    # test egress flag (same one that permits http upstream transports) also
    # permits http base URLs so dev-mode Obot deployments interoperate.
    import os
    return os.environ.get("OPENAPI_WORKER_ALLOW_LOCAL_TEST_EGRESS") == "1"
