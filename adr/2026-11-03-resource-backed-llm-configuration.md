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
scope each write to its owning resource. Provider creation uses CreateProvider and
returns AlreadyExists rather than replacing a concurrent winner. Existing
aggregates are edited with MutateProvider. Each mutation attempts one
read/change/write cycle and returns conflicts to the caller. Callers can explicitly
retry the whole operation to re-read the state used for their decisions. There is
no whole-provider replacement API.

Require the gateway credential codec so provider payloads and global settings follow Obot's configured encryption policy,
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

Load returns a LoadResult containing decoded Config and per-provider diagnostics.
Decode/decryption failure quarantines the provider and all its keys; healthy
providers remain usable. Diagnostics distinguish unavailable resources from
resources observed missing. Collection-read failures and unreadable routing rules
or global settings return a blocking error and no usable result.

Engine integration must allow degraded startup with healthy providers and expose
provider diagnostics through replica-local status. Refresh must disable any prior
runtime configuration for a quarantined provider, without deleting its stored
resource. Apply healthy changes before returning diagnostic errors for backoff
retries, including when desired contents have not changed. Repairing the resource
makes it eligible for application again. Blocking load errors fail startup or
preserve the running configuration during refresh.

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

### Bifrost upgrade compatibility

Provider/key payloads and routing rules use Bifrost JSON types, and global settings
are interpreted by the engine using Bifrost configuration types. Dependency updates
therefore require a stored-configuration compatibility review, including updates
opened by Dependabot. Compilation alone cannot detect fields silently ignored by
JSON decoding or changes to defaults and runtime semantics.

For each Bifrost dependency upgrade:

- Review changes to JSON field names and types, custom serialization, defaults,
  and configuration semantics across the affected Bifrost modules.
- Exercise representative configuration saved by the previous version through
  load, edit, and save. Check providers, keys, rules, and global settings for
  silently dropped fields, changed values, and changed effective behavior.
- If compatibility changes, choose a migration, compatibility decoder, or adapter
  based on the actual change. Document that decision and add regression fixtures
  for the affected previous-version payloads.

Not every dependency update requires a migration. Preserve the current storage
model unless a concrete upstream change justifies a different approach, and record
the compatibility assessment in the dependency-update PR before merging.

## References

- `pkg/llmcore/client`
- `pkg/storage/apis/obot.obot.ai/v1/llmgateway.go`
- `pkg/controller/everyreplicaroutes.go`
