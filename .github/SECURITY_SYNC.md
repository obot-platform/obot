# Security repository main sync

`obot-security/main` is disposable and must have the same commit history as
`obot/main`. Keep private security work on separate branches and in private PRs.
The sync replaces any destination-only commits on `main`; it does not merge or
rebase them. Other branches and tags are neither copied nor deleted.

The workflow definition lives in upstream `obot/main` so it survives every exact
mirror. Its sync job runs only in `obot-platform/obot-security`, on `main`, daily
at 09:00 UTC (02:00 America/Phoenix) or through `workflow_dispatch`. GitHub's daily
schedule is best effort. Pull requests run local regression tests without sync
credentials.

The job checks out the public implementation and fetches public `main` into a
temporary bare repository. It never checks out private code. An explicit
`--force-with-lease=refs/heads/main:<observed-sha>` protects against concurrent
destination updates. The result is checked against the captured upstream SHA;
if upstream advances afterward, the next run picks it up. Missing branches,
authentication failures, rejected pushes, and failed verification stop the run.
There is no automatic retry with a new lease or unconditional force push.

## Activation prerequisites

These are separate administration and rollout steps, not performed by the
workflow or its tests. Obtain approval before applying them.

1. Merge the workflow, script, tests, and upstream-only publishing/notification
   guards into `obot/main`.
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
   private PR workflows. The job's normal `GITHUB_TOKEN` has only Contents: read.
5. With separate approval, bootstrap private `main` to the reviewed upstream
   commit using an explicit lease. This introduces the sync workflow and the
   publishing/notification guards together. Verify the destination SHA, then
   manually dispatch the workflow and inspect its result before relying on the
   daily schedule.

The upstream-only guards leave PR checks available in the private repository.
When adding publishing, release, or notification jobs upstream, retain the same
repository guard so they cannot run from the private mirror. Do not copy public
release or notification credentials into the private repository.

## Validation and failures

Run `python3 tools/test-sync-main.py` and `shellcheck tools/sync-main.sh` locally.
Tests use temporary local repositories and require no GitHub credentials. Lint
the workflow with `actionlint .github/workflows/sync-security.yml`.

Failed runs remain visible in the private repository's Actions UI; GitHub Actions
notifications depend on each user's notification settings. No issue, Slack, or
email notification is sent by this workflow. Assign an operational owner and
confirm their notification settings during activation.

After a lease rejection or verification failure, inspect the intervening update
and rerun manually only when appropriate. Do not bypass the lease. A successful
no-op run still verifies that destination `main` matches the fetched source.
