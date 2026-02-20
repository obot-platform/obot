"""HTTP client for Obot nanobot APIs (projectsv2, agents, launch, version)."""
import json
from typing import Any, Optional
from urllib.parse import urlparse

import requests

from ..helper import api_log

MCP_SERVER_PREFIX = "ms1"
DEFAULT_TIMEOUT = 30


def _apply_auth(headers: dict, auth_header: str) -> None:
    if not auth_header:
        return
    auth_header = auth_header.strip()
    if auth_header.lower().startswith("cookie:"):
        headers["Cookie"] = auth_header[7:].strip()
    else:
        headers["Authorization"] = auth_header


class Client:
    """HTTP client for Obot REST API."""
    def __init__(self, base_url: str, auth_header: str):
        self.base_url = base_url.rstrip("/")
        self.auth_header = auth_header
        self._session = requests.Session()
        self._session.timeout = DEFAULT_TIMEOUT
        self._session.headers["Accept"] = "application/json"

    def _do(self, method: str, path: str, body: Any = None) -> tuple[bytes, int]:
        url = self.base_url + path
        headers = {}
        if body is not None:
            headers["Content-Type"] = "application/json"
        _apply_auth(headers, self.auth_header)
        req_body = json.dumps(body).encode() if body is not None else b""
        resp = self._session.request(method, url, data=req_body or None, headers=headers)
        resp_body = resp.content
        api_log.log_api_call(method, url, req_body, resp.status_code, resp_body)
        return resp_body, resp.status_code

    def get_version(self) -> tuple[Optional[dict], int]:
        """GET /api/version. Returns (parsed body or None, status)."""
        body, status = self._do("GET", "/api/version")
        if status != 200:
            return None, status
        return json.loads(body), status

    def create_project_v2(self, display_name: str) -> tuple[Optional[dict], int]:
        body, status = self._do("POST", "/api/projectsv2", {"displayName": display_name})
        if status not in (200, 201):
            return None, status
        return json.loads(body), status

    def list_projects_v2(self) -> tuple[list[dict], int]:
        body, status = self._do("GET", "/api/projectsv2")
        if status != 200:
            return [], status
        data = json.loads(body)
        return data.get("items", []), status

    def delete_project_v2(self, project_id: str) -> int:
        _, status = self._do("DELETE", f"/api/projectsv2/{project_id}")
        return status

    def create_agent(self, project_id: str, display_name: str, description: str = "") -> tuple[Optional[dict], int]:
        payload = {"displayName": display_name}
        if description:
            payload["description"] = description
        body, status = self._do("POST", f"/api/projectsv2/{project_id}/agents", payload)
        if status not in (200, 201):
            return None, status
        return json.loads(body), status

    def list_agents(self, project_id: str) -> tuple[list[dict], int]:
        body, status = self._do("GET", f"/api/projectsv2/{project_id}/agents")
        if status != 200:
            return [], status
        data = json.loads(body)
        return data.get("items", []), status

    def get_agent(self, project_id: str, agent_id: str) -> tuple[Optional[dict], int]:
        body, status = self._do("GET", f"/api/projectsv2/{project_id}/agents/{agent_id}")
        if status != 200:
            return None, status
        return json.loads(body), status

    def update_agent(
        self,
        project_id: str,
        agent_id: str,
        display_name: str,
        description: str,
    ) -> int:
        _, status = self._do(
            "PUT",
            f"/api/projectsv2/{project_id}/agents/{agent_id}",
            {"displayName": display_name, "description": description},
        )
        return status

    def launch_agent(self, project_id: str, agent_id: str) -> int:
        _, status = self._do("POST", f"/api/projectsv2/{project_id}/agents/{agent_id}/launch")
        return status

    def delete_agent(self, project_id: str, agent_id: str) -> int:
        _, status = self._do("DELETE", f"/api/projectsv2/{project_id}/agents/{agent_id}")
        return status

    def mcp_connect_url(self, agent_id: str) -> str:
        """Full URL for MCP gateway: base_url + /mcp-connect/ms1{agent_id}."""
        if not agent_id:
            return ""
        return f"{self.base_url}/mcp-connect/{MCP_SERVER_PREFIX}{agent_id}"

    def mcp_client_for_agent(self, agent_id: str) -> Optional["MCPClient"]:
        """Return MCP client for this agent (same auth)."""
        if not agent_id:
            return None
        from .mcp_client import MCPClient
        return MCPClient(self.mcp_connect_url(agent_id), self.auth_header)


def project_id(proj: Optional[dict]) -> str:
    """Extract project ID from API response (top-level id or metadata.id/name)."""
    if not proj:
        return ""
    if proj.get("id"):
        return str(proj["id"])
    meta = proj.get("metadata") or {}
    return meta.get("id") or meta.get("name") or ""


def agent_id(agent: Optional[dict]) -> str:
    """Extract agent ID from API response."""
    if not agent:
        return ""
    if agent.get("id"):
        return str(agent["id"])
    meta = agent.get("metadata") or {}
    return meta.get("id") or meta.get("name") or ""
