"""CLI: run evals from env. Usage: from eval_py: python -m eval.run [case_name]"""
import os
import sys
from eval.core.framework import run_all, run_from_env, pass_count
from eval.core.cases import all_cases

def main():
    case_name = (sys.argv[1] if len(sys.argv) > 1 else "").strip()
    cases = all_cases()
    if case_name:
        cases = [c for c in cases if c.name == case_name]
        if not cases:
            print("Unknown case: %r" % case_name)
            return 1
    base_url = os.environ.get("OBOT_EVAL_BASE_URL", "")
    auth_header = os.environ.get("OBOT_EVAL_AUTH_HEADER", "")
    if not base_url:
        print("OBOT_EVAL_BASE_URL not set; skipping evals")
        return 1
    results = run_all(cases, base_url, auth_header)
    passed = pass_count(results)
    for r in results:
        print("[%s] pass=%s duration=%.2fms msg=%s" % (r.name, r.pass_, r.duration_ms, r.message))
    if passed < len(results):
        print("evals: %d/%d passed" % (passed, len(results)))
        return 1
    return 0

if __name__ == "__main__":
    sys.exit(main())
