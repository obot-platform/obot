"""CLI: run evals from env. Usage: from eval_py: python -m eval.run"""
import sys
from eval.core.framework import run_from_env, pass_count
from eval.core.cases import all_cases

def main():
    results = run_from_env(all_cases())
    if results is None:
        print("OBOT_EVAL_BASE_URL not set; skipping evals")
        return 1
    passed = pass_count(results)
    for r in results:
        print("[%s] pass=%s duration=%.2fms msg=%s" % (r.name, r.pass_, r.duration_ms, r.message))
    if passed < len(results):
        print("evals: %d/%d passed" % (passed, len(results)))
        return 1
    return 0

if __name__ == "__main__":
    sys.exit(main())
