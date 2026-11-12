# 2026-11-11: Migrate legacy provider configuration and model references

- **Status:** Accepted
- **Date:** 2026-11-11
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8248

## Related ODPs

None. Calvin explicitly waived the merged-ODP prerequisite and authorized provider and model/reference migration without external compatibility for legacy model-record IDs.

## Context

v0.27 provider configuration and stored model references need to move to the gateway. Clients use standard model names; preserving external legacy record IDs and combining import with destructive cleanup would add unnecessary compatibility state.

## Decision

The elected controller calls provider import once from `PostStart`, after waiting for the gateway engine to initialize, then requests a normal model sync for successful imports. Import uses `MigrateKinmIfNotRun` with one completion marker per destination provider. Each callback validates all source keys before one create-only aggregate write. Existing gateway configuration wins, including a configured `bedrock_mantle` provider; providers configured through environment credentials are skipped by the importer. Azure API-key/Entra variants and Bedrock IAM/API-key variants become separate keys of shared `azure` and `bedrock` providers. Key-local endpoints, regions and Azure account metadata are preserved. Azure deployment names are read from legacy models without editing them. Startup environment credential configuration remains as on main. Before creating an imported provider, apply the existing environment proxy selection to its validated aggregate. The first durable write includes the applicable proxy, so resource watches on any replica cannot synchronize or discover an imported provider before its proxy is configured. Existing destinations (including explicit provider proxies) remain untouched; proxy selection retains the existing endpoint and `NO_PROXY` behavior.

Imports inherit Obot's configured private-IP allowance (`AllowPrivateIPMCP`) as the gateway provider's `allow_private_network` setting before validating base URLs and key endpoints. That gateway setting permits both private and loopback destinations; it does not inherit the separate MCP localhost restriction. Link-local, cloud metadata and unspecified addresses remain rejected even when private access is allowed. Existing gateway providers retain their configuration. If private access is disallowed, administrators must permit it or manually configure the gateway provider before retrying the import.

Bedrock IAM imports require both an explicit access-key ID and secret-access key, matching v0.27. Missing either or both rejects the family without recording completion; migration must not silently select the Obot server's AWS identity. Administrators can explicitly configure workload identity through normal gateway configuration separately.

Callbacks are idempotent under concurrent leaders or crashes: an existing destination is skipped intact. Failed families remain eligible for the next leader startup or process restart, while successful families are recorded independently. Missing sources and environment-owned or existing destinations also complete their family. Later source edits are not replayed, and deleting a completed destination does not reimport it.

Normal discovery owns gateway model creation, settings, pruning and outage retention. After publication succeeds, migration rewrites stored default aliases and explicit model access-policy references to gateway record IDs. Profiles are the existing policy records: their IDs, subjects, budgets and guardrail associations stay intact. Native wildcard patterns and `obot://` default-alias references retain their existing meaning. Multiple old IDs resolving to one gateway record are deduplicated within a Profile.

Only referenced legacy models need matching: exact native ID within the imported provider, then the latest dated version of an undated alias. Matching uses the normal catalog, including outage fallback; it does not prove live upstream availability. Unmatched referenced sources remain intact and retry on later successful model publication, with warnings for manual attention. Unreferenced legacy models never delay completion. Unsupported referenced legacy providers, including historical Vertex records, keep their references for manual correction and produce warnings, but do not block completion: automatic matching cannot resolve them. This does not add Vertex credential import or reference matching.

Do not migrate active state, display names, custom model aliases, usage or cost overrides. Profiles govern model access in this release; administrators must recreate any needed custom pricing through gateway configuration. No source settings conflict resolution is necessary, and existing gateway settings remain untouched.

Reference updates are naturally idempotent: rewritten references no longer name a legacy source. One completion annotation on the gateway ModelProvider skips future migration scans after all references to supported legacy providers match. The controller passes its already-loaded provider, avoiding another lookup. There are no per-source migration annotations or publication callbacks. Later source edits are never replayed into destination models; references added after global completion require manual updates.

Retain legacy manifests and credentials for recovery. Legacy models are excluded from usable-model grants and expose no custom alias to normal alias reconciliation, which removes their derived bindings. Administrators can inspect retained records. Requests use normal gateway resolution without old-ID mappings or migration lookups.

## Rationale

Existing aggregate storage and migration helpers suffice for provider import. Reference migration follows normal discovery rather than wrapping publication or creating another discovery mechanism. Restricting it to defaults and Profiles removes settings copying, deterministic conflict selection and per-source completion state. No journal, report API, request-time mapping or destructive cleanup is added. Reuse the startup proxy helper before publication instead of adding a second post-import proxy pass that could race resource reconciliation. Startup environment credentials and legacy routes remain as on main.

## Consequences

Administrators must review defaults and access policies/Profiles, including users, groups and wildcard patterns, and test allowed and denied requests as ordinary users before restoring traffic. Legacy disabled states are not preserved; equivalent restrictions must be expressed through Profiles. Custom aliases and cost overrides require manual configuration. Unmatched references to supported providers remain unusable until a gateway match exists or an administrator changes them; they can keep migration pending indefinitely. Unsupported-provider references require manual correction even after migration is marked complete. An outage-fallback catalog can still supply a match.

Provider creation and completion use separate stores. If creation succeeds but completion fails, retry skips the existing destination and records completion. Deleting that destination before completion can cause reimport; administrators must avoid deletion during interrupted upgrades. No receipt is retained to distinguish this case. Partial failures are logged; correct credentials or manually configure the destination and retry. Imported providers receive applicable environment proxy configuration before publication even if the startup proxy pass has already finished. Existing proxy-routing limitations remain; this does not change Bifrost transport behavior or add per-key endpoint proxy selection.

Model publication, reference updates and global completion are separate writes. A failed publication never starts reference migration. Later failures retry from current references without replaying completed rewrites or changing gateway settings. Resource-version conflicts fail the pass and retry using current records, preserving concurrent administrator edits. Derived alias cleanup follows normal controller reconciliation. Shared provider keys may serve the same eligible request, including retries; AWS model availability follows the existing `bedrock` adapter.

Use a database backup and a coordinated maintenance window. Mixed-version and zero-downtime upgrades are not guaranteed; rollback requires the pre-upgrade backup. Destructive source cleanup is separate future work.

## References

- [Resource-backed gateway configuration](2026-11-03-resource-backed-llm-configuration.md)
- `pkg/llmengine/legacyimport.go`
- `pkg/gateway/client/credential_migration.go`
- `pkg/llmengine/legacymigrate.go`
