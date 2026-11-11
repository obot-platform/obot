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

The elected controller calls provider import once from `PostStart`, after waiting for the gateway engine to initialize, then requests a normal model sync for successful imports. Import uses `MigrateKinmIfNotRun` with one completion marker per destination provider. Each callback validates all source keys before one create-only aggregate write. Existing gateway configuration wins, including a configured `bedrock_mantle` provider; providers configured through environment credentials are skipped by the importer. Azure API-key/Entra variants and Bedrock IAM/API-key variants become separate keys of shared `azure` and `bedrock` providers. Key-local endpoints, regions and Azure account metadata are preserved. Azure deployment names are read from legacy models without editing them. Startup environment configuration and proxy handling remain as on main.

Callbacks are idempotent under concurrent leaders or crashes: an existing destination is skipped intact. Failed families remain eligible for the next leader startup or process restart, while successful families are recorded independently. Missing sources and environment-owned or existing destinations also complete their family. Later source edits are not replayed, and deleting a completed destination does not reimport it.

Normal discovery matches legacy models within their imported provider by exact native ID, then by the latest dated version of an undated alias. Matching uses the normal catalog, including its outage fallback, rather than proving live upstream availability.

Resolve differing legacy model settings automatically rather than leaving matched sources pending:

- Existing gateway model settings always win, even when legacy sources disagree.
- For a new destination, prefer an exact native-ID record over an undated alias, then break ties by legacy provider name and record name in ascending lexical order.
- Copy the winner's active state, custom display name, alias, usage and cost override before publication. Do not combine fields from different sources or force the destination inactive because settings differ.
- Log the selected source when settings differ. After publication and reference writes succeed, mark every matched source complete, including the losing sources.

Missing destination models are a separate case: retain their references and keep retrying on later successful discovery syncs. A missing match can keep migration pending indefinitely; settings differences cannot.

After publication succeeds, stored default aliases and explicit policy references are rewritten to destination record IDs. Native wildcard policies retain main's cross-provider semantics. Subjects, budgets and guardrail associations are unchanged. One completion annotation on each retained source records its destination after all reference writes succeed. It prevents later source edits and gateway deletion/recreation from replaying migration. Once every supported source completes, a completion annotation on the gateway ModelProvider lets later discovery skip the legacy inventory, reference and alias scans entirely, using the ModelProvider already loaded by the controller without another storage read. Unmatched sources keep this gate open for retry; conflicting settings are resolved once by the winner rule. Legacy sources added after global completion require manual migration. Derived legacy alias bindings are released, and legacy models are excluded from usable-model grants; administrators can still inspect retained records. Requests use normal gateway resolution and never consult migration mappings.

## Rationale

The existing migration helper and aggregate storage suffice for provider import. Normal discovery continues to own model identities, pruning and outage retention; migration wraps its publication instead of building another discovery mechanism. No custom journal, report API, request-time old-ID mapping or destructive cleanup is added. Startup, proxies and legacy routes remain as on main.

Deterministic selection avoids permanent retries or mandatory manual activation for models that already have a valid destination. Selecting one source's settings together avoids inventing a combination no administrator configured. Stable ordering makes the choice independent of inventory order and leader changes. We accept losing-source settings being superseded in exchange for completing policy and default-reference migration automatically.

## Consequences

Unmatched models remain for manual attention, with warnings in the server logs. Deterministic resolution can discard another source's disabled state, custom alias or cost override; the retained manifests remain available for recovery. Administrators must review model settings, defaults and access policies/profiles, including users, groups and wildcard patterns, and test both allowed and denied requests as ordinary users before restoring traffic. Existing gateway models keep their settings even when the legacy model differs. Legacy manifests and credentials are retained for recovery; only model completion annotations and derived alias bindings change. Shared provider keys may serve the same eligible request, including retries. AWS model availability follows the existing `bedrock` adapter.

Provider creation and completion use separate stores. If creation succeeds but completion fails, retry skips the existing destination and records completion. Deleting that destination before completion can cause reimport; administrators must avoid deletion during interrupted upgrades. No receipt is retained to distinguish this case. Partial failures are logged; correct credentials or manually configure the destination and retry. Proxy-dependent installations may need manual gateway configuration before discovery succeeds; this change does not guarantee environment settings are applied before first discovery.

Model publication, reference updates and source completion are also separate writes. Failures retry on a later sync; existing gateway settings and reference edits are preserved. Do not delete destinations during an interrupted migration: before source completion, deletion can replay settings on recreation. Completed sources are not replayed.

Use a database backup and a coordinated maintenance window. Mixed-version and zero-downtime upgrades are not guaranteed; rollback requires the pre-upgrade backup. Destructive source cleanup is separate future work.

## References

- [Resource-backed gateway configuration](2026-11-03-resource-backed-llm-configuration.md)
- `pkg/llmengine/legacyimport.go`
- `pkg/gateway/client/credential_migration.go`
- `pkg/llmengine/legacymigrate.go`
