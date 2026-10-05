---
title: "Registries and skills overview"
---

Registries help users discover reusable server definitions and skills. Administrators curate what is available and grant access; users select the resources appropriate to their work.

| Resource | What it provides | Next step |
|---|---|---|
| MCP server definition | A hosted server's launch settings or a remote server's address, plus configuration fields | Browse [MCP Servers](../concepts/mcp-registry.md), then [create a vMCP](../mcp-gateway/server-types.md) |
| Skill | Instructions and supporting files an AI client or agent can install and use | [Publish and distribute skills](./publish-skills.md) |
| Git catalog | A repository containing MCP server definitions and vMCP definitions synchronized into Obot | Configure [Git Catalogs](../configuration/mcp-server-gitops.md) |

## Registry and catalog terminology

The **MCP registry** is the discovery feature. A **catalog entry** is one server definition in that registry. Adding an entry makes a definition available; it does not give every user access or create a client connection.

A **vMCP** supplies a connection endpoint using one or more of those definitions. An administrator can share a vMCP with users who do not have independent access to its source catalog entries. See [Share vMCPs](../mcp-gateway/publish.md).

## Choose what to publish

Publish server definitions when users should build their own vMCPs. Publish a shared vMCP when users should connect to a centrally configured set of tools. Publish a skill when users need reusable instructions and files; installing a skill does not itself grant access to an MCP server or model.

Git catalogs can manage both MCP entries and vMCP definitions, including component configuration, tool selections, and profiles. Edit a Git-managed vMCP in its source repository. Skills have their own Git sources and access policies. See [Roles and access policies](../security/policy-coverage.md) to choose the appropriate access control.
