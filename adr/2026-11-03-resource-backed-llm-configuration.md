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
including operation when encryption is disabled. Reads accept plaintext credential
envelopes even when encryption is enabled, to support records written before it
was enabled. Enabling encryption does not rewrite existing records; subsequent
persisted changes use the current policy.

Routing rules are non-secret configuration stored as plaintext JSON without
credential-envelope identity binding. Callers must not put credentials in CEL
expressions or header-match literals. The resource spec owns rule identity;
embedded payload IDs are ignored and target IDs are restored on read.

Provider aggregates may have zero keys. Key validation enforces nonempty, unique
IDs within the provider; provider-specific authentication validation belongs to
callers and the runtime. The upstream key value may be empty when authentication
uses provider-specific fields or ambient credentials.

Register resource watches on every replica. Their reconciliation handler is an
explicit placeholder until the core-engine follow-ups connect runtime application.
The existing transport engine retains its current configuration mechanism.

## Rationale

Resources reuse Obot's existing storage and watch infrastructure. Grouping keys
with their provider makes destination changes and confirmed key removal atomic
without requiring whole-configuration writes or a separate credential lifecycle.

## Consequences

Cross-resource reads are eventually consistent and do not form a transactional
snapshot. Deleting a provider retains routing rules that target it. Runtime
integration must tolerate these dangling references, keep healthy providers
serving, and report missing dependencies with bounded/backoff retries. Affected
requests fail or use their configured fallbacks; missing targets must not silently
disable or bypass routing policy.

Changes to these resources do not affect the running gateway until that integration
lands. The child PR that connects runtime application must update the placeholder
description above to reflect the behavior it implements. No migration from the
unmerged configuration-table draft is provided.

## References

- `pkg/llmcore/client`
- `pkg/storage/apis/obot.obot.ai/v1/llmgateway.go`
- `pkg/controller/everyreplicaroutes.go`
