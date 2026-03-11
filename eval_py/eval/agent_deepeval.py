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
import sys
from dataclasses import dataclass
from typing import Callable, List, Tuple

from deepeval import evaluate
from deepeval.metrics import GEval
from deepeval.test_case import LLMTestCase, LLMTestCaseParams

from .helper import paths


@dataclass
class AgentTrace:
    """Minimal agent trace for a single test case.

    This can be reconstructed either from:
    - raw SSE logs (news.txt, antv_charts.txt, python_review.txt), or
    - a static JSON file on disk.
    """

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


def _read_trace_from_json(case_name: str) -> AgentTrace:
    """
    Load a static AgentTrace from a JSON data file.

    This lets us run DeepEval against deterministic test fixtures instead of
    re-reading raw SSE logs every time.

    Expected schema in eval/data/{case_name}.json:

    {
      "user_prompt": "...",
      "final_report": "...",
      "tool_calls": ["search", "fetch_content"],
      "search_queries": ["US-China trade war tariffs latest news 2025", "..."]
    }
    """
    data_path = paths.data_path(f"{case_name}.json")
    if not os.path.exists(data_path):
        raise FileNotFoundError(data_path)
    with open(data_path, "r", encoding="utf-8") as f:
        obj = json.load(f)

    def _dedupe_repeated_lines(text: str) -> str:
        """Collapse exact duplicate lines while preserving first occurrence order."""
        seen: set[str] = set()
        out: list[str] = []
        for line in (text or "").splitlines():
            if line not in seen:
                seen.add(line)
                out.append(line)
        return "\n".join(out).strip()

    user_prompt = _dedupe_repeated_lines(obj.get("user_prompt") or "")
    final_report = _dedupe_repeated_lines(obj.get("final_report") or "")
    tool_calls = list(obj.get("tool_calls") or [])
    search_queries = list(obj.get("search_queries") or [])

    return AgentTrace(
        user_prompt=user_prompt,
        final_report=final_report,
        tool_calls=tool_calls,
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


def _build_python_review_test_case(trace: AgentTrace) -> LLMTestCase:
    """Create an LLMTestCase for the python_code_review workflow."""
    context: list[str] = [
        "You are evaluating an assistant's behavior on a simple Python code review exercise.",
        "",
        "The conversation has two user turns:",
        "1) User shows invalid code: `for i in range(5)\\n    print(i)`",
        "   - Assistant should explain that the colon `:` after `range(5)` is missing,",
        "     and provide corrected code with proper indentation.",
        "2) User then asks to modify it to print only even numbers.",
        "   - Assistant should update the code so that it prints 0, 2, 4 (only even numbers),",
        "     e.g. using `if i % 2 == 0` or `range(0, 5, 2)` or an equivalent correct approach.",
        "",
        "Evaluate whether the final assistant output (which may contain both turns) clearly:",
        "- Identifies the missing colon syntax error in the first snippet.",
        "- Provides a corrected loop with a colon and an indented print statement.",
        "- Shows or explains a solution that prints only even numbers for the second request.",
    ]
    return LLMTestCase(
        input=trace.user_prompt or "Python code review conversation.",
        actual_output=trace.final_report,
        context=context,
    )


def _build_python_review_metrics() -> list[GEval]:
    """DeepEval metrics for the python_code_review workflow."""
    eval_params = [
        LLMTestCaseParams.INPUT,
        LLMTestCaseParams.ACTUAL_OUTPUT,
        LLMTestCaseParams.CONTEXT,
    ]

    colon_metric = GEval(
        name="Python Review – Missing Colon Fix",
        criteria=(
            "Check that the assistant correctly handles the first user question:\n"
            "- Explicitly mentions that a colon `:` is missing after `range(5)` (or equivalent wording).\n"
            "- Provides corrected code using `for i in range(5):` with an indented `print(i)` line."
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    even_numbers_metric = GEval(
        name="Python Review – Even Numbers Update",
        criteria=(
            "Check that the assistant correctly handles the follow-up request:\n"
            "- Modifies the loop so that it prints only even numbers between 0 and 4 (0, 2, 4).\n"
            "- Uses a valid Python approach (e.g., `if i % 2 == 0` or `range(0, 5, 2)` or equivalent).\n"
            "- The explanation and code are consistent and syntactically valid."
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    return [colon_metric, even_numbers_metric]


def _build_antv_charts_test_case(trace: AgentTrace) -> LLMTestCase:
    """Create an LLMTestCase for the AntV dual-axes chart workflow."""
    context: list[str] = [
        "You are evaluating an assistant's behavior on an AntV Charts business visualization workflow.",
        "",
        "The task has three phases:",
        "1) Phase 1 — Dataset: Confirm the 12-month revenue/profit_margin dataset is valid (Jan–Dec, no nulls, numeric).",
        "2) Phase 2 — Chart config: Describe dual-axes setup (X=month, left Y=revenue columns, right Y=profit_margin line, title, tooltip, legend, formatting).",
        "3) Phase 3 — Analysis: Provide insights, highest revenue month, highest profit margin month, correlation, anomalies, risks, opportunities, executive summary.",
        "",
        "From the provided 2025 dataset: highest revenue = Dec (340000), highest profit_margin = Dec (35%).",
        "Evaluate whether the response confirms dataset validity, describes the chart configuration, and provides analysis that identifies Dec correctly and references the trend.",
    ]
    return LLMTestCase(
        input=trace.user_prompt or "AntV dual-axes chart workflow.",
        actual_output=trace.final_report,
        context=context,
    )


def _build_antv_charts_metrics() -> list[GEval]:
    """DeepEval metrics for the AntV dual-axes chart workflow."""
    eval_params = [
        LLMTestCaseParams.INPUT,
        LLMTestCaseParams.ACTUAL_OUTPUT,
        LLMTestCaseParams.CONTEXT,
    ]

    dataset_metric = GEval(
        name="AntV – Dataset validation",
        criteria=(
            "Check that the response explicitly confirms the dataset covers all 12 months (Jan–Dec) with both revenue and profit_margin present, "
            "and mentions at least one data quality check (e.g. no nulls, numeric fields) and that the dataset is suitable for charting."
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    config_metric = GEval(
        name="AntV – Chart configuration",
        criteria=(
            "Check that the response describes a dual-axes configuration: month on X-axis, revenue as columns on the left Y-axis, "
            "profit_margin as a line on the right Y-axis, and mentions formatting (e.g. currency for revenue, percentage for profit_margin) and distinct colors."
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    analysis_metric = GEval(
        name="AntV – Visual analysis",
        criteria=(
            "Check that the response identifies the correct highest revenue month (Dec) and highest profit margin month (Dec) from the 2025 dataset, "
            "and provides plausible insights, risks, or opportunities that reference the revenue and profit_margin trends."
        ),
        evaluation_params=eval_params,
        threshold=0.7,
    )

    return [dataset_metric, config_metric, analysis_metric]


def _run_deepeval_for_trace(
    trace: AgentTrace,
    build_test_case: Callable[[AgentTrace], LLMTestCase],
    build_metrics: Callable[[], list[GEval]],
    label: str,
) -> None:
    """Shared entrypoint: print trace summary and run DeepEval with given builders."""
    test_case = build_test_case(trace)
    metrics = build_metrics()

    print(f"=== {label} trace ===")
    print("Tools (ordered):", trace.tool_calls)
    print("Search queries:", trace.search_queries)
    print("User prompt snippet:", _safe_console(trace.user_prompt))
    print("Final output snippet:", _safe_console(trace.final_report))
    print()

    evaluate(test_cases=[test_case], metrics=metrics)


def run_deepeval_for_latest_step_eval() -> None:
    """Entry point: run DeepEval agent metrics on latest step-eval output.

    For backward compatibility this still reads step_eval_output.txt, reconstructs
    an AgentTrace from raw SSE, and then delegates to the shared runner.
    """
    _trajectory, raw_sse = _read_step_eval_raw_sse()
    if not raw_sse:
        raise RuntimeError("No raw SSE found in step_eval_output.txt")

    trace = _parse_sse_to_trace(raw_sse)
    _run_deepeval_for_trace(trace, _build_deepeval_test_case, _build_agent_metrics, "Latest step-eval agent")


def run_deepeval_for_static_case(case_name: str) -> None:
    """Run DeepEval for a static JSON-backed deep news test case.

    The JSON file eval/data/{case_name}.json should contain *deduplicated*,
    pre-defined data for that test:
      - user_prompt: the full instructions for the case
      - final_report: a single, cleaned-up assistant reply
      - tool_calls / search_queries: optional metadata for context

    This allows evaluating a stable, static trace without re-parsing raw
    event-stream logs or step-eval files.
    """
    trace = _read_trace_from_json(case_name)
    _run_deepeval_for_trace(trace, _build_deepeval_test_case, _build_agent_metrics, f"Static agent ({case_name})")


def run_deepeval_for_python_review_static(case_name: str = "python_review_1") -> None:
    """Run DeepEval for the python_code_review workflow using a static JSON trace.

    Expects eval/data/{case_name}.json to exist and contain a *deduplicated*
    AgentTrace. The JSON should be treated as a pre-defined, static fixture
    for this test (not a raw event-stream dump).
    """
    trace = _read_trace_from_json(case_name)
    _run_deepeval_for_trace(
        trace,
        _build_python_review_test_case,
        _build_python_review_metrics,
        f"Python review ({case_name})",
    )


def run_deepeval_for_antv_static(case_name: str = "antv_charts_1") -> None:
    """Run DeepEval for the AntV dual-axes chart workflow using a static JSON trace.

    Expects eval/data/{case_name}.json to exist and contain a *deduplicated*
    AgentTrace (user_prompt, final_report, tool_calls, search_queries).
    """
    trace = _read_trace_from_json(case_name)
    _run_deepeval_for_trace(
        trace,
        _build_antv_charts_test_case,
        _build_antv_charts_metrics,
        f"AntV charts ({case_name})",
    )


def run_all_static_workflows() -> None:
    """Run all curated static workflows sequentially and print their results."""
    print("=== Running Python review static workflow (python_review_1) ===")
    run_deepeval_for_python_review_static("python_review_1")
    print()

    print("=== Running deep news briefing static workflow (deep_news_briefing_1) ===")
    run_deepeval_for_static_case("deep_news_briefing_1")
    print()

    print("=== Running AntV charts static workflow (antv_charts_1) ===")
    run_deepeval_for_antv_static("antv_charts_1")
    print()


def _main() -> None:
    """Simple CLI dispatch for running DeepEval workflows."""
    if len(sys.argv) <= 1 or sys.argv[1] == "all":
        # Default: run the three curated static workflows.
        run_all_static_workflows()
    elif sys.argv[1] == "latest":
        # Backwards-compatible: evaluate the latest step-eval output.
        run_deepeval_for_latest_step_eval()
    elif sys.argv[1] == "python_review":
        run_deepeval_for_python_review_static("python_review_1")
    elif sys.argv[1] == "deep_news":
        run_deepeval_for_static_case("deep_news_briefing_1")
    elif sys.argv[1] == "antv":
        run_deepeval_for_antv_static("antv_charts_1")
    else:
        raise SystemExit(
            f"Unknown mode '{sys.argv[1]}'. "
            "Use one of: all, latest, python_review, deep_news, antv."
        )


if __name__ == "__main__":
    _main()

