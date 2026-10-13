---
name: obot-manage-board
description: Triage and release-plan the "Obot AI" GitHub project board (obot-platform org project 2). Use when the user asks to triage the Triage column (with or without milestones), sort issues into priority columns, decide which issues are must-fixes versus pushable, decide what belongs in the next release milestone, review a board column (Testing, In Progress, Next Up) against the milestone, apply board decisions (assign, add to milestone, change status, comment), or change a release milestone's due date.
---

# Obot Manage Board

Covers three jobs on the **Obot AI** board (`https://github.com/orgs/obot-platform/projects/2`):

1. **Triage**: sort the Triage column into must-fix, retest, push out, and not-engineering, with a recommended board column and a reason for each.
2. **Milestone review**: walk a column and decide what belongs in the next release milestone.
3. **Apply decisions**: carry out the user's terse instructions (assign, milestone, status, comment) and verify them.

## Pick the mode

- **Triage only** (the default for "triage the board" or "triage these issues"): run Job 1 and recommend board columns. Don't look up milestones, don't recommend milestone changes, and don't touch milestones when applying decisions, unless the user explicitly says `in` or names a milestone.
- **Release planning** (the user mentions a milestone, a release, a release date, or "what goes in vX.Y"): Job 1 also recommends which items go in the next milestone, and Job 2 and the due-date section apply.

If the request is ambiguous, use triage-only mode. When you finish, offer a milestone review in one line.

The board holds items from four repos: `obot-platform/obot`, `obot-platform/field-issues`, `obot-platform/security-issues`, and `obot-platform/ops-and-it-issues`. Each repo has its own copy of every release milestone (for example `vX.Y.0`).

**Prerequisites:** `gh` authenticated with the `project` scope (`gh auth refresh -s project`), and access to the obot-platform org so the private repos' items are readable.

`board.sh` in this directory does the board plumbing. Run it with no arguments for usage:

```bash
S=.claude/skills/obot-manage-board/board.sh
$S columns                                  # item count per column
$S list Triage                              # items in a column
$S dump Triage <scratch>/triage.txt         # bodies, comments, PRs mentioning each issue
$S show obot-platform/obot <num> <num>      # assignees, milestone, board status
$S status obot-platform/obot <num> "Next Up" # move to a column
$S member <first name>                      # resolve a first name to a GitHub login
$S milestones vX.Y.0                        # due date of a milestone in every repo
```

Write dumps and comment drafts to the session scratchpad, not the repo.

## Hard rules

- **Recommend first, write second.** Triage and milestone review only produce a recommendation. Change nothing on GitHub until the user says what to do. Then do exactly that and no more.
- **Never close issues.** QA closes them from the Testing column. That includes won't-fix decisions: move the issue to Testing and let QA close it.
- **Respect decisions, but flag new information.** Once the user decides on an item, carry it out and don't repeat the same argument later. Do raise it again when something has changed, such as a PR still in draft close to the due date, a new comment from the assignee, or a related fix that landed.
- **Only the Obot AI board.** field-issues items also sit on the Field Board. Don't touch that board.
- **Output format:** markdown links in the form `[#<num> Short title](https://github.com/obot-platform/obot/issues/<num>)`, with `field-issues#<num>` style labels for other repos. Default to plain bullets, which paste cleanly into Slack and GitHub.
- **Verify every write** with `board.sh show` and report what actually changed.

## Gather context

In both modes, establish what has shipped:

```bash
gh release list -R obot-platform/obot -L 5                 # latest release, e.g. vX.Y.Z
git fetch -q --tags upstream 2>/dev/null || git fetch -q --tags origin
git log --oneline main..<latest tag>                        # cherry-picks that shipped only in the patch release
git log --oneline <latest tag>..main                        # fixes on main, first shipping in the next minor
```

In release-planning mode, also find the **next milestone**: the lowest open `vX.Y.0` milestone. Note its due date and open count.

```bash
gh api repos/obot-platform/obot/milestones --jq '.[] | "\(.title) due=\(.due_on) open=\(.open_issues)"'
```

Check whether a feature has already shipped (`git ls-tree -r --name-only <tag> | grep -i <feature>`). A bug in a released feature affects users today. A bug in unreleased work only affects the release.

## Job 1: triage the Triage column

1. `board.sh dump Triage <scratch>/triage.txt`, then read the whole file.
2. For each bug, look for a fix that already exists: a merged PR in the dump, a commit on `main` since the issue was filed (`git log --oneline --since=<created date>`), or a route or file the fix would have touched. A fix merged after the report means **retest first**, not must-fix. Cite the PR and why it likely covers the bug.
3. Sort every item into one bucket, and recommend a board column for each:
   - **Must fix** (recommend Next Up; in release-planning mode, also the next milestone): a core or released feature is broken for real users or real clients, with no reasonable workaround. Examples: one bad component breaking a whole vMCP, OAuth failing for spec-compliant servers, generated client config that doesn't work. A bug that only affects the MCP Tester ranks lower, unless real clients through the vMCP fail too (check the comments).
   - **Retest first** (recommend Testing, with a comment asking for a retest): a merged fix probably covers it.
   - **Push out** (recommend High, Medium, or Low for each): cosmetic and UI polish, edge cases, design-stage features, external integrations, and roadmap requests. Group these as Bugs, UI polish, Features, and Customer requests.
   - **Not engineering / needs clarifying:** business or legal tasks, one-line requests that are ambiguous, and requests the code already satisfies (say where).
4. Judge by impact on users, not by category. A security or performance label doesn't make something a must-fix by itself. Low-severity security items are tracked publicly per `SECURITY.md`.
5. Give each item one line of reasoning: the user impact, plus what makes it cheap or expensive.

Present the buckets in that order. The user then answers item by item.

## Job 2: milestone review of a column (release-planning mode)

Run `board.sh dump "<Column>" <scratch>/col.txt` and read it. For PRs, the size (`+adds/-dels`) and draft state matter.

- **Testing:** everything belongs in the milestone. The exception is a fix that already shipped in a patch release (it shows in `git log main..<latest tag>`). Add PRs to the milestone too (`gh pr edit N --milestone`). A won't-fix waiting for QA to close stays in the milestone and in Testing.
- **In Progress / Next Up:** use judgment, and flag each point:
  - Fixed by a merged PR: recommend moving it to Testing.
  - Draft PR, large diff, or a migration a few days before the due date: recommend the next milestone.
  - A promise to a reporter ("should be in the next release"): include it.
  - Work that other planned work would make obsolete (for example, localizing a UI that is about to be replaced): keep it out.
  - Nothing has happened in weeks and there's no PR: it isn't really in progress, so suggest moving it back to a priority column.
  - Ops, billing, and other non-product work: keep it out of the product milestone.
- Finish with **per-person load**: count each assignee's open items in the milestone, and flag anyone who is overloaded.

## Job 3: apply the user's decisions

Shorthand:

- `in`: add to the next milestone and move to **Next Up**.
- `in, <name>`: also assign. Resolve the name with `board.sh member <name>`. If more than one member matches, ask.
- `me`: the user (`gh api user --jq .login`).
- `move to in progress`, `High`, `med`, `Low`: board status only. `med` means `Medium`.
- `same deal`: repeat the previous item's actions.
- `send to Testing with a comment`: move to Testing and post the comment.

Commands:

```bash
gh issue edit N -R <repo> --add-assignee <login> --milestone vX.Y.0
gh pr edit N -R <repo> --milestone vX.Y.0
$S status <repo> N "Next Up"
gh issue comment N -R <repo> -F <scratch>/cN.md
```

Comments are posted as the user, so write in their voice. Keep them short and plain, `@`-mention the person they're for, and add no sign-off.

- Asking someone to retest: name the PR, say what it changed that covers the bug, and ask for a retest. Mention anything that changed besides the fix.
- A nice-to-have assigned to someone: say it's a nice-to-have and that they can move it out of the milestone if they can't get to it.
- Never use closing keywords (`fixes #N` and so on).

After applying, run `board.sh show` on every item you touched and list what changed. Call out anything the user didn't decide, and anything that changed on the board since you last looked (someone may have moved an item or commented).

## Milestone due dates (release-planning mode)

A release date applies to the milestone in **all four repos**, and the dates must match:

```bash
$S milestones vX.Y.0
gh api -X PATCH repos/obot-platform/<repo>/milestones/<number> -f due_on=<YYYY-MM-DD>T00:00:00Z
```

GitHub stores due dates as midnight UTC whatever time you send, so US time zones may show the previous day. Check the other milestones (for example the next one) while you're there, and report what you changed and what was already correct.
