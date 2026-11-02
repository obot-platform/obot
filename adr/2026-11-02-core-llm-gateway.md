# 2026-11-02: Run the LLM gateway on Bifrost core

- **Status:** Accepted
- **Date:** 2026-11-02
- **Supersedes:** [Embedded LLM gateway](2026-09-26-embedded-llm-gateway.md)
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

- Bifrost core gateway proposal — local `obot-design-proposals` branch `bifrost-core-gateway`, document `proposals/2026-11-02-bifrost-core-gateway/README.md`. Calvin explicitly authorized implementation before proposal merge. Add its permanent link when published.

## Context

Obot used the complete Bifrost HTTP transport while disabling most of its features. Its configuration database, reloads, retired instances, and cross-replica advisory locks complicated deployment and configuration ownership. Craig's spike established that core supports the required inference formats and Obot policy plugins directly.

## Decision

Obot owns HTTP inference and management handling and embeds Bifrost core. Providers, keys, rules and settings live in `llm_core_*` tables in Obot's primary database. Complete provider and key documents use Obot's credential encryption policy, including its behavior without a configured encryption provider. No separate gateway encryption key is generated.

Writes advance a singleton revision in the same transaction. PostgreSQL readers use repeatable-read snapshots. Every replica polls the revision every five seconds; local writes also synchronize immediately. Provider and rule changes apply in place; failed updates remain retryable and do not acknowledge the revision.

Chat Completions, Responses and Messages remain public client formats, with translations provided by core. Obot retains access checks, budgets, guardrails, model synchronization, audit logging and pricing from Obot's model-data files. The framework in-memory tracer provides the pause/replay gate required for output guardrails and stops with the engine. Routing metadata reaches the proxy through request context callbacks before client sanitization.

## Rationale

Core provides inference, translation, retries and routing without the transport's database and lifecycle. Keeping the `llmengine` integration interface preserves internal consumers while replacing its implementation. Retaining Chat Completions avoids breaking internal callers and compatible clients.

## Consequences

Obot maintains streaming and management API compatibility directly. Core's internal API remains an upgrade constraint. The cutover is coordinated, without mixed transport/core replicas or a runtime flag. Unreleased gateway configuration must be re-entered; old gateway stores are left untouched. Legacy model-provider credential import remains supported. Historical audit data needs no migration.

## References

- `pkg/llmcore`: core engine and configuration persistence
- `pkg/llmengine`: lifecycle, administration, model catalog and HTTP integration
- `pkg/gateway/server/llmproxy_engine.go`: policy and audit integration
