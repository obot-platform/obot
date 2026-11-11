import asyncio
import json
import sys
import struct
from pathlib import Path

from .errors import WorkerError


class CompileHelperPool:
    def __init__(self, helpers: int = 1, timeout: float = 30.0) -> None:
        self.helpers = max(1, min(helpers, 2))
        self.timeout = timeout
        self._processes: list[asyncio.subprocess.Process] = []
        self._semaphore = asyncio.Semaphore(self.helpers)
        self._locks: list[asyncio.Lock] = [asyncio.Lock() for _ in range(self.helpers)]
        self.healthy = True

    async def start(self) -> None:
        while len(self._processes) < self.helpers:
            self._processes.append(await asyncio.create_subprocess_exec(
                sys.executable, "-m", "openapi_worker.compiler_helper", cwd=Path(__file__).parents[1],
                stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE
            ))

    async def compile(self, envelope: dict) -> tuple[int, int]:
        await self.start()
        payload = json.dumps(envelope, separators=(",", ":")).encode()
        if len(payload) > 4 * 1024 * 1024:
            raise WorkerError("snapshot_too_large", "snapshot exceeds compile envelope")
        async with self._semaphore:
            index = 0
            while index < len(self._locks) and self._locks[index].locked():
                index += 1
            index = min(index, len(self._locks) - 1)
            async with self._locks[index]:
                process = self._processes[index]
                try:
                    process.stdin.write(struct.pack(">I", len(payload)) + payload)
                    await process.stdin.drain()
                    header = await asyncio.wait_for(process.stdout.readexactly(4), self.timeout)
                    size = struct.unpack(">I", header)[0]
                    result = json.loads(await asyncio.wait_for(process.stdout.readexactly(size), self.timeout))
                except (OSError, EOFError, asyncio.TimeoutError, struct.error) as exc:
                    await self._replace(index)
                    raise WorkerError("compile_timeout", "snapshot compiler did not complete") from exc
                if not result.get("ok"):
                    raise WorkerError("snapshot_invalid", result.get("message", "snapshot compilation failed"))
                return int(result["schemaSize"]), int(result["planSize"])

    async def _replace(self, index: int) -> None:
        process = self._processes[index]
        process.kill()
        await process.wait()
        self._processes[index] = await asyncio.create_subprocess_exec(
            sys.executable, "-m", "openapi_worker.compiler_helper", cwd=Path(__file__).parents[1],
            stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE
        )

    async def close(self) -> None:
        for process in self._processes:
            if process.returncode is None:
                process.terminate()
        for process in self._processes:
            if process.returncode is None:
                try:
                    await asyncio.wait_for(process.wait(), 2)
                except asyncio.TimeoutError:
                    process.kill()
                    await process.wait()
        self._processes.clear()
