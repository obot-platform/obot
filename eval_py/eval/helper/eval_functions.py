"""Helpers for step eval: response success, elicitation, phase evaluation."""
import re
from dataclasses import dataclass
from typing import Optional


def response_success(status: int, response_text: str, err: Optional[Exception]) -> tuple[bool, str]:
    if err is not None:
        return False, str(err)
    if status != 200:
        return False, "status %d" % status
    return True, ""


@dataclass
class ElicitResult:
    needs_reply: bool
    reply: str


def needs_reply(response_text: str) -> ElicitResult:
    lower = (response_text or "").strip().lower()
    if not lower:
        return ElicitResult(False, "")
    if "what is your" in lower or "please provide" in lower or "do you want to" in lower or lower.endswith("?"):
        return ElicitResult(True, "Use defaults and continue.")
    return ElicitResult(False, "")


@dataclass
class PhaseEvalResult:
    pass_: bool
    message: str
    tool_calls_made: Optional[int] = None


def evaluate_phase_research(response_text: str) -> tuple[bool, str]:
    text = (response_text or "").strip()
    if not text:
        return False, "empty response"
    lower = text.lower()
    if "search" in lower or "source" in lower or re.search(r"\d+\s*sources?", text):
        return True, "research content present"
    return False, "no research/sources indication"


def evaluate_phase_content(response_text: str) -> tuple[bool, str]:
    text = (response_text or "").strip()
    if not text:
        return False, "empty response"
    return len(text) > 50, "content present" if len(text) > 50 else "insufficient content"


def evaluate_phase_publish(response_text: str) -> tuple[bool, str]:
    text = (response_text or "").strip()
    if not text:
        return False, "empty response"
    lower = text.lower()
    if "wordpress" in lower or "publish" in lower or re.search(r"https?://", text):
        return True, "publish/URL content present"
    return len(text) > 30, "minimal content"


def evaluate_phase_final(response_text: str) -> PhaseEvalResult:
    text = (response_text or "").strip()
    if not text:
        return PhaseEvalResult(False, "empty response", None)
    has_url = bool(re.search(r"https?://\S+", text))
    has_num = bool(re.search(r"\d+", text))
    pass_ = has_url and (has_num or len(text) > 20)
    msg = "has URL and content" if pass_ else "no URL or metrics"
    tool_calls = None
    m = re.search(r"(\d+)\s*(?:tool\s*calls?|calls?)", text, re.I)
    if m:
        tool_calls = int(m.group(1))
    return PhaseEvalResult(pass_, msg, tool_calls)


def task_outcome(final_text: str) -> tuple[str, str]:
    text = (final_text or "").strip().lower()
    if not text:
        return "unknown", "empty"
    if "error" in text or "failed" in text or "could not" in text:
        return "failed", "response indicates failure"
    if re.search(r"https?://", final_text) and ("publish" in text or "url" in text):
        return "success", "published or URL present"
    return "unknown", "inconclusive"


@dataclass
class TrajectorySummary:
    tool_call_count: int
    api_call_count: int
    has_assistant_text: bool
    has_final_output: bool


def validate_trajectory(s: TrajectorySummary) -> tuple[bool, str]:
    if not s.has_assistant_text:
        return False, "no assistant text"
    return True, "trajectory ok"


def validate_tool_and_api_counts(
    actual_tool_calls: int,
    actual_api_calls: int,
    stated_tool_calls: Optional[int],
    stated_api_calls: Optional[int] = None,
) -> tuple[bool, str]:
    if stated_tool_calls is not None and actual_tool_calls < stated_tool_calls:
        return False, "tool calls mismatch"
    return True, "counts ok"
