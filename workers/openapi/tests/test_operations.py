
import pytest

from openapi_worker.errors import WorkerError
from openapi_worker.operations import build_request, match_operation, sanitize_headers
from openapi_worker.snapshot import compile_snapshot, parse_snapshot


@pytest.fixture
def compiled(snapshot):
    return compile_snapshot(parse_snapshot(snapshot[0]))


def test_matches_static_escaped_path_and_presures_encoded_parameter(compiled):
    operation, parameters = match_operation(compiled, "GET", "/items/a%2Fb")
    assert operation.operation_key == "GET /items/{id}"
    assert parameters == {"id": "a/b"}
    method, path, headers, body = build_request(compiled, operation, kind="tool", input_value={"id": parameters["id"]})
    assert (method, path, headers, body) == ("GET", "/items/a%2Fb", [], None)


@pytest.mark.parametrize("path", ["/items/../private", "/items//x", "/items/%zz", "/items/%00"])
def test_rejects_unsafe_paths(compiled, path):
    with pytest.raises(WorkerError) as caught:
        match_operation(compiled, "GET", path)
    assert caught.value.code in {"invalid_request", "operation_not_found"}


def test_headers_drop_protected_and_apply_forwardable(compiled):
    result = sanitize_headers(compiled, [
        {"name": "Authorization", "values": ["caller"]},
        {"name": "X-Obot-Trace", "values": ["secret"]},
        {"name": "X-BUSINESS", "values": ["one", "two"]},
        {"name": "X-Unknown", "values": ["no"]},
    ])
    assert result == [{"name": "x-business", "values": ["one", "two"]}]
