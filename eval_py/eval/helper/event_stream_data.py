"""Persist event stream API response to eval/data/data.json and one combined output file."""
import json
import os
import time

from . import paths


def save_event_stream_response(
    case_name: str,
    session_id: str,
    response_text: str,
    raw_sse: str = "",
) -> None:
    """
    Append an event stream API response to eval/data/data.json.
    raw_sse: full SSE body in same format as curl (event:\\n data:\\n\\n).
    """
    data_file = paths.data_path("data.json")
    try:
        with open(data_file, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        data = {}

    if "event_stream_responses" not in data:
        data["event_stream_responses"] = []

    data["event_stream_responses"].append({
        "case": case_name,
        "session_id": session_id,
        "response_text": response_text,
        "response_length": len(response_text),
        "raw_sse": raw_sse,
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })

    os.makedirs(os.path.dirname(data_file), exist_ok=True)
    with open(data_file, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)


def write_step_eval_output_file(
    case_name: str,
    steps: list[str],
    raw_sse: str,
    session_id: str = "",
) -> str:
    """
    Write one file with eval steps and full event-stream data (same format as curl).
    Returns the path written.
    """
    out_path = paths.data_path("step_eval_output.txt")
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    lines = [
        "=== Step eval: %s ===" % case_name,
        "session_id: %s" % session_id,
        "timestamp: %s" % time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "",
        "--- Steps (trajectory) ---",
    ]
    for i, step in enumerate(steps, 1):
        lines.append("  %d. %s" % (i, step))
    lines.extend(["", "--- Event stream (api/events) raw response ---", ""])
    lines.append(raw_sse if raw_sse else "(no data)")
    with open(out_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    return out_path
