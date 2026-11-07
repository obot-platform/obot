---
title: "Create a vMCP"
---

A virtual MCP (vMCP) gives your AI client one connection URL for selected tools from one or more MCP servers. Start with a server already available in the Obot catalog, choose its tools, and make your first tool call.

**Already have a vMCP?** Follow [Connect to a vMCP](./connect.md). You do not need to create another one or have access to its source catalog entries.

## Before you begin

For this walkthrough, use an **Admin or Owner** account so you can create a vMCP and share it with another user. You need an Obot instance with a server available in **MCP Servers** and any credentials that service requires. Choose a server with a read-only tool you can safely try.

If you need an instance, start with [Docker evaluation](../installation/docker-deployment.md). If you are not an administrator, you can [create a personal vMCP](../mcp-gateway/server-types.md#virtual-mcps-create-a-personal-vmcp-as-a-user) from catalog entries available to you, or connect to one your administrator shares.

## Create from the catalog

1. Open **vMCPs** and select **Create vMCP**.
2. Browse the **MCP Servers** panel and drag an existing server onto the canvas.
3. Enter a name and description, then select **Create**.
4. Supply any required component configuration. Choose **Preconfigured** for values shared by its users or **Provided at connection** for values each user supplies, such as a personal API token. Complete service authorization if prompted.
5. In **Add Tools**, choose **Managed**, discover the tools, and select a read-only tool for this exercise. Confirm the selection.

Catalog entries provide the server definition; some still require credentials or service configuration. If the server you need is missing, ask an administrator to add it using [Add remote MCP servers](../mcp-gateway/register-remote.md) or [Add hosted MCP servers](../concepts/mcp-hosting.md).

## Try a tool

1. Open **Inspector** in the vMCP designer, or select **Test vMCP** on its card.
2. If **Start Session** appears, select it and complete the configuration and authorization prompts.
3. Select your read-only tool, enter its arguments, and select **Call**.
4. Confirm that the result shows **Succeeded** and the expected output.

## Share with a user or group

1. Open **Profiles** in the vMCP designer.
2. Grant your intended user or group access to the tool you selected.
3. Copy the vMCP's connection URL and send it to that user with [Connect to a vMCP](./connect.md).

New administrator-created vMCPs include an administrator profile. Keep access limited to the people who need it, and have the recipient test using their own account. See [Control MCP access](../mcp-gateway/access.md) for profile behavior and detailed access tests.

## Connect your client {#connect-to-a-shared-vmcp}

Follow [Connect to a vMCP](./connect.md) to add the connection URL to your AI client, sign in, and call a tool. Share that guide with anyone you grant access to this vMCP.

## Find the audit record

As an administrator or auditor, open **Operations > Audit Logs**, select **MCP**, and find the tool call by vMCP, user, and time. Confirm the caller and outcome. Viewing request and response bodies requires the Auditor role. If you do not have audit access, give the call details to an administrator or auditor.

## Next steps

- [Create vMCPs](../mcp-gateway/server-types.md) with multiple servers or a personal scope.
- [Share vMCPs](../mcp-gateway/publish.md) and manage updates to their server definitions.
- [Control MCP access](../mcp-gateway/access.md) and verify which tools each user can call.
- [Explore Obot](./choose.md), including model access, skills, and device management.
