"""CLI: run evals from env.

Usage (from eval_py):

    python -m eval.run                 # run all cases
    python -m eval.run <case_name>     # run a single case by name

Convenience group for conversation workflows (three test cases in one command):

    python -m eval.run nanobot_conversation_workflows

This runs:
- nanobot_python_code_review_conversation_eval
- nanobot_deep_news_briefing_conversation_eval
- nanobot_antv_dual_axes_conversation_eval
"""

import os
import sys

from eval.core.framework import pass_count, run_all
from eval.core.cases import all_cases


def main() -> int:
    case_name = (sys.argv[1] if len(sys.argv) > 1 else "").strip()
    cases = all_cases()

    if case_name:
        # Allow a convenience group that runs the three conversation workflows together.
        group_map = {
            "nanobot_conversation_workflows": [
                "nanobot_python_code_review_conversation_eval",
                "nanobot_deep_news_briefing_conversation_eval",
                "nanobot_antv_dual_axes_conversation_eval",
            ],
        }
        target_names = group_map.get(case_name) or [case_name]
        cases = [c for c in cases if c.name in target_names]
        if not cases:
            print("Unknown case or group: %r" % case_name)
            return 1

    base_url = os.environ.get("OBOT_EVAL_BASE_URL", "")
    auth_header = os.environ.get("OBOT_EVAL_AUTH_HEADER", "")
    if not base_url:
        print("OBOT_EVAL_BASE_URL not set; skipping evals")
        return 1

    results = run_all(cases, base_url, auth_header)
    passed = pass_count(results)
    for r in results:
        print(
            "[%s] pass=%s duration=%.2fms msg=%s"
            % (r.name, r.pass_, r.duration_ms, r.message)
        )
    if passed < len(results):
        print("evals: %d/%d passed" % (passed, len(results)))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
