import json
from urllib.parse import parse_qsl, quote, unquote_to_bytes

from .errors import WorkerError
from .protocol import MIB
from .snapshot import CompiledSnapshot, Operation, PROTECTED_HEADERS, canonical_header


def _strict_segments(raw_path: str) -> list[bytes]:
    if not raw_path.startswith("/") or raw_path == "/" or "//" in raw_path:
        raise WorkerError("invalid_request", "invalid raw path")
    output: list[bytes] = []
    for segment in raw_path[1:].split("/"):
        index = 0
        while index < len(segment):
            if segment[index] == "%":
                if index + 2 >= len(segment) or not all(c in "0123456789abcdefABCDEF" for c in segment[index + 1:index + 3]):
                    raise WorkerError("invalid_request", "invalid path escape")
                index += 3
            else:
                index += 1
        decoded = unquote_to_bytes(segment)
        try:
            decoded.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise WorkerError("invalid_request", "invalid path encoding") from exc
        if decoded in (b".", b"..") or b"\x00" in decoded or any(byte < 0x20 or byte == 0x7f for byte in decoded):
            raise WorkerError("invalid_request", "unsafe path segment")
        output.append(decoded)
    return output


def _template_segments(template: str) -> list[str]:
    if not template.startswith("/") or "//" in template:
        raise WorkerError("operation_not_found", "invalid operation template")
    return template[1:].split("/")


def match_operation(snapshot: CompiledSnapshot, method: str, raw_path: str) -> tuple[Operation, dict[str, str]]:
    request_segments = _strict_segments(raw_path)
    candidates: list[tuple[tuple[int, ...], Operation, dict[str, str]]] = []
    for operation in snapshot.operations.values():
        if operation.method != method:
            continue
        template = _template_segments(operation.path_template)
        if len(template) != len(request_segments):
            continue
        specificity: list[int] = []
        parameters: dict[str, str] = {}
        matched = True
        for expected, actual in zip(template, request_segments):
            if expected.startswith("{") and expected.endswith("}"):
                specificity.append(0)
                parameters[expected[1:-1]] = actual.decode("utf-8")
            elif expected.encode("utf-8") == actual:
                specificity.append(1)
            else:
                matched = False
                break
        if matched:
            candidates.append((tuple(specificity), operation, parameters))
    if not candidates:
        raise WorkerError("operation_not_found", "operation does not match path")
    best_specificity = max(item[0] for item in candidates)
    best = [item for item in candidates if item[0] == best_specificity]
    if len(best) != 1:
        raise WorkerError("operation_not_found", "ambiguous operation path")
    return best[0][1], best[0][2]


def _validate_parameter(parameter: dict, location: str) -> None:
    if parameter.get("in") != location or not isinstance(parameter.get("name"), str):
        raise WorkerError("invalid_request", "invalid parameter definition")
    style = parameter.get("style", "simple" if location in ("path", "header") else "form")
    supported = {
        "path": {"simple"}, "header": {"simple"},
        "query": {"form", "spaceDelimited", "pipeDelimited", "deepObject"},
    }
    if style not in supported[location]:
        raise WorkerError("invalid_request", "unsupported parameter style")


def build_request(snapshot: CompiledSnapshot, operation: Operation, *, kind: str, input_value: dict, body_media_type: str | None = None, matched_parameters: dict[str, str] | None = None) -> tuple[str, str, list[dict], bytes | None]:
    path_parts: list[str] = []
    query_pairs: list[tuple[str, str]] = []
    headers: list[dict] = []
    body_obj: object | None = None
    path_parameters: dict[str, str] = dict(matched_parameters or {})
    raw_query = str(input_value.get("rawQuery", "")) if kind == "http" else ""
    raw_query_pairs = parse_qsl(raw_query, keep_blank_values=True) if kind == "http" else []
    for parameter in operation.parameters:
        _validate_parameter(parameter, parameter["in"])
        name = parameter["name"]
        if kind != "http" and name not in input_value and parameter.get("required"):
            raise WorkerError("invalid_request", f"missing required parameter {name}")
        value = input_value.get(name)
        if parameter["in"] == "path":
            if kind == "http":
                continue
            path_parameters[name] = str(value)
        elif parameter["in"] == "query":
            if kind == "http":
                if parameter.get("required") and name not in dict(raw_query_pairs):
                    raise WorkerError("invalid_request", f"missing required parameter {name}")
                continue
            if value is not None:
                query_pairs.append((name, str(value)))
        elif parameter["in"] == "header":
            if value is not None and canonical_header(name) not in PROTECTED_HEADERS:
                headers.append({"name": name, "values": [str(value)]})
        elif parameter["in"] == "body" and value is not None:
            body_obj = value
    for segment in _template_segments(operation.path_template):
        if segment.startswith("{") and segment.endswith("}"):
            path_parts.append(quote(str(path_parameters[segment[1:-1]]), safe=""))
        else:
            path_parts.append(segment)
    path = "/" + "/".join(path_parts)
    if kind != "http" and query_pairs:
        raw_query = "&".join(f"{quote(key, safe='')}={quote(value, safe='')}" for key, value in query_pairs)
    if raw_query:
        path += "?" + raw_query
    if body_obj is not None:
        if body_media_type and body_media_type not in operation.request_media_types:
            raise WorkerError("unsupported_media_type", "body media type is not declared")
        body = json.dumps(body_obj, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    else:
        body = None
    if body is not None and len(body) > MIB:
        raise WorkerError("request_too_large", "request body exceeds limit")
    return operation.method, path, headers, body


def sanitize_headers(snapshot: CompiledSnapshot, supplied: list[dict]) -> list[dict]:
    forwardable = {canonical_header(item["name"]) for item in snapshot.policy.get("forwardable", [])}
    output: list[dict] = []
    for header in supplied:
        if not isinstance(header, dict) or not isinstance(header.get("name"), str) or not isinstance(header.get("values"), list):
            raise WorkerError("invalid_request", "invalid header input")
        name = canonical_header(header["name"])
        if name in PROTECTED_HEADERS or name.startswith("x-obot-") or name.startswith("x-forwarded-") or name not in forwardable:
            continue
        values = [str(value) for value in header["values"]]
        if sum(len(value) for value in values) > 64 * 1024:
            raise WorkerError("request_too_large", "request headers exceed limit")
        output.append({"name": name, "values": values})
    return output
