import base64
import json

import pytest

from conftest import make_snapshot
from openapi_worker.errors import WorkerError
from openapi_worker.snapshot import execution_digest, parse_snapshot


def test_snapshot_accepts_and_binds_canonical_digest():
    envelope, _, _ = make_snapshot()
    parsed = parse_snapshot(envelope)
    assert parsed.execution_digest == envelope["executionDigest"]


@pytest.mark.parametrize("mutate", ["execution", "schema", "noncanonical"])
def test_snapshot_rejects_digest_and_canonicality(mutate):
    envelope, _, _ = make_snapshot()
    if mutate == "execution":
        envelope["executionDigest"] = "0" * 64
    elif mutate == "schema":
        envelope["schemaBase64"] = base64.b64encode(b"changed").decode()
    else:
        raw = base64.b64decode(envelope["manifestBase64"])
        envelope["manifestBase64"] = base64.b64encode(raw + b"\n").decode()
    with pytest.raises(WorkerError) as caught:
        parse_snapshot(envelope)
    assert caught.value.code == "snapshot_invalid"


def test_digest_uses_length_prefixed_domain_manifest_and_policy():
    envelope, manifest, policy = make_snapshot()
    manifest_bytes = json.dumps(manifest, separators=(",", ":"), ensure_ascii=False).encode()
    policy_bytes = json.dumps(policy, separators=(",", ":"), ensure_ascii=False).encode()
    assert envelope["executionDigest"] == execution_digest(manifest_bytes, policy_bytes)


def test_authorization_is_allowed_only_as_credential():
    envelope, _, _ = make_snapshot(credential="Authorization")
    assert parse_snapshot(envelope).policy["credentials"][0]["name"] == "Authorization"

    envelope, _, _ = make_snapshot()
    policy = {
        "forwardable": [{"name": "Authorization", "required": False}],
        "credentials": [],
    }
    policy_bytes = json.dumps(policy, separators=(",", ":"), ensure_ascii=False).encode()
    envelope["policyBase64"] = base64.b64encode(policy_bytes).decode()
    envelope["executionDigest"] = execution_digest(base64.b64decode(envelope["manifestBase64"]), policy_bytes)
    with pytest.raises(WorkerError) as caught:
        parse_snapshot(envelope)
    assert caught.value.code == "snapshot_invalid"
