# 2026-09-23: Admit MCP servers on scout attestations without signature verification

- **Status:** Accepted
- **Date:** 2026-09-23
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [#6344](https://github.com/obot-platform/obot/issues/6344) — scanner integration for MCP servers
- [#4622](https://github.com/obot-platform/obot/issues/4622) — detecting drift in remote servers
- [#6789](https://github.com/obot-platform/obot/issues/6789) — image-registry allowlist, the precedent for an admission gate on catalog entries

## Related ODPs

None.

## Context

Administrators want evidence about a remote MCP server before users can create servers from its catalog entry. [scout](https://github.com/sebastienrousseau/scout) evaluates a server and writes an in-toto statement (predicate type `https://scoutmcp.io/attestation/mcp-evaluation/v1`) that carries every check's verdict, a score, and a digest over the target descriptor. Its verifier is the Apache-2.0, standard-library-only module `github.com/sebastienrousseau/scout-reporting`.

The statement is published at a URL the catalog entry references. Two things could be checked: that the statement is a valid scout statement about this entry's URL, and that it was produced by a party the administrator trusts. The second needs a signature (DSSE envelope, cosign, or similar) and a key-distribution story that does not exist in Obot yet.

## Decision

- A remote catalog entry with a `fixedURL` may carry `attestation.url`. A controller handler fetches it through `safehttp` with the local-network restrictions that apply to remote MCP servers, capped at 1 MiB, and records the outcome in `Status.Attestation`, keyed on the manifest hash.
- Verification is structural validation (`attestation.Parse`, `Validate`) plus subject binding (`Covers` against the entry's `fixedURL`). Signature verification is out of scope for this version.
- `CreateServer` refuses, with a `400`, an entry whose attestation status is missing, stale, unverified, or fails the admission policy. The policy is server configuration (`OBOT_SERVER_MCP_ATTESTATION_MIN_SCORE`, `OBOT_SERVER_MCP_ATTESTATION_DENY_FAIL_IN`) and is evaluated at admission time against the stored status, so a policy change does not require re-verifying entries.
- Entries without an attestation reference are not affected. Requiring one for every entry is a separate decision.

## Rationale

Subject binding without a signature still rules out the cheapest failure: a statement about one server presented for another, or a statement whose predicate was edited after the digest was computed. What it does not rule out is an attacker who controls the URL and can produce a whole new statement. For a URL the administrator controls, that is the same trust the catalog manifest already carries.

Keeping the verdict ids in the status, rather than only a pass/fail, lets the policy be evaluated by the API server without re-fetching and lets an operator tighten the policy without touching every entry.

Verification runs in the controller, not in the API handler, so a slow or unreachable attestation host cannot block catalog edits, and so GitOps-sourced entries are verified the same way as entries created through the API.

## Consequences

- An attestation is only as trustworthy as the URL it is served from. Documentation says so.
- Adding signature verification later is additive: a `verified` result gains a signer identity, and the policy gains a required-signer field. The status shape does not need to change.
- The `scout-reporting` module is a new direct dependency; it has no dependencies of its own.
- The status is refreshed on an interval (24 hours when verified, 10 minutes on failure), so a re-published statement is picked up without a manifest change. Rug-pull detection against the live server remains #4622's problem; this records what scout observed at `ranAt`, not what the server does now.

## References

- `pkg/mcp/attestation.go` — fetch, verify, policy
- `pkg/controller/handlers/mcpservercatalogentry/mcpservercatalogentry.go` — `ReconcileAttestation`
- `pkg/api/handlers/mcp.go` — `CreateServer` admission
- `docs/docs/configuration/mcp-server-attestation.md`
- scout ADR 0011, attestation format: https://github.com/sebastienrousseau/scout/blob/main/docs/adr/0011-attestation-format-is-apache.md
