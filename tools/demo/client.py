"""Helpers for driving a dev Obot (auth off): REST calls, the LLM gateway, and an MCP
client that talks to a vMCP through /mcp-connect. Everything here is plain plumbing."""
import json
import os
import urllib.error
import urllib.request

OBOT = os.environ.get("OBOT", "http://localhost:8090")


class Resp:
    def __init__(self, status, headers, text):
        self.status = status
        self.headers = headers
        self.text = text
        try:
            self.json = json.loads(text) if text else None
        except ValueError:
            self.json = None

    @property
    def ok(self):
        return 200 <= self.status < 300

    def header(self, name):
        for k, v in self.headers.items():
            if k.lower() == name.lower():
                return v
        return None

    def show(self, limit=2000):
        body = json.dumps(self.json, indent=2) if self.json is not None else self.text
        return f"HTTP {self.status}\n{clip(body, limit)}"


def clip(s, n=2000):
    s = s or ""
    return s if len(s) <= n else s[:n] + f"... [{len(s) - n} more chars]"


def call(method, path, body=None, headers=None, timeout=60):
    url = path if path.startswith("http") else OBOT + path
    hdrs = {"content-type": "application/json", "accept": "application/json"}
    if headers:
        hdrs.update(headers)
    data = None
    if body is not None:
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return Resp(r.status, dict(r.headers), r.read().decode("utf-8", "replace"))
    except urllib.error.HTTPError as e:
        return Resp(e.code, dict(e.headers or {}), e.read().decode("utf-8", "replace"))
    except Exception as e:  # network error / timeout
        return Resp(0, {}, f"{type(e).__name__}: {e}")




class MCPError(Exception):
    def __init__(self, code, message, data=None):
        super().__init__(f"{code}: {message}")
        self.code = code
        self.message = message
        self.data = data


class MCPClient:
    """Minimal streamable-HTTP MCP client for a vMCP at /mcp-connect/<id>."""

    def __init__(self, url, client_name="workflow-client", obot_session=None, extra_headers=None):
        self.url = url
        self.client_name = client_name
        self.obot_session = obot_session
        self.extra_headers = extra_headers or {}
        self.mcp_session = None
        self.next_id = 1

    def _headers(self):
        h = {"accept": "application/json, text/event-stream"}
        h.update(self.extra_headers)
        if self.mcp_session:
            h["Mcp-Session-Id"] = self.mcp_session
        if self.obot_session:
            h["X-Obot-Session-ID"] = self.obot_session
        return h

    @staticmethod
    def _parse(text):
        text = (text or "").strip()
        if not text:
            return None
        if text.startswith("{") or text.startswith("["):
            return json.loads(text)
        msgs = []
        for line in text.splitlines():
            if line.startswith("data:"):
                payload = line[5:].strip()
                if payload:
                    try:
                        msgs.append(json.loads(payload))
                    except ValueError:
                        pass
        for m in reversed(msgs):
            if isinstance(m, dict) and ("result" in m or "error" in m):
                return m
        return msgs[-1] if msgs else None

    def rpc(self, method, params=None, notify=False, timeout=60):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        if not notify:
            msg["id"] = self.next_id
            self.next_id += 1
        r = call("POST", self.url, msg, headers=self._headers(), timeout=timeout)
        sid = r.header("Mcp-Session-Id")
        if sid:
            self.mcp_session = sid
        if notify:
            return None
        if not r.ok:
            raise MCPError(r.status, f"HTTP {r.status}: {clip(r.text, 600)}")
        m = self._parse(r.text)
        if m is None:
            raise MCPError(-1, f"empty response (HTTP {r.status})")
        if "error" in m and m["error"]:
            e = m["error"]
            raise MCPError(e.get("code"), e.get("message"), e.get("data"))
        return m.get("result")

    def initialize(self):
        res = self.rpc("initialize", {
            "protocolVersion": "2025-06-18",
            "capabilities": {},
            "clientInfo": {"name": self.client_name, "version": "1.0"},
        })
        self.rpc("notifications/initialized", notify=True)
        return res

    def list_tools(self):
        return (self.rpc("tools/list", {}) or {}).get("tools", [])

    def call_tool(self, name, arguments):
        return self.rpc("tools/call", {"name": name, "arguments": arguments}, timeout=90)


def tool_result_text(result):
    if not result:
        return ""
    parts = []
    for c in result.get("content") or []:
        if c.get("type") == "text":
            parts.append(c.get("text", ""))
    if not parts and result.get("structuredContent") is not None:
        parts.append(json.dumps(result["structuredContent"]))
    return "\n".join(parts)


def find_tool(tools, base):
    """Return the real (possibly prefixed) tool name whose name ends with base."""
    for t in tools:
        if t["name"] == base:
            return t["name"]
    for t in tools:
        if t["name"].endswith(base):
            return t["name"]
    return None

