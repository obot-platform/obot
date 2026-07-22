"""API request/response logging to file when OBOT_EVAL_API_LOG is set."""
import threading
import time

_MAX_LOG_BODY = 2048
_file = None
_lock = threading.Lock()


def _truncate(body: bytes, max_len: int) -> str:
    s = body.decode("utf-8", errors="replace")
    if len(s) > max_len:
        return s[:max_len] + "... [truncated]"
    return s


def init_api_log(path: str) -> None:
    global _file
    if not path:
        return
    with _lock:
        if _file is not None:
            return
        _file = open(path, "a", encoding="utf-8")
        _file.write("\n=== API log started " + time.strftime("%Y-%m-%dT%H:%M:%S%z") + " ===\n\n")
        _file.flush()


def close_api_log() -> None:
    global _file
    with _lock:
        if _file is not None:
            _file.close()
            _file = None


def log_api_call(
    method: str,
    url: str,
    req_body: bytes,
    status: int,
    resp_body: bytes,
) -> None:
    with _lock:
        f = _file
    if f is None:
        return
    ts = time.strftime("%H:%M:%S.000", time.localtime())
    f.write("[%s] %s %s\n" % (ts, method, url))
    if req_body:
        f.write(_truncate(req_body, _MAX_LOG_BODY) + "\n")
    f.write("  -> %d (%d bytes)\n" % (status, len(resp_body)))
    if resp_body:
        f.write(_truncate(resp_body, _MAX_LOG_BODY) + "\n\n")
    else:
        f.write("\n")
    f.flush()


def log_api_stream_response(url: str, resp_body: bytes) -> None:
    """Append event-stream response for a GET that was already logged (stream kept open)."""
    with _lock:
        f = _file
    if f is None:
        return
    ts = time.strftime("%H:%M:%S.000", time.localtime())
    f.write("[%s] GET %s [event-stream response]\n" % (ts, url))
    f.write("  -> 200 (%d bytes)\n" % len(resp_body))
    if resp_body:
        f.write(_truncate(resp_body, _MAX_LOG_BODY) + "\n\n")
    else:
        f.write("\n")
    f.flush()
