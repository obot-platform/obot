---
title: "Connect to an existing Obot instance"
---

Use this quickstart when your organization already has an Obot URL and has granted you access to a vMCP.

## Before you begin

Ask your administrator for the Obot URL, a sign-in account, and the name or connection URL of an approved vMCP. Use an MCP client that supports streamable HTTP and OAuth, or one that can supply a Bearer token. You do not need model-provider credentials to connect to MCP tools.

## Connect and verify

1. Open Obot and sign in using its configured authentication provider.
2. Open **vMCPs** and select the endpoint your administrator shared with you.
3. Select **Connect**. Supply any requested connection configuration and authorize upstream services when prompted.
4. Copy the connection URL or use the client instructions provided by Obot. The URL has the form `https://<obot-host>/mcp-connect/<vmcp-id>`.
5. Add that URL to your AI client's MCP configuration and complete its Obot sign-in flow.
6. Refresh the client's tool list and run a harmless read-only tool. Confirm that the expected tool is available and produces a result.

A missing endpoint or tool can be an access-policy issue. Ask the administrator to check the vMCP's profiles and exposed tools; authenticating successfully does not grant additional access.

For clients that need an API key, follow [Connect AI clients](../mcp-gateway/connect-clients.md). For the Obot CLI and local bootstrap skills, follow [CLI setup](../reference/cli-api.md). CLI setup is optional for clients that connect directly through OAuth.

For model access instead, follow [Connect clients and applications](../llm-gateway/connect-clients.md).
