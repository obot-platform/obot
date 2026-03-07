#!/usr/bin/env python3
"""
Copy distinct data from step_eval_output.txt to another file.
- Deduplicate by (created, id): keep only the most recent block per id (by created timestamp).
- Collapse repeated control blocks (e.g. event: chat-in-progress + data: {}) so each distinct
  block is written once.
- Preserve order (same sequence); no duplicate data rows.

Reads line by line, writes only distinct blocks in order.

Usage (from eval_py):
  python copy_distinct_sse_data.py [input_file] [output_file]

Defaults:
  input_file  = eval/data/step_eval_output.txt
  output_file = eval/data/step_eval_output_distinct.txt
"""
import json
import os
import sys

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_INPUT = os.path.join(SCRIPT_DIR, "eval", "data", "step_eval_output.txt")
DEFAULT_OUTPUT = os.path.join(SCRIPT_DIR, "eval", "data", "step_eval_output_distinct.txt")


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


def extract_distinct_sse(input_path: str, output_path: str) -> None:
    """
    Read file line by line, collect SSE blocks, then write:
    - One block per (created, id): for same id keep only the block with latest created.
    - For blocks without id: output each distinct block content only once (first occurrence).
    Order preserved; no duplicate data rows.
    """
    with open(input_path, "r", encoding="utf-8") as f:
        content = f.read()

    marker = "Event stream (api/events) raw response:"
    if marker not in content:
        header = ""
        sse_content = content
    else:
        idx = content.index(marker)
        header_end = idx + len(marker)
        header = content[:header_end] + "\n\n"
        sse_content = content[header_end:].lstrip("\n")

    # Read line by line (we already have content; split for processing)
    sse_lines = sse_content.splitlines()
    blocks = _split_sse_blocks(sse_lines)

    # 1) For blocks with id: keep only latest created per id
    last_block_by_id: dict[str, list[str]] = {}
    last_created_by_id: dict[str, str] = {}
    for eid, created, block_lines in blocks:
        if eid is not None:
            if eid not in last_created_by_id or (created and created > last_created_by_id[eid]):
                last_created_by_id[eid] = created or ""
                last_block_by_id[eid] = block_lines

    # 2) For blocks without id: keep first occurrence of each distinct block content
    def block_signature(lines: list[str]) -> tuple[str, ...]:
        return tuple(lines)

    seen_control_signatures: set[tuple[str, ...]] = set()

    # Build output in same sequence: first occurrence of each id (output its latest block); control blocks once each
    seen_ids: set[str] = set()
    out_lines: list[str] = []
    for eid, _created, block_lines in blocks:
        if eid is None:
            sig = block_signature(block_lines)
            if sig not in seen_control_signatures:
                seen_control_signatures.add(sig)
                out_lines.extend(block_lines)
                out_lines.append("")
        else:
            if eid not in seen_ids:
                seen_ids.add(eid)
                out_lines.extend(last_block_by_id[eid])
                out_lines.append("")

    out_body = "\n".join(out_lines) + ("\n" if out_lines else "")

    out_dir = os.path.dirname(output_path)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write(header)
        f.write(out_body)

    control_count = sum(1 for eid, _, _ in blocks if eid is None)
    control_unique = len(seen_control_signatures)
    id_count = sum(1 for eid, _, _ in blocks if eid is not None)
    id_unique = len(last_block_by_id)
    print("Wrote distinct SSE to: %s" % output_path)
    print("  Control blocks (no id): %d -> %d (distinct)" % (control_count, control_unique))
    print("  Data blocks (with id): %d -> %d (most recent per id)" % (id_count, id_unique))


def main() -> None:
    input_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_INPUT
    output_path = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_OUTPUT
    if not os.path.isfile(input_path):
        print("Input file not found: %s" % input_path, file=sys.stderr)
        sys.exit(1)
    extract_distinct_sse(input_path, output_path)


if __name__ == "__main__":
    main()
