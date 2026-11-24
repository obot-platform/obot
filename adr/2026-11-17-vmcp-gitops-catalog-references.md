# 2026-11-17: Reference Obot-managed catalog entries from GitOps vMCPs

- **Status:** Proposed
- **Date:** 2026-11-17
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [#7981](https://github.com/obot-platform/obot/issues/7981) tracks the original vMCP synchronization work.

## Related ODPs

- [Virtual MCPs ODP](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md#gitops-vmcps) defines vMCP component snapshots, explicit upgrades, and GitOps synchronization.

## Context

An admin can create a catalog entry in Obot and use it in a vMCP through the UI. GitOps vMCPs can only use entries synced from a source in the same catalog. The existing Git reference is an entry key, optionally qualified with a source ID.

The original vMCP design called for explicit snapshot upgrades. An earlier, reverted implementation guarded snapshots against unrelated catalog changes and required source-backed entries. The current implementation syncs vMCP definitions through MCP catalog sync.

## Concepts

- **Catalog entry** describes an MCP server. Obot imports Git-synced entries from a configured source. Admins create and manage Obot-managed entries through Obot, for example in the UI.
- **vMCP and component.** A vMCP is the endpoint clients connect to. Each component adds one catalog entry to that endpoint.
- **GitOps vMCP** is a vMCP whose definition Obot syncs from Git.
- **Snapshot** is the saved copy of the entry configuration a component runs.
- **Entry ID** identifies a catalog entry within an Obot installation and survives a rename.
- **Digest and pin.** A digest identifies a revision of an entry's deployable configuration. A Git pin pairs the entry ID with the digest reviewed for deployment.

## Decision

A GitOps vMCP can use an Obot-managed catalog entry by pinning its ID and digest in Git. An admin enables this on the entry and copies both fields from its details after saving. The component runs the pinned snapshot until Git supplies a new pin that passes validation.

- The per-entry toggle is off by default and is settable through the UI, API, and Terraform. Turning it on lets **any Git source in that same catalog** reference the entry.
- Each component uses either an existing entry key or an entry ID and digest. One vMCP can use both forms across its components.
- Git-managed vMCPs are read-only in the UI. The API blocks direct edits, snapshot upgrades through **Update vMCP**, and deletion.
- When a pinned component has an update, the UI shows it and directs the admin to change the pin in Git. The Obot Admin MCP server returns per-component applied and current digests and update status.
- A failed pin leaves the last working deployment in place. Sync still removes definitions confirmed absent from sources it read completely.
- Keep the component ID for upgrades that use the same entry and destination. A different entry or destination requires a new component ID and fresh credentials.
- Remove the vMCP, a component, or its access grants through Git and a successful sync. A missing source entry triggers a warning in the UI and Admin MCP status, and the deployed snapshot can continue serving.

## Example

Copy these fields from the Obot-managed entry's details after saving it. The values are illustrative.

```yaml
# Copy the entry ID from the catalog entry's details in Obot.
mcpServerCatalogEntryID: default-internal-search-k7q9m
# Copy the current digest from the same details. Changing it requests a new revision.
catalogEntryDigest: "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

Git also contains `knowledge-base.yaml`.

```yaml
# The entry key is how a vMCP in the same source refers to this file.
entryKey: knowledge-base
name: Knowledge Base
runtime: remote
remoteConfig:
  fixedURL: https://kb.example.com/mcp
```

Commit `research.yaml` to use both entries. This example uses the default admin profile.

```yaml
type: vmcp
entryKey: research
displayName: Research
components:
  - id: internal-search
    # Paste the ID and digest copied from Obot.
    mcpServerCatalogEntryID: default-internal-search-k7q9m
    catalogEntryDigest: "v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  - id: knowledge-base
    # Use the key from knowledge-base.yaml in the same Git source.
    mcpServerCatalogEntryKey: knowledge-base
```

## Rationale

An entry ID survives a rename and distinguishes entries with similar names. A digest ties deployment to the revision reviewed in Git, even if the entry changes in Obot before sync. The entry keeps one owner and one credential lifecycle. A portable alias could simplify promotion across installations, but it would add a mapping for admins to maintain.

## Consequences

- Existing key-based manifests need no migration and retain their current behavior, including references of the form `sourceID::entryKey`. IDs and digests are installation-specific. Use per-environment values and upgrade Obot before publishing ID-based manifests.
- Offline validation checks the reference shape and rejects mixed fields on one component. On first use or a pin change, sync checks that the entry exists, has opted in, belongs to the same shared catalog, is editable and Obot-managed, and has no source URL, workspace owner, or detached state. An entry whose ownership an admin has accepted is eligible. Sync validates the digest, configuration and secret bindings before publishing. Turning the toggle off prevents new or changed pins. Existing deployments follow the Git removal rules.
- Freeze the `v1` digest as SHA-256 over the existing deployable snapshot projection and its JSON encoding at introduction. Include unsupported tools and the opaque static configuration revision. Exclude secret values, version bounds and informational fields such as catalog copy, icon, metadata, repository URL, entry key, tool preview and upgrade notes. Keep the `v1` verifier across upgrades and lock the projection and encoding with a fixed expected digest test.
- Record the applied pin with the snapshot and reuse it on resync while the component ID, entry ID and digest are unchanged, including on unrelated Git edits. Coordinate publication and cleanup so the referenced credential revision stays protected from validation until the snapshot is committed or the attempt fails. On failure, keep the previous deployment and credentials.
- Reject `trigger-update` for Git-managed vMCPs. Track definitions that fail validation separately from confirmed removals. If a source read is incomplete or a definition's identity is uncertain, keep deployments whose removal cannot be confirmed. For a pin failure, show the requested digest, current catalog digest, last applied digest, and whether the prior revision is still serving. A new vMCP with a failed pin has no serving revision.
- Enforce the new component ID on a change of entry or upstream destination. Review the profile grants and any catalog-owned credential for that destination before deploying. Static OAuth credentials retain their existing rotation behavior.
- Source deletion can leave the snapshot and its referenced static OAuth credential active. The missing-source warning describes that state. To revoke access, remove the definition, component, or applicable grants in Git and confirm a successful sync. If a pin failure prevents a grant edit from applying, resolve the failed pin or remove the affected definition. There is no emergency disable switch.
- A pin verifies a revision. On a fresh installation or when reverting to an older pin, the matching entry revision must be available unless that pin is already deployed. Verify mixed manifests, retries and unrelated Git edits, authorization, removal during pin failure, the UI and API upgrade paths, credential reuse and cleanup races, source deletion, and failed publication.

## References

- [Virtual MCP architecture](2026-09-01-virtual-mcps.md)
- [Reverted independent-sync ADR](https://github.com/obot-platform/obot/blob/79ad32a9a779b5d4c36fa58899ac8638c5b30320/adr/2026-09-10-vmcp-git-sync.md)
- [Current catalog sync implementation PR](https://github.com/obot-platform/obot/pull/8005)
- [GitOps vMCP manifest format](../docs/docs/configuration/mcp-server-gitops.md#vmcp-definitions)
- [GitOps manifest decoder](../pkg/mcpcatalog/validation.go)
- [Catalog sync component resolver](../pkg/controller/handlers/mcpcatalog/vmcp.go)
- [Snapshot drift and digest](../pkg/vmcp/drift.go)
