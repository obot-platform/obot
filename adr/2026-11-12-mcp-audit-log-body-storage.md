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

**Field format.** Each audit body column carries its own header: a zero byte,
which cannot begin JSON or base64, then a version byte and flags for
compressed, encrypted, and reference. Fields without the header are read as
before. A body is zstd-compressed when that makes it smaller, then encrypted
when a provider is configured, and stored as raw bytes. The version and flags
are part of the encryption context. Compression uses a 1 MiB window and at
most four pooled encoders and decoders, which bounds the memory they keep for
the life of the process. JSON gains little from longer back-references.

**Shared list results.** The `result` of a `tools/list`, `prompts/list`,
`resources/list`, or `resources/templates/list` response of at least 2 KiB is
stored once for the whole install in `mcp_audit_log_bodies`, keyed by an
HMAC-SHA256 of its bytes and of whether the writer encrypts. The audit row
keeps the rest of the JSON-RPC message and the hash, and readers splice the
result back in to reproduce the original bytes.

**Using a stored copy.** A writer reads a stored copy before referencing it,
and again about every twelve hours. If it cannot read the copy, or the copy is
not encrypted exactly when the writer encrypts, it stores the response inline.
When nothing is stored, the writer encodes the result once for the next write
to store. Until that write, later captures of the same result reference it
without looking it up or encoding it again.

**Refresh and retention.** A write refreshes a shared body's last-referenced
time when it is more than a day old, in the transaction that writes the
referencing row. A stored copy is never rewritten. Retention deletes shared
bodies last referenced more than two days before the audit row cutoff.

**HMAC key.** Generated on first use and kept in the `properties` table. A
replica that encrypts properties replaces it when it is stored unencrypted. A
key that cannot be read is left alone. When audit logs are encrypted but
properties are not, the key would be stored in the clear and would let a
database reader confirm a guessed result against the stored hashes, so list
responses are stored inline instead.

**Unreadable bodies.** A body in the new format that cannot be read is omitted
from the row and named in `unavailablePayloads`, rather than failing the
request.

## Rationale

- The header is per field because a response is added to a row by an `UPDATE`
  that can come from a different replica, or during an upgrade a different
  version, than the one that inserted the request.
- Shared bodies are matched by content, so they need no MCP server or user
  scope, and one copy serves every row with the same result.
- Hashing the writer's encryption mode keeps writers that encrypt and writers
  that do not from sharing a body, so enabling encryption stores a new,
  encrypted copy.
- A writer checks a copy before referencing it because each reference keeps
  retention from removing the copy. Referencing an unusable one would make
  every later row with that result unreadable.
- A copy or key the writer cannot read is never overwritten, so a replica
  holding the wrong key cannot damage what the others depend on.
- A last-referenced time avoids reference counts, which leak a body on a
  missed decrement, and avoids scanning the audit table for orphans.
- The header in the encryption context stops stored data being reinterpreted
  when the provider authenticates its context. It is not protection against
  someone who can write to the database.

Alternatives are discussed in the ODP.

## Consequences

- **Upgrade and rollback.** Replicas on an earlier version cannot read bodies
  written by this one. During a rolling upgrade, and after a rollback until
  the upgrade is reapplied, list pages and audit log exports that include
  payloads fail on old replicas if they contain any row written by the new
  version. This includes one-time and scheduled exports run by an old
  replica, which are marked failed. Views without payloads are unaffected and
  no data is lost.
- **Existing rows** are not rewritten. Storage falls as they age out.
- **Key rotation is not supported.** Shared bodies and the HMAC key stay
  under the encryption key they were written with, like other encrypted data
  in Obot. A shared body that keeps being referenced is never re-encrypted, so
  removing an old key makes the rows that reference such a body show it as
  unavailable, and later rows store that result inline until the old copy ages
  out.
- **Lost HMAC key.** If the encryption key the HMAC key is stored under is
  lost, list responses are stored inline until the
  `mcp-audit-log-body-hash-key` row is deleted from `properties`. Replicas
  then create a new key within about a minute.
- **Cleanup depends on** every row that references a shared body being
  written through the audit write path, nothing but retention deleting shared
  bodies, and a row's `created_at` plus clock skew between replicas running
  less than a day ahead of its write. Copying rows with SQL, or restoring
  `mcp_audit_logs` without `mcp_audit_log_bodies`, can leave rows whose bodies
  are unavailable.
- **Unusable copies** are only removed by retention. With retention disabled
  they stay, and matching responses are stored inline.
- **A narrow window remains.** A copy another replica stores between a
  response being captured and its row being written is referenced unread. If
  the writer cannot use it, those rows' list responses are unavailable to it.
- **Cost.** Reads take one extra query per page that references shared
  bodies. Capturing a list response reads the stored copy at most once every
  twelve hours per replica, or every ten minutes for a copy found unusable. A
  result that is not stored yet is looked up and encoded once per write that
  stores it, not once per capture. A list result of at least 2 KiB that
  differs on every response, such as a paginated or changing `resources/list`,
  still costs one lookup when captured and one single-row upsert when written.
- Thresholds and intervals are constants in
  `pkg/gateway/client/mcpauditlogbody.go`, not settings.

## References

- `pkg/gateway/client/mcpauditlogbody.go`
- `pkg/gateway/client/mcpauditlog.go`
- `pkg/gateway/types/mcpauditlog.go`
