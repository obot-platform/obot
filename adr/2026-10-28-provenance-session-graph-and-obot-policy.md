# 2026-10-28: One session graph and one policy engine across every surface

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None yet. This is the Obot 2.0 "Provenance" implementation.

## Related ODPs

None. An ODP should follow if the design changes materially from what shipped.

## Context

Obot observes an agent in four places, and each place had its own policy model and its own audit log: access control rules, model access policies, skill access rules, MCP filters, guardrail policies, Sentry enforcement allowlists, network policies, vMCP profiles and Claude Environment profiles on the policy side; MCP, LLM, egress, enforcement-decision and device-scan logs on the audit side. Correlation across them was filtering by user or session after the fact. Device enforcement was deliberately parameter-free and per call, with no memory of what came before in the session.

That leaves the problem that actually matters undecidable: "data that came from the CRM may not reach an unapproved destination in a session whose context also contains attacker-controllable content" (the lethal trifecta). Each surface sees one link of the chain. EDR sees the shell but not why it ran; the network sees the packet but not the prompt injection three steps earlier; identity knows who the user is but not what the model read. Obot sees all four, and now a fifth (Copilot Studio agents through its runtime monitoring hook), but only if they feed one model.

## Decision

`pkg/provenance` is the single entry point every surface calls. It has three layers.

- **A content-free session graph.** Each surface reports `Event`s (`llm.turn`, `tool.call`, `tool.result`, `file.read`, `file.write`, `shell.exec`, `net.egress`, `delegation`, `approval`, `policy.decision`, `kill`) with a causal `ParentID`. Events are stitched into one `Session` per agent session across surfaces: an explicit `X-Obot-Session-ID` first, then the agent's own session identifier (Claude Code `session_id`, `Mcp-Session-Id`, the Claude Environment session, a Copilot conversation ID) scoped by agent, then a fallback on the actor (device and agent, environment, or user, API key and agent) with a 30-minute idle timeout. Events never persist content: tool arguments and results are scanned and discarded; what is kept is labels, lineage and summaries.
- **Labels and lineage.** Tools carry capability labels (`reads-private`, `reads-untrusted`, `writes-external`, `writes-internal`, `exec`, `destructive`, `credential-bearing`), assigned heuristically, refined by the Supervisor, and overridable by an administrator. Content entering a session carries sensitivity and trust labels (`pii`, `secret`, `customer-data`, `financial`, `health`, `source-code`, `confidential`, `untrusted`, `honeytoken`). The session's taint is the union of everything that entered it and is never dropped; it is the safety backstop. Fingerprints (exact hashes of structured identifiers, shingle hashes of confidential text) let an outbound action be matched back to the event that introduced the data, which lowers the false-positive rate and explains decisions; they never grant permission on their own.
- **Obot Policy.** One CEL vocabulary over `subject`, `action`, `session` and `org`, with effects `allow`, `log`, `redact`, `downshift`, `quarantine`, `judge`, `require_approval` and `deny`, each policy in `shadow` or `enforce` mode. Every surface compiles to the same engine call: `Engine.Decide` takes an `Action` and a `Budget` and returns a `Decision` the surface must honor. Built-in policies (honeytoken exfiltration, lethal trifecta, secrets never leave, PII lineage, destructive after untrusted content, warrant drift, killed session) are seeded in shadow mode except the two that have no false positives. Policies are simulated against recorded events before they are enforced.

Around the engine:

- **The Supervisor** is the AI that helps run Obot. Every judgement it makes is a bounded decision: a question, content-free context, and a fixed set of options (the shape of OpenAI's Decisions API, served by any model through Obot's own LLM gateway). It resolves `judge` effects on the hot path within the surface's budget, pre-reviews approvals, drafts policies from plain language and tests them against history, narrates sessions, and labels tools. Without a Supervisor, `judge` degrades to `require_approval`.
- **Interventions richer than allow/deny:** approvals that a person resolves (with the Supervisor's recommendation attached), a kill switch at org, user, agent or session scope that every surface honors on its next decision, quarantine and credential downshift flags on the session, redaction spans, and intent-bound warrants that attenuate through delegation and whose drift is a policy input rather than an automatic block.
- **Surfaces are adapters.** The LLM gateway plugins ingest new tool results and decide the tool calls the model proposes before the client sees them. The MCP gateway runs an always-on in-process filter on `tools/call`, `tools/list` and `resources/read`. The Sentry decision endpoint accepts session, tool-use and input fields and decides with session context after the allowlist. The Claude Environments egress processor decides each request and ingests responses as untrusted content. Copilot Studio's external runtime monitoring hook is served at `/api/copilot-studio/runtime-monitor` within its one-second budget.
- **Storage** is the gateway database (GORM), with the hot session state (labels, fingerprints, counters, warrant) kept in memory per replica and written through on every change. Reference data (policies, kills, honeytokens, tool labels, settings) is cached for five seconds, which bounds how long a change takes to apply on other replicas.

## Rationale

- **One engine rather than nine policy types.** The decision that matters spans surfaces, so it cannot live in any one of them. Making the existing policy types sugar over Obot Policy, rather than adding a tenth type, is what lets a rule written once hold on a laptop, at the gateway and in a sandbox.
- **Session taint plus fingerprints rather than either alone.** Taint cannot be defeated by paraphrase but is coarse; fingerprints are precise but are defeated by paraphrase. Using taint for safety and fingerprints only to reduce false positives and explain decisions gets both properties.
- **Content-free by default.** Storing context windows is a privacy liability. Labels, hashes and summaries are enough to decide and to explain; the surface-native audit logs (which already have role-gated content) remain the place to see payloads, and events link to them.
- **Shadow mode and simulation first.** Nothing is enforced that has not been replayed against history. This is what makes it safe to seed strong policies.
- **Bounded decisions for the Supervisor.** A model that can only pick from options Obot offered has narrow authority and a fixed output contract, which is what makes it acceptable on an enforcement path. Degrading to a human approval when the model is absent or too slow keeps the path fail-safe.
- **Per-replica state with write-through** rather than a shared low-latency store. Decisions must not query the graph on the hot path. Last-writer-wins on session state is acceptable because the labels only ever grow and counters are advisory.

## Consequences

- Every `tools/call` through the MCP gateway is now parsed and correlated, including servers with no filters configured, which adds a storage create and delete per request.
- The LLM gateway must disable raw passthrough and may hold streams when a request declares tools, so first-token latency rises for tool-using clients.
- Session state can diverge briefly between replicas; a label added on one replica is visible on another only after that replica reloads the session (on cache miss) or the store is read.
- Agents that do not propagate a session identifier are stitched by actor with an idle timeout, and sessions report their coverage so blind spots are visible rather than hidden.
- The Sentry hook contract has grown (session, tool-use ID, input, cwd); the obot-sentry repository must be updated to send them before device decisions carry session context.
- The Copilot Studio payload shape is from Microsoft's public description of a preview feature and may need adjusting against the real contract.
- The existing policy types still exist and still enforce; folding them into Obot Policy is follow-up work.

## References

- `pkg/provenance/`: engine, model, policy vocabulary, simulation, in-memory store
- `pkg/provenance/supervisor/`, `pkg/provenance/redteam/`: the Supervisor and the assurance harness
- `pkg/provenance/llm/`, `pkg/provenance/device/`, `pkg/provenance/copilot/`: surface helpers
- `pkg/guardrails/llmplugin.go`, `pkg/api/handlers/mcpgateway/provenance.go`, `pkg/api/handlers/enforcement.go`, `pkg/claudecredentials/audit.go`, `pkg/api/handlers/copilotstudio.go`: the five enforcement points
- `pkg/gateway/client/provenance.go`, `pkg/gateway/types/provenance.go`: persistence
- `pkg/api/handlers/provenance.go`: API
- `ui/user/src/routes/{supervision,sessions,policies,approvals}`: admin UI
- [Guardrails ADR](2026-09-26-guardrails.md), which this builds on for content checks
- `obot-moonshot.md`: the design narrative
