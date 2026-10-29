// Package provenance is Obot's cross-surface session graph and information-flow
// policy engine.
//
// Obot observes an agent at four places: what enters a model's context through
// the LLM gateway, what the agent does with tools through the MCP gateway, what
// it does on a user's device through Obot Sentry hooks, and what leaves a
// hosted sandbox through the Claude Environments egress gateway. Each surface
// reports its activity here as an Event. Events are stitched into a Session,
// the session carries the labels of everything that has entered its context
// (its taint), and every outbound action is decided by Obot Policy, a CEL
// vocabulary over the subject, the action, the resource and the session.
//
// The package is split into three layers:
//
//   - The pure model and decision logic (event.go, labels.go, taint.go,
//     fingerprint.go, policy.go, decide.go) has no I/O. The engine's inputs are
//     an Action and a SessionState and its output is a Decision.
//   - Engine (engine.go) owns session state, records events through a Store and
//     evaluates policies. It is the single entry point the four surfaces call.
//   - Surface adapters (llm.go, mcp.go, device.go, egress.go) translate each
//     surface's native request into an Action and feed results back as
//     ingested content so the session's labels stay current.
package provenance
