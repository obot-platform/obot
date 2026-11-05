# Obot Provenance follow-up: third-party integrations

Status: proposed follow-up. The presentation describes these as **planned**. No third-party enforcement adapters or telemetry importers have been implemented.

The current implementation integrates five built-in surfaces: LLM gateway, MCP gateway, device hooks, Claude Environments egress, and Copilot Studio. The goal is to let customers retain gateways and endpoint/network tools they already operate while using Obot's session graph and decision engine.

## Enforcement adapters

Start with Envoy `ext_authz`, then Kong and other agent gateways. An adapter should:

- Authenticate as a scoped integration identity and supply an attributable subject, normalized action/resource, and verified session identifier.
- Call the common decision engine within an explicit deadline and honor its decision before allowing the relevant action to proceed.
- Document which effects it can enforce. Unsupported effects such as redaction, credential changes, or quarantine must have explicit behavior rather than silently succeeding.
- Define failure behavior, retries/idempotency, and how a human approval resumes the exact held action without authorizing a different request.
- Carry causal identifiers into the event record and report enforcement coverage honestly.

This is a new adapter around the existing engine, not a claim that the current internal Go interfaces are a supported public API.

## Telemetry ingestion

Import events from EDR, SASE, and external agent runtimes to fill observation gaps:

- Version and validate the event envelope, source identity, timestamps, causal references, and session correlation fields.
- Treat external labels and identities according to source trust. A client-supplied session ID or sensitivity label must not let one principal alter another principal's session.
- Deduplicate replayed events and preserve attribution to the original telemetry source.
- Preserve content minimization and bounded payload sizes.
- Display observed, detected, and enforced coverage separately. Ingesting telemetry alone does not give Obot permission or ability to stop an action.

## Before implementation

Follow the repository's ODP process before introducing this public integration contract. The proposal should define authentication and authorization, correlation guarantees, API/version compatibility, deadline/failure semantics, approval handoff, retention, and deployment topology. Record the accepted design and shipped differences in an ADR when implemented.

Contract tests should cover identity isolation, duplicate/out-of-order events, missing or spoofed correlation IDs, timeouts/unavailable Obot, unsupported effects, policy/kill propagation, and approval resume. A live integration test must show that the gateway blocks the real operation, rather than only returning a deny decision in an engine test.

## Suggested first milestone

A versioned decision endpoint plus one Envoy `ext_authz` adapter, with a small reference client and end-to-end tests. Add the event ingestion endpoint separately, so visibility and enforcement can be evaluated independently.
