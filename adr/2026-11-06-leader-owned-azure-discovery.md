# 2026-11-06: Refresh Azure deployments in the elected controller

- **Status:** Accepted
- **Date:** 2026-11-06
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None. This reuses the existing controller and resource storage boundaries.

## Context

Azure's inference endpoint lists model offerings rather than deployed models. ARM
results must survive restarts and be shared by replicas without making public
model listings depend on management-plane availability.

## Decision

The existing leader-elected ModelProvider controller refreshes Azure deployments
before back-populating models. It processes keys sequentially, with a separate
30-second timeout per key. Other providers retain their existing refresh paths.

Public model listings read successful snapshots from the provider aggregate.
Explicit Azure refresh requests enqueue the existing model sync and return HTTP
202 with `status: pending`; they do not wait for ARM. API configuration writes and
the existing startup/15-minute model sync schedule also trigger discovery. Direct
storage edits apply on every replica immediately through its resource watch;
Azure discovery follows on the next model sync.

Every replica's configuration watcher continues applying runtime configuration,
including snapshot changes, but does not enqueue model sync. This prevents
snapshot publication from recursively refreshing Azure.

## Rationale

The elected router already serializes reconciliation of the gateway ModelProvider.
Its workers inherit the leadership context. The pinned nah leader elector cancels
that context and terminates the process on unexpected leadership loss.

Cancellation alone cannot fence delayed writes. Discovery captures the provider's
resource version before ARM and conditionally publishes successful keys against
that version. A concurrent configuration edit or newer publication rejects the
write. Conflicts return to the controller for a fresh fetch; stale responses are
never retried against a newer version. A refresh ID makes even identical or empty
successes distinct publications, protecting against storage no-op updates.

## Consequences

Failures retain the last successful credential/account-matching snapshot. Empty
successes remove deployments. Fingerprints still invalidate snapshots when
credentials or account fields change. Entra and ARM retain the same effective
provider/global proxy configuration.

No per-request generation reservations or discovery annotations are required.
Snapshots remain in the existing encrypted provider payload, and each replica
continues enforcing deployment eligibility independently for inference.

## References

- [Resource-backed configuration](2026-11-03-resource-backed-llm-configuration.md)
- [Controller registration](../pkg/controller/routes.go)
- [Azure discovery](../pkg/llmengine/azurediscovery.go)
- [Conditional publication](../pkg/llmcore/client/providers.go)
