---
name: rewrite-security-advisory
description: Rewrite an obot GitHub security advisory into Obot's short, user-facing format (Summary / Am I affected? / Details / Impact / Mitigation / Severity / Credits). Use when the user gives a GHSA id or advisory URL and asks to rewrite, simplify, or reformat it. Writes the result to a markdown file in the repo root for the user to paste into the advisory editor.
---

# Rewrite Security Advisory

Take a verbose, reporter-submitted obot security advisory and rewrite it into
Obot's concise, user-facing format. The goal is a short writeup a deployer can
read and immediately know whether they're affected and what to do — not a
reproduction of the researcher's full report.

Output is a markdown file at the repo root named `<GHSA-id>.md`. The user reviews
it and pastes it into the GitHub advisory editor themselves. Do not push it to
GitHub or modify the live advisory unless explicitly asked.

## 1. Fetch the advisory

The advisory is usually a private draft, so the web URL 404s for WebFetch. Pull
it via the authenticated API instead:

```bash
gh api repos/obot-platform/obot/security-advisories/<GHSA-id> --jq '{summary, description, severity, cvss: .cvss.vector_string, score: .cvss.score, vuln_range: .vulnerabilities[0].vulnerable_version_range, patched: .vulnerabilities[0].patched_versions, credits: [.credits_detailed[].user.login], cwes: [.cwes[].cwe_id]}'
```

This gives you the title, the reporter's full description (Summary/Details/PoC/Impact),
CVSS vector + score, affected range, and the credited reporters.

## 2. Read the comments — they matter

The advisory's comment thread is NOT exposed by the GitHub API (REST or GraphQL),
and the web page 404s for WebFetch on drafts. **Ask the user to paste the
comments**, especially the obot team's response (often from the lead eng). The
comments frequently change the framing — e.g. clarifying that the reporter's
headline mechanism was actually a fail-safe and the real fix was something else,
or noting which parts were disputed. Do not finalize the rewrite without them.

## 3. Confirm the patched version

The advisory's `patched` field is often empty and `vuln_range` may be recorded as
a commit hash. **Always express versions as release tags, never commit hashes**
(users reason about releases, not commits). Ask the user which release contains
the fix if it isn't obvious; the affected range is then `<= <last release before
the fix>` (e.g. fixed in v0.23.0 → affected `<= v0.22.1`).

## 4. Write in Obot's format

Use exactly these sections, in this order. Keep it tight — a few sentences per
section. This is a summary for deployers, not the researcher's report.

- **Title** (`# ...`): a plain-language description of the actual issue and its
  real impact. Don't just copy the reporter's title if it leads with a mechanism
  that the team's comments downgraded (e.g. don't headline "audience confusion"
  if that turned out to be a fail-safe). Keep the `(incomplete fix of GHSA-...)`
  style suffix when the advisory metadata has one.
- **## Summary**: 2-4 sentences. What the flaw is and what an attacker could do.
- **## Am I affected?**: the affected release range (`<= vX.Y.Z`) plus the
  preconditions (config flags, required role, required user interaction). Note
  when a configuration makes it a non-issue.
- **## Details**: one paragraph on the root cause, written generically. Do NOT
  cite internal function names, file paths, or line numbers — those are
  implementation details that don't belong in a user-facing advisory. Describe
  behavior ("the authorizer default-allows unrecognized top-level paths"), not
  symbols (~~`checkUI`~~). Mention the relationship to prior advisories if any.
- **## Impact**: worst-case consequence in plain terms. Mirror what the user has
  liked before: state the worst case, then clarify what is NOT exposed/possible
  (e.g. "No credential values are exposed").
- **## Mitigation**: "Upgrade to **vX.Y.Z** or later, which ..." — describe what
  the fix does at a behavioral level (again, no symbol/commit references). If the
  team's comments listed multiple protections, summarize them.
- **## Severity**: `CVSS v3.1 Score: **<score>/10 (<Severity>)** — \`<vector>\``
- **## Credits**: the fixed boilerplate below.

### Credits boilerplate

```
The Obot team would like to thank [@<reporter>](https://github.com/<reporter>) for responsibly disclosing this issue in accordance with our [security policy](https://github.com/obot-platform/obot/?tab=security-ov-file).
```

If there are multiple reporters, thank each (`[@a](...) and [@b](...)`).

## 5. Write the file and present it

Write to `<repo-root>/<GHSA-id>.md`, then paste the full rendered text back in the
chat so the user can review inline. Flag anything you assumed (patched version,
title change) so they can correct it.

## Reference: a finished example

See the advisory writeups already in the repo root (e.g. `GHSA-jgh3-fggc-mcpm.md`,
`GHSA-pr6h-vr44-xq8j.md`, `GHSA-xwmw-prc4-v3cr.md`) for the exact tone and length
to match.
