---
description: Release notes and version-specific compatibility references.
title: "Release notes and compatibility"
---

Use the documentation version matching the running Obot release. **Next** describes unreleased work and can include capabilities absent from released images.

Release notes are maintained in [Obot's GitHub releases](https://github.com/obot-platform/obot/releases). Review the notes for each release involved in an upgrade, alongside [Upgrades and rollback](../operations/upgrades.md).

| Compatibility area | Reference |
|---|---|
| Provider routes, request formats, and client caveats | [LLM API compatibility](../llm-gateway/compatibility.md) |
| Local client inventory and enforcement coverage | [Device inventory](../device-management/inventory.md#supported-local-clients) and [enforcement](../device-management/enforcement.md) |
| Legacy MCP endpoints and vMCP transition | [vMCP migration](../mcp-gateway/publish.md#virtual-mcps-gitops-and-migration) |
| Git catalog formats and vMCP synchronization limits | [Git-backed configuration](../configuration/mcp-server-gitops.md) |
| Edition and feature availability | [Obot Editions](../enterprise/overview.md) |

A documentation reorganization does not change release compatibility. Keep existing endpoints during the documented transition and test representative clients before deploying a new server version.
