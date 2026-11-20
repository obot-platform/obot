# Security repository main sync

`obot-security/main` is disposable and must have the same commit history as
`obot/main`. Keep private security work on separate branches and in private PRs,
and do not merge it into `obot-security/main`. The sync replaces any
destination-only commits on `main`; it does not merge or rebase them. Other
branches and tags are neither copied nor deleted. The
[ADR](../adr/2026-11-19-security-repository-main-mirror.md) records why.

The workflow definition lives in upstream `obot/main` so it survives every exact
mirror. Its sync job runs only in `obot-platform/obot-security`, on `main`, daily
at 09:00 UTC (02:00 America/Phoenix) or through `workflow_dispatch`. GitHub's daily
schedule is best effort. Pull requests run the local regression tests in
`workflow-checks.yml`, without sync credentials.

The job checks out only the public sync script and compares the commits that
both repositories report for `main`. If they match, it verifies and stops without
fetching. Otherwise it fetches public `main` into a temporary bare repository; it
never checks out private code. Before pushing, it logs the destination commit it
replaces, and warns when that commit is not in upstream history, because the push
then discards commits that exist only on destination `main`.

An explicit `--force-with-lease=refs/heads/main:<observed-sha>` protects against
concurrent destination updates. The result is checked against the fetched
upstream SHA; if upstream advances afterward, the next run picks it up. Missing
branches, authentication failures, rejected pushes, and failed verification stop
the run. There is no automatic retry with a new lease or unconditional force push.

## Activation prerequisites

These are separate administration and rollout steps, not performed by the
workflow or its tests. Obtain approval before applying them.

1. Merge the workflow, script, tests, and upstream-only workflow guards into
   `obot/main`.
2. Install a dedicated GitHub App on **only** `obot-security`, with repository
   Contents: write and Workflows: write (plus implicit Metadata: read). Workflow
   permission is needed because upstream commits can change workflow files.
   The workflow further restricts each installation token to that repository and
   those permissions, and revokes it at the end of the job.
3. Approve and configure a ruleset bypass for that app. Direct ref replacement
   must pass the inherited PR, status-check, and non-fast-forward restrictions.
   Keep the app's installation limited to this repository; do not disable
   protections for the organization. Review the exact ruleset change separately.
4. In `obot-security`, create the `security-main-sync` environment with a
   deployment branch policy allowing only `main`. Store `SECURITY_SYNC_APP_ID`
   as an environment variable and `SECURITY_SYNC_APP_PRIVATE_KEY` as an
   environment secret. Do not expose the key as a repository-wide secret to
   private PR workflows. The sync job's `GITHUB_TOKEN` has only Contents: read,
   and the failure-report job's has only Issues: write.
5. Enable Issues in `obot-security`. Assign an operational owner, who watches the
   repository with at least **Custom → Issues** so that failure issues reach them.
6. Enable Depot for `obot-security`, including the `depot-ubuntu-22.04` runners
   and trust for the Depot project that `test-docker-build.yml` uses. Private PR
   checks run there; without it they stay queued until GitHub fails them after
   24 hours.
7. With separate approval, bootstrap private `main` to the reviewed upstream
   commit using an explicit lease. This introduces the sync workflow and the
   workflow guards together. Verify the destination SHA, then manually dispatch
   the workflow and inspect its result before relying on the daily schedule.

## Workflow guards

The mirror carries every upstream workflow. Any job that can run outside a pull
request (on a push, tag, schedule, release, issue, or manual dispatch) must have
an `if:` that names an exact repository, normally
`github.repository == 'obot-platform/obot'`, or must need a job that has one.
Publishing, release, and notification jobs are limited to upstream so they cannot
run from the mirror. Push-triggered CI is limited to upstream because the mirror's
`main` only receives commits that upstream already tested. Pull request checks
stay available in the mirror.

`tools/check-workflow-guards.py` enforces this on every pull request that changes
a workflow. It looks for the guard text; it does not evaluate the expressions, so
reviewers still need to check that a guard is not bypassed by an `||`.

Guards name the upstream repository instead of excluding `obot-security`, so they
fail closed: a renamed mirror or a second copy does not publish either. As a
result, forks no longer publish images from pushes, manual dispatches, or
schedules, and do not run push CI. Do not copy public release or notification
credentials into the private repository.

## Validation and failures

Run `python3 tools/check-workflow-guards.py` (requires PyYAML),
`python3 tools/test-sync-main.py`, and `shellcheck tools/sync-main.sh` locally.
The sync tests use temporary local repositories and require no GitHub
credentials. Lint the workflows with `actionlint`.

GitHub emails scheduled-run failures only to the user who last changed the
workflow's cron line. In the mirror, that is not necessarily anyone responsible
for the sync. When the sync job fails, a follow-up job opens an issue titled
"Security main sync failed" in `obot-security`, or comments on the open one. The
workflow sends no Slack or email notification itself. Close the issue after a run
succeeds.

After a lease rejection or verification failure, inspect the intervening update
and rerun manually only when appropriate. Do not bypass the lease. If a run
warns that destination `main` was not in upstream history, create a branch at
the SHA in the warning to keep the discarded commits; GitHub removes unreferenced
commits eventually. A no-op run still verifies that destination `main` matches
upstream.
