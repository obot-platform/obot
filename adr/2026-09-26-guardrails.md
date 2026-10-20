# 2026-09-26: Guardrails as Obot-owned checks in the LLM gateway and MCP gateway

- **Status:** Accepted
- **Date:** 2026-09-26
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [#8248: Unified LLM gateway with budgets and guardrails](https://github.com/obot-platform/obot/issues/8248)

## Related ODPs

There is no ODP for this decision.

## Context

The engine embedded as Obot's LLM gateway ([ADR](2026-09-26-embedded-llm-gateway.md)) has guardrails only in its Enterprise edition. They cover rules with CEL conditions, sampling, integrations with external services (AWS Bedrock Guardrails, Azure Content Safety, Google Model Armor, Lakera, Patronus, CrowdStrike and others), regex and secrets detection, and actions to block, redact or log. Obot already had message policies: natural-language rules checked by an LLM judge in its own LLM proxy. Obot's MCP gateway had configurable MCP hooks but nothing that inspects content.

## Decision

Obot implements guardrails itself (`pkg/guardrails`) and applies the same guardrails (each one configured check) to LLM gateway traffic, through guardrail policies, and to MCP traffic, through guardrail filters. Early drafts called these detectors and guardrail rules.

- **Data model.** Guardrail policies and guardrails live in Obot's database (`guardrail_policies`, `guardrails`). Guardrail credentials are kept in the encrypted credential store and are write-only through the API. There is no separate store of findings: what guardrail policies did is recorded on the request's LLM audit log entry, and what guardrail filters did is recorded as the filter's status in the MCP audit log, both without the offending content. A guardrail policy belongs to a gateway profile (see [the gateway ADR](2026-09-26-embedded-llm-gateway.md)) and applies to its users; a policy without a profile applies to no one.
- **Checkers are pluggable.** Each guardrail type registers a checker that reports findings and, where it can, the spans to redact. There are three built-in checkers (secrets, regex and PII, and an LLM judge) and integrations with eight external services. A redacting policy whose checker can't locate a finding blocks instead.
- **LLM gateway: two gateway engine plugins.** An input plugin is registered before the engine's built-in plugins and an output plugin after them, so the engine's request logging only ever sees content after it has been checked. They are registered as part of the engine's startup, so the gateway never serves a request without them, and the admin API can't disable them. When a guardrail applies, the plugins turn off the engine's raw passthrough (used for Claude Code), so the provider receives the checked request rather than the client's original body. Token counting requests are checked like any other input.
  - **Streams.** The output plugin holds a stream while any guardrail policy that can change it applies. It blanks the deltas as they pass through logging, checks the complete response, then rewrites the held chunks, either redacted or ended with an error.
  - **Identity.** Obot's proxy passes the caller's identity to the plugins through a per-request token that maps to an in-process registry. It is not sent in a header anyone can forge.
- **MCP gateway: guardrail filters.** Guardrails apply to MCP traffic as a type of MCP filter (`MCPWebhookValidation` with a `guardrail` configuration) alongside webhook, custom MCP server and built-in filters. A guardrail filter uses the same server targeting and selectors, runs in the same filter chain in the same order (by resource name), and is audited like other filters. It runs in-process: Obot creates no system MCP server, container or deployment for it, and the hook processor dispatches it to the guardrails engine instead of the hook runner. A block is a rejection in the chain and a redaction is a mutation, so later filters see the redacted message. It checks `tools/call` arguments and results, `resources/read` contents and `prompts/get` arguments and messages, and records its outcome as the filter's status in the MCP audit log, including, when the filter fails open, which guardrail failed and why. It can block or redact but not log only, since the MCP audit log has no place for findings that changed nothing. Guardrail policies apply to LLM traffic only.
- **Message policies are folded in.** A message policy is an LLM judge guardrail used by a blocking guardrail policy. Existing policies are converted once at startup. The message policy API, and the legacy per-provider LLM routes that enforced message policies, are removed.

## Rationale

- **Build rather than buy Enterprise.** The policy and guardrail model is small. The value is in the integrations and the enforcement points, and Obot needs those in the MCP gateway too, which the gateway engine doesn't cover.
- **Plugins inside the gateway engine rather than checks in Obot's proxy.** Only inside the engine can input be redacted before request logging and before fallbacks, retries and caching see it, and streamed output be held and rewritten in the engine's own chunk format for every API dialect.
- **One engine for both gateways.** Guardrails and conditions are shared, so an administrator configures, for example, secrets detection once for models and tools.
- **MCP guardrails as filters rather than a separate step.** A separate guardrail step after the filter chain had its own server targeting (CEL over the server's display name), covered only tool calls, and appeared in audit logs as a fake hook. As a filter type, MCP guardrails share the targeting, ordering, auditing and UI that administrators already use for MCP filters. Running them in-process, rather than as a filter server, avoids deploying a container to call code that is already in Obot.

## Consequences

- Guardrail policies can block or redact streamed responses only by holding them, which delays the first token until the response is complete. Log-only policies don't hold streams.
- External checkers add latency and cost to every guarded request. Sampling, conditions and fail-open (the default) limit the impact. Their request and response formats were written from vendor documentation and tested against mocks, not live services.
- The plugins depend on the gateway engine's plugin ordering and stream-pausing APIs, which are not a stable contract.
- MessagePolicy resources remain, read only by the one-time conversion, and past violation records stay in their table with no API to read them. Removing both is follow-up work.

## References

- `pkg/guardrails/`: engine, checkers, LLM gateway plugins, MCP checks, message policy conversion
- `pkg/api/handlers/mcpgateway/guardrails.go`, `proxy_hooks.go`: guardrail filters in the MCP filter chain
- `apiclient/types/mcpwebhookvalidation.go`: guardrail filter configuration
- `pkg/api/handlers/guardrails.go`: API
- `ui/user/src/routes/models/` and `ui/user/src/routes/mcp-servers/`: admin UI (the shared Guardrails tab and guardrail pages, guardrail policies in profiles, guardrail filters)
- [Engine documentation: Enterprise guardrails](https://docs.getbifrost.ai/enterprise/guardrails)
