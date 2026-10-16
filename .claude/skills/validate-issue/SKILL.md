---
name: validate-issue
description: QA-validate one obot-platform/obot issue that is waiting in the Testing Backlog — run a real Obot from the ghcr.io/obot-platform/obot:main image, exercise the reported behavior the way a human QA engineer would (browser and API, not code reading or unit tests), and post one AI-driven validation comment on the issue with a clear PASSED / FAILED / COULD NOT VALIDATE verdict and screenshots. Use when asked to validate, verify, or QA an Obot issue.
metadata:
  argument-hint: "<issue number>"
---

# Validate an issue like QA

You are a QA engineer checking whether a reported problem still exists in the
current `main` build of Obot. Issues in the Testing Backlog were mostly *not*
fixed on purpose: someone believes they were fixed along the way, or that the
behavior is no longer relevant (the feature changed or went away). Assume
nothing — find out. A human QA will read your comment and decide whether to
close the issue or send it back to development. Your job is to give her
trustworthy evidence, not an opinion about the code.

**Test the product, not the source.** Do not read the Go or Svelte code to
decide the verdict, and do not run unit or integration tests. You may read any
linked PR's description, the issue's comments, and the docs in `docs/` to
learn where in the product to look and how a feature is meant to be used.
The verdict comes only from what the running instance does.

**Ask for all your GitHub access in one request**, before you need any of it:
reading the issue and its links, posting and correcting your comment, pushing
screenshots, and — only if the issue needs Git-hosted test content — pushing to
the test-content repo (§3). One request is approved in one step; a second one
for something you forgot waits.

## 1. Understand the issue

```bash
gh api repos/obot-platform/obot/issues/<n>
gh api repos/obot-platform/obot/issues/<n>/comments
gh api "repos/obot-platform/obot/issues/<n>/timeline?per_page=100"   # linked PRs, cross-references
gh api repos/obot-platform/obot/pulls/<pr>                           # merged_at, description
```

Write down, before you start the instance:

- **The problem** — what the reporter saw go wrong, or what was asked for.
- **Expected behavior** — what "no longer a problem" looks like. For a
  feature request or UX gap, that is the capability or feedback the issue
  asks for being present.
- **Reproduction steps** — from the issue. If it gives none, derive the
  smallest user-level steps that exercise the reported behavior. The UI may
  have moved since it was filed: find where the same thing lives now.
- **Why it is thought resolved** — from the latest comments or a linked PR,
  if anything says. Often nothing does.
- **Prior AI validations** — if a comment starting with
  `🤖 **AI-driven validation**` already exists, read it; your comment is a new
  round and should say what changed since.

If the issue cannot be validated on a local Docker install at all, do as much
as you honestly can and use the verdict **COULD NOT VALIDATE** with the reason.
Recognise these early instead of spending an hour on them:

- **Google sign-in** (the Obot-hosted Gmail, Google Drive and other Google
  servers, a Google IdP): the OAuth handshake cannot be completed from this
  sandbox, and there is no Google account. Shared drives also need a paid
  Workspace account.
- Other cloud IdPs (Okta, Entra), paid third-party services, Windows/macOS
  device clients, a specific customer environment.

**Kubernetes** only when the behavior is really Kubernetes-specific (pods,
scheduling, Helm values) and cannot be seen on Docker. Then one local `kind`
cluster, the `main` image pinned by digest, and delete the cluster as soon as
your evidence is in: a cluster costs several GB of the pool's shared disk and a
lot of its CPU.

## 2. Run a real Obot

Always the `main` image, fresh, with Local auth so you can make users with
different roles:

```bash
docker pull ghcr.io/obot-platform/obot:main
docker image inspect ghcr.io/obot-platform/obot:main --format '{{index .RepoDigests 0}} {{.Created}}'

export OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL=owner@example.com
export OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN="$(openssl rand -hex 32)"
docker run -d --name obot \
  -v obot-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 8080:8080 \
  -e OBOT_SERVER_ENABLE_AUTHENTICATION=true \
  -e OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL \
  -e OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN \
  ghcr.io/obot-platform/obot:main
```

Activate the owner at `http://localhost:8080/activate#token=<setup-token>`.
Add more Local users (admin, power user, basic user) when the issue involves
roles or sharing. Use a different auth setup (bootstrap token, auth disabled)
only when the issue is specifically about it.

Record the image digest, its `Created` time and the version for the comment.
`/api/version` needs a signed-in session now; the UI footer shows the version
(`v0.0.0-dev+<commit>`) too. If a comment or linked PR points at a specific fix that merged
*after* the image's `Created` time, say so in the notes: that fix is not in
this build.

If the issue needs a model provider (chat, the LLM gateway) or another
service's credential and you have none, ask for it the way your environment
tells you to. If none is given, report **COULD NOT VALIDATE** for the parts
that need it and validate the rest.

## 3. Test it

Drive the UI in the desktop's Chromium (`chromium http://localhost:8080 &`,
then `xdotool`, or Playwright headed against `DISPLAY=:0`) — whichever lets
you act like a user and see what a user sees. Use `curl` against the API only
where the issue is about the API, MCP endpoints, or the registry.

- Follow the reproduction steps exactly first. Then try the obvious
  neighbours: another role, a page reload, a second object, the edge the
  issue names (empty value, long name, deletion, restart of the container).
- Every claim you will make needs evidence: a screenshot (`scrot -o`, or a
  Playwright screenshot of just the relevant region), an API response, or a
  log line from `docker logs obot`. Read each screenshot back to check it
  shows what you think it does, and that it holds no secret (setup token,
  API keys, bootstrap token).
- If you hit a different bug on the way, note it as a side finding; do not
  let it change the verdict unless it blocks the issue's own behavior.
- **An upgrade path is a neighbour worth trying** when the issue is about data
  a newer version reshapes (composite servers became vMCPs, for one): set the
  issue's state up on the last release image, then switch the same data
  volume to `:main`. One run found Obot refusing to start after exactly that.
- **Git-hosted test content** (a GitOps catalog, a skills repo): push it to the
  public test-content repo `cjellick/craig-catalog` — any branch, any files —
  and point Obot at it. Never put it in the screenshots repo.

Verdicts:

- **✅ PASSED** — the reported problem no longer happens on the steps and the
  neighbours you tried, or the feature/flow it is about no longer exists in a
  form where it could happen. In the second case say so plainly ("the X page
  was removed; Y now does this and does not show the problem") so QA can
  judge relevance.
- **❌ FAILED** — the reported problem still happens, or the requested
  capability is still missing. Say exactly which step and what you saw.
- **⚠️ COULD NOT VALIDATE** — environment or access stopped you. Say what.

Do not hedge between them. If part passed and part failed, it FAILED, and the
details say which part.

## 4. Upload screenshots

Screenshots go into the public repo `cjellick/obot-qa-screenshots`, under
`issue-<n>/` with a short timestamped folder so rounds do not collide:

```bash
git clone https://github.com/cjellick/obot-qa-screenshots /tmp/shots
mkdir -p /tmp/shots/issue-<n>/<YYYYMMDD-HHMM>
cp <pngs> /tmp/shots/issue-<n>/<YYYYMMDD-HHMM>/
cd /tmp/shots && git add -A && git commit -m "issue <n>: validation screenshots" && git push origin HEAD:main
```

If the push is rejected as non-fast-forward, `git pull --rebase` and push
again; never force. Embed each image as
`![what it shows](https://raw.githubusercontent.com/cjellick/obot-qa-screenshots/main/issue-<n>/<folder>/<file>.png)`.
Name files for what they show (`01-owner-sees-catalog.png`). Use a handful of
screenshots, not dozens.

## 5. Post the comment

Write it to a file, then post once:

```bash
gh api repos/obot-platform/obot/issues/<n>/comments -F body=@/tmp/comment.md
```

The comment has exactly this shape — the first line marks it as AI work, the
second line is the verdict, everything else is detail:

```markdown
🤖 **AI-driven validation** — this was tested by an AI QA agent against a real Obot instance, not by a human. A human QA makes the final call.

## Verdict: ✅ PASSED   <!-- or ❌ FAILED, or ⚠️ COULD NOT VALIDATE -->

<one or two sentences: what was checked and the outcome>

### Environment
- Image: `ghcr.io/obot-platform/obot:main@sha256:…` (built <created>)
- Version: <UI footer or `/api/version`>
- Auth: Local, users: owner (+ others used)
- Related: <linked PRs or comments that suggest it was resolved, or "none found">

### Steps and results
1. <step> — <what happened> ✅/❌
2. …

<screenshots, each after the step it proves>

### Notes
<what was not covered and why; side findings; anything QA should re-check by hand>
```

Keep it skimmable: QA reads the first two lines and only goes further when
she needs to. To fix a mistake in your comment, `PATCH` it
(`gh api -X PATCH repos/obot-platform/obot/issues/comments/<id> -F body=@/tmp/comment.md`)
rather than posting a second one.

Do not close, reopen, label, assign, or move the issue on the project board.
Those are the human QA's calls.

## 6. Finish

Print, as the last line of your final message, exactly:

```
validated #<n>: <PASSED|FAILED|COULD NOT VALIDATE> <comment URL>
```

Before that line, once your comment is posted, give this box's disk back —
the pool's disk is shared by every box, and a stopped box keeps whatever it
holds:

```bash
docker rm -f $(docker ps -aq) 2>/dev/null
docker volume prune -a -f
docker image prune -a -f
docker builder prune -a -f
```

Pull only the images the issue needs. Commit nothing to the obot repository.
