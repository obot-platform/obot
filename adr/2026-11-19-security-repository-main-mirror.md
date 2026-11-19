# 2026-11-19: Mirror upstream main into the private security repository

- **Status:** Accepted
- **Date:** 2026-11-19
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/ops-and-it-issues#85](https://github.com/obot-platform/ops-and-it-issues/issues/85)

## Related ODPs

None.

## Context

Embargoed security fixes are developed in the private `obot-platform/obot-security`
repository, on branches that start from its `main`. That `main` has to match
public `obot/main`, or private fixes are built and reviewed against a different
codebase from the one they will ship into. Because the private repository is a
copy of the public one, it also carries every upstream workflow, including the
ones that publish images, cut releases, and post to Slack.

## Decision

- `obot-security/main` is a disposable, exact mirror of `obot/main`. A daily
  scheduled workflow replaces it with upstream `main` under a lease on the commit
  it observed. It never merges or rebases, and it does not touch other branches
  or tags, where all private work lives.
- The workflow and script live upstream so that every mirror carries them
  forward. The sync job runs only in `obot-security` on `main`, using a GitHub
  App token limited to that repository. A failed sync opens or updates an issue
  there.
- Every upstream job that can run outside a pull request has an `if:` that names
  an exact repository (normally `github.repository == 'obot-platform/obot'`) or
  needs a job that does. `tools/check-workflow-guards.py` enforces this on pull
  requests that change workflows.

## Rationale

- An exact mirror keeps private branches on the same commits as upstream.
  Merging upstream into a private `main` would let the histories drift and let
  private commits reach `main`.
- Keeping the definition upstream avoids maintaining a separate repository for
  one job, at the cost of running upstream code with the sync token (see
  Consequences).
- Naming the upstream repository fails closed: a fork or a renamed mirror does
  not publish. A guard that excluded only `obot-security` would stop protecting
  the mirror if it were renamed or a second one were created.
- GitHub emails scheduled-run failures only to whoever last changed the cron
  line, so the workflow records failures as issues that the mirror's watchers
  see.

## Consequences

- Anything merged into `obot-security/main` is discarded at the next sync. The
  job logs the replaced commit and warns when it was not upstream, but it does
  not stop.
- New upstream workflows that publish, release, notify, or run on pushes must
  include a repository guard, and CI rejects them otherwise. Forks no longer
  publish images or run push CI from their own `main`.
- The sync runs the script from public `main` at run time, with a token that can
  read every private branch. Merge rights on public `main` therefore control
  access to embargoed work. The workflow comes from the mirror's `main` and the
  script from public `main`, so an incompatible script change can make every run
  fail until someone updates the mirror by hand.
- Pull request checks in the mirror run on Depot runners, so `obot-security`
  needs Depot access.

## References

- [Operational runbook](../.github/SECURITY_SYNC.md)
- [Sync workflow](../.github/workflows/sync-security.yml)
- [Sync script](../tools/sync-main.sh)
- [Workflow guard check](../tools/check-workflow-guards.py)
