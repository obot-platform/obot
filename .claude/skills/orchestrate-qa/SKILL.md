---
name: orchestrate-qa
description: Run AI-driven QA over a column of the obot-platform GitHub project (by default "Test Backlog" in org project 2) as a lead discobox — launch one worker discobox per issue that validates it against a real Obot under the validate-issue skill, keep a throttled pool of them within the shared pool's CPU, memory and disk, approve their GitHub access within a delegation, wake and unstick them, retire finished ones, and report verdicts with links. Use when asked to QA, validate, or verify the issues in the Testing Backlog (or another project column) with sub-discoboxes or worker agents.
metadata:
  argument-hint: "[column name, default \"Test Backlog\"] [max workers, default 5]"
---

# Orchestrate AI QA of a project column

You are the lead. Workers do the testing, one issue each, under the
`validate-issue` skill in this repository; you start them, keep the pool
healthy, answer what they ask, and keep the user's table. You never test an
issue yourself, and nobody closes, labels or moves an issue — that is the human
QA's call. The workers' comments are evidence for her, not a sign-off.

`qa.sh` beside this file is the loop. Run it with `bash` (never source it into
zsh: zsh does not word-split unquoted expansions, which silently broke GraphQL
pagination once). It keeps state in `~/.local/state/orchestrate` and finds use
IDs by their descriptions, so renewed grants just work. Its subcommands:
`backlog`, `launch`, `watch`, `status [all]`, `screen`, `say`, `cleanup`,
`power`, `approve`, `verdicts`, `retire`, `idle`, `pause`, `resume`, `results`.

Load the `discobox` and `discobox-access` skills first if they are not loaded:
they say how credentials, delegation and worker boxes work.

## 0. Before you start

- **Workers are cut from this checkout's committed HEAD** (`--include-dirty=false`).
  `validate-issue` must be committed here, or they never get it. Commit skill
  changes before launching; a worker keeps the skill it started with.
- **Two GitHub repos the user owns** back the evidence. Ask for them if you do
  not know them; the current ones are named in `validate-issue`:
  - a **public** repo for screenshots (a private one breaks the images for
    anyone without access to it);
  - a repo workers may push anything to, for test content Obot reads from Git
    (GitOps catalogs).
- **Credentials for third-party services** (Google, model providers) are not
  part of the plan. A worker that needs one asks; that request waits for the
  user. Google OAuth cannot be completed from a sandbox at all — those issues
  come back COULD NOT VALIDATE, which is a fine answer.
- Take a keepalive lease while the run lasts and tell the user: `watch`
  renews `/run/discobox/keepalive/qa-orchestrator` for two hours at a time and
  removes it when everything is done. Label this box in `~/.discobox/meta.json`.

## 1. Ask for everything up front

Three requests, each once, before the first worker, each run in the
background with `wait: true` (keep the request ID so `discobox-access wait`
can resume it). Word every use as the literal command; never write a sentence
that vouches for itself.

**The discobox API.** The usual ten lead uses (create with any prompt and no
grants from this directory; box ls/get; secret request ls; secret ls;
approve; deny; terminal ls/screen/wait; terminal input to answer questions or
tell it to continue; start/stop/restart; tag) — copy them from
`discobox-access` §5, with a `grantTTLSeconds` as long as the run. **Add an
eleventh** from the start:

```json
{"description": "discobox admin terminal input <terminal-id> --discobox-id <discobox-id> [--literal] <text>: ask a discobox I created to free its own Docker disk once its validation comment is posted (remove its own containers, prune its own volumes, images and build cache), or to avoid pulling images it does not need"}
```

Typing a cleanup instruction under the "answer or continue" use is refused,
correctly — it is a different act. `qa.sh cleanup` uses this one.

**GitHub, for the lead** (reads only):

```json
"uses": [
  {"description": "gh api graphql -f query=<query> [-F/-f variables]: read-only GraphQL queries (no mutations) on organization <org> projectV2 number <n>, to list its items, their Status field values, and the linked issue numbers, titles, and states"},
  {"description": "gh api GET repos/<org>/<repo>/issues/<number>, repos/<org>/<repo>/issues/<number>/comments and repos/<org>/<repo>/issues/<number>/timeline (with query parameters): read an issue, its comments and its linked pull requests"},
  {"description": "gh api GET repos/<org>/<repo>/pulls/<number> and repos/<org>/<repo>/pulls/<number>/files: read a pull request"}
]
```

**GitHub, to delegate — one request holding every worker use.** An approval
is judged against a single delegation grant. When the catalog repo was added
later as a second delegation, a worker's request that named both repos was
refused for reaching past "the" delegation, and had to be split by hand. So
put all of these in one `"purpose": "delegate"` request, each line starting
"for the discoboxes I create":

- read the issue it validates, its comments, timeline, linked PRs and commits
  (`gh api GET repos/<org>/<repo>/issues/<number>`, `…/comments`,
  `…/timeline`, `pulls/<number>`, `pulls/<number>/files`,
  `pulls/<number>/commits`, `commits/<sha>`, `search/issues`);
- post and correct its one validation comment (`gh api POST
  repos/<org>/<repo>/issues/<number>/comments -F body=@<file>`, `gh api -X
  PATCH repos/<org>/<repo>/issues/comments/<id> -F body=@<file>`);
- git clone, fetch and push to the screenshots repo, branch main only, files
  under `issue-<number>/`, fast-forward, never force;
- git clone, fetch, pull and push to the test-content repo, any branch and
  files, force-push allowed, and `gh api GET` on it.

## 2. Build the queue

```bash
bash qa.sh backlog        # reads the column, skips issues already handled or already AI-validated
```

Then reorder `~/.local/state/orchestrate/queue.txt` by hand: issues testable
on a local Docker install first; those needing Kubernetes, a GitOps source, an
OAuth provider or a third-party account last. Read the titles — it takes a
minute and the early results are the useful ones.

Most issues in a "Test Backlog" were **not fixed on purpose**: someone thinks
they were fixed along the way, or that the feature changed under them. The
worker's question is "does the reported problem still happen on `main`?",
and "the feature no longer exists in that form" is a PASS with a note. That
framing lives in `validate-issue`; keep it there.

## 3. Launch and watch

```bash
bash qa.sh launch one     # first worker alone: check it loads validate-issue and asks for GitHub
bash qa.sh screen <id> 30
bash qa.sh watch          # in the background; it launches the rest itself
```

**Throttle on the pool, not on a count.** The pool's CPU, memory and per-box
disk are shared with every box on it, including the user's other work — a
listing once showed 34 boxes, 10 of them other running work. `watch` starts
at most **one worker a minute**, and only while load is under the core count,
≥10G memory is free, and ≥15G disk is free (≥8G when nothing else of ours is
running, or the queue deadlocks). `ORCH_MAX` (default 5) is a ceiling, not a
target. Starting five at once took load to 23 on 14 cores: the load average
lags, so a burst passes the check before it shows.

**What costs the most:** a worker that stands up a local Kubernetes cluster
(kind) to reproduce something — two of them took free disk from 15G to 2.6G
in minutes. `validate-issue` keeps that to issues that are really
Kubernetes-specific and has the worker tear down after itself; ordinary
workers peak around 3–5G and give it back. Clearing `~/.cache` does nothing
for this: caches live on a separate pool disk.

`watch` handles on its own, without exiting: approving each worker's GitHub
request (retrying when the judge itself was unreachable), clearing "Login
expired" with `continue`, answering the first-run auto-mode dialog,
restarting a worker that stopped itself on idle and telling it to continue,
requeueing a box whose creation failed ("no runtime on its pool"),
retiring a worker the moment its verdict line appears, launching the next,
and renewing the lease. Its heartbeat only logs. **It does not exit on a
timer** — once it did, a missed wake-up left a worker stalled on "Login
expired" for ten hours.

It exits only on what needs you:

| Exit line | What to do |
| --- | --- |
| `VERDICT #n <id> validated #n: …` | Already retired. Read the comment (`qa.sh results`, or `gh api …/comments`), check it opens with the AI line and a verdict, and post the row. Restart `watch`. |
| `REFUSED #n <req>: …` | Read the request (`discobox secret request ls -o json`). Uses past the delegation go to the user, never widened by you. A request mixing two delegations can be narrowed: `approve <req> --use "<the part one grant covers>"`, then tell the worker to ask for the rest separately. |
| `NEEDS LEAD: <req>` | A non-GitHub credential (a model key, a Google account) or a request to delegate: it waits for the user. Say so on the table. |
| `IDLE #n <id>` + screen | Read it. Answer a question you can settle with `say`; put product or scope questions on the table as "waiting for you". |
| `LIMIT #n <id>` | The org's spend limit. `qa.sh pause` stops every worker (work survives); tell the user; after they raise it, `qa.sh resume`. |
| `DISK LOW` / `MEM LOW` | `qa.sh cleanup <ids>` to running workers, `pause` if it is critical, and tell the user which boxes to purge. |
| `ALL DONE` | §5. |

Restart `watch` after every exit. The loop holds no state of its own beyond
`~/.local/state/orchestrate`, so restarting it is always safe — but stop it
(TaskStop) before editing `qa.sh`: bash reads a running script from disk as
it goes.

## 4. Keep the user's table

Post it whenever a row changes, briefly, with the verdict news first. Rows are
issue, short title, worker box, status (working, waiting for you, verdict);
finished boxes collapse into one 🗑 row. Under the table, each comment's URL
on its own line (links inside table cells render badly in a terminal). Lead
with anything serious a worker found — an upgrade that crashes outranks a
cosmetic pass. Count passed / failed / could-not / running / queued every
time. Report the pool's headroom when it is what is holding the queue.

## 5. Finish

- `bash qa.sh results` — every verdict with its comment link, failures first.
- Give the user the failures with one line each on what is still wrong, then
  the could-not-validates with what a human needs to check them.
- Every worker is tagged `to-delete verdict=<v>` and stopped. You cannot
  delete a box; tell the user to purge them
  (`discobox ls --all --tag to-delete`, then `discobox admin box purge`) —
  stopped boxes keep their disk.
- Release the lease if `watch` did not, and leave this checkout's tree clean:
  the skills are committed, the state is outside the repository.
