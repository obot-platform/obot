"""
Evaluation framework for nanobot workflow. API-driven, no UI automation.
"""
from __future__ import annotations

import json
import os
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Callable, Optional

from ..helper import api_log

if TYPE_CHECKING:
    from ..clients.client import Client


@dataclass
class PromptResponse:
    phase: int
    prompt: str
    response: str


@dataclass
class TurnEvalDetail:
    """Per-turn DeepEval outcome (conversation workflow cases)."""

    turn_index: int
    passed: bool
    score: Optional[float] = None
    threshold: Optional[float] = None
    reason: str = ""
    prompt: str = ""

    def format_message(self) -> str:
        parts: list[str] = []
        if self.score is not None:
            parts.append("score=%.3f (threshold=%s)" % (self.score, self.threshold))
        if self.reason:
            parts.append("reason=%s" % self.reason)
        return "; ".join(parts) if parts else "evaluated (turn %d)" % self.turn_index


@dataclass
class Result:
    name: str = ""
    pass_: bool = False
    duration_ms: float = 0.0
    message: str = ""
    trajectory: list[str] = field(default_factory=list)
    prompt_responses: list[PromptResponse] = field(default_factory=list)
    turn_eval_details: list[TurnEvalDetail] = field(default_factory=list)
    # User prompt for single-turn cases without turn_eval_details (e.g. blog post elicitation).
    case_prompt: str = ""

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "pass": self.pass_,
            "duration_ms": self.duration_ms,
            "message": self.message,
            "trajectory": self.trajectory,
            "prompt_responses": [
                {"phase": p.phase, "prompt": p.prompt, "response": p.response}
                for p in self.prompt_responses
            ],
            "turn_eval_details": [
                {
                    "turn_index": t.turn_index,
                    "pass": t.passed,
                    "score": t.score,
                    "threshold": t.threshold,
                    "reason": t.reason,
                    "prompt": t.prompt,
                }
                for t in self.turn_eval_details
            ],
            "case_prompt": self.case_prompt,
        }


@dataclass
class Case:
    name: str
    description: str
    run: Callable[["Context"], Result]


class Context:
    def __init__(self, base_url: str, client: Client):
        self.base_url = base_url.rstrip("/")
        self.client = client
        self.trajectory: list[str] = []

    def append_step(self, fmt: str, *args) -> None:
        self.trajectory.append(fmt % args if args else fmt)


def run_all(cases: list[Case], base_url: str, auth_header: str) -> list[Result]:
    if not base_url:
        raise ValueError("OBOT_EVAL_BASE_URL is required")
    base_url = base_url.rstrip("/")
    log_path = (os.environ.get("OBOT_EVAL_API_LOG") or "").strip()
    if log_path:
        api_log.init_api_log(log_path)
        try:
            return _run_cases(cases, base_url, auth_header)
        finally:
            api_log.close_api_log()
    return _run_cases(cases, base_url, auth_header)


def _run_cases(cases: list[Case], base_url: str, auth_header: str) -> list[Result]:
    from ..clients.client import Client
    from ..helper.nanobot_setup import ensure_project_and_agent

    client = Client(base_url, auth_header)
    setup_err = ensure_project_and_agent(client)
    results = []
    for c in cases:
        ctx = Context(base_url, client)
        start = time.perf_counter()
        if setup_err:
            result = Result(pass_=False, message=setup_err)
        else:
            result = c.run(ctx)
        result.name = c.name
        result.duration_ms = (time.perf_counter() - start) * 1000
        result.trajectory = list(ctx.trajectory)
        results.append(result)
    return results


def pass_count(results: list[Result]) -> int:
    return sum(1 for r in results if r.pass_)
