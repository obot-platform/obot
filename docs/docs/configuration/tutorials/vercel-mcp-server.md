---
title: "Vercel MCP Server"
displayed_sidebar: sidebar
---

# Vercel MCP Server

Obot's MCP Catalog includes an entry for [Vercel's official MCP server](https://vercel.com/docs/agent-resources/vercel-mcp). It is ready to use: there is no Vercel app to create and no credentials to configure.

### Why Vercel sign-in uses the Obot CLI

Vercel's OAuth server only accepts approved clients or redirects to `localhost`. Obot can't receive a `localhost` redirect itself, so the Vercel entry uses a [localhost OAuth callback](../../mcp-gateway/register-remote.md#localhost-oauth-callbacks): you run a short `obot mcp login` command on your own computer, it receives the redirect from Vercel, and it hands the sign-in back to Obot. You only do this once; after that, your MCP client connects to Obot as usual.

### Connect to Vercel

1. Add the Vercel catalog entry to a [vMCP](../../mcp-gateway/server-types.md), or use a vMCP that already includes it.
2. Install the [Obot CLI](../../reference/cli-api.md) on a computer with a web browser.
3. Connect to the vMCP. When Obot asks you to sign in to Vercel, select **Authenticate**.
4. Copy the displayed `obot mcp login` command and run it. Sign in to Vercel in the browser that opens, and grant access to the teams you want to use.
5. Return to Obot. The connection continues once sign-in finishes.

The command expires after ten minutes. See [MCP localhost OAuth sign-in](../../reference/cli-api.md#mcp-localhost-oauth-sign-in) if it expires or the CLI cannot open a local port.

Vercel tools act with your existing Vercel permissions, including changes to projects, domains, and environment variables. To give users read-only access, disable the write tools in the vMCP or grant only read tools in its profiles.
