---
name: docs-release
description: Cut the versioned docs snapshot for an obot minor release (vX.Y.0) - snapshot docs/ as the new version, drop the oldest retained version, fix redirects for removed pages, bump the release-export test fixture, verify the build, and open the PR from the user's fork. Use when the user asks to do the docs release, cut docs for a release, or version the docs.
---

# Docs Release

Snapshots the current docs (`docs/docs/`) as the new release version so it is served at the docs.obot.ai root, and drops the oldest retained version. Previous examples: PR #8335 (v0.27) and the `docs-release-v0.28` branch (v0.28).

Background reading if anything below looks out of date: `docs/README.md` ("Release versions" and "Agent-readable documentation") and `docs/CLAUDE.md` ("Renaming or removing a page", "Cutting and dropping a version").

## Inputs

- **Version** (e.g. `v0.29.0`): ask if not given. Always minor releases (`vX.Y.0`); patch releases do not get docs snapshots.
- **Version to drop**: the last entry in `docs/versions.json`. The convention is the latest release plus the three before it, so after adding one there should be four entries.

## Procedure

Run make commands from the repo root. Docker must be running (`gen-docs-release` uses a `node` container) and `jq` must be installed (`remove-docs-version`).

### 1. Branch from upstream main

```bash
git fetch upstream main
git checkout -b docs-release-vX.Y upstream/main
```

### 2. Snapshot and drop

```bash
make gen-docs-release version=vX.Y.0
make remove-docs-version version=<OLDEST>
cat docs/versions.json   # expect four entries, new version first
```

Do not edit `docusaurus.config.ts` for the version change itself; it reads `versions.json`.

### 3. Find pages removed since the previous release

```bash
cd docs
diff <(cd versioned_docs/version-<PREV> && find . -name '*.md*' | sort) \
     <(cd versioned_docs/version-vX.Y.0 && find . -name '*.md*' | sort) | grep '^[<>]'
grep -rl '^draft: true' versioned_docs/version-vX.Y.0
```

For each page that exists in the previous release but not the new one (`<` lines), or is now a draft, follow `docs/CLAUDE.md` "Renaming or removing a page":
- add a redirect in `docusaurus.config.ts` (`@docusaurus/plugin-client-redirects`) so the root URL doesn't 404 once the new version is served there, and
- add a `PATH_REDIRECTS` entry in `docs/plugins/utils.ts`.

Also check existing redirects whose targets no longer exist in the new version; the build fails on them.

### 4. Agent-docs housekeeping

These come from the agent-readable docs exporter (#8369):

- **`docs/plugins/agent-docs/summaries.json`**: hand-written `llms.txt` summary overrides keyed by version. Delete the key for the dropped version if present.
- **`docs/tests/scripts/release-exports.mjs`**: an isolated rollover test that cuts a *fake* future release. Bump `const next = "vX.Y.0"` and the `/Use vX\.Y\.0/` regex to the release after this one, and update the matching version in `docs/README.md` ("isolated vX.Y.0 snapshot", "once vX.Y.0 is released"). Check for other assertions tied to a specific release (for example the v0.27.0 per-provider-routes check) and keep them pinned to a version that is still retained, or drop them when that version is removed.
- `docs/tests/agent-docs/exports.test.mjs` uses synthetic versions; leave it alone.

### 5. Verify

```bash
cd docs
npm run verify               # same checks CI runs, plus typecheck
npm run test:exports:release # not in CI; run it manually
```

Both must pass. `verify` should finish with no `[canonical-urls] No valid canonical target` warnings.

If the release notes link to docs pages, confirm each exists at the build root, e.g. `test -f build/<path>/index.html`, and that heading anchors exist (`grep 'id="<anchor>"'`).

### 6. Show the user, then commit and open the PR

Show the user the change summary (`git status --short | grep -v versioned_docs/` plus the counts of added/removed snapshot files) and wait for approval before committing or pushing.

Then:

```bash
git add -A docs
git commit -m "chore: release vX.Y docs"
git push origin docs-release-vX.Y     # origin is the user's fork
gh pr create --repo obot-platform/obot --base main --head cjellick:docs-release-vX.Y \
  --title "chore: release vX.Y docs" --body-file <file>
```

Never push to `obot-platform/obot`. The PR body should say which version was cut and dropped, list any redirects or test fixture changes and why, and state the verification run. Do not use closing keywords next to issue references.

The new version goes live at the root once the PR merges and the docs site deploys, so coordinate merge timing with publishing the GitHub release.
