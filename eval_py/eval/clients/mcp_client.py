"""MCP JSON-RPC client for Obot MCP gateway (initialize, chat, events stream)."""
import json
import threading
import time
import uuid
from dataclasses import dataclass
from typing import Any, Callable, Optional
from urllib.parse import urlencode

import requests

from ..helper import api_log

MCP_TIMEOUT = 60
EVENTS_STREAM_MAX_WAIT = 120


@dataclass
class EventsStreamResult:
    assistant_text: str
    raw_sse: str
    tool_call_count: int = 0
    api_call_count: int = 0


def _apply_auth(headers: dict, auth_header: str) -> None:
    if not auth_header:
        return
    auth_header = auth_header.strip()
    if auth_header.lower().startswith("cookie:"):
        headers["Cookie"] = auth_header[7:].strip()
    else:
        headers["Authorization"] = auth_header


class MCPClient:
    """JSON-RPC client for Obot MCP gateway."""
    def __init__(self, mcp_url: str, auth_header: str):
        self.mcp_url = mcp_url.rstrip("/")
        self.auth_header = auth_header
        self._session = requests.Session()
        self._session.timeout = MCP_TIMEOUT

    def _do(
        self,
        method: str,
        params: dict,
        session_id: Optional[str] = None,
        query: Optional[dict] = None,
    ) -> tuple[bytes, int, dict]:
        url = self.mcp_url
        if query:
            url = url + "?" + urlencode(query)
        body = {
            "jsonrpc": "2.0",
            "id": str(uuid.uuid4()),
            "method": method,
            "params": params,
        }
        req_body = json.dumps(body).encode()
        headers = {"Content-Type": "application/json", "Accept": "application/json"}
        if session_id:
            headers["Mcp-Session-Id"] = session_id
        _apply_auth(headers, self.auth_header)
        resp = self._session.post(url, data=req_body, headers=headers)
        out = resp.content
        api_log.log_api_call("POST", url, req_body, resp.status_code, out)
        return out, resp.status_code, dict(resp.headers)

    def initialize(self) -> tuple[str, int]:
        _, status, headers = self._do("initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {"elicitation": {}},
            "clientInfo": {"name": "obot-eval-py", "version": "0.0.1"},
        })
        if status != 200:
            return "", status
        session_id = headers.get("Mcp-Session-Id") or headers.get("mcp-session-id", "")
        if not session_id:
            raise RuntimeError("initialize: missing Mcp-Session-Id header")
        return session_id, status

    def notifications_initialized(self, session_id: str) -> int:
        _, status, _ = self._do("notifications/initialized", {}, session_id=session_id)
        return status

    def chat_send(
        self,
        session_id: str,
        chat_tool_name: str,
        prompt: str,
        progress_token: Optional[str] = None,
    ) -> tuple[bytes, int]:
        progress_token = progress_token or str(uuid.uuid4())
        params = {
            "name": chat_tool_name,
            "arguments": {"prompt": prompt, "attachments": []},
            "_meta": {"ai.nanobot.async": True, "progressToken": progress_token},
        }
        query = {"method": "tools/call", "toolcallname": chat_tool_name}
        out, status, _ = self._do("tools/call", params, session_id=session_id, query=query)
        return out, status

    def events_stream_url(self, session_id: str) -> str:
        return self.mcp_url.rstrip("/") + "/api/events/" + session_id

    # User-Agent matching browser so event-stream responses are returned (server may gate on it)
    USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"

    def _read_events_stream(self, session_id: str) -> tuple[str, str]:
        """Read GET api/events stream; return (assistant_text, raw_sse)."""
        url = self.events_stream_url(session_id)
        headers = {
            "Accept": "text/event-stream",
            "User-Agent": self.USER_AGENT,
        }
        if session_id:
            headers["Mcp-Session-Id"] = session_id
        _apply_auth(headers, self.auth_header)
        resp = self._session.get(
            url, headers=headers, stream=True, timeout=EVENTS_STREAM_MAX_WAIT
        )
        if resp.status_code != 200:
            err_body = b""
            try:
                for chunk in resp.iter_content(chunk_size=1024):
                    err_body += chunk
                    if len(err_body) >= 4096:
                        break
            except Exception:
                pass
            finally:
                resp.close()
            api_log.log_api_call("GET", url, b"", resp.status_code, err_body or b"(no body)")
            return "", ""

        api_log.log_api_call(
            "GET", url, b"", 200, b"(event-stream, reading until chat-done or timeout)"
        )
        out: list[str] = []
        raw_lines: list[str] = []
        current_event: Optional[str] = None
        current_data_lines: list[str] = []
        start = time.time()

        def flush_event():
            nonlocal current_event, current_data_lines
            # SSE: multiple data lines are joined with \n
            if current_data_lines:
                data_str = "\n".join(current_data_lines).strip()
                if data_str and data_str != "{}":
                    try:
                        ev = json.loads(data_str)
                        if ev.get("role") == "assistant":
                            for item in ev.get("items", []):
                                if item.get("type") == "text" and item.get("text"):
                                    out.append(item["text"])
                    except json.JSONDecodeError:
                        pass
            current_data_lines = []

        try:
            for raw_line in resp.iter_lines(decode_unicode=True):
                if time.time() - start > EVENTS_STREAM_MAX_WAIT:
                    break
                if raw_line is None:
                    continue
                line = raw_line.strip("\r\n") if raw_line else ""
                raw_lines.append(line if line else "")

                if line == "":
                    flush_event()
                    if current_event == "chat-done":
                        break
                    current_event = None
                    current_data_lines = []
                    continue
                if line.startswith("event:"):
                    flush_event()
                    current_event = line[6:].strip()
                    current_data_lines = []
                elif line.startswith("data:"):
                    current_data_lines.append(line[5:].strip())
        except (requests.exceptions.ReadTimeout, requests.exceptions.ConnectionError):
            pass
        except Exception:
            pass
        finally:
            resp.close()
        flush_event()

        # Rebuild raw SSE in same format as curl (event:\n data:\n\n)
        raw_sse = "\n".join(raw_lines)

        result = "".join(out)
        summary = "(event-stream response, %d bytes assistant text)" % len(result)
        if len(result) > 0 and len(result) <= 500:
            summary = result.encode("utf-8")
        elif len(result) > 500:
            summary = (result[:500] + "... [truncated]").encode("utf-8")
        else:
            summary = summary.encode("utf-8")
        api_log.log_api_stream_response(url, summary)
        return result, raw_sse

    def get_response_from_events(self, session_id: str) -> str:
        assistant_text, _ = self._read_events_stream(session_id)
        return assistant_text

    def get_response_from_events_async(
        self, session_id: str, send_chat_fn: Callable[[], None]
    ) -> tuple[str, str]:
        """Open event stream first, then call send_chat_fn(); read until chat-done. Returns (assistant_text, raw_sse)."""
        result_holder: list[tuple[str, str]] = []

        def read_thread():
            res = self._read_events_stream(session_id)
            result_holder.append(res)

        t = threading.Thread(target=read_thread, daemon=True)
        t.start()
        time.sleep(0.5)
        try:
            send_chat_fn()
        finally:
            pass
        t.join(timeout=EVENTS_STREAM_MAX_WAIT)
        if result_holder:
            return result_holder[0]
        return "", ""
