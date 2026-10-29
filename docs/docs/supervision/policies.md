---
title: "Policies"
---

Obot Policy is one rule language for every surface. A policy is a condition, written in [CEL](https://cel.dev), plus an effect. The same policy applies on a laptop, at the MCP and LLM gateways, in a Claude Environments sandbox, and for a Copilot Studio agent.

## Fields

| Field | Meaning |
|---|---|
| Condition | A CEL expression over `subject`, `action`, `session` and `org` that evaluates to a boolean |
| Effect | `allow`, `log`, `redact`, `downshift`, `quarantine`, `judge`, `require_approval` or `deny` |
| Mode | `shadow` records what the policy would have done without changing the outcome; `enforce` applies it |
| Surfaces | Which surfaces the policy covers; empty means all |
| Priority | Breaks ties between policies with the same effect; higher wins |
| Guidance | Plain-language guidance for the Supervisor when the effect is `judge` |
| Approvers | Who can resolve an approval the policy requires: `admin`, `manager`, `group:<name>`, `user:<id>` |

When several policies match one action, the strongest enforced effect wins. Shadow matches are recorded next to it, so you can see what a draft policy would have done alongside what was actually enforced.

## Vocabulary

```
subject   id, email, groups, agent, device, environment, api_key, delegation
action    surface, kind, tool, server, server_id, model, provider, host, method, path,
          command, file, capabilities, labels, honeytoken, trifecta,
          sink {external, destination, writes, exec},
          source {private, untrusted},
          lineage {hit, exact, count, labels}
session   id, labels, risk, events, tool_calls, denied, duration_seconds, surfaces,
          trifecta, killed,
          warrant {present, task, scopes, drifted, covers}
org       approved_destinations, trusted_hosts

host_matches(host, patterns)   true when host matches any pattern ("*.example.com", "example.com", "*")
```

`action.kind` is one of `tool.call`, `shell.exec`, `file.read`, `file.write`, `net.egress` or `llm.turn`. `action.labels` are the sensitivity labels found in the action's own content; `action.lineage.labels` are the labels of earlier content found again in it. `session.labels` is everything that has entered the session.

## Examples

Hold any external write from a session that has read both untrusted content and confidential data:

```
action.trifecta
```

Never let a secret that entered the session leave it:

```
"secret" in action.lineage.labels && action.sink.external
```

Ask the Supervisor before a destructive tool runs in a session that has read untrusted content:

```
"untrusted" in session.labels && ("destructive" in action.capabilities || action.sink.exec)
```

Require approval for engineering agents reaching hosts outside the approved list:

```
"engineering" in subject.groups && action.kind == "net.egress"
  && !host_matches(action.host, org.approved_destinations)
```

## Built-in policies

Obot seeds these on first start. They begin in shadow mode, except the honeytoken and killed-session rules, which cannot produce false positives.

| Policy | Effect |
|---|---|
| Honeytoken exfiltration | deny (enforced) |
| Killed session | deny (enforced) |
| Secrets never leave | deny |
| Lethal trifecta | require_approval |
| PII lineage to external destination | require_approval |
| Destructive action after untrusted content | judge |
| Outside the declared task | judge |

Built-in policies can be disabled or switched to enforce but not deleted.

## Simulate before you enforce

The policy editor runs any policy against the sessions recorded in the last N days and reports how many actions it would have matched, how many outcomes would have changed, how many sessions and users it touches, a false-positive hint (matches on sessions that never tripped an enforced policy), and a sample of matched events. Use it before switching a policy to enforce.

## Draft from plain language

Describe a rule in a sentence and the Supervisor drafts the condition, explains what it checks, and generates example cases showing what it will and will not match. Review the draft, simulate it, then save it. See [The Supervisor](./supervisor.md).
