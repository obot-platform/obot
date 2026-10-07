---
title: "Connect to a vMCP"
---

Use a vMCP to connect your AI client to tools available through Obot. You can connect to one an administrator shared with you or one you created yourself. You do not need to create another endpoint or have access to a shared vMCP's source catalog entries.

## Connect your client

You need your organization's Obot URL, a sign-in account, and access to the vMCP. Use an MCP client that supports Streamable HTTP and OAuth, or follow the [API-key instructions](../mcp-gateway/connect-clients.md#clients-using-api-keys). MCP connections do not require model-provider credentials.

1. Sign in to Obot, open **vMCPs**, and select the vMCP you created or that your administrator shared.
2. Select **Connect**. Use the provided client instructions or copy the **Connection URL**. It has the form `https://<obot-host>/mcp-connect/<vmcp-id>`.
3. Add the URL to your AI client's MCP configuration and complete the Obot sign-in flow. Supply any requested connection values and authorize upstream services when prompted.
4. Refresh the client's tool list and run a read-only tool. Confirm that it returns the expected result.

If an endpoint or tool is missing, ask the administrator to check its selected tools and profile grants. For configuration changes, API keys, or connection errors, see [Connect AI clients](../mcp-gateway/connect-clients.md) and [Troubleshooting](../mcp-gateway/troubleshooting.md).

## Next steps

- [Check the audit record](../mcp-gateway/connect-clients.md#check-the-audit-record) with an administrator or auditor.
- [Create a vMCP](./govern.md) if you want to assemble your own endpoint from catalog servers.
- Browse [Explore Obot](./choose.md) for model access, skills, and device management.
