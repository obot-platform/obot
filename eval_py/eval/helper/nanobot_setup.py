"""Ensure a projectsv2 project + nanobot agent exist for eval runs (no UI)."""
from __future__ import annotations

import os
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from ..clients.client import Client


def _truthy_env(name: str) -> bool:
    v = (os.environ.get(name) or "").strip().lower()
    return v in ("1", "true", "yes", "on")


def ensure_project_and_agent(client: "Client") -> Optional[str]:
    """
    If there are no projects (or no agents in the first project), create them via REST.

    Uses the same APIs as the Nanobot UI: POST /api/projectsv2 and
    POST /api/projectsv2/{id}/agents. You do not need to open the UI or call
    POST .../launch; the MCP gateway starts the nanobot server on first connect.

    Set OBOT_EVAL_SKIP_AUTO_PROJECT=1 to disable (evals fail fast if nothing exists).

    Optional: OBOT_EVAL_PROJECT_DISPLAY_NAME, OBOT_EVAL_AGENT_DISPLAY_NAME
    """
    if _truthy_env("OBOT_EVAL_SKIP_AUTO_PROJECT"):
        return None

    from ..clients.client import agent_id, project_id

    projects, st = client.list_projects_v2()
    if st != 200:
        return "list projects (before setup): status=%s" % st

    if not projects:
        pname = (os.environ.get("OBOT_EVAL_PROJECT_DISPLAY_NAME") or "eval-py").strip() or "eval-py"
        proj, cst = client.create_project_v2(pname)
        if cst not in (200, 201) or not proj:
            return "create project: status=%s body=%r" % (cst, proj)
        projects = [proj]

    pid = project_id(projects[0])
    if not pid:
        return "project ID empty after list/create"

    agents, st = client.list_agents(pid)
    if st != 200:
        return "list agents: status=%s" % st

    if not agents:
        aname = (os.environ.get("OBOT_EVAL_AGENT_DISPLAY_NAME") or "Eval agent").strip() or "Eval agent"
        agent, ast = client.create_nanobot_agent(pid, display_name=aname)
        if ast not in (200, 201) or not agent:
            return "create nanobot agent: status=%s body=%r" % (ast, agent)

    return None
