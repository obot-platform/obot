---
name: draft-release-blog
description: Draft a release announcement blog post for an obot release, modeled on the existing obot.ai blog voice. Use when the user asks to draft a release blog, write a launch post, or write a blog announcement for a vX.Y.0 release. Outputs a markdown file the user can paste into the CMS. Can also create the post as a Wordpress draft via the `obot-wordpress` MCP server when the user asks, but never publishes a live post without explicit confirmation.
---

# Draft Release Blog Post

Drafts a blog post announcing an obot release. Sources the feature spine from the draft GitHub release (the output of the `draft-release` skill or an existing draft on GitHub), then does additional research to add the kind of detail, context, and rationale that release notes deliberately omit but a blog post needs.

The blog is for obot.ai's hosted Wordpress CMS. The default output is a markdown file at `/tmp/release-blog-<VERSION>.md` that the user can paste into the CMS themselves. If the user explicitly asks to post it (e.g. "post it up there", "create the draft in Wordpress"), the skill can also create the post as a Wordpress **draft** via the `obot-wordpress` MCP server (see step 8). The skill never publishes a live post without explicit user confirmation of the `publish` status.

## Voice reference

Read both before drafting, every time:

- https://obot.ai/blog/obot-v0210-network-egress-control-for-mcp-servers/ — single-feature deep-dive shape, first-person personal voice, ~600 words
- https://obot.ai/blog/announcing-obot-mcp-platform-v0-16-0-api-keys-model-access-policies-and-other-enterprise-ready-controls/ — multi-feature roundup shape, "we are excited" framing, ~1200 words

Author byline is **Craig Jellick** (`craig@obot.ai`). The personal voice in the deep-dive shape is his — when the post uses "I", that's him. Don't use "I" in the roundup shape; use "we".

## Hard constraints

- No emojis. No em dashes (`—`). Use plain hyphens or rewrite. No "thrilled / proud / revolutionary / game-changing / unlock the power of" hype words.
- Use straight quotes (`'` and `"`), not curly quotes.
- Do not invent statistics, customer quotes, partner endorsements, or technical claims not grounded in the PRs, issues, code, or docs you have actually read.
- The markdown file at `/tmp/release-blog-<VERSION>.md` is always the first output. Posting to Wordpress (step 8) is opt-in: only do it when the user explicitly asks.
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

### 2. Choose the blog shape

The two reference posts use different shapes. Pick based on the release character:

- **Single-feature deep-dive** (model on v0.21.0 blog): use when the release has one obviously dominant feature and one or two minor supporting items. ~500-800 words. Sections: opener, "Why we built this", "How it works", optional partner/collab section, "What's next".
- **Multi-feature roundup** (model on v0.16.0 blog): use when the release has 3-6 features of comparable weight. ~1000-1400 words. Sections: opener with bulleted summary, one subsection per feature, "Additional improvements" if relevant, "Get started" CTA.

Present the recommended shape to the user before drafting and let them override. If they pick deep-dive but there are multiple features, the others can be mentioned briefly at the end ("This release also includes X, Y, Z — see the release notes for details").

### 3. Deep research for blog-quality detail

This is the step that separates a blog from rephrased release notes. The release notes answer *what*. The blog needs *why* and *context*. For each feature being covered:

- Re-read the PR(s) and any linked issues (remember: this repo does NOT use `closes #N`, so grep PR body/comments for issue numbers; see the related memory). The design discussions, alternatives considered, and user-reported problems live here.
- Look at the actual code or config surface the feature exposes. The blog should be able to describe what configuring this feature looks like in concrete terms ("you get a section for network egress policies", "you define the external domains the server should be allowed to reach"), not abstract terms.
- Check `docs/` for any feature documentation written for this release. Pull concrete language from the docs where it's clearer than what's in the PR.
- For security/compliance features, look for real-world context that explains why the feature matters now. The v0.21.0 post cites the Axios supply chain compromise. Use external context only when you can name it specifically — never write "recent attacks have shown..." without naming one.
- Note any partner/integration angle (e.g. Aviatrix in v0.21.0). If the feature involves a named third party, plan to dedicate a short section to that collaboration.

The goal: walk away from research with enough material to write three things per feature that release notes don't have:
1. The user problem in concrete terms (not "users wanted X" but "before this, admins had to do Y, and Y broke when Z").
2. A specific, visualizable example of using the feature.
3. Forward-looking context (what comes next, what this unlocks, what's still open).

### 4. Surface places that need the user's personal voice

The deep-dive shape has lines that can only come from the user. Examples from v0.21.0:

> What I appreciate about working with their team: they think about this the same way we do.

> We're already in conversations with other networking companies about additional integrations, so if you're using a different platform, let us know what you'd like to see supported.

These are POV statements about partners, future plans, or personal stake. The skill should NOT invent these. Before drafting, identify 2-4 places in the planned structure where personal POV would belong, then ask the user:

```
For the blog draft, these spots are best filled by your voice rather than mine. Want to give me a sentence or two for each, or should I leave them as `[POV: ...]` placeholders for you to fill in after?

1. <Feature A> — your take on the partner / why you bet on this approach / what you'd say to skeptics
2. <Feature B> — what's next / what you're excited about next quarter
3. ...
```

Use AskUserQuestion when the placeholders map to clean forks. Otherwise plain text reply.

### 5. Confirm shape, angles, and POV with the user

Before writing prose, post a short plan:

```
Blog plan for <VERSION>:

Shape: <Deep-dive | Roundup>
Working title: <draft title>
Opening hook angle: <one sentence>

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

- Title format: For deep-dive, `New in Obot: <Feature Phrase>` (e.g. "New in Obot: Network Egress Control for MCP Servers"). For roundup, `Announcing Obot MCP Platform <VERSION>: <feature, feature, and other theme>`.
- Open with a one-paragraph hook. Deep-dive: first person, states what was built and the partner if any. Roundup: "We're excited to announce..." + brief framing of the release theme.
- Use `## Why we built this` / `## How it works` / `## What's next` headings in the deep-dive shape, or `## <Feature Name>` headings in the roundup shape. Match the reference structure exactly.
- Concrete > abstract in every sentence. If a sentence could appear in any company's launch blog, rewrite or delete it.
- End with a CTA. Reference posts use either `reach out at info@obot.ai` or `Check out our installation guide`. Default to `info@obot.ai` for deep-dives where there's a partner/conversation angle, installation guide for roundups.

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

Write the post to `/tmp/release-blog-<VERSION>.md`. Print:
- The file path
- The chosen shape and word count
- A list of any `[POV: ...]` placeholders that still need the user's personal voice
- A reminder that this is a draft markdown file for the CMS — the skill has not published anything

End the turn there. Do not run any publish/upload commands. Do not modify the GitHub release. Do not commit anything to the repo.

### 8. (Optional) Post to Wordpress as a draft via the MCP server

Skip this step unless the user explicitly asks for it. Phrases that mean yes: "post it to Wordpress", "create the draft", "post it up there", "put it in the CMS". If the user just says "good draft" or stops at step 7, leave it as a local file.

When the user does ask, post the blog as a Wordpress **draft** (never `publish` without explicit confirmation) via the `obot-wordpress` MCP server. The flow has four sub-steps.

**8a. Authenticate (one time per session).** Call `mcp__obot-wordpress__authenticate` to start the OAuth flow. Share the authorization URL it returns with the user; tell them the redirect to `localhost` may show a connection error and that's fine. The server's full tool set (`create_post`, `list_categories`, `list_users`, `update_post`, etc.) becomes available once auth completes. If auto-completion doesn't fire after they authorize, ask for the full callback URL from their browser and pass it to `mcp__obot-wordpress__complete_authentication`.

**8b. Convert the markdown to HTML.** Wordpress's `create_post` expects the post body as HTML. A helper script lives next to this SKILL.md:

```bash
uv run "$CLAUDE_PROJECT_DIR/.claude/skills/draft-release-blog/md_to_html.py" \
  /tmp/release-blog-<VERSION>.md \
  > /tmp/release-blog-<VERSION>.html
```

It strips the YAML frontmatter so it doesn't render in the post body and emits HTML5 with the Python `markdown` library's `extra` and `sane_lists` extensions. Dependencies are declared inline (PEP 723) so `uv run` resolves them automatically — no virtualenv setup needed. Important: redirect only stdout (no `2>&1`) so uv's "Installed N packages" line doesn't get mixed into the HTML.

**8c. Look up category and author IDs.** The `create_post` tool wants integer IDs, not names. Run these in parallel:

```
mcp__obot-wordpress__list_categories(search_query="Blog")  # → expect id 6
mcp__obot-wordpress__list_users()                           # → find Craig Jellick, expect id 9
```

IDs may differ if the Wordpress site is reconfigured. Always look them up; don't hardcode.

**8d. Create the draft.** Call `mcp__obot-wordpress__create_post` with:

- `title`: the full title from the markdown frontmatter (`title:` line), unquoted.
- `content`: the HTML from step 8b, pasted in as a single string.
- `status`: `"draft"`. Never `"publish"` unless the user has explicitly confirmed publishing live.
- `author_id`: the integer from `list_users` for Craig Jellick.
- `categories`: the Blog category ID as a string (e.g. `"6"`), since the tool accepts a comma-separated list.

The response returns a post `id` and a `link` like `https://obot.ai/?p=<id>`. Share both with the user and remind them to review and publish from the WP admin (Posts → Drafts).

**Sanity checks before posting:**

- Run `grep -nF $'\xe2\x80\x94' /tmp/release-blog-<VERSION>.md` (or `grep -nF $'—'`) to confirm no em dashes slipped in. The hard constraints forbid them; the markdown editor sometimes auto-substitutes when copy-pasting.
- Confirm the HTML file does not start with a uv "Installed N packages" line.
- Confirm the title in the frontmatter matches the title you're sending to `create_post`.

## Tone reference snippets

From v0.21.0 (deep-dive):

> I'm excited to announce a new framework in Obot that lets you define and enforce network egress policies for the MCP servers running in your Obot environment.

> The MCP server works exactly like it did before. It just can't access external sites it shouldn't.

From v0.16.0 (roundup):

> We are excited to announce the release of Obot v0.16.0, a significant update focused on enterprise readiness and operational control.

> Perfect for CI/CD pipelines, automation scripts, monitoring tools, and other integrations.

Match that register: confident, concrete, no rhetorical flourishes, technical without being dry, first-person plural for the company and first-person singular only when speaking with personal stake.
