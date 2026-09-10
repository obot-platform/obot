# 2026-09-10: Synchronize vMCP definitions independently

- **Status:** Accepted
- **Date:** 2026-09-10
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [#7981: Allow syncing vMCP definitions from remote or local catalogs](https://github.com/obot-platform/obot/issues/7981)

## Related ODPs

- [Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md)

## Context

The virtual MCP design requires independent Git synchronization without exposing running endpoints to implicit catalog snapshot changes or deleting definitions still used by instances.

## Decision

Store vMCP sources and sync status in a separate `VMCPCatalog` resource. Reuse MCP catalog source reading and credential resolution, with a strict vMCP-only source schema. The default catalog is seeded once from a comma-delimited configuration value, whose default is empty.

Identify each vMCP by its catalog, source, and display name. Component references and profile tool maps use `sourceURLOrPath::entryKey`. Resolve these against synced catalog entries and derive internal component IDs from the full reference. This preserves the existing credential key encoding, which cannot accept periods in component IDs.

Resolve references on every sync, but refresh snapshots and fixed configuration only when the vMCP definition changes. Unresolved or ambiguous references are sync errors; unknown configuration and tool names are allowed. Store fixed values through the existing vMCP credential mechanism.

Reject API mutation of source-managed definitions. When a definition disappears, delete it only if it has no instances; otherwise retain it as detached and permit ordinary API editing and deletion. Detached definitions are not pruned on later syncs. Reintroducing a definition restores source management. Catalog finalization applies the same removal rules.

## Rationale

Separate catalogs prevent source schema confusion. Definition digests make catalog drift observable without deploying it automatically. Stable component identities retain configuration across label changes. Detachment preserves existing connections while returning management to the administrator.

## Consequences

Renaming a vMCP in its source creates a new identity. Component references must be unique within a vMCP. Source errors preserve missing definitions until a complete sync succeeds; catalogs retry failures so initial MCP catalog synchronization can finish first.

## References

- [Virtual MCP architecture](2026-09-01-virtual-mcps.md)
- [Sync implementation](../pkg/controller/handlers/mcpcatalog/vmcp.go)
