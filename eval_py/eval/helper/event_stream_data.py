"""Persist event stream API response to eval/data/data.json and one combined output file."""
import json
import os
import time

from . import paths


def _split_sse_blocks(lines: list[str]) -> list[tuple[str | None, str | None, list[str]]]:
    """
    Split SSE lines into blocks (event + data lines, split on blank).
    Returns list of (eid, created, block_lines). eid/created from data JSON if present.
    """
    blocks: list[tuple[str | None, str | None, list[str]]] = []
    current: list[str] = []
    current_data_payloads: list[str] = []

    for line in lines:
        s = line.strip()
        if s == "":
            if current:
                created, eid = None, None
                data_joined = "\n".join(current_data_payloads).strip()
                if data_joined and data_joined != "{}":
                    try:
                        obj = json.loads(data_joined)
                        created = obj.get("created")
                        eid = obj.get("id")
                    except json.JSONDecodeError:
                        pass
                blocks.append((str(eid) if eid is not None else None,
                               str(created) if created is not None else None,
                               list(current)))
                current = []
                current_data_payloads = []
            continue
        current.append(line)
        if s.startswith("data:"):
            current_data_payloads.append(s[5:].strip())

    if current:
        data_joined = "\n".join(current_data_payloads).strip()
        created, eid = None, None
        if data_joined and data_joined != "{}":
            try:
                obj = json.loads(data_joined)
                created = obj.get("created")
                eid = obj.get("id")
            except json.JSONDecodeError:
                pass
        blocks.append((str(eid) if eid is not None else None,
                       str(created) if created is not None else None,
                       list(current)))

    return blocks


def make_distinct_sse(raw_sse: str) -> str:
    """
    Deduplicate SSE: one block per message id (latest created); control blocks (no id) once per
    distinct content. Preserves order. Returns distinct raw_sse string.
    """
    if not raw_sse or not raw_sse.strip():
        return raw_sse
    lines = raw_sse.splitlines()
    blocks = _split_sse_blocks(lines)

    last_block_by_id: dict[str, list[str]] = {}
    last_created_by_id: dict[str, str] = {}
    for eid, created, block_lines in blocks:
        if eid is not None:
            if eid not in last_created_by_id or (created and created > last_created_by_id[eid]):
                last_created_by_id[eid] = created or ""
                last_block_by_id[eid] = block_lines

    seen_control_signatures: set[tuple[str, ...]] = set()
    seen_ids: set[str] = set()
    out_lines: list[str] = []
    for eid, _created, block_lines in blocks:
        if eid is None:
            sig = tuple(block_lines)
            if sig not in seen_control_signatures:
                seen_control_signatures.add(sig)
                out_lines.extend(block_lines)
                out_lines.append("")
        else:
            if eid not in seen_ids:
                seen_ids.add(eid)
                out_lines.extend(last_block_by_id[eid])
                out_lines.append("")

    return "\n".join(out_lines) + ("\n" if out_lines else "")


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


def save_event_stream_response_phase(
    case_name: str,
    session_id: str,
    phase: int,
    response_text: str,
    raw_sse: str = "",
    tools_used: list[str] | None = None,
) -> None:
    """Append one phase's event stream response to data.json (includes phase index and tools_used)."""
    data_file = paths.data_path("data.json")
    try:
        with open(data_file, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        data = {}

    if "event_stream_responses" not in data:
        data["event_stream_responses"] = []

    entry = {
        "case": case_name,
        "session_id": session_id,
        "phase": phase,
        "response_text": response_text,
        "response_length": len(response_text),
        "raw_sse": raw_sse,
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    if tools_used is not None:
        entry["tools_used"] = tools_used
    data["event_stream_responses"].append(entry)

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
    # Auto-write distinct file so copy_distinct_sse_data.py is not needed
    distinct_path = paths.data_path("step_eval_output_distinct.txt")
    distinct_sse = make_distinct_sse(raw_sse) if raw_sse else ""
    with open(distinct_path, "w", encoding="utf-8") as f:
        body = distinct_sse if distinct_sse else "(no data)"
        f.write("\n".join(lines[:-1]) + body)
    return out_path


def write_step_eval_output_file_multi_phase(
    case_name: str,
    steps: list[str],
    raw_sse_per_phase: list[str],
    session_id: str = "",
    tools_per_phase: list[list[str]] | None = None,
) -> str:
    """
    Write one file with eval steps, tools used per phase, and full event-stream data.
    raw_sse_per_phase[i] is the raw SSE for phase i.
    tools_per_phase[i] is the list of tool names used in phase i (if provided).
    Returns the path written.
    """
    out_path = paths.data_path("step_eval_output.txt")
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    lines = [
        "=== Step eval: %s (multi-phase) ===" % case_name,
        "session_id: %s" % session_id,
        "phases: %d" % len(raw_sse_per_phase),
        "timestamp: %s" % time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "",
        "--- Steps (trajectory) ---",
    ]
    for i, step in enumerate(steps, 1):
        lines.append("  %d. %s" % (i, step))
    for phase_idx, raw_sse in enumerate(raw_sse_per_phase):
        lines.extend([
            "",
            "--- Phase %d ---" % phase_idx,
        ])
        if tools_per_phase is not None and phase_idx < len(tools_per_phase) and tools_per_phase[phase_idx]:
            lines.append("Tools used: %s" % ", ".join(tools_per_phase[phase_idx]))
            lines.append("")
        lines.extend([
            "Event stream (api/events) raw response:",
            "",
            raw_sse if raw_sse else "(no data)",
        ])
    with open(out_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    # Auto-write distinct file (same header, distinct SSE per phase) so copy_distinct_sse_data.py is not needed
    distinct_path = paths.data_path("step_eval_output_distinct.txt")
    distinct_lines = [
        "=== Step eval: %s (multi-phase) ===" % case_name,
        "session_id: %s" % session_id,
        "phases: %d" % len(raw_sse_per_phase),
        "timestamp: %s" % time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "",
        "--- Steps (trajectory) ---",
    ]
    for i, step in enumerate(steps, 1):
        distinct_lines.append("  %d. %s" % (i, step))
    for phase_idx, raw_sse in enumerate(raw_sse_per_phase):
        distinct_lines.extend(["", "--- Phase %d ---" % phase_idx])
        if tools_per_phase is not None and phase_idx < len(tools_per_phase) and tools_per_phase[phase_idx]:
            distinct_lines.append("Tools used: %s" % ", ".join(tools_per_phase[phase_idx]))
            distinct_lines.append("")
        distinct_sse = make_distinct_sse(raw_sse) if raw_sse else ""
        distinct_lines.extend([
            "Event stream (api/events) raw response:",
            "",
            distinct_sse if distinct_sse else "(no data)",
        ])
    with open(distinct_path, "w", encoding="utf-8") as f:
        f.write("\n".join(distinct_lines))
    return out_path
