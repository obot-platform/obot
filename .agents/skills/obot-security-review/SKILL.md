---
name: obot-security-review
description: Run an independent security review of a finished Obot change in a fresh context, using the repo's threat model. Use when a change is done and about to become a pull request, when asked for a security review, or when CLAUDE.md/AGENTS.md says one is due. Not for work in progress.
---

# Security review of a finished change

The review must come from a context that did not write the change. An agent that just built a feature will find the
reasons it is safe; a fresh one reads the code as an attacker would. So the author does not review its own work here.
It hands the change to a reviewer that starts with nothing but the change and this brief.

## When

Once, when the work is done: before opening a PR, or when the developer says the change is finished. Not after each
edit, and not while the developer is still iterating. If the change grows after the review, review the new part
before the PR is updated.

Skip it when the change touches no runtime code: docs, comments, tests, or agent skills only. Say so in the PR.

## How

1. **Pick the reviewer.** Start a subagent, or a new session, with no history from the work. Any harness that can
   start a fresh agent works. Claude Code's built-in `/security-review` is not a substitute on its own: it runs from
   the current session, so the context that wrote the change decides which findings count, and it uses generic rules
   that don't match `THREAT_MODEL.md` (it skips denial of service, which Obot treats as in scope, and doesn't know the
   Obot-specific checks below). Use this brief with a fresh subagent. If you also run `/security-review`, run it from
   a new session and have it read `THREAT_MODEL.md` first.
2. **Give it the change and the brief below, and nothing else.** No summary of what the change does or why it is safe.
   Name the base to diff against (usually `origin/main`).
3. **Triage what comes back.** Fix real findings, then have the reviewer check the fix. For a finding you think is
   wrong, say why in the PR rather than silently dropping it. Ask the developer when a fix would change product
   behaviour.
4. **Record it in the PR description**, one line: `Security review: no findings`, `Security review: fixed <what>`,
   or `Security review: n/a (<why>)`.

## Reviewer brief

> You are reviewing a change to Obot for security vulnerabilities. You did not write it and have not been told it is
> safe. Read it with `git diff <base>...HEAD` (and `git status` for anything uncommitted), then read the code around
> it: a vulnerability usually lives in how the change meets existing code, not in the diff alone.
>
> Read `THREAT_MODEL.md` first. It defines who is trusted and what is out of scope. Report only issues that are in
> scope under it. In particular, Admins and Owners are fully trusted, and Power Users can already run their own code
> and choose outbound URLs, so "a Power User could do X" is only a finding when it crosses one of the lines the threat
> model lists. `docs/docs/security/` describes how the existing controls work.
>
> Look hardest at what this codebase gets wrong most often:
>
> - **Authorization.** A new or changed API route must be covered by the rules in `pkg/api/authz/`, and a handler must
>   check that the caller may see or change the specific resource, not just that they are logged in. Look for one user
>   reaching another user's servers, credentials, projects, or audit data, and for a standard user reaching admin
>   behaviour.
> - **Credentials and tokens.** Secrets must not reach logs, API responses, URLs, error messages, or another user. OAuth
>   tokens must stay bound to the user and server they were issued for. Remote MCP servers are untrusted and must not
>   obtain credentials for another service, user, or Obot itself.
> - **Outbound requests.** New code that connects to a user- or server-supplied URL should use `pkg/safehttp`, and
>   must not return response bodies or send Obot's credentials to an attacker (see the SSRF section of the threat
>   model).
> - **MCP server isolation** on the Kubernetes backend: pod security, NetworkPolicy, and what a server can reach.
> - **Unauthenticated paths.** Anything reachable without logging in.
> - **Injection.** Values from users or MCP servers flowing into shell commands, SQL, file paths, templates, or HTML.
>
> For each finding give: the attacker's starting role, the steps, what they gain, the file and line, and how confident
> you are. A finding you reached only by reading code is a lead; say what would confirm it. Do not report missing
> hardening with no impact, style issues, or anything the threat model puts out of scope. If you find nothing, say
> "no findings" and list what you checked.
>
> Do not edit files.
