"""
Fetch project_id, assistant_id, and mcp_server_id(s) from Obot API and print
curl commands for listing MCP servers and configuring them. Uses OBOT_EVAL_BASE_URL
and OBOT_EVAL_AUTH_HEADER. Run from eval_py: python print_mcp_curl.py
"""
import json
import os
import sys

# Allow importing from eval
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from eval.clients.client import Client, project_id, agent_id


def main() -> None:
    base_url = (os.environ.get("OBOT_EVAL_BASE_URL") or "http://localhost:8080").rstrip("/")
    auth_header = os.environ.get("OBOT_EVAL_AUTH_HEADER") or ""
    if not auth_header:
        print("Set OBOT_EVAL_AUTH_HEADER (e.g. Cookie: obot_access_token=...)")
        sys.exit(1)

    # Cookie value for curl (strip "Cookie: " so we have e.g. obot_access_token=TOKEN)
    cookie_val = auth_header.strip()
    if cookie_val.lower().startswith("cookie:"):
        cookie_val = cookie_val[7:].strip()

    c = Client(base_url, auth_header)

    # Resolve project_id
    projects, status = c.list_projects_v2()
    if status != 200 or not projects:
        print("No projects or list failed (status=%s). Create a project first." % status)
        sys.exit(1)
    pid = project_id(projects[0])
    if not pid:
        print("Could not get project ID from response")
        sys.exit(1)

    # Resolve agent_id (assistant_id)
    agents, status = c.list_agents(pid)
    if status != 200 or not agents:
        print("No agents or list failed (status=%s). Create an agent in the project first." % status)
        sys.exit(1)
    aid = agent_id(agents[0])
    if not aid:
        print("Could not get agent ID from response")
        sys.exit(1)

    # List MCP servers for this project/assistant (may 403 if no access)
    mcp_ids = []
    body, status = c._do("GET", f"/api/assistants/{aid}/projects/{pid}/mcpservers")
    if status == 200:
        try:
            data = json.loads(body)
            items = data.get("items") or data.get("mcpservers") or []
            if isinstance(items, list):
                for item in items:
                    mid = item.get("mcpID") or (item.get("manifest") or {}).get("mcpID") or item.get("id")
                    if mid:
                        mcp_ids.append(mid)
        except json.JSONDecodeError:
            pass
    else:
        # Fallback: list user-level MCP servers
        body2, status2 = c._do("GET", "/api/mcp-servers")
        if status2 == 200:
            try:
                data2 = json.loads(body2)
                items2 = data2.get("items") or data2.get("servers") or []
                if isinstance(items2, list):
                    for item in items2:
                        mid = item.get("id") or item.get("name") or (item.get("metadata") or {}).get("name")
                        if mid:
                            mcp_ids.append(str(mid))
            except (json.JSONDecodeError, TypeError):
                pass
        if not mcp_ids:
            print("(List project mcpservers returned %s; no user mcp-servers fallback.)" % status)

    print("project_id=%s" % pid)
    print("assistant_id=%s" % aid)
    print("mcp_server_id(s)=%s" % (", ".join(mcp_ids) if mcp_ids else "(none – add MCP servers to project or use GET /api/mcp-servers)"))
    print()
    print_curls(base_url, cookie_val, pid, aid, mcp_ids)


def print_curls(
    base_url: str, cookie_val: str, project_id: str, assistant_id: str, mcp_server_ids: list[str]
) -> None:
    # cookie_val is the full cookie (e.g. obot_access_token=...)
    cookie_header = "Cookie: " + cookie_val
    print("# --- List project MCP servers ---")
    print(
        'curl -s -X GET "%s/api/assistants/%s/projects/%s/mcpservers" \\\n'
        '  -H "%s" \\\n  -H "Content-Type: application/json"'
        % (base_url, assistant_id, project_id, cookie_header)
    )
    print()
    for mcp_id in mcp_server_ids:
        print("# --- Configure MCP server (user-level): %s ---" % mcp_id)
        print(
            'curl -s -X POST "%s/api/mcp-servers/%s/configure" \\\n'
            '  -H "%s" \\\n'
            '  -H "Content-Type: application/json" \\\n'
            '  -d "{\\"EXA_API_KEY\\": \\"YOUR_EXA_API_KEY\\"}"'
            % (base_url, mcp_id, cookie_header)
        )
        print()
        print("# --- Configure MCP server (catalog fallback): %s ---" % mcp_id)
        print(
            'curl -s -X POST "%s/api/mcp-catalogs/default/servers/%s/configure" \\\n'
            '  -H "%s" \\\n'
            '  -H "Content-Type: application/json" \\\n'
            '  -d "{\\"EXA_API_KEY\\": \\"YOUR_EXA_API_KEY\\"}"'
            % (base_url, mcp_id, cookie_header)
        )
        print()


if __name__ == "__main__":
    main()
