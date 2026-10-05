---
description: MCP server definition formats and validation.
title: "Catalog schemas"
---

Use these references when authoring MCP server and vMCP definitions in Git. A catalog entry defines how a server runs or where it is reached. A separate `type: vmcp` item defines components, configuration policies, tool selections, and profiles.

| Format | Reference |
|---|---|
| Catalog metadata, runtime, configuration values, and resource requirements | [Catalog YAML structure](../configuration/mcp-server-gitops.md#yaml-configuration-structure) |
| Catalog file selection and validation | [Selecting files](../configuration/mcp-server-gitops.md#selecting-catalog-files) and [CLI validation](../configuration/mcp-server-gitops.md#validating-catalog-entries) |
| vMCP components, profiles, and fixed configuration | [vMCP definitions](../configuration/mcp-server-gitops.md#vmcp-definitions) |
| Registry contribution example | [MCP Registry API](../functionality/mcp-registry-api.md#server-entry-format) |
| Per-server domain egress settings | [Egress YAML examples](../configuration/mcp-server-egress-control.md#yaml-configuration-examples) |

Validate changes with `obot mcp validate-catalog` before synchronizing a source. The validator for the installed Obot version is authoritative for supported fields. `runtime: composite` is not accepted; build endpoints from catalog entries using [Create vMCP](../mcp-gateway/server-types.md).

For existing Git-synced composites, use the [migration command](../configuration/mcp-server-gitops.md#migrating-git-synced-composites-to-vmcps) to generate matching vMCP definitions while retaining component IDs and connections. See [Git Catalogs](../configuration/mcp-server-gitops.md) for source setup and validation commands.
