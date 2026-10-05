---
title: "Govern your first MCP server"
---

Create a shared vMCP with an explicit tool grant, then verify a connection through Obot. Use an Admin or Owner account and a reachable remote MCP endpoint exposing at least one harmless tool. If you need an Obot instance first, use [Docker evaluation](../installation/docker-deployment.md).

## Register the server

1. Open **MCP Servers** and select **Add MCP Server**.
2. Choose **Remote Server**. Enter its name, short description, and HTTP or HTTPS MCP URL, then save.
3. Review the access-policy prompt. Catalog policies determine who can use the entry when assembling personal vMCPs. They are separate from the profiles on a shared vMCP.
4. If the server needs headers or static OAuth, finish that configuration using [Register remote servers](../mcp-gateway/register-remote.md).

## Create a governed endpoint

1. Open **vMCPs** and select **Create vMCP**.
2. Drag your server from the MCP Servers panel onto the canvas. Enter a name and description and select **Create**.
3. Configure the component's required values. Use **Preconfigured** for shared values and **Provided at connection** for values each user supplies.
4. When **Add Tools** offers **As-is** or **Managed**, choose **Managed** for this exercise. Discover the tools, select only those needed, and confirm the selection. As-is passes through tools and their definitions, including future upstream changes.
5. Open **Profiles** and grant your test user or group the intended component tools. Review the default administrator profile as well: grants from matching profiles are combined.

## Verify the result

Open **Inspector** in the vMCP designer (or **Test vMCP** from its card) to test the connection, or connect an MCP client using the displayed connection URL. List tools and call one of the allowed tools with harmless input.

For an access test, use an account that matches only the intended profile. Confirm that an ungranted tool is absent from the tool list and cannot be invoked by name. An administrator account may match another broader profile and is not a substitute for testing the intended user.

Open **Audit Logs** and locate the request by user, server, and time. Metadata is visible according to your role; viewing request and response bodies requires the Auditor role.

Keep the resulting connection URL for your clients. Use [vMCP snapshots and updates](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates) when changing its backing catalog entry.
