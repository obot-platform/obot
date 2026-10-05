---
title: Connect AI clients
---

### Connect to a vMCP {#virtual-mcps-connect-to-a-vmcp}

Using a vMCP's connection URL in an MCP client of your choice will automatically create an instance on your first connection and you will be prompted to provide configuration or authenticate with any third-party services.

You can also setup a connection in the UI by:

1. Select **Connect** on the vMCP.
2. Select **Continue** or **Configure** when prompted.
3. Supply fields marked **Provided at connection**.
4. Complete upstream OAuth authentication if a component requires it.
5. Copy the **Connection URL** or use the instructions for a supported client.

The endpoint uses streamable HTTP and has this form:

```text
https://your-obot-instance/mcp-connect/{vmcp-id}
```

Reconnecting or updating configuration reuses that instance instead of creating another connection identity. Each user authenticates to the Obot Gateway and receives only the tools granted to that user.

Use **Test vMCP** beside the connection action to configure the connection if needed and open Obot's MCP tester.

