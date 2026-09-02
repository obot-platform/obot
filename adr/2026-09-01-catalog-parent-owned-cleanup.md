# 2026-09-01: Clean up catalog children from the parent

- **Status:** Accepted
- **Date:** 2026-09-01
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Catalog entries and catalog-scoped MCP servers used generic deletion references to watch their
parent catalog. The controller's dependency tracking reacts to every parent update, including
hourly sync-status updates. A catalog with hundreds of entries therefore reconciled every child at
once even when the catalog contents had not changed, creating bursts of database work and
connection-pool contention.

Source revision or desired-object hashes could avoid some scheduled applies, but using source state
alone as the reconciliation gate would stop the controller from repairing cluster-side drift and
pruning orphaned entries.

## Decision

Catalog children no longer register generic deletion references to their parent catalog. MCPCatalog
and SystemMCPCatalog finalizers instead list and delete their children when the parent itself is
deleted. Hourly catalog reconciliation, apply, drift repair, and system-catalog pruning remain
unchanged.

## Rationale

Parent-owned cleanup preserves deletion behavior while removing the dependency edge that turns an
unrelated catalog status write into hundreds of child reconciliations. It addresses the observed
fan-out without making source immutability an assumption of the reconciliation model.

## Consequences

Catalog deletion cleanup is now explicit in the catalog controller and must include any new child
type that previously would have referenced a catalog through generic cleanup. In return, catalog
status and annotation updates no longer requeue every catalog entry or catalog-scoped MCP server.

## References

N/A.
