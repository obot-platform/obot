# 2026-11-03: Recover legacy model-provider imports through resource reconciliation

- **Status:** Superseded
- **Date:** 2026-11-03
- **Supersedes:** None
- **Superseded by:** [Explicit shared pools for legacy cloud providers](2026-11-05-shared-legacy-provider-pools.md)

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

None. This extends the resource-backed configuration decision; Calvin approved the migration plan in this implementation session.

## Context

v0.27 installations have independently configured model providers. Importing settings and keys separately can leave incomplete providers after a crash. Azure and Azure Entra, and the two Bedrock authentication variants, can refer to different accounts, endpoints or regions. Merging their keys changes routing and policy meaning. The pinned engine does not support independently named Azure or Bedrock Mantle instances.

## Decision

Create each imported provider, its keys and source provenance in one codec-protected `LLMGatewayProvider` aggregate. Persist administrator selections, source outcomes and old model-ID mappings in the existing `LLMGatewayConfig` singleton, with resource-version conflict retries. Aggregate provenance is the recovery evidence when the separate outcome write has not completed. Resource callbacks have no external side effects.

When both variants of a cloud family are configured, import neither until an administrator explicitly chooses one. Persist that decision before creating the provider; retain the other source's credentials, models and references for manual setup. Existing gateway providers are never overwritten by an import. Environment overrides retain their existing precedence.

Reconciliation runs on startup and gateway resource events on every replica. Cross-resource visibility is eventually consistent: temporary gaps preserve source data, and retries converge. Only verified imports with discovered target models can move references and remove source records. Copy administrator edits once, persist compatibility mappings, then update references before cleanup.

Retained proxy routes normalize supported v0.27 request paths and bind model resolution and listings to the source provider. They use the existing gateway policy, metering and audit pipeline. Requests for unselected or unsupported configurations return actionable errors rather than being rerouted to another account.

## Consequences

Cloud conflicts can require administrator intervention during upgrade. Retained source records are recovery material, not a running legacy backend. Named cloud-instance support and migration of unreleased gateway stores are outside this change. Rolling back still requires a pre-upgrade database backup.

## References

- [Resource-backed LLM configuration](2026-11-03-resource-backed-llm-configuration.md)
- `pkg/llmengine/migration.go`
- `pkg/gateway/server/llmproxy_legacy.go`
