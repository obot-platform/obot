"""Evaluate content publishing workflow response (captured text)."""
import os
import re
from dataclasses import dataclass
from typing import Optional


@dataclass
class WorkflowEvalResult:
    pass_: bool
    has_url: bool
    has_title: bool
    sources_used: Optional[int]
    tool_calls_made: Optional[int]
    message: str
    published_url: str
    title: str


def evaluate_content_publishing_response(response_text: str) -> WorkflowEvalResult:
    text = (response_text or "").strip()
    if not text:
        return WorkflowEvalResult(False, False, False, None, None, "empty response", "", "")
    has_url = bool(re.search(r"https?://[^\s\)\]\"']+", text))
    published_url = ""
    if has_url:
        m = re.search(r"https?://[^\s\)\]\"']+", text)
        if m:
            published_url = m.group(0)
    title = ""
    title_m = re.search(r"(?i)(?:title\s*[:\-]\s*)(.+?)(?:\n|$)", text)
    has_title = False
    if title_m:
        has_title = True
        title = title_m.group(1).strip()
    else:
        for line in text.split("\n"):
            line = line.strip()
            if line and not line.startswith("http") and 5 <= len(line) <= 100:
                has_title = True
                title = line
                break
    sources_used = None
    m = re.search(r"(?i)(?:sources?\s+used|number\s+of\s+sources?)\s*[:\-]?\s*(\d+)", text)
    if m:
        sources_used = int(m.group(1))
    if sources_used is None:
        m2 = re.search(r"(\d+)\s*(?:sources?|sources\s+used)", text, re.I)
        if m2:
            sources_used = int(m2.group(1))
    tool_calls_made = None
    m = re.search(r"(?i)(?:tool\s+calls?\s+made|number\s+of\s+tool\s+calls?)\s*[:\-]?\s*(\d+)", text)
    if m:
        tool_calls_made = int(m.group(1))
    if tool_calls_made is None:
        m2 = re.search(r"(\d+)\s*(?:tool\s+calls?|tool\s+calls?\s+made)", text, re.I)
        if m2:
            tool_calls_made = int(m2.group(1))
    pass_ = has_url and (sources_used is not None or tool_calls_made is not None)
    if not has_url:
        message = "response missing published post URL"
    elif sources_used is None and tool_calls_made is None:
        message = "response missing number of sources used and/or tool calls made"
    else:
        message = "has URL"
        if has_title:
            message += ", title"
        if sources_used is not None:
            message += ", sources=%d" % sources_used
        if tool_calls_made is not None:
            message += ", tool_calls=%d" % tool_calls_made
    return WorkflowEvalResult(pass_, has_url, has_title, sources_used, tool_calls_made, message, published_url, title)


def read_captured_response() -> tuple[str, bool]:
    s = os.environ.get("OBOT_EVAL_CAPTURED_RESPONSE", "").strip()
    if s:
        return s, True
    path = os.environ.get("OBOT_EVAL_CAPTURED_RESPONSE_FILE", "")
    if not path:
        return "", False
    try:
        with open(path, encoding="utf-8") as f:
            return f.read().strip(), True
    except OSError:
        return "", False
