# 2026-11-11: Import legacy provider configuration only

- **Status:** Accepted
- **Date:** 2026-11-11
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248
- https://github.com/obot-platform/obot/issues/8495

## Related ODPs

None. Calvin explicitly waived the merged-ODP prerequisite and approved narrowing implementation to provider configuration only.

## Context

v0.27 provider credentials and connection settings need to move into the existing gateway aggregate. Keeping this import separate from model reconciliation, policy/default reference migration, legacy endpoint compatibility and cleanup limits its scope and leaves normal request handling unchanged.

## Decision

The elected controller calls provider import once from `PostStart`, after waiting for the gateway engine to initialize, then requests a normal model sync for successful imports. Import uses `MigrateKinmIfNotRun` with one completion marker per destination provider. Each callback validates all source keys before one create-only aggregate write. Existing gateway configuration wins, including a configured `bedrock_mantle` provider; providers configured through environment credentials are skipped by the importer. Azure API-key/Entra variants and Bedrock IAM/API-key variants become separate keys of shared `azure` and `bedrock` providers. Key-local endpoints, regions and Azure account metadata are preserved. Azure deployment names are read from legacy models without editing them. Startup environment configuration and proxy handling remain as on main.

Callbacks are idempotent under concurrent leaders or crashes: an existing destination is skipped intact. Failed families remain eligible for the next leader startup or process restart, while successful families are recorded independently. Missing sources and environment-owned or existing destinations also complete their family. Later source edits are not replayed, and deleting a completed destination does not reimport it.

## Rationale

The existing migration helper and aggregate storage suffice. There is no custom journal, receipt, report API, request-time mapping or cleanup mechanism. Normal discovery, request resolution, model handling and legacy routes remain as on main.

## Consequences

This is a provider configuration import, not a complete automatic v0.27 upgrade. Administrators must review model settings, recreate default aliases and repoint model access policies/profiles before restoring traffic. Retained legacy records are untouched, including their existing listing/alias behavior; there is no new isolation guarantee. Shared provider keys may serve the same eligible request, including retries. AWS model availability follows the existing `bedrock` adapter.

Creation and completion use separate stores. If creation succeeds but completion fails, retry skips the existing destination and records completion. Deleting that destination before completion can cause reimport; administrators must avoid deletion during interrupted upgrades. No receipt is retained to distinguish this case. Partial failures are logged; correct credentials or manually configure the destination and retry. Proxy-dependent installations may need manual gateway configuration before discovery succeeds; this change does not guarantee environment settings are applied before first discovery.

Use a database backup and a coordinated maintenance window. Mixed-version and zero-downtime upgrades are not guaranteed; rollback requires the pre-upgrade backup. Source deletion and any automatic model/reference migration are separate future work.

## References

- [Resource-backed gateway configuration](2026-11-03-resource-backed-llm-configuration.md)
- `pkg/llmengine/legacyimport.go`
- `pkg/gateway/client/credential_migration.go`
