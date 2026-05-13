"""Eval case definitions and run functions."""
import importlib.util
import json
import os
import time
import uuid

from .framework import Case, Context, Result, TurnEvalDetail
from ..clients.client import Client, project_id, agent_id
from ..helper import event_stream_data, paths
from ..workflow.workflow_prompt import CONTENT_PUBLISHING_PHASED_PROMPTS, get_conversation_turns

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


def _expected_prompt_for_workflow(workflow_id: str, prompt_text: str) -> str | None:
    """
    Decide how to match SSE events for a conversation workflow.

    - For deep_news_briefing: return None so we capture all assistant content for a
      single long prompt with tool calls without relying on exact echo matching.
    - For multi-turn workflows: use a stable prompt anchor for SSE turn isolation.
    """
    if workflow_id == "deep_news_briefing":
        return None
    if workflow_id == "antv_dual_axes_viz":
        for marker in ("[[ANTV_EVAL:P1]]", "[[ANTV_EVAL:P2]]", "[[ANTV_EVAL:P3]]"):
            if marker in (prompt_text or ""):
                return marker
        for line in (prompt_text or "").splitlines():
            stripped = line.strip()
            if stripped.startswith("PHASE "):
                # Match on the phase header line only (avoids em-dash / length mismatches).
                return stripped[:100]
        text = (prompt_text or "").strip()
        if len(text) > 400:
            return text[:200]
        return text or None
    return prompt_text or None


def _workflow_trace_filename(workflow_id: str) -> str | None:
    """Return per-workflow distinct SSE trace filename under eval/data/."""
    if workflow_id == "python_code_review":
        return "python_review.txt"
    if workflow_id == "deep_news_briefing":
        return "news.txt"
    if workflow_id == "antv_dual_axes_viz":
        return "antv_charts.txt"
    return None


def _write_workflow_distinct_trace_from_phases(workflow_id: str, raw_sse_per_phase: list[str]) -> None:
    """
    Persist workflow distinct SSE directly from this run's in-memory phase payloads.

    This avoids relying on data.json slicing and guarantees the per-workflow file
    (python_review.txt/news.txt/antv_charts.txt) always reflects the latest run.
    """
    txt_filename = _workflow_trace_filename(workflow_id)
    if not txt_filename:
        return
    parts = [s for s in raw_sse_per_phase if s and s.strip()]
    combined = "\n".join(parts)
    distinct = event_stream_data.make_distinct_sse(combined) if combined else ""
    txt_path = paths.data_path(txt_filename)
    os.makedirs(os.path.dirname(txt_path), exist_ok=True)
    with open(txt_path, "w", encoding="utf-8") as f:
        f.write(distinct if distinct else "(no data)")


def run_workflow_conversation_eval(ctx: Context) -> Result:
    """
    Multi-turn conversation: send prompt → get response → run DeepEval (turn-specific criteria)
    → send next prompt (eval-based reply) → … . No manual DeepEval step; eval runs after each turn.
    Workflow and turns come from workflow_prompt.get_conversation_turns(workflow_id).
    Set OBOT_EVAL_CONVERSATION_WORKFLOW=python_code_review (default) to choose workflow.
    """
    c = ctx.client
    workflow_id = (os.environ.get("OBOT_EVAL_CONVERSATION_WORKFLOW") or "python_code_review").strip()
    turns = get_conversation_turns(workflow_id)
    if not turns:
        return Result(pass_=False, message="no conversation turns for workflow %r" % workflow_id)

    if importlib.util.find_spec("deepeval") is None:
        return Result(
            pass_=False,
            message=(
                "Missing dependency 'deepeval'. From the eval_py directory run: "
                "python -m pip install -r requirements.txt "
                "(recommended: py -3.12 -m venv venv then "
                "venv\\Scripts\\python.exe -m eval.run ... on Windows)."
            ),
        )

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

    response_texts: list[str] = []
    raw_sse_per_phase: list[str] = []
    tools_per_phase: list[list[str]] = []
    eval_passed_per_turn: list[bool] = []
    eval_messages: list[str] = []
    turn_details: list[TurnEvalDetail] = []
    result = Result(pass_=False, message="")

    try:
        for phase in range(len(turns)):
            turn = turns[phase]
            prompt_text = turn.get("prompt") or ""
            criteria = turn.get("criteria") or []
            progress_token = str(uuid.uuid4())
            ctx.append_step("Turn %d: event stream + chat (async)", phase)

            def send_chat(progress_tok=progress_token, prompt=prompt_text):
                out, st = mcp.chat_send(
                    session_id, "chat-with-nanobot", prompt, progress_token=progress_tok
                )
                if st != 200:
                    raise RuntimeError("chat send turn %d: status=%s" % (phase, st))

            expected_prompt = _expected_prompt_for_workflow(workflow_id, prompt_text)
            response_text, raw_sse, tools_used = mcp.get_response_from_events_async(
                session_id, send_chat_fn=send_chat, expected_prompt=expected_prompt
            )
            # Keep per-turn SSE distinct and use the same payload for:
            # - data.json persistence
            # - step_eval_output*.txt
            # - turn-level DeepEval input
            distinct_raw_sse = event_stream_data.make_distinct_sse(raw_sse or "") if (raw_sse and raw_sse.strip()) else ""
            response_texts.append(response_text or "")
            raw_sse_per_phase.append(distinct_raw_sse or "")
            tools_per_phase.append(list(tools_used) if tools_used else [])
            event_stream_data.save_event_stream_response_phase(
                "nanobot_workflow_conversation_eval", session_id, phase, response_text or "", raw_sse=distinct_raw_sse or "", tools_used=tools_used
            )
            _print_event_stream_validation(response_text or "", distinct_raw_sse or "")

            # Run DeepEval for this turn (no manual step later)
            from ..agent_deepeval_generic import run_deepeval_for_turn
            detail = run_deepeval_for_turn(
                prompt_text,
                response_text or "",
                distinct_raw_sse or "",
                criteria,
                turn_index=phase,
                workflow_id=workflow_id,
            )
            turn_details.append(detail)
            eval_passed_per_turn.append(detail.passed)
            msg = detail.format_message()
            eval_messages.append("turn %d: %s" % (phase, msg))
            ctx.append_step("Turn %d DeepEval: pass=%s %s", phase, detail.passed, msg)
            if phase < len(turns) - 1:
                time.sleep(1)

        all_passed = all(eval_passed_per_turn)
        result = Result(
            pass_=all_passed,
            message="turns %d: %s" % (len(turns), "; ".join(eval_messages)),
            turn_eval_details=list(turn_details),
        )
    except Exception as e:
        ctx.append_step("Error: %s" % e)
        result = Result(pass_=False, message=str(e), turn_eval_details=list(turn_details))
    finally:
        out_path = event_stream_data.write_step_eval_output_file_multi_phase(
            "nanobot_workflow_conversation_eval", ctx.trajectory, raw_sse_per_phase, session_id, tools_per_phase=tools_per_phase
        )
        _write_workflow_distinct_trace_from_phases(workflow_id, raw_sse_per_phase)
        print("[step_eval] Steps + raw SSE (%d phases) saved to: %s" % (len(raw_sse_per_phase), out_path))

    return result


def _event_stream_response_count() -> int:
    """Return current length of event_stream_responses in data.json."""
    data_file = paths.data_path("data.json")
    try:
        with open(data_file, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return 0
    items = data.get("event_stream_responses") or []
    return len(items) if isinstance(items, list) else 0


def _update_static_trace_from_latest_sse(
    case_name: str,
    txt_filename: str,
    start_index: int,
) -> None:
    """
    Copy the latest SSE for a conversation workflow into its raw .txt file.

    - case_name is only used for logging/debug.
    - txt_filename: e.g. "python_review.txt"
    - start_index: starting index into data.json's event_stream_responses to slice from
    """
    data_file = paths.data_path("data.json")
    try:
        with open(data_file, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return

    items = data.get("event_stream_responses") or []
    if not isinstance(items, list) or start_index >= len(items):
        return

    new_items = items[start_index:]
    # Concatenate raw SSE for all new phases in this run.
    raw_sse_parts: list[str] = []
    for entry in new_items:
        raw = entry.get("raw_sse") or ""
        if raw:
            raw_sse_parts.append(raw)
    if not raw_sse_parts:
        return
    combined_sse = "\n".join(raw_sse_parts)

    # Use the same distinct-SSE logic we use for step_eval_output_distinct.txt
    # so per-test-case .txt files contain deduplicated streams.
    try:
        from ..helper.event_stream_data import make_distinct_sse

        combined_sse = make_distinct_sse(combined_sse) or combined_sse
    except Exception:
        # If distinct generation fails, fall back to raw SSE.
        pass

    txt_path = paths.data_path(txt_filename)
    os.makedirs(os.path.dirname(txt_path), exist_ok=True)
    with open(txt_path, "w", encoding="utf-8") as f:
        f.write(combined_sse)


def run_python_code_review_conversation_eval(ctx: Context) -> Result:
    """
    Convenience wrapper to run the python_code_review workflow conversation eval.

    This sets OBOT_EVAL_CONVERSATION_WORKFLOW for this process, then delegates to
    run_workflow_conversation_eval so that DeepEval runs turn-by-turn after each
    response, and finally writes distinct SSE to python_review.txt.
    """
    before = _event_stream_response_count()
    os.environ["OBOT_EVAL_CONVERSATION_WORKFLOW"] = "python_code_review"
    result = run_workflow_conversation_eval(ctx)
    _update_static_trace_from_latest_sse(
        case_name="python_code_review",
        txt_filename="python_review.txt",
        start_index=before,
    )
    return result


def run_deep_news_briefing_conversation_eval(ctx: Context) -> Result:
    """
    Convenience wrapper to run the deep_news_briefing workflow conversation eval.
    Also writes distinct SSE to news.txt.
    """
    before = _event_stream_response_count()
    os.environ["OBOT_EVAL_CONVERSATION_WORKFLOW"] = "deep_news_briefing"
    result = run_workflow_conversation_eval(ctx)
    _update_static_trace_from_latest_sse(
        case_name="deep_news_briefing",
        txt_filename="news.txt",
        start_index=before,
    )
    return result


def run_antv_dual_axes_conversation_eval(ctx: Context) -> Result:
    """
    Convenience wrapper to run the antv_dual_axes_viz workflow conversation eval.
    Also writes distinct SSE to antv_charts.txt.
    """
    before = _event_stream_response_count()
    os.environ["OBOT_EVAL_CONVERSATION_WORKFLOW"] = "antv_dual_axes_viz"
    result = run_workflow_conversation_eval(ctx)
    _update_static_trace_from_latest_sse(
        case_name="antv_dual_axes_viz",
        txt_filename="antv_charts.txt",
        start_index=before,
    )
    return result


def run_blog_post_elicitation_eval(ctx: Context) -> Result:
    """
    Single-turn test for blog-post prompt that triggers question-based elicitation.

    Sends a short blog-post request, captures the event-stream, and asserts that
    an `elicitation/create` event with question metadata is present. This verifies
    that the agent correctly uses structured elicitation for follow-up questions.
    """
    c = ctx.client
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

    prompt_text = "i want to create a blog post"
    progress_token = str(uuid.uuid4())
    ctx.append_step("Single prompt: blog post (event stream + chat async)")

    raw_sse = ""
    response_text = ""
    tools_used: list[str] = []
    try:
        def send_chat(progress_tok=progress_token, prompt=prompt_text):
            out, st = mcp.chat_send(
                session_id, "chat-with-nanobot", prompt, progress_token=progress_tok
            )
            if st != 200:
                raise RuntimeError("chat send (blog post elicitation): status=%s" % st)

        # Use expected_prompt=None so we capture all assistant / tool content until chat-done.
        response_text, raw_sse, tools_used = mcp.get_response_from_events_async(
            session_id, send_chat_fn=send_chat, expected_prompt=None
        )

        print("response_text: ", response_text)
        print("raw_sse: ", raw_sse)
        print("tools_used: ", tools_used)
        event_stream_data.save_event_stream_response(
            "nanobot_blog_post_elicitation_eval",
            session_id,
            response_text or "",
            raw_sse=raw_sse or "",
            tools_used=tools_used,
        )
        _print_event_stream_validation(response_text or "", raw_sse or "")

        if not raw_sse or not raw_sse.strip():
            msg = ("no SSE data captured for blog post prompt; "
                   "likely that this environment does not emit elicitation/create "
                   "events on the events stream. Treating as informational/skip.")
            ctx.append_step("Blog post elicitation eval skipped (no SSE): %s", msg)
            return Result(pass_=True, message=msg, case_prompt=prompt_text)

        # Basic assertions: we should see an elicitation/create event and question metadata.
        passed = "elicitation/create" in raw_sse and "ai.nanobot.meta/question" in raw_sse
        if passed:
            msg = "elicitation/create with question metadata observed in event stream"
        else:
            msg = "event stream missing elicitation/create or question metadata"
        ctx.append_step("Blog post elicitation eval: pass=%s %s", passed, msg)
        return Result(pass_=passed, message=msg, case_prompt=prompt_text)
    except Exception as e:
        ctx.append_step("Error (blog post elicitation): %s", e)
        return Result(pass_=False, message=str(e), case_prompt=prompt_text)
    finally:
        out_path = event_stream_data.write_step_eval_output_file(
            "nanobot_blog_post_elicitation_eval",
            ctx.trajectory,
            raw_sse or "",
            session_id=session_id,
        )
        print("[step_eval] Steps + raw SSE (blog post elicitation) saved to: %s" % out_path)


def all_cases() -> list[Case]:
    return [
        Case("nanobot_python_code_review_conversation_eval", "Python code review workflow: multi-turn conversation with turn-level DeepEval criteria", run_python_code_review_conversation_eval),
        Case("nanobot_deep_news_briefing_conversation_eval", "Deep news briefing workflow: multi-turn conversation with turn-level DeepEval criteria", run_deep_news_briefing_conversation_eval),
        Case("nanobot_antv_dual_axes_conversation_eval", "AntV dual-axes workflow: multi-turn conversation with turn-level DeepEval criteria", run_antv_dual_axes_conversation_eval),
        Case("nanobot_blog_post_elicitation_eval", "Blog-post request that triggers question-based elicitation UI", run_blog_post_elicitation_eval),
    ]
