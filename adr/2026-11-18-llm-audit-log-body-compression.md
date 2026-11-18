# 2026-11-18: Compress LLM audit log bodies

- **Status:** Proposed
- **Date:** 2026-11-18
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#8523](https://github.com/obot-platform/obot/issues/8523)

## Related ODPs

None. This applies the body format accepted in
[2026-11-12: Compress MCP audit log bodies and store list results once](2026-11-12-mcp-audit-log-body-storage.md)
to LLM audit logs.

## Context

LLM audit rows keep full request and response bodies of up to 5 MiB each. These
are JSON conversations that grow with every turn and compress well. Bodies are
encrypted before insert when a provider is configured, so Postgres cannot
compress them. MCP audit bodies had the same problem, which #8528 addressed.

## Decision

**Field format.** `request_body`, `policy_modified_request_body` and
`response_body` in `llm_audit_logs` use the MCP audit body field format and
code: a zero byte, a version byte and flags, then the body, zstd-compressed when
that makes it smaller and encrypted when a provider is configured. The header is
part of the encryption context. Headers stay under the existing per-field
encryption. Fields without the header are read as before.

**Shared coders.** LLM and MCP audit bodies share one pool of at most four zstd
encoders and decoders with a 1 MiB window, so the memory they keep does not
grow.

**No shared bodies.** LLM bodies are always stored inline. A field carrying the
reference flag is treated as unreadable.

**Unreadable bodies.** A body in the new format that cannot be read is omitted
from the row and named in `unavailablePayloads`, as for MCP audit logs, rather
than failing the request or export.

## Rationale

- Reusing the MCP format keeps one codec, one set of size limits and one
  compatibility story for all audit bodies.
- The header is per field, so rows written before and after the change can be
  read side by side without a migration. The columns are already `bytea`.
- Deduplicating list results works for MCP because identical results repeat.
  LLM requests repeat only in part, such as a growing conversation history,
  tool definitions or a system prompt, so whole-body sharing would save
  little. Sharing parts of a body would need its own design.

## Consequences

- **Upgrade and rollback.** Replicas on an earlier version cannot read LLM
  bodies written by this one. During a rolling upgrade, and after a rollback
  until the upgrade is reapplied, LLM audit log details and exports that
  include payloads fail on old replicas if they contain any row written by the
  new version. Views without payloads are unaffected and no data is lost.
- **Existing rows** are not rewritten. Storage falls as they age out.
- **Retention** is unchanged, since there are no shared bodies to clean up.
- **Cost.** Writes compress each body once, and reads decompress the bodies of
  the rows they return. Views without payloads skip the body columns, as they
  do now.

## References

- [2026-11-12: Compress MCP audit log bodies and store list results once](2026-11-12-mcp-audit-log-body-storage.md)
- `pkg/gateway/client/mcpauditlogbody.go`
- `pkg/gateway/client/llmauditlog.go`
- `pkg/gateway/types/llmauditlog.go`
