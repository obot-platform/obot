import base64
import hashlib
import json
import secrets
import time

import pytest


def make_snapshot(path="/items/{id}", credential="authtoken"):
    schema = json.dumps({"openapi": "3.1.0", "info": {"title": "items", "version": "1"}, "paths": {path: {"get": {"operationId": "getItems"}}}}).encode()
    manifest = {
        "manifestVersion": 1,
        "schemaDigest": hashlib.sha256(schema).hexdigest(),
        "baseURL": "https://api.example.test",
        "operations": [{
            "operationKey": "GET " + path, "method": "GET", "pathTemplate": path, "legacyToolName": "get_items",
            "parameters": [{"name": "id", "in": "path", "required": True, "style": "simple"}, {"name": "tag", "in": "query", "required": False, "style": "form", "explode": True}],
            "inputSchema": {"type": "object"}, "requestMediaTypes": [],
        }],
        "compilerSemanticsVersion": 1, "pathMatcherVersion": 1, "parameterBindingVersion": 1, "protocolVersion": 1,
    }
    policy = {"forwardable": [{"name": "x-business", "required": False}], "credentials": [{"name": credential, "required": True, "prefix": None, "sensitive": True}]}
    manifest_bytes = json.dumps(manifest, separators=(",", ":"), ensure_ascii=False).encode()
    policy_bytes = json.dumps(policy, separators=(",", ":"), ensure_ascii=False).encode()
    from openapi_worker.snapshot import encode_snapshot
    envelope = encode_snapshot(manifest, policy, schema)
    return envelope, manifest, policy


@pytest.fixture
def snapshot():
    return make_snapshot()


@pytest.fixture
def lease():
    now = int(time.time() * 1000)
    return {
        "leaseID": "lease-1", "leaseIssuedAtUnixMs": now - 10, "leaseDeadlineUnixMs": now + 30_000,
        "fenceDeadlineUnixMs": now + 29_000, "dispatchedAtUnixMs": now, "budgetKeyHash": "a" * 64,
        "clockSkewLimitMs": 500, "fencingMarginMs": 1_000,
    }


@pytest.fixture
def credential():
    return secrets.token_urlsafe(24)
