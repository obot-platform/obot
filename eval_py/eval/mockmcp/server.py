"""Minimal MCP HTTP server: initialize, tools/list, tools/call (echo tool)."""
import json
import socket
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional

SESSION_ID = "eval-mock-session"


class MockMCPHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length) if length else b""
        try:
            data = json.loads(body.decode())
        except json.JSONDecodeError:
            self.send_response(400)
            self.end_headers()
            return
        method = data.get("method", "")
        params = data.get("params") or {}
        req_id = data.get("id")
        out = None
        status = 200
        if method == "initialize":
            out = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {"tools": {}},
                    "serverInfo": {"name": "mock", "version": "0.0.1"},
                },
            }
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Mcp-Session-Id", SESSION_ID)
            self.end_headers()
            self.wfile.write(json.dumps(out).encode())
            return
        if method == "notifications/initialized":
            self.send_response(202)
            self.end_headers()
            return
        if method == "tools/list":
            out = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "tools": [
                        {
                            "name": "echo",
                            "description": "Echo the message",
                            "inputSchema": {
                                "type": "object",
                                "properties": {"message": {"type": "string"}},
                                "required": ["message"],
                            },
                        }
                    ],
                },
            }
        elif method == "tools/call":
            name = (params.get("name") or (params.get("arguments") or {}).get("name", ""))
            args = params.get("arguments") or {}
            if name == "echo":
                msg = args.get("message", "")
                out = {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "result": {
                        "content": [{"type": "text", "text": msg}],
                        "isError": False,
                    },
                }
            else:
                out = {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32601, "message": "method not found"}}
                status = 404
        if out is None:
            out = {"jsonrpc": "2.0", "id": req_id, "error": {"code": -32601, "message": "method not found"}}
            status = 404
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(out).encode())


def _find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def run_mock_echo_test(message: str) -> tuple[str, Optional[Exception]]:
    """Start mock server, call echo with message, return (response_text, error)."""
    port = _find_free_port()
    server = HTTPServer(("127.0.0.1", port), MockMCPHandler)
    thread = threading.Thread(target=server.serve_forever)
    thread.daemon = True
    thread.start()
    base_url = "http://127.0.0.1:%d" % port
    try:
        import requests
        session_id = None
        resp = requests.post(base_url, json={
            "jsonrpc": "2.0",
            "id": "1",
            "method": "initialize",
            "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "eval", "version": "0"}},
        }, timeout=5)
        if resp.status_code != 200:
            return "", RuntimeError("initialize: status %s" % resp.status_code)
        session_id = resp.headers.get("Mcp-Session-Id") or resp.headers.get("mcp-session-id")
        resp = requests.post(base_url, json={
            "jsonrpc": "2.0",
            "id": "2",
            "method": "notifications/initialized",
            "params": {},
        }, headers={"Mcp-Session-Id": session_id} if session_id else {}, timeout=5)
        resp = requests.post(
            base_url + "?method=tools%2Fcall&toolcallname=echo",
            json={
                "jsonrpc": "2.0",
                "id": "3",
                "method": "tools/call",
                "params": {"name": "echo", "arguments": {"message": message}},
            },
            headers={"Mcp-Session-Id": session_id} if session_id else {},
            timeout=5,
        )
        if resp.status_code != 200:
            return "", RuntimeError("tools/call: status %s" % resp.status_code)
        data = resp.json()
        content = data.get("result", {}).get("content", [])
        for item in content:
            if item.get("type") == "text":
                return item.get("text", ""), None
        return "", None
    except Exception as e:
        return "", e
    finally:
        server.shutdown()
