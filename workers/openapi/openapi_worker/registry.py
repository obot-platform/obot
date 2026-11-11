import asyncio
from collections import OrderedDict

from .compiler import CompileHelperPool
from .errors import WorkerError
from .snapshot import CompiledSnapshot, SnapshotEnvelope, compile_snapshot, parse_snapshot


class SnapshotRegistry:
    def __init__(self, cache_entries: int = 32, cache_bytes: int = 16 * 1024 * 1024, queue: int = 4) -> None:
        self.cache_entries = cache_entries
        self.cache_bytes = cache_bytes
        self.queue_limit = queue
        self.entries: OrderedDict[str, CompiledSnapshot] = OrderedDict()
        self.pending: dict[str, asyncio.Task[CompiledSnapshot]] = {}
        self.active: dict[str, int] = {}
        self.bytes = 0
        self.compile_count = 0
        self.lock = asyncio.Lock()
        self.helper = CompileHelperPool()
        self.closed = False

    async def get(self, digest: str) -> CompiledSnapshot | None:
        async with self.lock:
            if self.closed:
                raise WorkerError("worker_unavailable", "snapshot registry is closed", retryable=True)
            return self.entries.get(digest)

    async def ensure(self, envelope: dict) -> CompiledSnapshot:
        parsed = parse_snapshot(envelope)
        async with self.lock:
            if self.closed:
                raise WorkerError("worker_unavailable", "snapshot registry is closed", retryable=True)
            existing = self.entries.get(parsed.execution_digest)
            if existing is not None:
                self.entries.move_to_end(parsed.execution_digest)
                return existing
            task = self.pending.get(parsed.execution_digest)
            if task is not None:
                return await asyncio.shield(task)
            if len(self.pending) >= self.helper.helpers + self.queue_limit:
                raise WorkerError("compile_busy", "cold compile queue is full", retryable=True)
            self.pending[parsed.execution_digest] = asyncio.create_task(self._compile(parsed, envelope))
            task = self.pending[parsed.execution_digest]
        try:
            return await asyncio.shield(task)
        finally:
            async with self.lock:
                self.pending.pop(parsed.execution_digest, None)

    async def _compile(self, parsed: SnapshotEnvelope, envelope: dict) -> CompiledSnapshot:
        async with self.lock:
            self.compile_count += 1
        await self.helper.compile(envelope)
        compiled = compile_snapshot(parsed)
        async with self.lock:
            if self.closed:
                raise WorkerError("worker_unavailable", "snapshot registry is closed", retryable=True)
            if parsed.execution_digest in self.entries:
                return self.entries[parsed.execution_digest]
            required = compiled.schema_size
            while self.bytes + required > self.cache_bytes or len(self.entries) >= self.cache_entries:
                evicted = next((digest for digest, item in reversed(self.entries.items()) if not self.active.get(digest)), None)
                if evicted is None:
                    raise WorkerError("snapshot_cache_full", "snapshot cache is full")
                item = self.entries.pop(evicted)
                self.bytes -= item.schema_size
            self.entries[parsed.execution_digest] = compiled
            self.bytes += required
            return compiled

    async def acquire(self, digest: str) -> CompiledSnapshot:
        snapshot = await self.get(digest)
        if snapshot is None:
            raise WorkerError("snapshot_required", "snapshot is required", retryable=True)
        async with self.lock:
            self.active[digest] = self.active.get(digest, 0) + 1
        return snapshot

    async def release(self, digest: str) -> None:
        async with self.lock:
            count = self.active.get(digest, 0) - 1
            if count > 0:
                self.active[digest] = count
            else:
                self.active.pop(digest, None)

    async def close(self) -> None:
        async with self.lock:
            self.closed = True
        await self.helper.close()
