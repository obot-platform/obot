
import asyncio

from conftest import make_snapshot
from openapi_worker.registry import SnapshotRegistry


async def test_registry_holds_multiple_snapshots_and_deduplicates_cold_compile():
    first = make_snapshot("/items/{id}")
    second = make_snapshot("/orders/{id}", credential="x-order-token")
    registry = SnapshotRegistry()
    try:
        await registry.ensure(first[0])
        await registry.ensure(second[0])
        assert set(registry.entries) == {first[0]["executionDigest"], second[0]["executionDigest"]}
        before = registry.compile_count
        compiled = await asyncio.gather(*[registry.ensure(first[0]) for _ in range(20)])
        assert registry.compile_count == before
        assert all(item.execution_digest == first[0]["executionDigest"] for item in compiled)
    finally:
        await registry.close()
