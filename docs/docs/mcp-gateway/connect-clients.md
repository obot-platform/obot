---
title: Connect AI clients
---

For new connections, use a [vMCP](./server-types.md), even when it exposes only one server. Copy its connection URL from Obot rather than constructing one from the catalog entry's ID.

## OAuth clients

### Connect to a vMCP {#virtual-mcps-connect-to-a-vmcp}


Paste the vMCP's connection URL into the AI client's MCP server settings. On first connection, sign in to Obot, supply any requested configuration, and authorize any remote services that require OAuth. Obot saves this setup for your user account.

To set up a connection in the UI:

1. Select **Connect** on the vMCP.
2. To finish setup before opening the client, follow **click here** under **Preconfigure**, then select **Continue**. If setup is unnecessary, copy the URL directly.
3. If a configuration form appears, supply the fields requested at connection time and select **Configure**.
4. Complete upstream OAuth authentication if a remote component requires it.
5. Copy the **Connection URL** or use the instructions for a supported client.

The endpoint uses Streamable HTTP and has this form:

```text
https://your-obot-instance/mcp-connect/{vmcp-id}
```

When the same user reconnects to this vMCP, Obot uses their saved setup. To change a requested value, open **Connect** again. Under **Preconfigure**, follow the **click here** link to edit configuration, change the values, and select **Update**. This link appears when the vMCP has user-configurable fields. Other users sign in and supply their own requested values; each receives the tools their access grants allow.

To test the connection inside Obot before setting up an AI client, follow the verification steps below.

## Clients using API keys

Create an [agent authorization scope](../functionality/agent-auth-scopes.md) with only the MCP capabilities the integration needs. Configure the client's HTTP transport with the copied connection URL and an `Authorization: Bearer <key>` header. The scope cannot exceed its owner's access.

The authorization-scope guide includes [VS Code](../functionality/agent-auth-scopes.md#vs-code), [Agno](../functionality/agent-auth-scopes.md#agno), and [LangChain](../functionality/agent-auth-scopes.md#langchain) examples. Keep keys in client-supported secret storage rather than source control.

## Verify

### Test a tool in Obot

1. Open **vMCPs**, open the vMCP, and select **Inspector**. Alternatively, select **Test vMCP** using the icon beside **Connect** on its card.
2. If **Start Session** appears, select it and complete the connection prompts. Supply any requested fields and authorize remote services if prompted.
3. Inspect the displayed tool list. Select a read-only tool whose effect you understand, fill in its arguments, and select **Call**.
4. Confirm that the result shows **Succeeded** and the expected output. Record the vMCP name, tool name, and time of the call.

Inspector tests access as the signed-in user. An administrator's successful test does not verify another user's grants. To check a user's experience, that user should repeat the test under their own account.

### Check the audit record

An administrator or auditor can open **Operations > Audit Logs**, select **MCP**, and locate the recent `tools/call` record using the vMCP name, tool name, and timestamp. Check the user attribution and outcome. Users without audit access can give those details to an administrator or auditor to check for them.

### Test the AI client

After adding the copied connection URL to the AI client, open its MCP tools list and run the same read-only tool. Have an administrator or auditor check the new audit record. This verifies the client's connection as well as the vMCP's configuration.

If tools are missing, ask the vMCP administrator to check the component's selected tools and the user's profile grants. For connection or OAuth errors, see [Troubleshooting](./troubleshooting.md).
