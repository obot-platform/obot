"""Write a compact eval run summary (JSON + text) after `run_all`."""
from __future__ import annotations

import json
import os
import time
from typing import TYPE_CHECKING

from ..helper import paths

if TYPE_CHECKING:
    from .framework import Result


def _mean(values: list[float]) -> float | None:
    if not values:
        return None
    return sum(values) / len(values)


def build_run_summary_payload(results: list[Result]) -> dict:
    """Structured summary for JSON export and pretty-print."""
    total = len(results)
    passed_n = sum(1 for r in results if r.pass_)
    all_scores: list[float] = []
    cases_out: list[dict] = []
    for r in results:
        turns_out = []
        for t in r.turn_eval_details:
            if t.score is not None:
                all_scores.append(t.score)
            turns_out.append(
                {
                    "turn": t.turn_index,
                    "pass": t.passed,
                    "score": t.score,
                    "threshold": t.threshold,
                    "reason": t.reason,
                    "prompt": t.prompt,
                }
            )
        cases_out.append(
            {
                "name": r.name,
                "pass": r.pass_,
                "duration_ms": round(r.duration_ms, 2),
                "message": r.message,
                "case_prompt": r.case_prompt or "",
                "turns": turns_out,
            }
        )
    mean_score = _mean(all_scores)
    overall = passed_n > 0 if total else False
    return {
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "cases_total": total,
        "cases_passed": passed_n,
        "cases_failed": total - passed_n,
        "case_pass_rate": (passed_n / total) if total else 0.0,
        "overall_pass": overall,
        "turns_scored": len(all_scores),
        "mean_turn_score": mean_score,
        "cases": cases_out,
    }


def _md_fenced_snippet(text: str, max_len: int = 800) -> str:
    """Short fenced block for GitHub job markdown (escapes closing fences)."""
    body = (text or "").strip()
    if len(body) > max_len:
        body = (
            body[:max_len]
            + "\n\n_(Truncated for job summary; full prompt and fields are in the artifact JSON.)_"
        )
    body = body.replace("```", "`\u200b``")
    return "```text\n%s\n```\n" % body


def append_github_job_summary(payload: dict) -> None:
    """
    If GITHUB_STEP_SUMMARY is set (GitHub Actions), append a short markdown report.
    Full prompts and reasons remain in eval_run_summary.json (upload as artifact).
    """
    path = (os.environ.get("GITHUB_STEP_SUMMARY") or "").strip()
    if not path:
        return
    total = int(payload["cases_total"] or 0)
    passed_n = int(payload["cases_passed"] or 0)
    rate = 100.0 * float(payload["case_pass_rate"] or 0.0)
    overall = bool(payload.get("overall_pass"))
    lines = [
        "## Nanobot Python evals",
        "",
        "**Overall: %s** — %d / %d cases passed (%.0f%% case pass rate)."
        % ("PASS" if overall else "FAIL", passed_n, total, rate),
        "",
        "_Job fails only when every case fails; at least one passing case yields a green workflow._",
        "",
    ]
    ms = payload.get("mean_turn_score")
    if ms is not None:
        lines.append(
            "Mean DeepEval turn score: **%.3f** (%d scored turns)."
            % (float(ms), int(payload.get("turns_scored") or 0))
        )
        lines.append("")
    lines.append(
        "For full prompts, per-turn scores, and reasons, open the **eval-run-summary** "
        "workflow artifact (`eval_run_summary.json`)."
    )
    lines.append("")
    for c in payload["cases"]:
        status = "**PASS**" if c["pass"] else "**FAIL**"
        lines.append("### %s - %s" % (c["name"], status))
        lines.append("")
        if c.get("case_prompt"):
            lines.append("**Prompt**")
            lines.append("")
            lines.append(_md_fenced_snippet(c["case_prompt"], max_len=600))
        for t in c["turns"]:
            sc = t["score"]
            sc_s = ("%.3f" % sc) if sc is not None else "n/a"
            lines.append(
                "- Turn **%d**: pass=%s, score=%s (threshold=%s)"
                % (t["turn"], t["pass"], sc_s, t["threshold"])
            )
            if t.get("prompt"):
                lines.append("")
                lines.append(_md_fenced_snippet(t["prompt"], max_len=600))
        lines.append("")
    with open(path, "a", encoding="utf-8") as fh:
        fh.write("\n".join(lines))


def write_run_summary(results: list[Result]) -> tuple[str, str]:
    """
    Write eval/data/eval_run_summary.json and eval/data/eval_run_summary.txt.
    Returns (json_path, txt_path).
    """
    payload = build_run_summary_payload(results)
    base = paths.data_path("")
    json_path = paths.data_path("eval_run_summary.json")
    txt_path = paths.data_path("eval_run_summary.txt")

    os.makedirs(base, exist_ok=True)
    append_github_job_summary(payload)
    with open(json_path, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2, ensure_ascii=False)

    lines = [
        "=== Eval run summary ===",
        "generated_at: %s" % payload["generated_at"],
        "overall: %s (fail only when all cases fail)"
        % ("PASS" if payload.get("overall_pass") else "FAIL"),
        "cases: %d passed / %d total (%.1f%% case pass rate)"
        % (payload["cases_passed"], payload["cases_total"], 100.0 * float(payload["case_pass_rate"])),
    ]
    if payload["mean_turn_score"] is not None:
        lines.append(
            "DeepEval turns: %d scored, mean score=%.3f"
            % (payload["turns_scored"], float(payload["mean_turn_score"]))
        )
    else:
        lines.append("DeepEval turns: no numeric scores in this run (e.g. non-conversation cases only).")
    lines.append("")
    for c in payload["cases"]:
        lines.append("[%s] pass=%s (%.2f ms)" % (c["name"], c["pass"], c["duration_ms"]))
        if c["message"]:
            lines.append("  message: %s" % c["message"])
        if c.get("case_prompt"):
            lines.append("  case_prompt:")
            for pl in c["case_prompt"].splitlines():
                lines.append("    %s" % pl)
        for t in c["turns"]:
            sc = t["score"]
            sc_s = ("%.3f" % sc) if sc is not None else "n/a"
            lines.append(
                "  turn %d: pass=%s score=%s threshold=%s"
                % (t["turn"], t["pass"], sc_s, t["threshold"])
            )
            if t.get("prompt"):
                lines.append("    prompt:")
                for pl in t["prompt"].splitlines():
                    lines.append("      %s" % pl)
            if t.get("reason"):
                lines.append("    reason: %s" % t["reason"])
        lines.append("")
    with open(txt_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines).rstrip() + "\n")
    return json_path, txt_path
