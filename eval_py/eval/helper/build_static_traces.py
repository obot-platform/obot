from __future__ import annotations

"""
Build static JSON test-case files from raw event-stream logs.

This reads the SSE logs in eval/data/*.txt (news, antv_charts, python_review),
parses them into minimal AgentTrace-like dicts, and writes compact JSON fixtures
that agent_deepeval can load later.

We intentionally inline a lightweight SSE parser here so this script does not
depend on the deepeval library.
"""

import json
import os
import re
from dataclasses import dataclass
from typing import List

from . import paths, event_stream_data


@dataclass
class Trace:
    user_prompt: str
    final_report: str
    tool_calls: List[str]
    search_queries: List[str]


def _parse_sse_to_trace(raw_sse: str) -> Trace:
    """Minimal copy of agent_deepeval._parse_sse_to_trace (no deepeval deps)."""
    current_event: str | None = None
    current_data_lines: List[str] = []

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

        role = ev.get("role")
        items = ev.get("items") or []
        if isinstance(items, list):
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

        if current_event == "notifications/message":
            data_field = ev.get("data") or {}
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
    final_report = "".join(assistant_text_parts).strip()

    seen = set()
    ordered_tools: List[str] = []
    for name in tool_calls:
        if name not in seen:
            seen.add(name)
            ordered_tools.append(name)

    return Trace(
        user_prompt=user_prompt,
        final_report=final_report,
        tool_calls=ordered_tools,
        search_queries=search_queries,
    )


def _build_case(case_name: str, source_txt: str) -> None:
    """Parse one raw SSE txt file into a JSON trace fixture."""
    src_path = paths.data_path(source_txt)
    if not os.path.exists(src_path):
        raise FileNotFoundError(src_path)

    with open(src_path, "r", encoding="utf-8") as f:
        raw_sse = f.read()

    trace = _parse_sse_to_trace(raw_sse)

    out_obj = {
        "user_prompt": trace.user_prompt,
        "final_report": trace.final_report,
        "tool_calls": trace.tool_calls,
        "search_queries": trace.search_queries,
    }

    out_path = paths.data_path(f"{case_name}.json")
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(out_obj, f, indent=2, ensure_ascii=False)

    print(f"[build_static_traces] Wrote {out_path}")


def main() -> None:
    # Map logical case names to their raw SSE txt sources
    cases = [
        ("deep_news_briefing_1", "news.txt"),
        ("antv_charts_1", "antv_charts.txt"),
        ("python_review_1", "python_review.txt"),
    ]
    for case_name, source in cases:
        _build_case(case_name, source)


if __name__ == "__main__":
    main()


