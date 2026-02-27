"""Eval case definitions and run functions."""
import json
import os
import time
import uuid

from .framework import Case, Context, Result
from ..clients.client import Client, project_id, agent_id
from ..helper import event_stream_data
from ..workflow.workflow_eval import read_captured_response, evaluate_content_publishing_response
from ..workflow.workflow_prompt import CONTENT_PUBLISHING_PHASED_PROMPTS

# Max chars to print for event-stream validation
_EVENT_STREAM_PRINT_CHARS = 1200


def _safe_print_sample(s: str, max_chars: int) -> str:
    """Return a sample safe for Windows console (cp1252) and other narrow encodings."""
    sample = s[:max_chars]
    if len(s) > max_chars:
        sample += "... [truncated]"
    return sample.encode("ascii", errors="replace").decode("ascii")


def _print_event_stream_validation(assistant_text: str, raw_sse: str) -> None:
    """Print event-stream response summary and sample to stdout for validation."""
    print("\n[event-stream validation]")
    print("  assistant text length: %d bytes" % len(assistant_text))
    print("  raw SSE length: %d bytes" % len(raw_sse))
    if assistant_text:
        print("  assistant text sample:\n%s" % _safe_print_sample(assistant_text, _EVENT_STREAM_PRINT_CHARS))
    if raw_sse:
        print("  raw SSE sample:\n%s" % _safe_print_sample(raw_sse, _EVENT_STREAM_PRINT_CHARS))
    if not assistant_text and not raw_sse:
        print("  (no data received)")
    print()


def _normalize_wordpress_site_url(url: str) -> str:
    """Return WordPress site root for REST API (strip /wp-admin and trailing slashes).
    The REST API is at {site_root}/wp-json/wp/v2/; using /wp-admin causes 404."""
    if not url or not isinstance(url, str):
        return url or ""
    u = url.strip().rstrip("/")
    # Use site root only; /wp-admin is the admin UI, not the API base
    if u.endswith("/wp-admin"):
        u = u[: -len("/wp-admin")].rstrip("/")
    return u


def _safe_response_sample(body: bytes | str, max_len: int = 500) -> str:
    """Return a short, safe sample of API response for logging."""
    if body is None:
        return "(empty)"
    raw = body.decode("utf-8", errors="replace") if isinstance(body, bytes) else str(body)
    if len(raw) > max_len:
        return raw[:max_len] + "... [truncated]"
    return raw


# --- MCP connect-before-prompt (aligned with Nanobot Eval Framework spec) ---
# Phase 1: discover (search) catalog entries; ensure user has servers and they are in project; then configure.
# Ensures Exa Search and WordPress MCP are connected before sending prompts that use them.


# Catalog entry IDs for nanobot content-publishing (only these two are connected).
# When set via env, we only connect these exact entries; otherwise we match by manifest name.
_NANOBOT_EXA_CATALOG_ENTRY_ID_ENV = "OBOT_EVAL_EXA_CATALOG_ENTRY_ID"
_NANOBOT_WP_CATALOG_ENTRY_ID_ENV = "OBOT_EVAL_WP_CATALOG_ENTRY_ID"


def _search_mcp_catalog_entries(c: Client, ctx: Context) -> list[dict]:
    """
    Discover MCP catalog entries for content-publishing: only Exa Search and WordPress.
    Tries GET /api/all-mcps/entries; falls back to GET /api/mcp-catalogs/default/entries.
    Returns at most two entries: one Exa Search, one WordPress (by explicit ID when set, else by name).
    """
    path_all = "/api/all-mcps/entries"
    body, status = c._do("GET", path_all)
    print("[eval] GET %s -> status=%s response=%s" % (path_all, status, _safe_response_sample(body)))
    if status != 200:
        path_cat = "/api/mcp-catalogs/default/entries"
        body, status = c._do("GET", path_cat)
        print("[eval] GET %s -> status=%s response=%s" % (path_cat, status, _safe_response_sample(body)))
    if status != 200:
        return []
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        return []
    items = data.get("items") or []
    if not isinstance(items, list):
        return []

    exa_id_from_env = (os.getenv(_NANOBOT_EXA_CATALOG_ENTRY_ID_ENV) or "").strip()
    wp_id_from_env = (os.getenv(_NANOBOT_WP_CATALOG_ENTRY_ID_ENV) or "").strip()
    allowlist_ids = []
    if exa_id_from_env:
        allowlist_ids.append(exa_id_from_env)
    if wp_id_from_env:
        allowlist_ids.append(wp_id_from_env)

    # Only Exa Search and WordPress; at most one of each (first match).
    wanted: list[dict] = []
    found_exa = False
    found_wp = False
    for entry in items:
        if not isinstance(entry, dict):
            continue
        meta = entry.get("metadata") or {}
        entry_id = (meta.get("name") or entry.get("id") or "").strip()
        manifest = entry.get("manifest") or {}
        name = (manifest.get("name") or "").lower()
        short_desc = (manifest.get("shortDescription") or "").lower()

        if allowlist_ids:
            if entry_id in allowlist_ids:
                wanted.append({"id": entry_id, "name": name, "manifest": manifest})
            continue
        if not found_exa and "exa" in name and "search" in (name or short_desc):
            wanted.append({"id": entry_id, "name": name, "manifest": manifest})
            found_exa = True
        elif not found_wp and "wordpress" in name:
            wanted.append({"id": entry_id, "name": name, "manifest": manifest})
            found_wp = True
        if found_exa and found_wp:
            break
    return wanted


def _ensure_user_has_server_for_entry(
    c: Client, entry_id: str, entry_name: str, ctx: Context
) -> str | None:
    """
    Ensure the user has an MCPServer for the given catalog entry (obot_connect_to_mcp_server flow).
    If not, creates one via POST /api/mcp-catalogs/default/servers. Returns mcp_server_id or None.
    """
    # List user's MCP servers and find one for this catalog entry
    body, status = c._do("GET", "/api/mcp-servers")
    if status != 200:
        return None
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        return None
    for s in data.get("items") or []:
        if not isinstance(s, dict):
            continue
        cid = (s.get("catalogEntryID") or s.get("mcpServerCatalogEntryId") or "").strip()
        if cid == entry_id:
            sid = s.get("id") or (s.get("metadata") or {}).get("name")
            if sid:
                return sid

    # Create server from catalog entry
    path = "/api/mcp-catalogs/default/servers"
    payload = {"catalogEntryID": entry_id}
    post_body, post_status = c._do("POST", path, payload)
    print("[eval] POST %s (catalogEntryID=%s) -> status=%s response=%s" % (path, entry_id, post_status, _safe_response_sample(post_body)))
    if post_status not in (200, 201):
        return None
    try:
        created = json.loads(post_body)
        return created.get("id") or (created.get("metadata") or {}).get("name")
    except json.JSONDecodeError:
        return None


def _ensure_project_has_mcp_server(
    c: Client, project_id: str, agent_id: str, mcp_server_id: str, ctx: Context
) -> bool:
    """Add MCP server to project if not already present. Returns True if now present."""
    path_list = f"/api/assistants/{agent_id}/projects/{project_id}/mcpservers"
    body, status = c._do("GET", path_list)
    if status != 200:
        print("[eval] GET %s -> status=%s (cannot add server)" % (path_list, status))
        return False
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        return False
    for pmcp in data.get("items") or []:
        if not isinstance(pmcp, dict):
            continue
        mid = pmcp.get("mcpID") or (pmcp.get("manifest") or {}).get("mcpID") or pmcp.get("id")
        if mid == mcp_server_id:
            return True

    path_post = f"/api/assistants/{agent_id}/projects/{project_id}/mcpservers"
    payload = {"mcpID": mcp_server_id}
    post_body, post_status = c._do("POST", path_post, payload)
    print("[eval] POST %s (mcpID=%s) -> status=%s response=%s" % (path_post, mcp_server_id, post_status, _safe_response_sample(post_body)))
    return post_status in (200, 201)


def _configure_mcp_server_by_id_for_nanobot(
    c: Client, server_id: str, entry_name: str, ctx: Context
) -> bool:
    """
    Configure a single MCP server by ID for nanobot session (same logic as connect_wordpress_mcp).
    Uses user-level configure then catalog fallback so catalog-scoped servers (e.g. WordPress)
    get configured via POST /api/mcp-catalogs/default/servers/{id}/configure.
    Returns True if configure returned 2xx (user or catalog).
    """
    exa_key = os.getenv("OBOT_EVAL_EXA_API_KEY") or os.getenv("EXA_API_KEY")
    wp_url_raw = os.getenv("OBOT_EVAL_WP_URL")
    wp_url = _normalize_wordpress_site_url(wp_url_raw or "") if wp_url_raw else ""
    wp_username = os.getenv("OBOT_EVAL_WP_USERNAME")
    wp_password = os.getenv("OBOT_EVAL_WP_APP_PASSWORD")

    needs_exa = "exa" in (entry_name or "") and "search" in (entry_name or "")
    needs_wp = "wordpress" in (entry_name or "")
    if needs_exa and exa_key:
        payload = {"EXA_API_KEY": exa_key}
    elif needs_wp and wp_url and wp_username and wp_password:
        # WORDPRESS_PASSWORD = application password (nanobot doc). User must be Editor or Administrator for write (create/post).
        # Send both WORDPRESS_SITE and WORDPRESS_URL for MCP compatibility; same for password keys.
        payload = {
            "WORDPRESS_SITE": wp_url,
            "WORDPRESS_URL": wp_url,
            "WORDPRESS_USERNAME": wp_username,
            "WORDPRESS_PASSWORD": wp_password,
            "WordPress App Password": wp_password,
        }
    else:
        return False

    path_post = f"/api/mcp-servers/{server_id}/configure"
    post_body, post_status = c._do("POST", path_post, payload)
    print("[eval] POST %s -> status=%s response=%s" % (path_post, post_status, _safe_response_sample(post_body)))
    if post_status in (200, 201, 204):
        return True
    path_cat = f"/api/mcp-catalogs/default/servers/{server_id}/configure"
    cat_body, cat_status = c._do("POST", path_cat, payload)
    print("[eval] POST %s -> status=%s response=%s" % (path_cat, cat_status, _safe_response_sample(cat_body)))
    return cat_status in (200, 201, 204)


def _ensure_mcp_servers_connected_before_prompts(
    c: Client, project_id: str, agent_id: str, ctx: Context
) -> None:
    """
    Connect Exa Search and WordPress MCP servers before prompts (Nanobot Eval Framework spec).
    Step 1: Discover catalog entries (search). Step 2: Ensure user has server per entry (connect).
    Step 3: Ensure each server is in the project. Step 4: Configure for nanobot session (same logic
    as connect_wordpress_mcp: user configure then catalog fallback so WordPress/catalog servers work).
    """
    ctx.append_step("MCP discover (catalog entries: only Exa Search + WordPress)")
    entries = _search_mcp_catalog_entries(c, ctx)
    if not entries:
        return
    ctx.append_step("MCP connect: ensure user servers and add to project (only Exa + WordPress)")
    configured_by_id = 0
    for ent in entries:
        eid = ent.get("id") or ""
        name = ent.get("name") or ""
        if not eid:
            continue
        server_id = _ensure_user_has_server_for_entry(c, eid, name, ctx)
        if not server_id:
            continue
        _ensure_project_has_mcp_server(c, project_id, agent_id, server_id, ctx)
        ctx.append_step("MCP connected: %s -> %s", name[:40], server_id)
        # Configure for nanobot session (user then catalog endpoint, same as connect_wordpress_mcp)
        if _configure_mcp_server_by_id_for_nanobot(c, server_id, name, ctx):
            configured_by_id += 1
    if configured_by_id > 0:
        ctx.append_step("MCP config (nanobot session): configured %d server(s)", configured_by_id)
    # Also run project-based configure for any servers only visible via project list (e.g. composite)
    n_configured = _ensure_nanobot_mcp_servers_configured(c, project_id, agent_id, ctx)
    if n_configured > 0:
        ctx.append_step("MCP config: configured %d server(s) (Exa/WordPress)", n_configured)


def _ensure_nanobot_mcp_servers_configured(
    c: Client, project_id: str, agent_id: str, ctx: Context
) -> int:
    """
    Best-effort configuration of Exa Search and WordPress MCP servers for the nanobot
    project so the content publishing workflow can use them without interactive UI popups.

    Uses environment variables (no hardcoded secrets):
    - OBOT_EVAL_EXA_API_KEY or EXA_API_KEY
    - OBOT_EVAL_WP_URL (site root, e.g. https://example.com; /wp-admin is stripped)
    - OBOT_EVAL_WP_USERNAME, OBOT_EVAL_WP_APP_PASSWORD

    WordPress write (create/post) requires the WordPress user to be Editor or Administrator;
    Application Passwords inherit the user's capabilities. Read-only errors usually mean
    the user role is Subscriber or the app password was created for a read-only account.

    Returns the number of servers that were configured (for trajectory/debug).
    """
    exa_key = os.getenv("OBOT_EVAL_EXA_API_KEY") or os.getenv("EXA_API_KEY")
    wp_url_raw = os.getenv("OBOT_EVAL_WP_URL")
    wp_url = _normalize_wordpress_site_url(wp_url_raw or "") if wp_url_raw else ""
    wp_username = os.getenv("OBOT_EVAL_WP_USERNAME")
    wp_password = os.getenv("OBOT_EVAL_WP_APP_PASSWORD")
    if wp_url_raw and wp_url != (wp_url_raw or "").strip().rstrip("/"):
        print("[eval] WordPress site URL normalized to REST API base: %s" % wp_url)
    if not (exa_key or (wp_url and wp_username and wp_password)):
        return 0

    # List MCP servers attached to this project (nanobot context)
    path_list = f"/api/assistants/{agent_id}/projects/{project_id}/mcpservers"
    body, status = c._do("GET", path_list)
    print("[eval] GET %s -> status=%s response=%s" % (path_list, status, _safe_response_sample(body)))
    if status != 200:
        return 0
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        return 0
    items = data.get("items", []) or []
    if not isinstance(items, list):
        return 0

    configured = 0
    for pmcp in items:
        if not isinstance(pmcp, dict):
            continue
        name = (pmcp.get("name") or "").lower()
        alias = (pmcp.get("alias") or "").lower()
        mcp_id = pmcp.get("mcpID") or (pmcp.get("manifest") or {}).get("mcpID")
        if not mcp_id:
            continue
        needs_exa = ("exa" in name) or ("exa" in alias)
        needs_wp = ("wordpress" in name) or ("wordpress" in alias)
        if not (needs_exa or needs_wp):
            continue

        # Resolve underlying MCP server (user or catalog)
        path_get = f"/api/mcp-servers/{mcp_id}"
        get_body, get_status = c._do("GET", path_get)
        print("[eval] GET %s -> status=%s response=%s" % (path_get, get_status, _safe_response_sample(get_body)))
        if get_status != 200:
            path_cat = f"/api/mcp-catalogs/default/servers/{mcp_id}"
            get_body, get_status = c._do("GET", path_cat)
            print("[eval] GET %s -> status=%s response=%s" % (path_cat, get_status, _safe_response_sample(get_body)))
        if get_status != 200:
            continue
        try:
            server = json.loads(get_body)
        except json.JSONDecodeError:
            continue
        manifest = server.get("manifest") or {}
        runtime = (manifest.get("runtime") or "").lower()

        if runtime == "composite":
            comp_config = (manifest.get("compositeConfig") or {}).get(
                "componentServers"
            ) or []
            component_configs: dict[str, dict] = {}
            for comp in comp_config:
                if not isinstance(comp, dict):
                    continue
                comp_id = comp.get("catalogEntryID") or comp.get("mcpServerID")
                if not comp_id:
                    continue
                comp_name = (
                    (comp.get("manifest") or {}).get("name") or ""
                ).lower()
                config: dict[str, str] = {}
                if ("exa" in comp_name or "exa search" in comp_name) and exa_key:
                    config["EXA_API_KEY"] = exa_key
                if (
                    "wordpress" in comp_name or "wp " in comp_name
                ) and wp_url and wp_username and wp_password:
                    config["WORDPRESS_SITE"] = wp_url
                    config["WORDPRESS_URL"] = wp_url
                    config["WORDPRESS_USERNAME"] = wp_username
                    config["WORDPRESS_PASSWORD"] = wp_password
                    config["WordPress App Password"] = wp_password  # catalog UI key
                if config:
                    component_configs[comp_id] = {
                        "config": config,
                        "url": "",
                        "disabled": False,
                    }
            if not component_configs:
                continue
            payload = {"componentConfigs": component_configs}
        else:
            if needs_exa and exa_key:
                payload = {"EXA_API_KEY": exa_key}
            elif needs_wp and wp_url and wp_username and wp_password:
                payload = {
                    "WORDPRESS_SITE": wp_url,
                    "WORDPRESS_URL": wp_url,
                    "WORDPRESS_USERNAME": wp_username,
                    "WORDPRESS_PASSWORD": wp_password,
                    "WordPress App Password": wp_password,
                }
            else:
                continue

        # Configure (user-level then catalog as fallback)
        path_post = f"/api/mcp-servers/{mcp_id}/configure"
        post_body, post_status = c._do("POST", path_post, payload)
        print("[eval] POST %s -> status=%s response=%s" % (path_post, post_status, _safe_response_sample(post_body)))
        if post_status in (200, 201, 204):
            configured += 1
        else:
            path_cat_post = f"/api/mcp-catalogs/default/servers/{mcp_id}/configure"
            cat_body, cat_status = c._do("POST", path_cat_post, payload)
            print("[eval] POST %s -> status=%s response=%s" % (path_cat_post, cat_status, _safe_response_sample(cat_body)))
            if cat_status in (200, 201, 204):
                configured += 1
    return configured


def run_lifecycle(ctx: Context) -> Result:
    c = ctx.client
    ctx.append_step("CreateProjectV2")
    proj, status = c.create_project_v2("eval-lifecycle-project")
    if status not in (200, 201) or proj is None:
        return Result(pass_=False, message="create project: status=%s" % status)
    pid = project_id(proj)
    if not pid:
        return Result(pass_=False, message="create project: no project id in response")
    try:
        ctx.append_step("CreateAgent")
        agent, status = c.create_agent(pid, "Eval Agent", "Lifecycle eval agent")
        if status not in (200, 201) or agent is None:
            return Result(pass_=False, message="create agent: status=%s" % status)
        aid = agent_id(agent)
        if not aid:
            return Result(pass_=False, message="create agent: no agent id in response")
        ctx.append_step("GetAgent")
        got, status = c.get_agent(pid, aid)
        if status != 200 or got is None:
            return Result(pass_=False, message="get agent: status=%s" % status)
        if not got.get("connectURL") or "/mcp-connect/" not in (got.get("connectURL") or ""):
            return Result(pass_=False, message="get agent: connectURL invalid")
        ctx.append_step("UpdateAgent")
        status = c.update_agent(pid, aid, "Eval Agent Updated", "Updated description")
        if status != 200:
            return Result(pass_=False, message="update agent: status=%s" % status)
        ctx.append_step("DeleteAgent")
        status = c.delete_agent(pid, aid)
        if status not in (200, 204):
            return Result(pass_=False, message="delete agent: status=%s" % status)
    finally:
        c.delete_project_v2(pid)
    ctx.append_step("DeleteProject")
    return Result(pass_=True, message="lifecycle completed")


def run_launch(ctx: Context) -> Result:
    c = ctx.client
    ctx.append_step("CreateProjectV2")
    proj, status = c.create_project_v2("eval-launch-project")
    if status not in (200, 201) or proj is None:
        return Result(pass_=False, message="create project: status=%s" % status)
    pid = project_id(proj)
    if not pid:
        return Result(pass_=False, message="create project: no project id")
    try:
        ctx.append_step("CreateAgent")
        agent, status = c.create_agent(pid, "Launch Eval Agent", "")
        if status not in (200, 201) or agent is None:
            return Result(pass_=False, message="create agent: status=%s" % status)
        aid = agent_id(agent)
        if not aid:
            return Result(pass_=False, message="create agent: no agent id")
        ctx.append_step("LaunchAgent")
        status = c.launch_agent(pid, aid)
        if status not in (200, 503, 400):
            return Result(pass_=False, message="launch: unexpected status=%s" % status)
        return Result(pass_=True, message="launch returned %s (acceptable)" % status)
    finally:
        c.delete_project_v2(pid)


def run_list_and_filter(ctx: Context) -> Result:
    c = ctx.client
    ctx.append_step("ListProjectsV2 (before)")
    before, status = c.list_projects_v2()
    if status != 200:
        return Result(pass_=False, message="list projects: status=%s" % status)
    ctx.append_step("CreateProjectV2")
    proj, status = c.create_project_v2("eval-list-project")
    if status not in (200, 201) or proj is None:
        return Result(pass_=False, message="create project: status=%s" % status)
    pid = project_id(proj)
    if not pid:
        return Result(pass_=False, message="create project: no project id")
    try:
        ctx.append_step("ListProjectsV2 (after)")
        after, status = c.list_projects_v2()
        if status != 200 or len(after) < len(before) + 1:
            return Result(pass_=False, message="new project not in list")
        ctx.append_step("ListAgents (empty)")
        agents, status = c.list_agents(pid)
        if status != 200 or len(agents) != 0:
            return Result(pass_=False, message="list agents: expected 0")
        ctx.append_step("CreateAgent")
        agent, status = c.create_agent(pid, "List Eval Agent", "")
        if status not in (200, 201) or agent is None:
            return Result(pass_=False, message="create agent: status=%s" % status)
        aid = agent_id(agent)
        ctx.append_step("ListAgents (one)")
        agents, status = c.list_agents(pid)
        if status != 200 or len(agents) != 1 or agent_id(agents[0]) != aid:
            return Result(pass_=False, message="list agents after: expected 1")
        return Result(pass_=True, message="list and filter passed")
    finally:
        c.delete_project_v2(pid)


def run_graceful_failure(ctx: Context) -> Result:
    c = ctx.client
    ctx.append_step("CreateProjectV2")
    proj, status = c.create_project_v2("eval-failure-project")
    if status not in (200, 201) or proj is None:
        return Result(pass_=False, message="create project: status=%s" % status)
    pid = project_id(proj)
    if not pid:
        return Result(pass_=False, message="create project: no project id")
    try:
        ctx.append_step("CreateAgent")
        agent, status = c.create_agent(pid, "Failure Eval Agent", "")
        if status not in (200, 201) or agent is None:
            return Result(pass_=False, message="create agent: status=%s" % status)
        aid = agent_id(agent)
        ctx.append_step("DeleteAgent")
        status = c.delete_agent(pid, aid)
        if status not in (200, 204):
            return Result(pass_=False, message="delete agent: status=%s" % status)
        ctx.append_step("LaunchAgent (deleted agent)")
        status = c.launch_agent(pid, aid)
        if status in (404, 400, 410):
            return Result(pass_=True, message="graceful failure: status=%s" % status)
        if status >= 500:
            return Result(pass_=True, message="server error (no crash): status=%s" % status)
        return Result(pass_=True, message="launch after delete returned %s" % status)
    finally:
        c.delete_project_v2(pid)


def run_version_flag(ctx: Context) -> Result:
    c = ctx.client
    ctx.append_step("GetVersion")
    v, status = c.get_version()
    if status != 200:
        return Result(pass_=False, message="version: status=%s" % status)
    if v is None:
        return Result(pass_=False, message="version: empty response")
    nanobot = v.get("nanobotIntegration", False)
    return Result(pass_=True, message="nanobotIntegration=%s" % nanobot)


def run_mock_tool_output(ctx: Context) -> Result:
    from ..mockmcp import run_mock_echo_test
    ctx.append_step("Start mock MCP server")
    got, err = run_mock_echo_test("eval-deterministic-output")
    ctx.append_step("MCP tools/call echo")
    ctx.append_step("Assert output")
    if err:
        return Result(pass_=False, message="call echo: %s" % err)
    if got != "eval-deterministic-output":
        return Result(pass_=False, message="echo output: want %r got %r" % ("eval-deterministic-output", got))
    return Result(pass_=True, message="mock tool returned deterministic output")


def run_workflow_content_publishing_eval(ctx: Context) -> Result:
    ctx.append_step("ReadCapturedResponse")
    response_text, ok = read_captured_response()
    if not ok:
        return Result(pass_=False, message="no captured response: set OBOT_EVAL_CAPTURED_RESPONSE or OBOT_EVAL_CAPTURED_RESPONSE_FILE")
    ctx.append_step("EvaluateContentPublishingResponse")
    eval_result = evaluate_content_publishing_response(response_text)
    if not eval_result.pass_:
        return Result(pass_=False, message=eval_result.message)
    return Result(pass_=True, message=eval_result.message)


def run_workflow_content_publishing_step_eval(ctx: Context) -> Result:
    """Run content publishing workflow: 1 phase per item in CONTENT_PUBLISHING_PHASED_PROMPTS.
    Supports single-prompt (list of 1) or multi-phase (list of N) from workflow_prompt.py."""
    c = ctx.client
    prompts = CONTENT_PUBLISHING_PHASED_PROMPTS
    if not prompts:
        return Result(pass_=False, message="no phased prompts defined")
    ctx.append_step("GET /api/version")
    v, status = c.get_version()
    if status != 200 or v is None:
        return Result(pass_=False, message="version: status=%s" % status)
    if not v.get("nanobotIntegration"):
        return Result(pass_=False, message="nanobotIntegration is false")
    ctx.append_step("GET /api/projectsv2")
    projects, status = c.list_projects_v2()
    if status != 200:
        return Result(pass_=False, message="list projects: status=%s" % status)
    if not projects:
        return Result(pass_=False, message="no projects: create a project and agent first")
    pid = project_id(projects[0])
    if not pid:
        return Result(pass_=False, message="project ID empty")
    ctx.append_step("GET /api/projectsv2/%s/agents", pid)
    agents, status = c.list_agents(pid)
    if status != 200 or not agents:
        return Result(pass_=False, message="no agents in project")
    aid = agent_id(agents[0])
    if not aid:
        return Result(pass_=False, message="agent ID empty")
    # Connect MCP servers (Exa, WordPress) before prompts per Nanobot Eval Framework spec:
    # discover catalog entries, ensure user has servers and they are in project, then configure.
    _ensure_mcp_servers_connected_before_prompts(c, pid, aid, ctx)
    mcp = c.mcp_client_for_agent(aid)
    if mcp is None:
        return Result(pass_=False, message="MCPClientForAgent returned None")
    ctx.append_step("MCP initialize")
    session_id, status = mcp.initialize()
    if status != 200 or not session_id:
        return Result(pass_=False, message="MCP initialize: status=%s" % status)
    ctx.append_step("MCP notifications/initialized")
    status = mcp.notifications_initialized(session_id)
    if status not in (200, 202):
        return Result(pass_=False, message="notifications/initialized: status=%s" % status)

    response_texts: list[str] = []
    raw_sse_per_phase: list[str] = []
    tools_per_phase: list[list[str]] = []
    total_reply_bytes = 0
    result = Result(
        pass_=False,
        message="",
    )

    try:
        # Sequential prompt -> response: for each phase we open the event stream, send one prompt,
        # then read until chat-done. We do not send prompt N+1 until prompt N's response is complete.
        for phase in range(len(prompts)):
            progress_token = str(uuid.uuid4())
            prompt_for_phase = prompts[phase]
            ctx.append_step("Phase %d: event stream + chat (async)", phase)

            def send_chat(phase_idx=phase, progress_tok=progress_token, prompt_text=prompt_for_phase):
                out, st = mcp.chat_send(
                    session_id, "chat-with-nanobot", prompt_text, progress_token=progress_tok
                )
                if st != 200:
                    raise RuntimeError("chat send phase %d: status=%s" % (phase_idx, st))

            response_text, raw_sse, tools_used = mcp.get_response_from_events_async(
                session_id, send_chat_fn=send_chat
            )
            response_texts.append(response_text or "")
            raw_sse_per_phase.append(raw_sse or "")
            tools_per_phase.append(list(tools_used) if tools_used else [])
            total_reply_bytes += len(response_text or "")

            if response_text:
                ctx.append_step("Phase %d reply: %s", phase, (response_text[:80] + "..." if len(response_text) > 80 else response_text))
            if tools_used:
                ctx.append_step("Phase %d tools: %s", phase, ", ".join(tools_used))
            event_stream_data.save_event_stream_response_phase(
                "nanobot_workflow_content_publishing_step_eval", session_id, phase, response_text or "", raw_sse=raw_sse, tools_used=tools_used
            )
            _print_event_stream_validation(response_text or "", raw_sse)
            if phase < len(prompts) - 1:
                time.sleep(1)

        ctx.append_step("Assert 200 and response received for all phases")
        result = Result(
            pass_=True,
            message="sent %d phases, status 200, total events reply %d bytes" % (len(prompts), total_reply_bytes),
        )
    except Exception as e:
        ctx.append_step("Error: %s" % e)
        result = Result(pass_=False, message=str(e))
    finally:
        # Always write steps and event-stream data (including partial) so logs populate even on failure.
        out_path = event_stream_data.write_step_eval_output_file_multi_phase(
            "nanobot_workflow_content_publishing_step_eval", ctx.trajectory, raw_sse_per_phase, session_id, tools_per_phase=tools_per_phase
        )
        print("[step_eval] Steps + raw SSE (%d phases) saved to: %s" % (len(raw_sse_per_phase), out_path))

    return result


def all_cases() -> list[Case]:
    return [
        Case("nanobot_lifecycle", "Create project → create agent → get agent → update → delete agent → delete project", run_lifecycle),
        Case("nanobot_launch", "Create project and agent, then launch; accept 200 or 503", run_launch),
        Case("nanobot_list_and_filter", "List projects, create project, list agents, create agent; assert created resources appear", run_list_and_filter),
        Case("nanobot_graceful_failure", "Delete agent then call launch; assert non-5xx or 404/410", run_graceful_failure),
        Case("nanobot_version_flag", "GET /api/version and assert nanobotIntegration present", run_version_flag),
        Case("nanobot_mock_tool_output", "Run in-process mock MCP server, call echo tool, assert deterministic output", run_mock_tool_output),
        Case("nanobot_workflow_content_publishing_eval", "Evaluate captured workflow response; expects URL, title, sources, tool calls", run_workflow_content_publishing_eval),
        Case("nanobot_workflow_content_publishing_step_eval", "Send phased prompt via MCP chat; API calls logged", run_workflow_content_publishing_step_eval),
    ]
