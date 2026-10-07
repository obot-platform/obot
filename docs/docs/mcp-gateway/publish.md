---
title: "Share vMCPs"
---

Share a vMCP when users need a governed endpoint with configured tools. Users connect to its URL; they do not need to assemble the underlying server definitions themselves.

To publish reusable server definitions instead, use the [MCP server registry](../concepts/mcp-registry.md). See the [registry overview](../registries/overview.md) for the distinction between server definitions, vMCP endpoints, and skills.

Administrators create shared vMCPs. A consumer of a shared vMCP does not need independent access to its source catalog entries. Personal vMCPs remain owner-only and depend on their owner's catalog access.

To publish a shared endpoint:

1. Select existing servers from the catalog that provide the tools your users need.
2. [Create a shared vMCP](./server-types.md#virtual-mcps-create-a-shared-vmcp-as-an-administrator) as an administrator and add those entries.
3. Configure each component's values and select its exposed tools. Choose **Managed** when you need a fixed tool selection.
4. Open **Profiles** and grant the intended users or groups access to the appropriate tools. Review broad and default grants.
5. Test the connection, then send consumers its connection URL and the [Connect to a vMCP](../start-here/connect.md).

For catalog publication and programmatic discovery, see [MCP catalogs](../concepts/mcp-registry.md), the [Registry API](../functionality/mcp-registry-api.md), and [Git-backed configuration](../configuration/mcp-server-gitops.md). Git catalogs can also publish vMCP definitions, including their configuration policies, tool selections, and profiles.

## Snapshots and updates {#virtual-mcps-snapshots-and-updates}

A vMCP keeps its own copy of each server's catalog definition. For a vMCP managed in Obot, editing the catalog entry later does not automatically update its existing components. The owner or administrator applies an available update.

For a **Git-managed vMCP**, a successful catalog sync resolves its component definitions from the source and updates its configuration and profiles. Make changes in Git; the UI and API do not edit these vMCP definitions. Removing the vMCP definition from the source deletes the vMCP. The manual update example below applies to vMCPs managed in Obot.

For example, an administrator changes a catalog entry from container image version 1 to version 2. Existing vMCPs continue to use the version 1 image reference until their owner or administrator applies the available update. Users connecting to a shared vMCP do not need to recreate their client connection; the administrator manages that update.

- Obot indicates when a newer catalog definition is available. The vMCP owner or administrator decides when to apply it.
- Deleting the catalog entry does not automatically stop existing vMCPs that use its saved definition. Delete the affected vMCP when it must no longer run.
- Security fixes to a catalog definition also need to be applied to existing vMCPs.

This saved copy is called a **snapshot**. It preserves the server definition, not everything the server depends on. An image tag can still point to a different image, a remote service can change, and a Kubernetes Secret binding still reads from the referenced Secret. Pin container images by digest when the deployed image must stay fixed.

## GitOps and migration {#virtual-mcps-gitops-and-migration}

[Git Catalogs](../configuration/mcp-server-gitops.md#vmcp-definitions) synchronize both catalog entries and `type: vmcp` definitions. Keep entry keys and component IDs stable so synchronization preserves identity and stored configuration.

Obot converts existing composite servers to vMCPs during upgrade. A Git-synced composite becomes an orphaned vMCP until its source publishes a matching definition. Use `obot mcp generate-vmcp-catalog` and follow [Migrating Git-synced composites](../configuration/mcp-server-gitops.md#migrating-git-synced-composites-to-vmcps) to resume Git management without replacing user connections. Other legacy MCP server connections remain available for compatibility; use vMCPs for new endpoints.
