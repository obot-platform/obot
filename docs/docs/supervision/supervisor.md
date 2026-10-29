---
title: "The Supervisor"
---

The Supervisor is the AI that helps run Supervision. The deterministic policy engine handles hard constraints; the Supervisor handles judgement.

## Bounded decisions

Every judgement the Supervisor makes is a bounded decision: a question, a content-free context, and a fixed set of options to choose from. It answers with one of the offered options, a rationale and a confidence. It can never choose something Obot did not offer.

This is the shape of a decisions-style model API. Obot serves it through its own LLM gateway with any model, and can point it at an external decisions endpoint instead. Set the model and enable or disable the Supervisor on the Supervision settings page.

## What it does

- **Judge.** When a policy's effect is `judge`, the Supervisor sees the session's labels and recent events, the lineage evidence, the action and its arguments, and the policy's guidance, and chooses allow, require approval or deny. It must answer within the surface's time budget; when it cannot, or when it is disabled, the effect degrades to require approval.
- **Triage approvals.** Every approval request is pre-reviewed. The person who opens it sees a recommendation (approve, deny or escalate) with a rationale, next to the raw reason.
- **Draft policies.** A plain-language rule becomes a CEL condition, an explanation and a set of example cases that are evaluated against the draft so you can see what it catches.
- **Narrate sessions.** The session replay page asks the Supervisor for a plain-language account of what a session did, the moments that matter, and what a reviewer should look at.
- **Label tools.** New tools get heuristic capability labels at first sight; the Supervisor refines them in the background. An administrator's labels are never overwritten.

## What it sees

The Supervisor sees event summaries, labels, decisions and lineage, which contain no content, plus the arguments of the one action being judged. It never sees tool results, files or conversation history.

## Red team

The Supervision overview can run a built-in assurance harness that drives the policy engine through ten scenarios (model request, MCP read and write, delegated authority, unsafe arguments, injected tool output, direct shell, outbound data flow, sensitive file access, multi-agent delegation, and a complete investigation trace) and scores each as observed, detected, blocked and remediated. With the built-in policies in shadow mode most scenarios are observed and detected but not blocked; the scorecard says so.
