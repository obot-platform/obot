"""Pytest tests for eval framework."""
import os
import pytest

# Add parent so "from eval import ..." works
import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

from eval.core.framework import run_all, run_from_env, pass_count
from eval.core.cases import all_cases, run_mock_tool_output, run_version_flag
from eval.core import framework


def test_run_from_env_skips_when_no_base_url(monkeypatch):
    monkeypatch.delenv("OBOT_EVAL_BASE_URL", raising=False)
    results = run_from_env(all_cases())
    assert results is None


def test_mock_tool_output():
    cases = [c for c in all_cases() if c.name == "nanobot_mock_tool_output"]
    assert len(cases) == 1
    results = run_all(cases, "http://localhost:8080", "")
    assert len(results) == 1
    assert results[0].pass_
    assert "deterministic" in results[0].message


def test_nanobot_workflow_requires_base_url():
    with pytest.raises(ValueError):
        run_all(all_cases(), "", "")


def test_workflow_content_publishing_eval_skip_when_no_capture(monkeypatch):
    monkeypatch.delenv("OBOT_EVAL_CAPTURED_RESPONSE", raising=False)
    monkeypatch.delenv("OBOT_EVAL_CAPTURED_RESPONSE_FILE", raising=False)
    cases = [c for c in all_cases() if c.name == "nanobot_workflow_content_publishing_eval"]
    results = run_all(cases, "http://localhost:8080", "")
    assert len(results) == 1
    assert not results[0].pass_
    assert "no captured response" in results[0].message


def test_all_cases_count():
    cases = all_cases()
    assert len(cases) >= 7
    names = [c.name for c in cases]
    assert "nanobot_version_flag" in names
    assert "nanobot_mock_tool_output" in names
