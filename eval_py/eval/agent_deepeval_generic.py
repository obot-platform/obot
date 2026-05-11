"""Generic agent-level DeepEval over step-eval event-stream logs.

This variant is **scenario-agnostic**:
- It reads the latest step-eval output (trajectory + raw SSE) from eval/data/step_eval_output.txt.
- It reconstructs a minimal AgentTrace:
    - user_prompt: original user instruction text
    - final_report: final assistant message text
    - tool_calls: ordered tool names used during the turn
    - search_queries: extracted from notifications (e.g. DuckDuckGo messages)
- It then runs **generic, process-focused** DeepEval metrics that look only at:
    - Flow and planning
    - Tool usage and retries
    - Goal alignment between input and final answer

There is **no scenario-specific logic** here (no hard-coded topic, tools, or phases).
"""

from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass
from typing import List, Tuple

from deepeval import evaluate
from deepeval.metrics import GEval
from deepeval.test_case import LLMTestCase, LLMTestCaseParams

from .core.framework import TurnEvalDetail
from .helper import paths


@dataclass
class AgentTrace:
    """Minimal agent trace reconstructed from event-stream raw SSE."""

    user_prompt: str
    final_report: str
    tool_calls: List[str]
    search_queries: List[str]
    # Generic, derived process features (for richer, non-scenario-specific context)
    tool_error_counts: dict
    assistant_step_summaries: List[str]


def _safe_console(text: str, max_len: int = 200) -> str:
    """Return a snippet safe for Windows console encodings."""
    snippet = text[:max_len]
    if len(text) > max_len:
        snippet += "..."
    # Replace characters not representable in cp1252 with '?'
    return snippet.encode("cp1252", errors="replace").decode("cp1252")


def _read_step_eval_raw_sse() -> Tuple[str, str]:
    """
    Return (trajectory_text, raw_sse) from step_eval_output_distinct.txt.

    Supports two formats:
    - Multi-phase (e.g. python_code_review): "--- Phase 0 ---" then "Event stream (api/events) raw response:" then SSE.
    - Single-prompt (e.g. deep_news_briefing): "--- Event stream (api/events) raw response ---" then SSE.
    """
    path = paths.data_path("step_eval_output_distinct.txt")
    if not os.path.exists(path):
        raise FileNotFoundError(path)
    with open(path, "r", encoding="utf-8") as f:
        lines = f.read().splitlines()

    trajectory_lines: List[str] = []
    raw_sse_lines: List[str] = []

    in_steps = False
    in_phase0 = False
    in_single_sse = False  # single-prompt format: "--- Event stream ... ---" then SSE

    for line in lines:
        if line.startswith("--- Steps (trajectory) ---"):
            in_steps = True
            in_phase0 = False
            in_single_sse = False
            continue
        if line.startswith("--- Phase 0 ---"):
            in_steps = False
            in_phase0 = True
            in_single_sse = False
            continue
        # Single-prompt format (e.g. nanobot_deep_news_briefing_single_prompt_eval): no Phase 0, direct "--- Event stream ... ---"
        if line.strip().startswith("---") and "Event stream (api/events) raw response" in line:
            in_steps = False
            in_phase0 = False
            in_single_sse = True
            continue  # skip the header line itself
        if in_steps:
            trajectory_lines.append(line)
        if in_phase0:
            if "Event stream (api/events) raw response:" in line:
                continue
            raw_sse_lines.append(line)
        if in_single_sse:
            raw_sse_lines.append(line)

    trajectory_text = "\n".join(trajectory_lines).strip()
    raw_sse = "\n".join(raw_sse_lines).strip()
    return trajectory_text, raw_sse


def _parse_sse_to_trace(raw_sse: str) -> AgentTrace:
    """Parse event-stream raw SSE into a minimal AgentTrace.

    - Deduplicates events by (created, id) so repeated SSE events are only processed once.
    - Collects:
        - user_prompt: all user text items (role == "user")
        - final_report: all assistant text items (role == "assistant"), concatenated
        - tool_calls: ordered list of tool names (type == "tool")
        - search_queries: strings like "Searching ... for: X" from notifications/message events
    """
    current_event: str | None = None
    current_data_lines: List[str] = []
    seen_event_keys: set[tuple[str, str]] = set()

    user_prompt_parts: List[str] = []
    assistant_text_parts: List[str] = []
    tool_calls: List[str] = []
    search_queries: List[str] = []
    tool_error_counts: dict[str, int] = {}
    assistant_step_summaries: List[str] = []

    def flush_event() -> None:
        nonlocal current_event, current_data_lines
        if not current_data_lines:
            current_data_lines = []
            return
        data_str = "\n".join(current_data_lines).strip()
        current_data_lines = []
        if not data_str or data_str == "{}":
            return
        try:
            ev = json.loads(data_str)
        except json.JSONDecodeError:
            return

        created = ev.get("created")
        eid = ev.get("id")
        event_key = (str(created), str(eid)) if created is not None and eid is not None else None
        is_duplicate = event_key in seen_event_keys if event_key else False
        if event_key:
            seen_event_keys.add(event_key)

        role = ev.get("role")
        items = ev.get("items") or []
        if isinstance(items, list) and not is_duplicate:
            for item in items:
                if not isinstance(item, dict):
                    continue
                if item.get("type") == "text" and item.get("text"):
                    if role == "user":
                        user_prompt_parts.append(item["text"])
                    elif role == "assistant":
                        assistant_text_parts.append(item["text"])
                        # Capture a few short assistant step summaries (status/progress messages)
                        if len(assistant_step_summaries) < 6:
                            snippet = item["text"].strip()
                            if snippet:
                                assistant_step_summaries.append(snippet[:200])
                if item.get("type") == "tool" and item.get("name"):
                    name = item["name"]
                    tool_calls.append(name)
                    # Very generic error signal: explicit isError flag or obvious error text
                    out = item.get("output") or {}
                    is_err_flag = bool(out.get("isError"))
                    # Some tools embed errors only in text content
                    text_blobs = []
                    for k in ("content", "result", "message"):
                        v = out.get(k)
                        if isinstance(v, str):
                            text_blobs.append(v)
                        elif isinstance(v, list):
                            for elt in v:
                                if isinstance(elt, dict) and isinstance(elt.get("text"), str):
                                    text_blobs.append(elt["text"])
                    joined = " ".join(text_blobs)
                    looks_error = "error" in joined.lower() or "no results were found" in joined.lower()
                    if is_err_flag or looks_error:
                        tool_error_counts[name] = tool_error_counts.get(name, 0) + 1

        # Extract generic search queries from notifications/message events
        if current_event == "notifications/message":
            data_field = ev.get("data") or {}
            stack = [data_field]
            while stack:
                node = stack.pop()
                if isinstance(node, dict):
                    for v in node.values():
                        stack.append(v)
                elif isinstance(node, str):
                    # Very lightweight "Searching ... for: X" matcher
                    m = re.search(r"Searching .* for: (.+)", node)
                    if m:
                        search_queries.append(m.group(1).strip())

    for raw_line in raw_sse.splitlines():
        line = raw_line.strip("\r\n")
        if line == "":
            flush_event()
            current_event = None
            continue
        if line.startswith("event:"):
            flush_event()
            current_event = line[6:].strip()
            continue
        if line.startswith("data:"):
            current_data_lines.append(line[5:].strip())
            continue

    flush_event()

    user_prompt = "\n".join(user_prompt_parts).strip()
    final_report = "".join(assistant_text_parts).strip()

    # Deduplicate tools in order
    seen = set()
    ordered_tools: List[str] = []
    for name in tool_calls:
        if name not in seen:
            seen.add(name)
            ordered_tools.append(name)

    return AgentTrace(
        user_prompt=user_prompt,
        final_report=final_report,
        tool_calls=ordered_tools,
        search_queries=search_queries,
        tool_error_counts=tool_error_counts,
        assistant_step_summaries=assistant_step_summaries,
    )


def _build_deepeval_test_case(trace: AgentTrace) -> LLMTestCase:
    """Create a **generic** LLMTestCase for DeepEval (process-focused)."""
    tool_summary = f"Tools used in order: {trace.tool_calls}"
    search_summary = f"Search queries (from notifications): {trace.search_queries}"
    distinct_tools = sorted(set(trace.tool_calls))
    tool_counts = {t: trace.tool_calls.count(t) for t in distinct_tools}
    error_summary = f"Tool errors (by name): {trace.tool_error_counts}"
    step_summaries = trace.assistant_step_summaries

    context: List[str] = [
        "You are evaluating an autonomous AI agent's behavior based on an event-stream trace.",
        "",
        "You are given:",
        "- INPUT: the original user instruction.",
        "- ACTUAL_OUTPUT: the agent's final response.",
        "- CONTEXT: a summarized view of tools used, retries, and searches.",
        "",
        "Evaluate the PROCESS (flow, tool usage, goal alignment), not factual correctness:",
        "- Did the agent appear to understand the goal and decompose it into sensible steps?",
        "- Did it make forward progress instead of looping or stalling?",
        "- Did it use tools and retries sensibly (no obvious spamming, no giving up immediately)?",
        "- Did it stop using tools and switch to answering once it had enough information?",
        "- Is the final answer on-track with the original instruction?",
        "",
        tool_summary,
        f"Distinct tools and counts: {tool_counts}",
        search_summary,
        error_summary,
        f"Assistant step summaries (first few status messages): {step_summaries}",
    ]

    return LLMTestCase(
        input=trace.user_prompt or "User instruction (missing in logs).",
        actual_output=trace.final_report,
        context=context,
    )


def _build_agent_metrics() -> list[GEval]:
    """Generic, process-focused DeepEval metrics (scenario-agnostic)."""
    eval_params = [
        LLMTestCaseParams.INPUT,
        LLMTestCaseParams.ACTUAL_OUTPUT,
        LLMTestCaseParams.CONTEXT,
    ]

    flow = GEval(
        name="Flow and Planning",
        criteria=(
            "Given INPUT (user instruction), ACTUAL_OUTPUT (final answer), and CONTEXT (tool log summary):\n"
            "- Did the agent appear to understand the goal and follow a coherent multi-step process?\n"
            "- Did it make forward progress instead of looping or stalling?\n"
            "- Does the overall sequence of actions look reasonable for the type of task implied by the INPUT?\n"
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    tools = GEval(
        name="Tool Usage and Retries",
        criteria=(
            "Based on the tools and searches listed in CONTEXT:\n"
            "- Did the agent choose tools that make sense for the goal (e.g., search/fetch for research tasks)?\n"
            "- Did it handle tool failures sensibly (some retries or fallbacks, but not infinite loops)?\n"
            "- Did it stop using tools once enough information was gathered and switch to answering?\n"
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    alignment = GEval(
        name="Goal Alignment",
        criteria=(
            "Compare INPUT and ACTUAL_OUTPUT:\n"
            "- Is the final answer clearly aimed at the user's goal?\n"
            "- Does it stay on-topic and reflect the kind of work implied by the tool usage in CONTEXT?\n"
            "- Even if imperfect, is it a reasonable attempt at what was asked?\n"
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    robustness = GEval(
        name="Robustness to Failures",
        criteria=(
            "Using CONTEXT (tool usage, errors, retries, and step summaries):\n"
            "- When tools failed or were rate-limited, did the agent adjust (e.g., change queries, switch tools, or proceed with partial information)?\n"
            "- Did it avoid clearly wasteful behavior (e.g., calling the same failing tool many times with no change)?\n"
            "- Did it still attempt a reasonable final answer instead of stopping as soon as tools failed?\n"
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    return [flow, tools, alignment, robustness]


def run_deepeval_for_turn(
    user_prompt: str,
    assistant_text: str,
    raw_sse: str,
    criteria: List[str],
    turn_index: int = 0,
) -> TurnEvalDetail:
    """
    Run DeepEval on a single conversation turn with custom criteria.
    Used by conversation workflow: after each turn we evaluate the response, then send the next prompt.
    """
    if not criteria:
        return TurnEvalDetail(
            turn_index=turn_index,
            passed=True,
            score=None,
            threshold=None,
            reason="no criteria (skip eval)",
            prompt=user_prompt or "",
        )

    # Prefer final assistant text reconstructed from raw SSE (deduplicated by event id),
    # so DeepEval sees the cleaned-up reply instead of streaming repetitions.
    if raw_sse:
        try:
            trace = _parse_sse_to_trace(raw_sse)
            if trace.final_report:
                assistant_text = trace.final_report
        except Exception:
            # Fall back silently to provided assistant_text
            pass

    context = [
        "You are evaluating an assistant's response for a single conversation turn.",
        "Check that the response satisfies all of the following criteria:",
        "",
    ] + ["- " + c for c in criteria]
    test_case = LLMTestCase(
        input=user_prompt or "(no prompt)",
        actual_output=assistant_text or "(no response)",
        context=context,
    )
    eval_params = [
        LLMTestCaseParams.INPUT,
        LLMTestCaseParams.ACTUAL_OUTPUT,
        LLMTestCaseParams.CONTEXT,
    ]
    metric = GEval(
        name="Turn %d criteria" % turn_index,
        criteria="\n".join(criteria),
        evaluation_params=eval_params,
        threshold=0.7,
    )
    try:
        metric.measure(test_case)
        score = getattr(metric, "score", None)
        threshold = getattr(metric, "threshold", 0.7)
        reason = getattr(metric, "reason", "")
        passed = (score is None) or (score >= threshold)
        reason_str = str(reason) if reason else ""
        sc = float(score) if score is not None else None
        th = float(threshold) if threshold is not None else None
        return TurnEvalDetail(
            turn_index=turn_index,
            passed=passed,
            score=sc,
            threshold=th,
            reason=reason_str,
            prompt=user_prompt or "",
        )
    except Exception as e:
        return TurnEvalDetail(
            turn_index=turn_index,
            passed=False,
            score=None,
            threshold=None,
            reason="eval error (turn %d): %s" % (turn_index, str(e)),
            prompt=user_prompt or "",
        )


def run_deepeval_generic_for_latest_step_eval() -> None:
    """Entry point: run generic DeepEval process metrics on latest step-eval output."""
    trajectory, raw_sse = _read_step_eval_raw_sse()
    if not raw_sse:
        raise RuntimeError("No raw SSE found in step_eval_output_distinct.txt")

    trace = _parse_sse_to_trace(raw_sse)
    test_case = _build_deepeval_test_case(trace)
    metrics = _build_agent_metrics()

    print("=== Generic Agent trace (summary) ===")
    print("Tools (ordered):", trace.tool_calls)
    print("Search queries:", trace.search_queries)
    print("User prompt snippet:", _safe_console(trace.user_prompt))
    print("Final report snippet:", _safe_console(trace.final_report))
    print()

    evaluate(test_cases=[test_case], metrics=metrics)


if __name__ == "__main__":
    run_deepeval_generic_for_latest_step_eval()

