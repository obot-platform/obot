# 2026-11-12: Compress MCP audit log bodies and store list results once

- **Status:** Accepted
- **Date:** 2026-11-12
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#8418](https://github.com/obot-platform/obot/issues/8418)

## Related ODPs

- [2026-11-10: Deduplicate and compress MCP audit log bodies](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-11-10-audit-log-body-dedup-compression/README.md)

## Context

`mcp_audit_logs` was 95–99% of the database in active hosted environments. Almost
all of it was request and response bodies, and most of those bytes were list
responses (`tools/list` and similar) that repeat from one call to the next.
Bodies were encrypted before insert, so Postgres could neither compress them
nor share identical copies.

## Decision

Each audit body column holds its own format marker. A field written by this
version starts with a zero byte, which cannot begin JSON or base64, followed by
a version byte and flags for compressed, encrypted, and reference. Fields
without the marker are read as before. Bodies are zstd-compressed and then
encrypted, and stored as raw bytes rather than base64.

The `result` of a `tools/list`, `prompts/list`, `resources/list`, or
`resources/templates/list` response of at least 2 KiB is stored once, for the
whole install, in `mcp_audit_log_bodies`, keyed by an HMAC-SHA256 of its bytes.
The audit row keeps the rest of the JSON-RPC message and the hash, and readers
splice the result back in to reproduce the original bytes. The HMAC key is
generated on first use and kept in the `properties` table.

Each shared body records when it was last referenced. A write refreshes that
time when it is more than a day old, in the transaction that writes the
referencing row. Retention deletes shared bodies last referenced more than two
days before the audit row cutoff.

A body in the current format that cannot be read is omitted from the row and
named in `unavailablePayloads`, rather than failing the request.

## Rationale

The marker is per field because a response is added to a row by an `UPDATE`
that can come from a different replica, and during an upgrade a different
version, than the one that inserted the request.

Shared bodies are matched by content, so they need no MCP server or user scope
to be correct, and one copy serves every row with the same result. A shared
body is encrypted under a context containing its hash; the reference stays
inside the row's encrypted field, which keeps the row's existing context.

A last-referenced time avoids reference counts, which every batched retention
delete would have to maintain and which leak a body permanently on a missed
decrement, and avoids scanning the audit table for orphans. It is safe because
retention is the only code that deletes audit rows.

Alternatives are discussed in the ODP.

## Consequences

- Replicas on a version before this one cannot read bodies written by this
  one. During a rolling upgrade, and after a rollback until the upgrade is
  reapplied, payload requests for those rows fail on old replicas. Metadata is
  unaffected and no data is lost.
- Existing rows are not rewritten. Storage falls as they age out through
  retention.
- Any new code path that deletes audit rows, or inserts rows with a
  `created_at` earlier than the write, must preserve the cleanup rule above or
  shared bodies may be deleted while still referenced.
- Reading rows that reference shared bodies takes one extra query per page.
- Bodies are no longer readable with plain SQL on installs without encryption.
- Compression and deduplication thresholds are constants in
  `pkg/gateway/client/mcpauditlogbody.go`, not settings.

## References

- `pkg/gateway/client/mcpauditlogbody.go`
- `pkg/gateway/client/mcpauditlog.go`
- `pkg/gateway/types/mcpauditlog.go`
