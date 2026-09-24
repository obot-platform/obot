---
title: MCP Server Attestation
---

## Overview

A remote MCP server catalog entry can reference an evaluation attestation produced by [scout](https://github.com/sebastienrousseau/scout). Obot fetches the statement, checks that it is well formed and that it is about the entry's URL, shows the result on the entry, and refuses to create a server from the entry when the attestation is missing, unverified, or fails the admission policy.

The statement is an [in-toto](https://in-toto.io) statement with the predicate type `https://scoutmcp.io/attestation/mcp-evaluation/v1`. It records every check scout ran against the server, the outcome of each, and a score. Obot verifies it with the Apache-2.0 [`scout-reporting`](https://github.com/sebastienrousseau/scout-reporting) module, which depends only on the Go standard library.

:::note Scope

Obot checks the statement's structure and its subject binding. It does not verify a signature over the statement. Treat the attestation URL as trusted content served from a location you control, such as a release asset or an object store bucket, until signature verification is added. See [ADR 2026-09-23](https://github.com/obot-platform/obot/blob/main/adr/2026-09-23-scout-attestation-admission.md).

:::

## Producing a statement

Run scout against the server URL the catalog entry uses and publish the output at an HTTPS URL:

```bash
scout check https://api.example.com/mcp --output attestation > statement.json
```

The statement's subject digest is computed over the transport and the exact endpoint. A statement produced for `https://api.example.com/mcp` does not cover `https://api.example.com/mcp/` or a different host, and Obot reports such an entry as `subjectMatch: false`.

## Referencing a statement from a catalog entry

Add `attestation.url` to a remote entry that has a `fixedURL`:

```yaml
name: Example API
description: Remote server with a scout attestation.
runtime: remote
remoteConfig:
  fixedURL: https://api.example.com/mcp
attestation:
  url: https://releases.example.com/mcp/statement.json
```

Constraints:

- `attestation` is only accepted on `remote` entries with `remoteConfig.fixedURL`. Entries that use `hostname` or `urlTemplate` have no single URL for the statement to cover.
- `attestation.url` must be an `https` URL without user information.
- The URL is fetched with the same local-network restrictions that apply to remote MCP servers, and the response is limited to 1 MiB.

## Verification status

Obot verifies the statement when the entry is created or its manifest changes, retries a failed verification every 10 minutes, and re-fetches a verified statement every 24 hours. The result is shown as `attestationStatus` on the catalog entry:

```json
{
  "attestationStatus": {
    "verified": true,
    "subjectMatch": true,
    "score": 88,
    "grade": "B",
    "failCount": 1,
    "failedChecks": ["protocol.origin"],
    "failedCategories": ["protocol"],
    "instrument": "scout 0.0.4",
    "ranAt": "2026-09-22T12:00:00Z",
    "checkedAt": "2026-09-23T08:14:02Z"
  }
}
```

`verified` means the statement was fetched and is structurally valid. `subjectMatch` means its subject digest covers the entry's `fixedURL`. When either is false, `error` explains why.

## Admission policy

Server creation from an entry with an `attestation` reference is refused with a `400` response when the status is missing, was computed for an earlier version of the entry, is unverified, or does not satisfy the policy below. Entries without an `attestation` reference are unaffected.

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OBOT_SERVER_MCP_ATTESTATION_MIN_SCORE` | Minimum score (0-100) the statement must carry. `0` disables the score gate. | `0` |
| `OBOT_SERVER_MCP_ATTESTATION_DENY_FAIL_IN` | Comma-separated check categories in which a failed check refuses admission. A failed check is in a category when the part of its id before the first dot, or its phase, equals it, such as `auth` or `protocol`. The policy reads `failedCategories`, which covers every failed check; `failedChecks` is capped at 32 ids for display. | - |

For example, to require a score of at least 80 and refuse servers with any failed authentication or protocol check:

```bash
OBOT_SERVER_MCP_ATTESTATION_MIN_SCORE=80
OBOT_SERVER_MCP_ATTESTATION_DENY_FAIL_IN=auth,protocol
```

The policy is evaluated when a server is created, so changing it takes effect on the next creation without re-verifying entries.
