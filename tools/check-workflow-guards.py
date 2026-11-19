#!/usr/bin/env python3
"""Require workflow jobs that can run outside pull requests to name one repository.

obot-security mirrors obot main, so it carries every upstream workflow. A job
started by a push, tag, schedule, release, issue, or manual dispatch must not
publish, notify, or spend runner time from that mirror. A job passes when its
`if:` compares github.repository to an exact repository, or when it needs a job
that passes and does not run after skipped dependencies through always() or
cancelled(). This checks a convention; it does not evaluate the expressions.
"""

from pathlib import Path
import re
import sys

import yaml


ROOT = Path(__file__).resolve().parent.parent
WORKFLOWS = ROOT / ".github" / "workflows"
# Pull requests in the mirror are intentional, and reusable workflows are
# checked through the jobs that call them.
UNGUARDED_EVENTS = {"pull_request", "workflow_call"}
REPOSITORY_PIN = re.compile(r"github\.repository\s*==\s*'[^']+'")
RUNS_AFTER_SKIPPED_NEEDS = re.compile(r"\b(?:always|cancelled)\s*\(")


def events(workflow):
    # YAML 1.1 reads the bare key `on` as true.
    on = workflow.get("on", workflow.get(True))
    if isinstance(on, str):
        return {on}
    if isinstance(on, (list, dict)):
        return set(on)
    raise ValueError("missing or invalid `on`")


def unguarded_jobs(jobs):
    guarded = {}

    def is_guarded(name, path=()):
        if name not in guarded:
            job = jobs.get(name) or {}
            condition = str(job.get("if", ""))
            needs = job.get("needs", [])
            if isinstance(needs, str):
                needs = [needs]
            guarded[name] = bool(REPOSITORY_PIN.search(condition)) or (
                not RUNS_AFTER_SKIPPED_NEEDS.search(condition)
                and any(n in jobs and n not in path and is_guarded(n, path + (name,)) for n in needs)
            )
        return guarded[name]

    return [name for name in jobs if not is_guarded(name)]


def main():
    problems = []
    checked = 0
    files = sorted([*WORKFLOWS.glob("*.yml"), *WORKFLOWS.glob("*.yaml")])
    for path in files:
        name = path.relative_to(ROOT)
        try:
            workflow = yaml.safe_load(path.read_text())
            triggers = events(workflow) - UNGUARDED_EVENTS
        except (yaml.YAMLError, ValueError, AttributeError) as error:
            problems.append(f"{name}: cannot read triggers: {error}")
            continue
        if not triggers:
            continue
        jobs = workflow.get("jobs") or {}
        checked += len(jobs)
        problems.extend(
            f"{name}: job '{job}' can run on {', '.join(sorted(triggers))} without "
            "`if: github.repository == 'obot-platform/obot'` (or a needed job that has it)"
            for job in unguarded_jobs(jobs)
        )

    if problems:
        print("\n".join(problems), file=sys.stderr)
        print(
            "\nobot-security mirrors main and carries these workflows. "
            "See .github/SECURITY_SYNC.md.",
            file=sys.stderr,
        )
        return 1
    print(f"Checked {len(files)} workflows: all {checked} jobs that can run outside pull requests name a repository.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
