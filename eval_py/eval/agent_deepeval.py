"""Agent-level DeepEval for nanobot step eval using event-stream logs.

This module reads the latest step-eval files under eval/data/:
- step_eval_output.txt: trajectory + raw SSE (event-stream)
- data.json: per-phase event-stream summaries (optional)
- eval-api-log.txt: API-level log (optional)

From the raw SSE we reconstruct an agent trace for a single phase:
- user goal / prompt
- ordered tool calls (by name)
- search queries (from DuckDuckGo notifications)
- final assistant report text

We then run DeepEval **agent-style** metrics, NOT plain LLM eval:
- Task Completion
- Tool Strategy
- Source Selection Quality
- Cross-Referencing Integrity
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

from .helper import paths


@dataclass
class AgentTrace:
    """Minimal agent trace reconstructed from event-stream raw SSE."""

    user_prompt: str
    final_report: str
    tool_calls: List[str]
    search_queries: List[str]


def _safe_console(text: str, max_len: int = 200) -> str:
    """Return a snippet safe for Windows console encodings."""
    snippet = text[:max_len]
    if len(text) > max_len:
        snippet += "..."
    # Replace characters not representable in cp1252 with '?'
    return snippet.encode("cp1252", errors="replace").decode("cp1252")


def _read_step_eval_raw_sse() -> Tuple[str, str]:
    """
    Return (trajectory_text, raw_sse_for_phase_0) from step_eval_output.txt.

    Assumes format written by write_step_eval_output_file_multi_phase().
    """
    path = paths.data_path("step_eval_output.txt")
    if not os.path.exists(path):
        raise FileNotFoundError(path)
    with open(path, "r", encoding="utf-8") as f:
        lines = f.read().splitlines()

    trajectory_lines: List[str] = []
    raw_sse_lines: List[str] = []

    in_steps = False
    in_phase0 = False

    for line in lines:
        if line.startswith("--- Steps (trajectory) ---"):
            in_steps = True
            in_phase0 = False
            continue
        if line.startswith("--- Phase 0 ---"):
            in_steps = False
            in_phase0 = True
            continue
        if in_steps:
            trajectory_lines.append(line)
        if in_phase0:
            # After the "Event stream" header, we just copy through to end-of-file
            if "Event stream (api/events) raw response:" in line:
                continue
            raw_sse_lines.append(line)

    trajectory_text = "\n".join(trajectory_lines).strip()
    raw_sse = "\n".join(raw_sse_lines).strip()
    return trajectory_text, raw_sse


def _parse_sse_to_trace(raw_sse: str) -> AgentTrace:
    """Parse event-stream raw SSE into a minimal AgentTrace.
    Deduplicates by (created, id) so repeated SSE events (same message sent many times
    with progressive tool output) are only processed once, matching MCP client behavior.
    """
    current_event: str | None = None
    current_data_lines: List[str] = []
    seen_event_keys: set[tuple[str, str]] = set()

    user_prompt_parts: List[str] = []
    assistant_text_parts: List[str] = []
    tool_calls: List[str] = []
    search_queries: List[str] = []

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
                if item.get("type") == "tool" and item.get("name"):
                    tool_calls.append(item["name"])

        # DuckDuckGo notifications live in notifications/message events
        if current_event == "notifications/message":
            # Very lightweight extract of "Searching DuckDuckGo for: X"
            data_field = ev.get("data") or {}
            # Walk nested dicts defensively
            stack = [data_field]
            while stack:
                node = stack.pop()
                if isinstance(node, dict):
                    for v in node.values():
                        stack.append(v)
                elif isinstance(node, str):
                    m = re.search(r"Searching DuckDuckGo for: (.+)", node)
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
    # Join with "" so streamed tokens (one per SSE item) form one coherent report;
    # using "\n" would put each token on its own line and make output look fragmented.
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
    )


def _task_relevant_tools(tool_calls: List[str]) -> List[str]:
    """DuckDuckGo/briefing-relevant tool names only (search, fetch_content)."""
    want = {"search", "fetch_content"}
    return [t for t in tool_calls if t in want]


def _build_deepeval_test_case(trace: AgentTrace) -> LLMTestCase:
    """Create an LLMTestCase with rich agent context for DeepEval."""
    task_tools = _task_relevant_tools(trace.tool_calls)
    context: List[str] = [
        "User goal: Produce a deep, multi-source news briefing on the US–China trade war and tariffs.",
        "Required tool: DuckDuckGo MCP server from Obot (search and fetch_content).",
        "Expected process: (1) Up to 5 searches; if 0 results, try shorter queries or fetch_content on known URLs e.g. Wikipedia. (2) Select up to 6 URLs. (3) Fetch content from at least 2 sources; stop after 2+ loaded. (4) Cross-reference. (5) Final report with sections: ## US–China Trade War Briefing, ### Confirmed facts, ### Conflicting or single-source claims, ### Key data points, ### Note on sources.",
        f"DuckDuckGo tools used (ordered): {task_tools}",
        f"All tool calls (ordered): {trace.tool_calls}",
        f"Search queries detected from notifications: {trace.search_queries}",
    ]
    # If output looks truncated (stream may have ended before agent finished), tell judge to score what's present
    report = (trace.final_report or "").strip()
    if report and (len(report) < 800 or report.endswith(" its") or report.endswith("—") or report.endswith("...")):
        context.append("Note: The actual output may be truncated (stream timeout or tool failures); evaluate the structure and content that is present.")

    return LLMTestCase(
        input=trace.user_prompt or "US–China trade war deep briefing via DuckDuckGo MCP.",
        actual_output=trace.final_report,
        context=context,
    )


def _build_agent_metrics() -> list[GEval]:
    """Agent-style DeepEval metrics (no pure LLM answer scoring)."""
    # GEval requires evaluation_params as a list of LLMTestCaseParams indicating
    # which fields of the test case are given to the judge model.
    eval_params = [
        LLMTestCaseParams.INPUT,
        LLMTestCaseParams.ACTUAL_OUTPUT,
        LLMTestCaseParams.CONTEXT,
    ]

    partial_note = " If the output is incomplete or truncated, score based on what is present (phases attempted, tools used, any briefing content)."
    task_completion = GEval(
        name="Task Completion",
        criteria=(
            "Did the agent fully execute all required phases of the task:\n"
            "- Multi-angle searching\n"
            "- Source selection\n"
            "- Deep reading\n"
            "- Cross-referencing\n"
            "- Final structured report\n"
            + partial_note
        ),
        evaluation_params=eval_params,
        threshold=0.70,
    )

    tool_strategy = GEval(
        name="Tool Strategy",
        criteria=(
            "Did the agent use the DuckDuckGo tools appropriately?\n"
            "- search used for discovery\n"
            "- fetch_content used only on selected URLs\n"
            "- No obvious misuse or redundant calls\n"
            + partial_note
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    source_quality = GEval(
        name="Source Selection Quality",
        criteria=(
            "Evaluate whether the (implied or referenced) sources are:\n"
            "- Diverse (different outlets)\n"
            "- Substantive (analysis, reporting, background)\n"
            "- Relevant to US–China trade and tariffs\n"
            + partial_note
        ),
        evaluation_params=eval_params,
        threshold=0.70,
    )

    cross_reference = GEval(
        name="Cross-Referencing Integrity",
        criteria=(
            "Did the agent correctly:\n"
            "- Identify facts confirmed by multiple sources\n"
            "- Flag or discuss conflicting claims\n"
            "- Mark or treat single-source claims as less certain\n"
            "- Reference evidence consistently in the report\n"
            + partial_note
        ),
        evaluation_params=eval_params,
        threshold=0.70,
    )

    return [task_completion, tool_strategy, source_quality, cross_reference]


def run_deepeval_for_latest_step_eval() -> None:
    """Entry point: run DeepEval agent metrics on latest step-eval output."""
    trajectory, raw_sse = _read_step_eval_raw_sse()
    if not raw_sse:
        raise RuntimeError("No raw SSE found in step_eval_output.txt")

    trace = _parse_sse_to_trace(raw_sse)
    test_case = _build_deepeval_test_case(trace)
    metrics = _build_agent_metrics()

    print("=== Agent trace (summary) ===")
    print("Tools (ordered):", trace.tool_calls)
    print("Search queries:", trace.search_queries)
    print("User prompt snippet:", _safe_console(trace.user_prompt))
    print("Final report snippet:", _safe_console(trace.final_report))
    print()

    evaluate(test_cases=[test_case], metrics=metrics)


if __name__ == "__main__":
    run_deepeval_for_latest_step_eval()

