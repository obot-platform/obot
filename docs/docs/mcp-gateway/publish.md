---
title: "Share vMCPs"
---

## Snapshots and updates {#virtual-mcps-snapshots-and-updates}

Adding a component copies a snapshot of its catalog entry into the vMCP. The running component uses that snapshot, not the mutable MCP server configuration.

- Changing a source MCP server does not automatically change an existing vMCP.
- Obot reports when a newer source definition is available. Adopting it is an explicit vMCP update.
- If the source entry is deleted or temporarily unavailable, the stored snapshot remains usable.
- Deleting a catalog entry is therefore not an emergency stop. Delete the affected vMCP when it must no longer run.

The snapshot makes deployments stable and reviewable, but it also means an older or vulnerable definition can remain active until an administrator updates or deletes the vMCP.


## GitOps and migration {#virtual-mcps-gitops-and-migration}

In this release, [MCP Server GitOps](../configuration/mcp-server-gitops.md) synchronizes catalog entries that can be selected as vMCP components. It does not synchronize vMCP definitions, profiles, configuration policies, or component tool selections.

Direct vMCP GitOps synchronization is planned for the next release, alongside migration of existing MCP servers to vMCPs. Until then, legacy MCP server connections remain available for compatibility. Build new connections as vMCPs and update clients to use their vMCP connection URLs.
