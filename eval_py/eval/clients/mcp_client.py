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
EVENTS_STREAM_MAX_WAIT = 600


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

    def _read_events_stream(
        self, session_id: str, stream_ready: Optional[threading.Event] = None
    ) -> tuple[str, str, list[str]]:
        """Read GET api/events stream; return (assistant_text, raw_sse, tools_used).
        Captures the current turn only: events after history-end until chat-done.
        If stream_ready is set, it is signaled once the stream is open (200) and reading.
        """
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
            return "", "", []

        api_log.log_api_call(
            "GET", url, b"", 200, b"(event-stream, reading until chat-done or timeout)"
        )
        if stream_ready is not None:
            stream_ready.set()
        out: list[str] = []
        tools_used: list[str] = []
        seen_created_id: set[tuple[str, str]] = set()
        # Collect (message_key, block_lines); message_key = (created, id) for data blocks, None for others
        blocks_in_order: list[tuple[Optional[tuple[str, str]], list[str]]] = []
        after_history_until_done = False
        current_event: Optional[str] = None
        current_data_lines: list[str] = []
        current_raw_lines: list[str] = []
        start = time.time()

        def _message_key_from_data(data_lines: list[str]) -> Optional[tuple[str, str]]:
            """Parse first data line as JSON; return (created, id) if present else None."""
            if not data_lines:
                return None
            data_str = "\n".join(data_lines).strip()
            if not data_str or data_str == "{}":
                return None
            try:
                ev = json.loads(data_str)
                created = ev.get("created")
                eid = ev.get("id")
                if created is not None and eid is not None:
                    return (str(created), str(eid))
            except json.JSONDecodeError:
                pass
            return None

        def flush_event():
            nonlocal current_event, current_data_lines, current_raw_lines
            if not current_raw_lines:
                current_data_lines = []
                current_raw_lines = []
                return
            lines_copy = list(current_raw_lines)
            if not after_history_until_done:
                blocks_in_order.append((None, lines_copy))
                current_data_lines = []
                current_raw_lines = []
                return
            message_key = _message_key_from_data(current_data_lines)
            if current_data_lines:
                data_str = "\n".join(current_data_lines).strip()
                if data_str and data_str != "{}":
                    try:
                        ev = json.loads(data_str)
                        created = ev.get("created")
                        eid = ev.get("id")
                        if created is not None and eid is not None:
                            seen_created_id.add((str(created), str(eid)))
                        if ev.get("role") == "assistant":
                            for item in ev.get("items", []):
                                if item.get("type") == "text" and item.get("text"):
                                    out.append(item["text"])
                                if item.get("type") == "tool" and item.get("name"):
                                    tools_used.append(item["name"])
                    except json.JSONDecodeError:
                        pass
            blocks_in_order.append((message_key, lines_copy))
            current_data_lines = []
            current_raw_lines = []

        try:
            for raw_line in resp.iter_lines(decode_unicode=True):
                if time.time() - start > EVENTS_STREAM_MAX_WAIT:
                    break
                if raw_line is None:
                    continue
                line = raw_line.strip("\r\n") if raw_line else ""

                if line == "":
                    current_raw_lines.append("")
                    flush_event()
                    if current_event == "chat-done":
                        break
                    current_event = None
                    continue
                if line.startswith("event:"):
                    flush_event()
                    current_event = line[6:].strip()
                    if current_event == "history-end":
                        after_history_until_done = True
                    elif current_event == "chat-done":
                        after_history_until_done = False
                    current_raw_lines.append(line)
                    current_data_lines = []
                elif line.startswith("data:"):
                    current_data_lines.append(line[5:].strip())
                    current_raw_lines.append(line)
        except (requests.exceptions.ReadTimeout, requests.exceptions.ConnectionError):
            pass
        except Exception:
            pass
        finally:
            resp.close()
        flush_event()

        # Collapse: for each (created, id) keep only the last block; preserve order of first occurrence
        last_block_for_key: dict[tuple[str, str], list[str]] = {}
        for msg_key, lines in blocks_in_order:
            if msg_key is not None:
                last_block_for_key[msg_key] = lines
        seen_keys: set[tuple[str, str]] = set()
        raw_sse_lines: list[str] = []
        for msg_key, lines in blocks_in_order:
            if msg_key is None:
                raw_sse_lines.extend(lines)
            elif msg_key not in seen_keys:
                seen_keys.add(msg_key)
                raw_sse_lines.extend(last_block_for_key[msg_key])

        # Do not deduplicate by line: blank lines separate SSE events; dropping them breaks the stream
        raw_sse = "\n".join(raw_sse_lines)
        result = "".join(out)
        tools_used_dedup = list(dict.fromkeys(tools_used))
        summary = "(event-stream response, %d bytes assistant text)" % len(result)
        if len(result) > 0 and len(result) <= 500:
            summary = result.encode("utf-8")
        elif len(result) > 500:
            summary = (result[:500] + "... [truncated]").encode("utf-8")
        else:
            summary = summary.encode("utf-8")
        api_log.log_api_stream_response(url, summary)
        return result, raw_sse, tools_used_dedup

    def get_response_from_events(self, session_id: str) -> str:
        assistant_text, _, _ = self._read_events_stream(session_id)
        return assistant_text

    STREAM_READY_WAIT = 15

    def get_response_from_events_async(
        self, session_id: str, send_chat_fn: Callable[[], None]
    ) -> tuple[str, str, list[str]]:
        """Open event stream first, signal when ready, then call send_chat_fn(); read until chat-done.
        Ensures prompt is sent only after the stream is open so response is received in sequence.
        Returns (assistant_text, raw_sse, tools_used)."""
        result_holder: list[tuple[str, str, list[str]]] = []
        stream_ready = threading.Event()

        def read_thread():
            res = self._read_events_stream(session_id, stream_ready=stream_ready)
            result_holder.append(res)

        t = threading.Thread(target=read_thread, daemon=True)
        t.start()
        if not stream_ready.wait(timeout=self.STREAM_READY_WAIT):
            t.join(timeout=2)
            return "", "", []
        try:
            send_chat_fn()
        finally:
            pass
        t.join(timeout=EVENTS_STREAM_MAX_WAIT)
        if result_holder:
            return result_holder[0]
        return "", "", []
