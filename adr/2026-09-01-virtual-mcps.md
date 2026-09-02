# 2026-09-01: Expose MCP servers through virtual MCPs

- **Status:** Accepted
- **Date:** 2026-09-01
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [obot-platform/obot#7724](https://github.com/obot-platform/obot/issues/7724) — implement virtual MCP resources, configuration, connections, and migration.
- [obot-platform/obot#7597](https://github.com/obot-platform/obot/issues/7597) — add the virtual MCP user interface.

## Related ODPs

- [2026-09-01: Virtual MCPs](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-09-01-virtual-mcps/README.md)
- [2026-08-16: Thin composites](https://github.com/obot-platform/obot-design-proposals/blob/main/proposals/2026-08-16-thin-composites/README.md) — superseded by the Virtual MCPs proposal.

## Context

Obot exposes hosted MCP endpoints through several related resource types: standalone servers, composite servers, and multi-user server instances. Composition, configuration ownership, access, tool restrictions, and connection behavior differ depending on which resource created the endpoint.

Running endpoints also depend on mutable catalog entries. A catalog edit, failed synchronization, or deletion can therefore change or stop a server without an operator choosing to deploy that change. Catalog configuration further splits environment variables and remote headers across different schemas and requires catalog authors to declare `serverUserType`, even though sharing behavior depends on the deployed configuration.

## Decision

Virtual MCPs (vMCPs) will be the only resource used to expose newly created MCP endpoints. A vMCP may contain one or many catalog components, so a single server and an aggregation use the same resource and connection model. Existing standalone and composite servers remain available only for migration compatibility.

Each component stores a complete snapshot of its catalog entry and a configuration policy that classifies every input as fixed, user-allowed, or prohibited. Runtime resolution uses the snapshot, not the live catalog entry. Catalog drift and missing sources are reported, but a snapshot changes only through an explicit, validated upgrade.

Catalog configuration will use one typed `configuration` collection for environment variables, headers, files, dynamic files, and interpolation-only values. Catalog entries will no longer declare `serverUserType`. A vMCP uses a shared runtime when it has no user-allowed non-header configuration; otherwise it uses a runtime dedicated to the user. Administrators may force dedicated operation.

Access and tool grants are defined by additive user and group profiles embedded in the vMCP. Every user has at most one `vMCPInstance` and one instance-scoped credential per vMCP. The instance may enable any subset of the tools granted by all matching profiles, but it cannot widen that union. Profile, group, component, and policy changes reconcile existing instances and credentials.

Only administrators may create vMCPs consumable by other users. A user may create a personal vMCP from catalog entries they can access, but cannot share it. Loss of catalog access removes affected components from personal vMCPs; it does not prune administrator-created shared vMCPs.

Git-managed vMCPs are synchronized independently of MCP catalogs. Removing a Git-managed definition or source catalog entry does not delete a vMCP that still has instances; removal is reported for explicit administrative cleanup.

## Rationale

One endpoint abstraction makes composition a cardinality choice instead of a separate architecture. It also gives clients, credentials, authorization, and tool scoping one consistent lifecycle.

Snapshots make deployed behavior stable and reviewable. Operators can inspect source changes before adopting them, while catalog synchronization and cleanup are no longer implicit deployment or revocation mechanisms.

Deriving runtime sharing from user-controlled configuration keeps tenancy decisions with the deployed vMCP rather than its catalog author. Defaulting configuration to prohibited and making profiles grant-only provide conservative, composable authorization rules.

Keeping legacy servers during migration avoids breaking existing connection URLs. Evolving standalone and composite resources separately, or resolving catalog entries live, would preserve the inconsistent lifecycle and mutable runtime contract this decision removes.

## Consequences

- New MCP endpoints, whether single-component or aggregated, must be represented by vMCPs.
- vMCPs duplicate catalog data and can lag behind or outlive their sources. Status and operational guidance must make drift, missing sources, and vulnerable snapshots visible.
- Catalog deletion is not an emergency stop. Administrators must disable or delete deployed vMCPs explicitly.
- A user-allowed non-header input requires a dedicated runtime unless the policy changes; user-allowed headers alone do not.
- Tightened configuration policies, reduced tool grants, and lost profile membership must contract existing instance credentials and tool scopes.
- Existing servers require explicit conversion, and clients must reconnect to a new vMCP URL before legacy endpoints are removed.
- GitOps catalogs must migrate to the flattened configuration schema, and GitOps vMCPs require a synchronization path outside catalogs.

## References

- [Model Context Protocol Registry](https://registry.modelcontextprotocol.io/)
