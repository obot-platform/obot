---
name: draft-release-blog
description: Draft a release announcement blog post for an obot release, modeled on the existing obot.ai blog voice. Use when the user asks to draft a release blog, write a launch post, or write a blog announcement for a vX.Y.0 release. Outputs a markdown file the user can paste into the CMS. Can also create the post as a Wordpress draft via the `obot-wordpress` MCP server when the user asks, but never publishes a live post without explicit confirmation.
---

# Draft Release Blog Post

Drafts a blog post announcing an obot release. Sources the feature spine from the draft GitHub release (the output of the `draft-release` skill or an existing draft on GitHub), then does additional research to add the kind of detail, context, and rationale that release notes deliberately omit but a blog post needs.

The blog is for obot.ai's hosted Wordpress CMS. The default output is a markdown file at `<repo root>/release-blog-<VERSION>.md` (i.e. the current project directory) that the user can paste into the CMS themselves. Writing it into the repo makes it easy to review in the IDE; it is an intentionally untracked file — do not commit it and do not add it to `.gitignore`. If the user explicitly asks to post it (e.g. "post it up there", "create the draft in Wordpress"), the skill can also create the post as a Wordpress **draft** via the `obot-wordpress` MCP server (see step 8). The skill never publishes a live post without explicit user confirmation of the `publish` status.

## Voice reference

Read both before drafting, every time. Both are multi-feature roundups, which is the shape every obot release blog uses:

- https://obot.ai/blog/announcing-obot-platform-v0-22-0-centrally-managed-skills-fleet-scanning-and-enterprise-controls-for-mcp/ — ~2,100 words, one `## <Feature Name>` section per feature, closes with a multi-CTA "Get started". Note: this post opens with a "client zoo" problem-first hook. Use it as a reference for body voice and structure only. Do NOT copy its opener; the required opener pattern is in step 6.
- https://obot.ai/blog/announcing-obot-platform-v0-23-0/ — ~1,800 words, opens with the mandated pattern ("The Obot Platform v0.23.0 release is out."), same per-feature section structure and multi-CTA close. This is the opener to model.

Author byline is **Craig Jellick** (`craig@obot.ai`). Write in the first-person plural ("we") throughout — these are company announcements, not personal essays. Reserve first-person singular for a specific personal-stake line only if the user supplies one.

## Hard constraints

- No emojis. No em dashes (`—`). Use plain hyphens or rewrite. No "thrilled / proud / revolutionary / game-changing / unlock the power of" hype words.
- Use straight quotes (`'` and `"`), not curly quotes.
- Do not invent statistics, customer quotes, partner endorsements, or technical claims not grounded in the PRs, issues, code, or docs you have actually read.
- The markdown file at `<repo root>/release-blog-<VERSION>.md` is always the first output. Posting to Wordpress (step 8) is opt-in: only do it when the user explicitly asks.
- When posting to Wordpress, the post status MUST be `draft` unless the user explicitly confirms `publish`. Never default to publishing live.
- Never write the post as a "we shipped a lot of stuff this quarter" generic update. Anchor on the specific release.

## Procedure

### 1. Determine the source material

Confirm with the user which release the blog is for. Then find the canonical feature list:

```bash
# Look for an existing draft (or published) release for the version:
gh release view <VERSION> --repo obot-platform/obot
```

If a draft or published release exists, its Big Updates section is the spine. The blog highlights those same features in the same order — do not introduce features that didn't make it into the release notes, and do not silently demote features the release notes promoted.

If no release exists yet, ask the user whether to run `draft-release` first, or proceed by surveying commits directly. Do not invent a feature list out of commits alone for the blog — the release notes are where editorial alignment happens, and the blog should follow that.

### 2. Structure the roundup

Every obot release blog uses the same multi-feature roundup shape as the two reference posts. Target ~1,800-2,100 words. The structure is:

- **Opener** (1-2 short paragraphs). Always lead with the plain fact that the release is out, then state concretely what it adds. There is one opener pattern, not a menu: start with `The Obot Platform <VERSION> release is out.` (or a very close variant), then short factual sentences naming the headline change and the other notable ones. No problem-first hook, no suspense, no marketing framing. See step 6 for the required pattern, the model opener, and the banned phrasings.
- **One `## <Feature Name>` section per feature**, in the same order as the release notes' Big Updates. Each section gives the concrete problem, how the feature works, and (where relevant) a short code/config example.
- **`## Additional Improvements`** with one short bullet per notable smaller item.
- **`## Upgrade Notes`** with one short bullet per breaking change or migration callout, followed by a link to the version's GitHub release notes for complete instructions.
- **`## Get started`** closing CTA (see step 6 for the exact links).

There is no separate deep-dive shape. If one feature dominates the release, give it the longest section and let the smaller ones be brief; do not switch to a single-feature format.

### 3. Deep research for blog-quality detail

This is the step that separates a blog from rephrased release notes. The release notes answer *what*. The blog needs *why* and *context*. For each feature being covered:

- Re-read the PR(s) and any linked issues (remember: this repo does NOT use `closes #N`, so grep PR body/comments for issue numbers; see the related memory). The design discussions, alternatives considered, and user-reported problems live here.
- Look at the actual code or config surface the feature exposes. The blog should be able to describe what configuring this feature looks like in concrete terms ("you get a section for network egress policies", "you define the external domains the server should be allowed to reach"), not abstract terms.
- Check `docs/` for any feature documentation written for this release. Pull concrete language from the docs where it's clearer than what's in the PR.
- For security/compliance features, look for real-world context that explains why the feature matters now (an earlier obot post, for example, cited the Axios supply chain compromise). Use external context only when you can name it specifically — never write "recent attacks have shown..." without naming one.
- Note any partner/integration angle. If the feature involves a named third party, plan to dedicate a short section to that collaboration.

Research implementation details to establish accuracy, but write from the user's perspective. Explain what users can now do and why it matters. Avoid internal IDs, controller behavior, database shapes, resolution mechanics, and other implementation details unless readers need them to use or upgrade the feature.

The goal: walk away from research with enough material to write three things per feature that release notes don't have:
1. The user problem in concrete terms (not "users wanted X" but "before this, admins had to do Y, and Y broke when Z").
2. A specific, visualizable example of using the feature.
3. Forward-looking context (what comes next, what this unlocks, what's still open).

### 4. Surface places that need the user's voice

Even in the roundup shape, some lines can only come from the user: forward-looking commitments, statements about a partner, or a "why we bet on this" take. For example:

> We're already in conversations with other networking companies about additional integrations, so if you're using a different platform, let us know what you'd like to see supported.

> This is just the start of what we're building here.

These are POV statements about partners, future plans, or company stake. The skill should NOT invent them. Before drafting, identify 2-4 places in the planned structure where such POV would belong, then ask the user:

Look for POV opportunities that answer:

- Why this capability matters now
- Why Obot chose this approach
- How organizations differ in the level of control or flexibility they need
- What direction the capability may take next
- Why a particular integration or partner came first

```
For the blog draft, these spots are best filled by your voice rather than mine. Want to give me a sentence or two for each, or should I leave them as `[POV: ...]` placeholders for you to fill in after?

1. <Feature A> — your take on the partner / why you bet on this approach / what you'd say to skeptics
2. <Feature B> — what's next / what you're excited about next quarter
3. ...
```

Use AskUserQuestion when the placeholders map to clean forks. Otherwise plain text reply.

### 5. Confirm angles and POV with the user

Before writing prose, post a short plan:

```
Blog plan for <VERSION>:

Working title: <draft title>
Opening: leads with "The Obot Platform <VERSION> release is out." then <one line on the concrete facts the first paragraph will state>

Sections:
1. <Section name> — <one-line description of angle/content>
2. ...

Features covered: <list>
Features omitted from blog (but in release notes): <list, with brief reason>

POV inputs needed from you: <list from step 4, or "none">

OK to draft, or want to adjust?
```

Wait for confirmation or adjustments before writing prose.

### 6. Draft the post

Match the reference posts:

- Title format: `Announcing Obot Platform <VERSION>: <feature, feature, and theme>` (e.g. "Announcing Obot Platform v0.22.0: Centrally Managed Skills, Fleet Scanning, and Enterprise Controls for MCP"). The subtitle after the colon is optional — v0.23.0 used just `Announcing Obot Platform v0.23.0`. Use "Obot Platform", not "Obot MCP Platform".
- **Opener: always lead with the release being out, stated plainly.** Start with `The Obot Platform <VERSION> release is out.` (or a very close variant), then short, concrete, factual sentences: what the headline change is, followed by a sentence listing the other notable additions. Sound like an engineer explaining the release to another engineer, not a copywriter creating suspense. Lead with a fact, never a hook.

  Banned in the opener (and generally): rhetorical questions; "We're thrilled/proud/excited to announce"; "visibility/security/X is more important than ever"; the cadence "as X grows" or "as teams adopt Y, the same question keeps coming up"; dramatic before/after contrasts; and phrases like "changes everything", "closes the gap", "pulls into view", and "the theme of this release".

  Model opener (from the v0.24.0 post):

  > The Obot Platform v0.24.0 release is out. It adds audit logging for AI activity that previously happened outside Obot. A new companion tool, Obot Sentry, captures tool calls made by Claude Code, Codex, VS Code, and Cursor on developer machines and sends them to Obot's existing audit logs. This release also adds full LLM Gateway audit logs, three new model providers, and a reworked MCP proxy that substantially reduces the resources required to run Obot.
- Use `## <Feature Name>` headings, one per feature, then separate `## Additional Improvements`, `## Upgrade Notes`, and `## Get started` sections.
- Keep Additional Improvements and Upgrade Notes scannable: one short bullet per item. Do not reproduce detailed migration steps, cleanup commands, or implementation notes in the blog. End Upgrade Notes with a link to `https://github.com/obot-platform/obot/releases/tag/<VERSION>` for the complete details.
- For supporting features, lead with the intended scenario instead of lower-level implementation or security details. For example, frame local authentication as a fast path for evaluations and development or test environments, not as a password-storage feature.
- Concrete > abstract in every sentence. If a sentence could appear in any company's launch blog, rewrite or delete it.
- End `## Get started` with one casual sentence that summarizes the release, followed by the CTA paragraph. Do not introduce a new product thesis, roadmap, or upgrade summary in the closing.
- In the CTA paragraph, include a link to try Obot free, a link to request a demo, and a link to the docs, followed by a short invitation to email feedback to `info@obot.ai`. Use the same links the current reference posts use — verify them by re-reading the reference post rather than inventing URLs.

Add a frontmatter block at the top of the markdown for the CMS to consume:

```markdown
---
title: <full title>
category: Blog
author: Craig Jellick
date: <today, ISO format>
version: <VERSION>
---
```

### 7. Write to disk and report

Write the post to `$CLAUDE_PROJECT_DIR/release-blog-<VERSION>.md` (the repo root of the current project). This is an intentionally untracked file for easy IDE review; do not commit it and do not touch `.gitignore`. Print:
- The file path
- The word count (target ~1,800-2,100)
- A list of any `[POV: ...]` placeholders that still need the user's personal voice
- A reminder that this is a draft markdown file for the CMS, written into the repo root for review — the skill has not published anything and the file should not be committed

End the turn there. Do not run any publish/upload commands. Do not modify the GitHub release. Do not commit anything to the repo.

### 8. (Optional) Post to Wordpress as a draft via the MCP server

Skip this step unless the user explicitly asks for it. Phrases that mean yes: "post it to Wordpress", "create the draft", "post it up there", "put it in the CMS". If the user just says "good draft" or stops at step 7, leave it as a local file.

When the user does ask, post the blog as a Wordpress **draft** (never `publish` without explicit confirmation) via the `obot-wordpress` MCP server. The flow has four sub-steps.

**8a. Authenticate (one time per session).** Call `mcp__obot-wordpress__authenticate` to start the OAuth flow. Share the authorization URL it returns with the user; tell them the redirect to `localhost` may show a connection error and that's fine. The server's full tool set (`create_post`, `list_categories`, `list_users`, `update_post`, etc.) becomes available once auth completes. If auto-completion doesn't fire after they authorize, ask for the full callback URL from their browser and pass it to `mcp__obot-wordpress__complete_authentication`.

**8b. Convert the markdown to HTML.** Wordpress's `create_post` expects the post body as HTML. A helper script lives next to this SKILL.md:

Immediately before conversion, re-read the markdown from disk and confirm it contains the user's latest requested revisions. Record its SHA-256 hash so the exact source state being uploaded is explicit.

```bash
shasum -a 256 "$CLAUDE_PROJECT_DIR/release-blog-<VERSION>.md"
uv run "$CLAUDE_PROJECT_DIR/.claude/skills/draft-release-blog/md_to_html.py" \
  "$CLAUDE_PROJECT_DIR/release-blog-<VERSION>.md" \
  > /tmp/release-blog-<VERSION>.html
```

The markdown source lives in the repo root; the HTML is just a transient intermediate for the Wordpress upload, so it stays in `/tmp`.

It strips the YAML frontmatter so it doesn't render in the post body and emits HTML5 with the Python `markdown` library's `extra` and `sane_lists` extensions. Dependencies are declared inline (PEP 723) so `uv run` resolves them automatically — no virtualenv setup needed. Important: redirect only stdout (no `2>&1`) so uv's "Installed N packages" line doesn't get mixed into the HTML.

**8c. Look up category and author IDs.** The `create_post` tool wants integer IDs, not names. Run these in parallel:

```
mcp__obot-wordpress__list_categories(search_query="Blog")  # → expect id 6
mcp__obot-wordpress__list_users(context="view")             # → find Craig Jellick, expect id 9
```

IDs may differ if the Wordpress site is reconfigured. Always look them up; don't hardcode.

**8d. Create the draft.** Call `mcp__obot-wordpress__create_post` with:

- `title`: the full title from the markdown frontmatter (`title:` line), unquoted.
- `content`: the HTML from step 8b, pasted in as a single string.
- `status`: `"draft"`. Never `"publish"` unless the user has explicitly confirmed publishing live.
- `author_id`: the integer from `list_users` for Craig Jellick.
- `categories`: the Blog category ID as a string (e.g. `"6"`), since the tool accepts a comma-separated list.

The response returns a post `id` and a `link` like `https://obot.ai/?p=<id>`. Share both with the user and remind them to review and publish from the WP admin (Posts → Drafts).

After creation, retrieve the post by ID and verify that its status is `draft`, its title matches the frontmatter, and its author and category match the resolved IDs. Report only the verified result.

**Sanity checks before posting:**

- Run `grep -nF $'\xe2\x80\x94' "$CLAUDE_PROJECT_DIR/release-blog-<VERSION>.md"` (or `grep -nF $'—'`) to confirm no em dashes slipped in. The hard constraints forbid them; the markdown editor sometimes auto-substitutes when copy-pasting.
- Confirm the HTML file does not start with a uv "Installed N packages" line.
- Confirm the title in the frontmatter matches the title you're sending to `create_post`.
- Re-run the SHA-256 command and confirm the markdown has not changed since conversion. If it changed, discard the generated HTML and convert again.

## Tone reference snippets

The opener always leads with the plain announcement, as in v0.23.0:

> The Obot Platform v0.23.0 release is out. It is a step toward deeper integration between Obot and the clients and agents that connect to it.

For the body, match this register: confident, concrete, no rhetorical flourishes, technical without being dry, first-person plural for the company. For example (v0.23.0):

> A developer points their agent at the gateway and gets to work.

Lead with a plain statement of what shipped, never with a hook, a rhetorical question, or hype. The v0.22.0 post opened with a 'client zoo' problem framing; do not copy that approach for the opener. Lead with the release being out instead.
