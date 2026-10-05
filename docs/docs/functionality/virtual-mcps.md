---
title: Virtual MCPs (vMCPs)
description: Combine MCP servers behind one governed connection endpoint
---

# Virtual MCPs (vMCPs)

A virtual MCP (vMCP) exposes one or more MCP servers through one Obot Gateway endpoint. The source servers become **components** of the vMCP. Each component keeps its own deployment and configuration behavior, while the vMCP provides one place to manage the connection, tools, and access.

:::warning Transitional release

vMCPs are in a transitional state. Legacy MCP server connections continue to work in this release, but new connection workflows should use vMCPs.

In a future release, Obot will migrate all MCP servers to vMCPs, and vMCPs will be the only way to connect to MCP servers through the Obot Gateway.

GitOps synchronization currently manages MCP catalog entries, not vMCPs. Direct GitOps synchronization of vMCP definitions is planned for a future release.

:::

## How vMCPs work {#how-vmcps-work}

Continue to [How vMCPs work](../mcp-gateway/server-types.md#virtual-mcps-how-vmcps-work).

## Shared and personal vMCPs {#shared-and-personal-vmcps}

Continue to [Shared and personal vMCPs](../mcp-gateway/server-types.md#virtual-mcps-shared-and-personal-vmcps).

### Create a shared vMCP as an administrator {#create-a-shared-vmcp-as-an-administrator}

Continue to [Create a shared vMCP as an administrator](../mcp-gateway/server-types.md#virtual-mcps-create-a-shared-vmcp-as-an-administrator).

### Create a personal vMCP as a user {#create-a-personal-vmcp-as-a-user}

Continue to [Create a personal vMCP as a user](../mcp-gateway/server-types.md#virtual-mcps-create-a-personal-vmcp-as-a-user).

## Configuration policies {#configuration-policies}

Continue to [Configuration policies](../mcp-gateway/server-types.md#virtual-mcps-configuration-policies).

## Tools and profiles {#tools-and-profiles}

Continue to [Tools and profiles](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles).

## Connect to a vMCP {#connect-to-a-vmcp}

Continue to [Connect to a vMCP](../mcp-gateway/connect-clients.md#virtual-mcps-connect-to-a-vmcp).

## Snapshots and updates {#snapshots-and-updates}

Continue to [Snapshots and updates](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates).

## GitOps and migration {#gitops-and-migration}

Continue to [GitOps and migration](../mcp-gateway/publish.md#virtual-mcps-gitops-and-migration).
