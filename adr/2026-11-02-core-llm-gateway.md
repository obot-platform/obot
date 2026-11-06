# 2026-11-02: Run the LLM gateway on Bifrost core

- **Status:** Accepted
- **Date:** 2026-11-02
- **Supersedes:** [Embedded LLM gateway](2026-09-26-embedded-llm-gateway.md)
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

None.

## Context

Obot used the complete Bifrost HTTP transport while disabling most of its features. Its configuration database, reloads, retired instances, and cross-replica advisory locks complicated deployment and configuration ownership. Craig's spike established that core supports the required inference formats and Obot policy plugins directly.

## Decision

Obot owns HTTP inference and management handling and embeds Bifrost core. Providers, keys, rules and settings use [Obot resource storage](2026-11-03-resource-backed-llm-configuration.md). Each provider and its keys form one atomic resource; routing rules are independent resources and global settings use a singleton. Provider and settings documents use Obot's credential encryption policy, including its behavior without a configured encryption provider. No separate gateway encryption key is generated.

Resource versions protect scoped updates from concurrent edits. Every-replica resource watches reload desired state and synchronize the local engine; there is no configuration polling loop or aggregate revision. Cross-resource reads are eventually consistent. Rules remain active when dependencies disappear, affected requests fail instead of bypassing routing, and incomplete application is retried while healthy providers continue serving. Successful provider updates are tracked individually so retries do not restart them. Model discovery is triggered after successful reconciliation and retried if its trigger fails.

Management writes are asynchronous:

- A successful management API response means the new configuration was saved to Obot storage. It does not mean every replica is already serving it. Each replica's controller applies the saved configuration to its local engine afterwards and retries until it succeeds.
- If applying fails on a replica, the error appears in that replica's status. It does not turn the already-saved write into an HTTP error.
- Internal setup and import code that needs the new configuration active right away calls `Engine.Sync` to apply it immediately instead of waiting for the controller.

  Code boundaries:

- Management operations are plain Go methods on `Engine`. Obot's public API layer handles HTTP routing, request restrictions and response formatting. Bifrost does not serve management HTTP.
- Client inference requests pass through Obot's access, budget, guardrail and response handling, then go directly to `Engine.ServeHTTP`.
- Obot's own trusted message-policy LLM calls skip HTTP and call the engine directly in Go. They still go through the same core normalization, routing and plugin chain.

Chat Completions, Responses and Messages remain public client formats, with translations provided by core. Obot retains access checks, budgets, guardrails, model synchronization, audit logging and pricing from Obot's model-data files. The framework in-memory tracer provides the pause/replay gate required for output guardrails and stops with the engine. Routing metadata reaches the proxy through request context callbacks before client sanitization.

## Rationale

Core provides inference, translation, retries and routing without the transport's database and lifecycle. Keeping the `llmengine` integration interface preserves internal consumers while replacing its implementation. Retaining Chat Completions avoids breaking internal callers and compatible clients.

## Consequences

Obot maintains streaming and management API compatibility directly. Core's internal API remains an upgrade constraint. The cutover is coordinated, without mixed transport/core replicas or a runtime flag. Unreleased gateway configuration must be re-entered; old gateway stores are left untouched. Automatic legacy credential import, model-reference migration and cleanup are disabled in v0.28.0. Migration support is planned for a future patch release; existing LLM gateway users should hold off on updating. Historical audit data needs no migration.

## References

- `pkg/llmcore`: core engine and configuration persistence
- `pkg/llmengine`: lifecycle, administration, model catalog and HTTP integration
- `pkg/gateway/server/llmproxy_engine.go`: policy and audit integration
