---
displayed_sidebar: sidebar
title: MCP Servers
description: Managing MCP servers in the MCP Platform
---

For the current task-based instructions, see [Hosted MCP servers](../concepts/mcp-hosting.md). This page preserves existing bookmarks and specialized reference material.

## Overview {#overview}


Managing MCP servers in Obot starts with adding them to the platform. Administrators can control which servers are available to users and how they are configured. Servers may be added individually through the UI, or managed via a Git repository.

Create a catalog entry as a **Hosted Server** when Obot will run it, or a **Remote Server** when it already runs elsewhere. The entry defines its runtime or URL and configuration fields. Add it to a vMCP to choose which values are **Preconfigured** for its connections and which are **Provided at connection** by each user. See [Configure servers in a vMCP](../mcp-gateway/server-types.md).


:::warning Migrate to virtual MCPs

Virtual MCPs (vMCPs) replace the existing MCP server model and are now the recommended way to connect through the Obot Gateway. Create all new connection endpoints as [vMCPs](../mcp-gateway/server-types.md).

Existing MCP servers remain operational in this release. Obot will automatically migrate any remaining servers to vMCPs in a future update.
:::

## Server types {#server-types}


New catalog entries use **Hosted Server** or **Remote Server**. Shared values and user-provided values are configured on the vMCP component. The single-user and multi-user terminology below describes existing legacy deployments, which remain available during the vMCP transition.

### Single-user server (legacy) {#single-user-server}


Single-user servers establish a one-to-one mapping between users and server instances. Each user has their own server instance deployed and provides their own individual credentials (such as personal API keys). Most `stdio` servers were designed with this model in mind. The intended use was to run on the individual's laptop.

This model provides maximum isolation and is ideal when:

- Users need to connect with their personal accounts (e.g., individual GitHub tokens)
- Security policies require user-level credential isolation
- Different users need different configurations or permissions

Keep in mind the gateway will deploy these servers in their own environment on a hosted platform. MCP servers that expect local access to the users filesystem, run local executables, or write output to the local disk will not work as expected.

By default, Obot also blocks MCP servers from connecting to `localhost` addresses, private IP addresses, or link-local addresses. Administrators can opt out with `OBOT_SERVER_DISALLOW_LOCALHOST_MCP=false`, `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP=false`, or `OBOT_SERVER_DISALLOW_LINK_LOCAL_MCP=false`, but should only do so when the server is expected to reach services inside the Obot runtime environment.

**Configuration**: Define parameters that users must provide when enabling the server (e.g., API keys). For each parameter, specify a user-friendly name, description, environment variable name, and whether it's required or sensitive. Values are passed as environment variables to the server process.

### Multi-user server (legacy) {#multi-user-server}


Multi-user servers address organizational deployment patterns through two primary configurations:

1. **Shared credentials**: Organizations provide centralized credentials (e.g., a weather API key) that all users can leverage
2. **Self-authenticating servers**: Servers that handle OAuth or multi-tenancy internally, enabling secure multi-user access

This approach is optimal when:

- The organization owns shared service accounts or API keys
- You want to simplify user onboarding by eliminating individual setup
- The service supports organizational or tenant-based access
- Usage monitoring and control at the organizational level is important

Multi-user servers still require the user to authenticate to the gateway's configured identity provider.

**Configuration**: Pre-configure any required API keys or environment variables. These values are deployed with the server instance. Users connect without being prompted for configuration and authenticate using the built-in authentication or OAuth per the MCP specification.

### Multi-user catalog entry (legacy) {#multi-user-catalog-entry}


A multi-user catalog entry can be deployed as a shared multi-user server. Instead of creating one server per user when users connect, Obot creates a shared deployment from the catalog entry and then creates per-user MCP server instances for the users who connect to that deployment.

Use a multi-user catalog entry when:

- Administrators or Power User+ users should publish an approved multi-user server configuration before it is deployed
- Different workspaces or catalogs need their own shared deployment from the same catalog entry
- The shared server may need deployment-level configuration, such as a URL, shared API key, or environment variables
- Individual users still need per-user headers or authentication after connecting to the shared deployment

Multi-user catalog entries are configured with `serverUserType: multiUser`. Admin and Power User+ users can deploy them as multi-user servers.

Catalog entry updates are tracked against deployed servers. When a catalog entry changes, affected deployments show an update action and a diff of the current deployed manifest versus the updated catalog entry. Applying the update refreshes the shared deployment from the catalog entry. Users may need to reconfigure their own instance if the update introduces new required per-user configuration.

### Remote server {#remote-server}

Continue to [Remote server](../mcp-gateway/register-remote.md#mcp-servers-remote-server).

## Adding a server {#adding-a-server}

Continue to [Adding a server](../concepts/mcp-hosting.md#mcp-servers-adding-a-server).

## Basic configuration {#basic-configuration}

Continue to [Basic configuration](../concepts/mcp-hosting.md#mcp-servers-basic-configuration).

## Runtime selection {#runtime-selection}

Continue to [Runtime selection](../concepts/mcp-hosting.md#mcp-servers-runtime-selection).

### NPX: Node/Typescript Based MCP Servers {#npx-nodetypescript-based-mcp-servers}

Continue to [NPX: Node/Typescript Based MCP Servers](../concepts/mcp-hosting.md#mcp-servers-npx-nodetypescript-based-mcp-servers).

### UVX: For Python-based packages {#uvx-for-python-based-packages}

Continue to [UVX: For Python-based packages](../concepts/mcp-hosting.md#mcp-servers-uvx-for-python-based-packages).

### Containerized: For Docker-based deployments {#containerized-for-docker-based-deployments}

Continue to [Containerized: For Docker-based deployments](../concepts/mcp-hosting.md#mcp-servers-containerized-for-docker-based-deployments).

## Kubernetes Secret Bindings {#kubernetes-secret-bindings}

Continue to [Kubernetes Secret Bindings](../concepts/mcp-hosting.md#mcp-servers-kubernetes-secret-bindings).

### Required Kubernetes Secret Label {#required-kubernetes-secret-label}

Continue to [Required Kubernetes Secret Label](../concepts/mcp-hosting.md#mcp-servers-required-kubernetes-secret-label).

### Configure a Binding in the Admin UI {#configure-a-binding-in-the-admin-ui}

Continue to [Configure a Binding in the Admin UI](../concepts/mcp-hosting.md#mcp-servers-configure-a-binding-in-the-admin-ui).

#### New Catalog Entry {#new-catalog-entry}

Continue to [New Catalog Entry](../concepts/mcp-hosting.md#mcp-servers-new-catalog-entry).

#### Git-ops Managed Template {#git-ops-managed-template}

Continue to [Git-ops Managed Template](../concepts/mcp-hosting.md#mcp-servers-git-ops-managed-template).

## Post-deployment management {#post-deployment-management}


After successfully adding a server:

- The server appears in the available servers list for authorized users
- Server entries can now be added to authorization groups for different teams
- Users can integrate the server into their clients to access tools in conversations and tasks
- Administrative monitoring of usage and auditing is available through the MCP Platform
