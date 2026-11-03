# 2026-11-03: Store LLM gateway configuration as Obot resources

- **Status:** Accepted
- **Date:** 2026-11-03
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

None for this storage alternative.

## Context

The core gateway needs configuration persistence owned by Obot. The initial storage
approach used configuration tables and a shared counter, with whole-configuration
mutations. Providers are independently editable, while a provider's settings and
keys must change together when an administrator changes its destination.

## Decision

Store provider/key aggregates, individual routing rules, and singleton global
settings as Obot resources. Use resource versions for optimistic concurrency and
scope each write to its owning resource. Require the gateway credential codec so
provider payloads and global settings follow Obot's configured encryption policy,
including operation when encryption is disabled.

Register resource watches on every replica. Their reconciliation handler is an
explicit placeholder until the core-engine follow-ups connect runtime application.
The existing transport engine retains its current configuration mechanism.

## Rationale

Resources reuse Obot's existing storage and watch infrastructure. Grouping keys
with their provider makes destination changes and confirmed key removal atomic
without requiring whole-configuration writes or a separate credential lifecycle.

## Consequences

Cross-resource reads are eventually consistent. Runtime integration must tolerate
missing dependencies, preserve routing policy, and retry incomplete application.
Changes to these resources do not affect the running gateway until that integration
lands. No migration from the unmerged configuration-table draft is provided.

## References

- `pkg/llmcore/client`
- `pkg/storage/apis/obot.obot.ai/v1/llmgateway.go`
- `pkg/controller/everyreplicaroutes.go`
