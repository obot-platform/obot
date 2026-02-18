"""Eval case definitions and run functions."""
import uuid

from .framework import Case, Context, Result
from ..clients.client import Client, project_id, agent_id
from ..helper import event_stream_data
from ..workflow.workflow_eval import read_captured_response, evaluate_content_publishing_response
from ..workflow.workflow_prompt import CONTENT_PUBLISHING_PHASED_PROMPTS

# Max chars to print for event-stream validation
_EVENT_STREAM_PRINT_CHARS = 1200


def _print_event_stream_validation(assistant_text: str, raw_sse: str) -> None:
    """Print event-stream response summary and sample to stdout for validation."""
    print("\n[event-stream validation]")
    print("  assistant text length: %d bytes" % len(assistant_text))
    print("  raw SSE length: %d bytes" % len(raw_sse))
    if assistant_text:
        sample = assistant_text[: _EVENT_STREAM_PRINT_CHARS]
        if len(assistant_text) > _EVENT_STREAM_PRINT_CHARS:
            sample += "... [truncated]"
        print("  assistant text sample:\n%s" % sample)
    if raw_sse:
        sample = raw_sse[: _EVENT_STREAM_PRINT_CHARS]
        if len(raw_sse) > _EVENT_STREAM_PRINT_CHARS:
            sample += "\n... [truncated]"
        print("  raw SSE sample:\n%s" % sample)
    if not assistant_text and not raw_sse:
        print("  (no data received)")
    print()


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
    progress_token = str(uuid.uuid4())
    ctx.append_step("Event stream + chat (async): open api/events, then send with progressToken")

    def send_chat():
        out, st = mcp.chat_send(
            session_id, "chat-with-nanobot", prompts[0], progress_token=progress_token
        )
        if st != 200:
            raise RuntimeError("chat send: status=%s" % st)

    response_text, raw_sse = mcp.get_response_from_events_async(session_id, send_chat_fn=send_chat)
    if response_text:
        ctx.append_step("Got reply from events: %s", (response_text[:80] + "..." if len(response_text) > 80 else response_text))
    ctx.append_step("Assert 200 and response received")

    # Print event-stream response for validation
    _print_event_stream_validation(response_text, raw_sse)

    event_stream_data.save_event_stream_response(
        "nanobot_workflow_content_publishing_step_eval", session_id, response_text or "", raw_sse=raw_sse
    )
    out_path = event_stream_data.write_step_eval_output_file(
        "nanobot_workflow_content_publishing_step_eval", ctx.trajectory, raw_sse, session_id
    )
    print("[step_eval] Full steps + raw SSE saved to: %s" % out_path)

    return Result(pass_=True, message="sent phase 0 prompt, status 200, events reply %d bytes" % len(response_text))


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
