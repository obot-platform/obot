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
class Result:
    name: str = ""
    pass_: bool = False
    duration_ms: float = 0.0
    message: str = ""
    trajectory: list[str] = field(default_factory=list)
    prompt_responses: list[PromptResponse] = field(default_factory=list)

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
    log_path = os.environ.get("OBOT_EVAL_API_LOG")
    if log_path:
        api_log.init_api_log(log_path)
        try:
            return _run_cases(cases, base_url, auth_header)
        finally:
            api_log.close_api_log()
    return _run_cases(cases, base_url, auth_header)


def _run_cases(cases: list[Case], base_url: str, auth_header: str) -> list[Result]:
    from ..clients.client import Client
    client = Client(base_url, auth_header)
    results = []
    for c in cases:
        ctx = Context(base_url, client)
        start = time.perf_counter()
        result = c.run(ctx)
        result.name = c.name
        result.duration_ms = (time.perf_counter() - start) * 1000
        result.trajectory = list(ctx.trajectory)
        results.append(result)
    return results


def run_from_env(cases: list[Case]) -> Optional[list[Result]]:
    base_url = os.environ.get("OBOT_EVAL_BASE_URL", "")
    auth_header = os.environ.get("OBOT_EVAL_AUTH_HEADER", "")
    if not base_url:
        return None
    return run_all(cases, base_url, auth_header)


def write_results_json(results: list[Result], path: str) -> None:
    with open(path, "w") as f:
        json.dump([r.to_dict() for r in results], f, indent=2)


def pass_count(results: list[Result]) -> int:
    return sum(1 for r in results if r.pass_)
