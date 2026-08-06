---
name: draft-release
description: Draft a GitHub release (as a draft, not published) for an upcoming obot minor release by analyzing git history since the previous release. Use when the user asks to draft a release, prepare release notes, or write release notes for an upcoming vX.Y.0 release. Creates a DRAFT GitHub release; never publishes it and never creates the git tag.
---

# Draft Release Notes

Drafts release notes for the next obot minor release and creates a draft (unpublished) GitHub release containing them. The release stays in draft state for the user to review, edit, and publish themselves. Never creates the git tag, never publishes the release, never pushes anything.

## Inputs

- **Target version** (e.g. `v0.22.0`): ask the user if not provided. If recent pre-release tags exist (e.g. `v0.22.0-rc3`), suggest the corresponding minor (`v0.22.0`) and confirm.
- **Diff baseline** (e.g. `v0.21.3`): auto-detect as the highest non-pre-release tag, regardless of whether it is a minor or patch, **that is also an ancestor of `HEAD`**. This is the range used to find changes (`<DIFF_BASELINE>..HEAD`) and the left side of the "Full Changelog" compare link. Use `git tag --sort=-v:refname | grep -v -- '-' | head -1`, then verify with `git merge-base --is-ancestor <candidate> HEAD`. Patch releases are sometimes cut from a separate release branch that never merges back into `main` — if the highest-sorted tag fails the ancestor check, walk down to the next-highest tag and check again until one passes. Do not silently trust version-sort order alone.
- **Style reference** (e.g. `v0.21.0`): auto-detect as the highest `vX.Y.0` tag that is not a pre-release. Use `git tag --list 'v*.*.0' --sort=-v:refname | grep -v -- '-' | head -1`. This release's notes are the tone/structure template. It is often the same as the diff baseline but may differ when patch releases exist between minors.

## Hard constraints

- Do NOT create git tags. Do NOT run `git tag` or `git push --tags`. The draft release does not require the tag to exist locally — GitHub creates it only when the draft is published, which the user does manually.
- `gh release create` is allowed ONLY with `--draft`. Never run it without `--draft`. Never run `gh release edit --draft=false`. Never publish.
- Do NOT model the notes on patch releases (`vX.Y.Z` where Z > 0) or pre-releases (`-rc`, `-alpha`, `-beta`). Those use minimal notes. Always model on the most recent `vX.Y.0`.
- No emojis. No em dashes (`—`). Use plain hyphens or rewrite the sentence. No "we're thrilled / excited to / proud to" beyond the one opening line the template already uses. No marketing fluff.
- Use straight quotes (`'` and `"`), not curly quotes.
- Do not invent PRs, authors, or commit messages in "What's Changed". That section must be sourced from the GitHub API, not synthesized.
- Never run `gh release create` (or any other command that writes to GitHub) until the user has seen the fully assembled draft (opener, Big Updates, Improvements, Upgrade Notes, What's Changed, Full Changelog line — the whole document, not just the feature list from step 5) and explicitly approved it. Step 5's feature-list confirmation is necessary but not sufficient; the user must sign off on the actual prose before anything is pushed to GitHub.

## Procedure

### 1. Confirm version, diff baseline, and style reference

```bash
# Diff baseline: latest non-pre-release tag (minor OR patch):
git tag --sort=-v:refname | grep -v -- '-' | head -1
# Style reference: latest published minor (vX.Y.0, no pre-releases):
git tag --list 'v*.*.0' --sort=-v:refname | grep -v -- '-' | head -1
# Recent tags to suggest the target version:
git tag --sort=-creatordate | head -10
```

Confirm the target version, diff baseline, and style reference with the user before proceeding. The diff baseline and style reference often differ — e.g. baseline `v0.21.3`, style `v0.21.0`. Use the baseline for everything that touches commit range, and the style reference for tone/structure.

### 2. Read the style reference's notes for tone and structure

```bash
gh release view <STYLE_REF> --repo obot-platform/obot
```

Match its structure exactly:
1. One-paragraph opener: `We're excited to announce the <VERSION> release of the Obot MCP Platform. This release <one-sentence theme summary>.`
2. `## Big Updates` with 3-6 `### Feature Name` subsections. Each subsection is 1-3 short paragraphs in plain prose. Link to docs with `[docs](https://docs.obot.ai/...)` when a docs page exists.
3. Optional `## Improvements` section with a short bulleted list of smaller usability/perf items. Include only if there are clearly several smaller user-facing improvements worth calling out. Omit otherwise (v0.21.0 has no Improvements section).
4. `## Upgrade Notes` — usually `There are no major breaking changes in this release.` If the diff contains breaking changes (removed APIs, renamed config, schema migrations, removed env vars), call them out explicitly and concretely. Check commits with `chore: remove`, `BREAKING`, or schema/migration changes.
5. `## What's Changed` — generated via API (see step 7).
6. Trailing `**Full Changelog**: https://github.com/obot-platform/obot/compare/<DIFF_BASELINE>...<VERSION>`.

### 3. Survey the commit range

```bash
git log <DIFF_BASELINE>..HEAD --oneline
git log <DIFF_BASELINE>..HEAD --format='%H%n%s%n%b%n---'  # full bodies if more context needed
```

Identify candidate big features. Heuristics:
- `feat:` commits are top candidates.
- `enhance:` commits that introduce notable user-visible capability also qualify.
- Group related commits into one theme (e.g. several `image pull secrets` commits become one "Image Pull Secrets" feature).
- `fix:`, `chore:`, `docs:`, dependabot, and CI commits are NOT big features.

Aim for 3-6 Big Updates. If the range is small, fewer is fine. If many candidates exist, prioritize features that change what users can do, not internal refactors.

**Deep-read each candidate before drafting its gist.** Commit subjects are too thin to build a release note on. For every candidate Big Update (and every PR that gets merged into one as part of a theme), do all of:

```bash
# Full PR body + comments + reviewers:
gh pr view <NUM> --repo obot-platform/obot --comments

# Files changed (helps confirm scope and spot user-facing surface area):
gh pr view <NUM> --repo obot-platform/obot --json files --jq '.files[].path'
```

**Finding linked issues.** This repo deliberately does NOT use GitHub's `closes #N` / `fixes #N` keywords, so the `closingIssuesReferences` field will almost always be empty. Issues are referenced in PR body or comments as plain text. Grep the PR body and the comment thread (both come back from `gh pr view --comments`) for:
- bare `#<number>` references
- `obot-platform/obot#<number>`
- full URLs like `https://github.com/obot-platform/obot/issues/<number>`
- "issue 1234", "see 1234", or similar prose mentions of a 3-5 digit number near words like "issue", "tracked in", "addresses", "from"

For every issue number found that way, read it:

```bash
gh issue view <ISSUE_NUM> --repo obot-platform/obot --comments
```

Be slightly generous — false positives (numbers that turn out to be PR numbers, version numbers, or unrelated) are cheap to read and discard. Missing the issue that explains the *why* is what you're avoiding. Linked issues usually contain the user problem, the design discussion, and the constraints that the PR body and commit message omit. That context is what makes the difference between a release note that says "added X" and one that explains what X actually unlocks for users.

If a candidate is a theme spanning multiple PRs, deep-read the largest one (or two) — not every single follow-up fix PR. Use judgement: enough reading to write an accurate gist, not exhaustive.

Do not write up a feature whose PR(s) and linked issues you have not actually read in this session.

### 4. Check the milestone for `release-note` issues

The team flags items that need an explicit callout in the release (deprecations, upgrade warnings, behavior changes, required config changes) by labeling GitHub issues with `release-note` and attaching them to the milestone matching the release.

Look them up:

```bash
# Try the version both with and without the leading "v" — milestones are usually "v0.22.0" but check both.
gh issue list --repo obot-platform/obot --milestone "<VERSION>" --label "release-note" --state all \
  --json number,title,body,url,state,labels --limit 50
gh issue list --repo obot-platform/obot --milestone "<VERSION_NO_V>" --label "release-note" --state all \
  --json number,title,body,url,state,labels --limit 50
```

If neither returns results, also check whether the milestone exists at all:

```bash
gh api repos/obot-platform/obot/milestones --jq '.[] | {title, number, state}'
```

If the milestone is missing or has no `release-note` issues, note that to the user and continue without callouts. Do not invent callouts.

For each issue found, draft a concise note:
- One or two sentences max. State the deprecation, warning, or required action plainly.
- Link to the issue at the end: `See [#<NUMBER>](<URL>) for details.`
- Classify each as either:
  - **Upgrade Notes item** (default): goes into the `## Upgrade Notes` section as a bullet or short paragraph. Use for deprecations, removed config, required migrations, behavior changes that affect all users.
  - **Feature-attached callout**: render as a blockquote inside the relevant `### Feature` subsection in Big Updates. Use only when the note is specifically about how to enable, configure, or be aware of one of the features being highlighted. Mirror the blockquote style from v0.19.0's Message Policies entry.

Do not paste the issue body verbatim. Distill it. The issue is for details; the release note is the headline.

### 5. Confirm the feature list and release-note callouts with the user (required, do not skip)

Before writing any prose, present the candidate Big Updates AND the release-note callouts back to the user for review. This catches three failure modes: highlighting the wrong things, mis-describing what a feature actually does, and misclassifying a release-note callout. Format the review like this:

```
Candidate Big Updates for <VERSION> (range <DIFF_BASELINE>..HEAD):

1. <Proposed feature title>
   Gist: <one or two sentences summarizing what this feature is and why it matters>
   Source PRs: #6567, #6605, #6636, ...

2. <Proposed feature title>
   Gist: ...
   Source PRs: ...

Also considered but dropped (say if any should be promoted):
- <feature> — reason dropped (e.g. "internal refactor only", "rolled into #N", "fix not feat")

Release-note callouts found in milestone <VERSION>:

1. [#<NUM>] <issue title>
   Proposed note: <drafted one or two sentences>
   Classified as: Upgrade Notes  (or: Feature-attached callout under "<Feature Title>")

(If none: "No release-note issues found in milestone <VERSION>.")

Questions for you:
a. Are these the right features to highlight? Any to add, drop, or merge?
b. For each kept feature, is the gist accurate? Reply with corrections inline or just "all good".
c. Any feature here that should be downgraded to the Improvements bullet list instead?
d. For each release-note callout: is the wording accurate, and is it classified in the right place (Upgrade Notes vs. attached to a feature)?
```

Use AskUserQuestion when there are concrete forks (e.g. multiple ways to title or scope a feature, or whether a callout belongs in Upgrade Notes vs. on a feature). For open-ended gist corrections, plain text reply is fine. Wait for the user's response and incorporate corrections before proceeding to step 6. Do not draft prose for a feature or callout whose wording the user has not confirmed or corrected.

If the user corrects a gist or callout, mirror their wording closely in the draft. They know the feature better than the commit messages or issue body do.

### 6. Draft Big Updates, Improvements, and Upgrade Notes

For each Big Update:
- `### Title` in title case, matching the style of previous minors (e.g. "Aviatrix Integration for MCP Server Egress Control", "JumpCloud Authentication Provider").
- 1-3 short paragraphs. First paragraph states what the feature is. Second paragraph (optional) explains the impact or how it is used. Third paragraph (optional) links to docs or notes follow-ups.
- Avoid restating the title in the opening sentence verbatim; instead state the capability.
- No bulleted lists inside Big Updates subsections (the previous minors don't use them there).

For Improvements (optional): a bulleted list of 4-10 short items. One line each. Skip the section entirely if there is nothing meaningful to list.

For Upgrade Notes: if step 4 surfaced release-note issues classified as Upgrade Notes items, include them as bullets (or short paragraphs if longer). Each ends with a `See [#<NUMBER>](<URL>) for details.` link. If there were no callouts and no breaking changes detected in the commits, fall back to the default `There are no major breaking changes in this release.` line.

For feature-attached callouts: render the confirmed note as a blockquote (`> ...`) inside the relevant `### Feature` subsection in Big Updates, ending with the issue link.

### 7. Generate "What's Changed" without hallucinating

Use the GitHub-authoritative source — the same endpoint the release UI uses:

```bash
gh api -X POST repos/obot-platform/obot/releases/generate-notes \
  -f tag_name="<VERSION>" \
  -f previous_tag_name="<DIFF_BASELINE>" \
  -f target_commitish="main" \
  --jq .body
```

Paste the `## What's Changed` section and `**Full Changelog**` line from the response verbatim. Do not edit PR titles, authors, or links. If the endpoint also returns headings above What's Changed (it sometimes includes a "## What's Changed" header itself), keep only the list and the Full Changelog line, integrating with the rest of the draft.

If `gh api` fails (auth, network), fall back to:

```bash
gh pr list --repo obot-platform/obot --state merged \
  --search "merged:>=<DIFF_BASELINE_DATE> base:main" \
  --json number,title,author,url --limit 200
```

Format each as `* <title> by @<author> in <url>`. If using this fallback, tell the user the list came from `gh pr list` and recommend they regenerate it from the GitHub UI when they create the release, since the UI's list is the canonical one.

### 8. Show the full assembled draft and wait for approval (required, do not skip)

Write the fully assembled notes (opener, Big Updates, Improvements, Upgrade Notes, What's Changed, Full Changelog line) to `<REPO_ROOT>/release-notes-<VERSION>.md` — in the project working directory itself (not `/tmp`), so the user can open it directly in their IDE for review/editing.

Then show the complete rendered document to the user in the chat too — not just a summary or the feature list from step 5, and not just a pointer to the file. This is a second, distinct checkpoint from step 5: step 5 confirms *which* features and callouts to include, this step confirms the *actual wording* of the finished document, since prose can drift from an approved gist during drafting.

Explicitly ask for approval before doing anything on GitHub, e.g. "Here's the full draft, also written to `release-notes-<VERSION>.md` if you'd rather review it in your IDE — let me know if you want changes, or I'll go ahead and create the GitHub draft release." If the user edits the file directly, re-read it before proceeding rather than relying on your in-memory copy. Do not proceed to step 9 until the user responds with approval (or with edits, which you incorporate and then re-confirm if they're substantial). Do not treat silence, an unrelated reply, or a reply about something else in the conversation as approval.

### 9. Create the draft release on GitHub

Only after the user has approved the full draft in step 8. Reuse the same `<REPO_ROOT>/release-notes-<VERSION>.md` file written in step 8 (re-read it first if the user said they edited it) as the body:

```bash
gh release create <VERSION> \
  --repo obot-platform/obot \
  --draft \
  --title "<VERSION>" \
  --target main \
  --notes-file release-notes-<VERSION>.md
```

Notes on the flags:
- `--draft` is mandatory. Never omit it. The draft does not create the tag — GitHub creates the tag only when the draft is published (and the user does that themselves through the UI).
- `--title "<VERSION>"` matches the project convention (release title equals the tag name, e.g. `v0.22.0`).
- `--target main` so the tag, when eventually published, points at `main`. Adjust only if the user has explicitly told you to release from another branch.
- Do NOT pass `--latest`, `--prerelease`, or `--generate-notes`. We already have the notes; we are not publishing; this is not a pre-release.

If a draft already exists for this version, `gh release create` will fail with "already exists". In that case ask the user whether to delete the existing draft (`gh release delete <VERSION> --repo obot-platform/obot --yes`) and recreate, or leave the existing one alone. Do not delete without asking.

Print the draft URL returned by `gh release create` and a one-line summary of what's in the draft (feature count, callout count). End the turn telling the user the release exists as a draft on GitHub — they should review it there, edit if needed, and publish manually when ready. Remind them publishing is what creates the actual tag.

## Tone reference (from v0.21.0)

> We're excited to announce the v0.21.0 release of the Obot MCP Platform. This release introduces MCP server egress control with an Aviatrix integration and improves the admin dashboard experience.
>
> ## Big Updates
>
> ### Aviatrix Integration for MCP Server Egress Control
>
> Obot now supports MCP server egress control for Kubernetes-hosted MCP servers, with Aviatrix as the first supported network policy provider.
>
> Administrators can configure domain allowlists for individual `npx`, `uvx`, and `containerized` MCP servers. ...

Match that register: declarative, concrete, no hype words, no rhetorical flourishes.
