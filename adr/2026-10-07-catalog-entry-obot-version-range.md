# 2026-10-07: Gate catalog entries by Obot version at catalog sync time

- **Status:** Accepted
- **Date:** 2026-10-07
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8172

## Related ODPs

None.

## Context

Every Obot version that points at a catalog source reads the same catalog. An entry
that depends on newer Obot functionality, such as a built-in filter with custom UI,
was offered by older Obot versions that could not support it. Catalog authors needed
a way to declare which Obot versions an entry applies to.

## Decision

MCP catalog entries and system catalog entries accept optional `minObotVersion` and
`maxObotVersion` fields. Both bounds are inclusive full release versions with a
leading `v`, and an omitted bound is unrestricted. Development builds (`v0.0.0-...`)
ignore the bounds.

The bounds are enforced when a catalog source is synced. The range is checked before
any other validation of an entry, and an entry outside the running version's range is
skipped as if it were absent from the source. A vMCP whose component references a
skipped entry, in any source, is skipped the same way. Neither produces a sync error.
Catalogs re-sync on startup whenever the Obot version changes.

The fields are rejected on entries that are not synced from a catalog source, because
nothing enforces them there.

## Rationale

Skipping entries at sync time puts one gate in front of every consumer: the catalog
APIs, the MCP Registry API, access control, server and filter creation, and search.
Filtering at request time would need the check in each of those handlers, and a
missed one would expose an entry that should be unavailable. The running version only
changes on restart, so a startup sync keeps sync-time filtering current.

Checking the range before other validation keeps catalogs forward compatible. An entry
for a newer Obot version can use runtimes or fields that this version does not
understand without failing the sync.

Explicit `minObotVersion` and `maxObotVersion` fields were chosen over a range
expression such as `>=v0.21.0 <v0.23.0` because they need no operator syntax and are
easy for catalog authors to read and validate.

## Consequences

- An entry that leaves the supported range is handled like an entry removed from its
  catalog source. Existing servers and filters keep running, but an entry that is in
  use is detached rather than hidden, and an existing filter whose system entry is
  gone cannot be edited.
- Obot versions released before this decision ignore the fields and show gated
  entries. Gating applies only from the first release that includes it.
- Publishing multiple versions of the same entry for different Obot version ranges is
  not supported. The catalog validation CLI rejects the duplicate entry keys and names
  this would need.
- vMCP catalog items do not support version bounds.

## References

- `pkg/version/range.go`
- `pkg/controller/handlers/mcpcatalog/mcpcatalog.go`
- `pkg/controller/handlers/mcpcatalog/vmcp.go`
