import json
import struct
import sys

from .snapshot import compile_snapshot, parse_snapshot


def _read_exact(stream, size: int) -> bytes:
    data = bytearray()
    while len(data) < size:
        chunk = stream.read(size - len(data))
        if not chunk:
            raise EOFError
        data.extend(chunk)
    return bytes(data)


def main() -> int:
    while True:
        try:
            size = struct.unpack(">I", _read_exact(sys.stdin.buffer, 4))[0]
            if size > 4 * 1024 * 1024:
                raise ValueError("message too large")
            envelope = json.loads(_read_exact(sys.stdin.buffer, size))
            compiled = compile_snapshot(parse_snapshot(envelope))
            result = {"ok": True, "schemaSize": compiled.schema_size, "planSize": compiled.plan_size}
        except Exception as exc:
            result = {"ok": False, "error": type(exc).__name__, "message": str(exc)}
        body = json.dumps(result, separators=(",", ":")).encode()
        sys.stdout.buffer.write(struct.pack(">I", len(body)) + body)
        sys.stdout.buffer.flush()


if __name__ == "__main__":
    raise SystemExit(main())
