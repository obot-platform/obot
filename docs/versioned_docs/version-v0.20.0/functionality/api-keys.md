---
title: API Keys
---

# API Keys

API Keys provide programmatic access to MCP servers through external MCP clients. Instead of using interactive browser-based OAuth authentication, you can create API keys to authenticate requests from scripts, automation tools, or other applications.

## Overview

API keys are designed for machine-to-machine communication with MCP servers. Each key:

- Belongs to a specific user
- Is scoped to specific MCP servers (or all servers)
- Can have an optional expiration date
- Provides access only to MCP server connections (not the full Obot API)

API keys use the format `ok1-<userId>-<keyId>-<secret>` and are passed as Bearer tokens in the Authorization header.

## Creating an API Key

1. Click your profile icon in the top-right corner of the navigation bar
2. Select **API Keys** from the dropdown menu
   - Note: if you are an admin, you can find the API Keys in the sidebar under the User Management section, rather than in the profile dropdown.
3. Click **Create API Key**
4. Fill in the required information:
   - **Name** (required): A descriptive name to identify the key's purpose
   - **Description** (optional): Additional context about what the key is used for
   - **Expiration Date** (optional): When the key should automatically expire. Keys without an expiration date remain valid until deleted.
   - **MCP Servers** (required): Select which MCP servers this key can access. You can:
     - Select **All MCP Servers** to grant access to all servers you currently have access to, including any servers you gain access to in the future
     - Select individual servers to restrict the key to only those specific servers
5. Click **Create API Key**

After creation, you'll see a dialog displaying the full API key. **Copy and save this key immediately** - it will only be shown once and cannot be retrieved later.

## Using an API Key

Include the API key in the Authorization header when connecting to MCP servers:

```bash
Authorization: Bearer ok1-123-456-abcdefghijklmnopqrstuvwxyz
```

API keys only grant access to:
- MCP server connections via the `/mcp-connect/` endpoints
- The `/api/me` endpoint to verify authentication

They cannot be used to access other Obot API endpoints.

### Testing an API Key

To test an API key, you can use the `/api/me` endpoint:

```bash
curl -H "Authorization: Bearer <key>" <obot host>/api/me
```

If the key is valid, you should receive a response with your user information.

### Configuring MCP Clients

Once you have an API key, you can configure various MCP clients to connect to your Obot MCP servers. The MCP endpoint URL follows this pattern:

```
https://<obot-host>/mcp-connect/<server-name>/mcp
```

Where `<server-name>` is the name of the MCP server you want to connect to.

#### VS Code

Configure your `.vscode/mcp.json` file to connect to Obot MCP servers using HTTP transport with Bearer token authentication:

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "obot-api-key",
      "description": "Obot API Key",
      "password": true
    }
  ],
  "servers": {
    "my-obot-server": {
      "type": "http",
      "url": "<connection URL>",
      "headers": {
        "Authorization": "Bearer ${input:obot-api-key}"
      }
    }
  }
}
```

VS Code will prompt you to enter your API key when connecting. To configure servers globally across all workspaces, add the configuration to your user settings instead.

#### Agno

[Agno](https://www.agno.com/) is a Python agent framework that supports MCP integration. Use `StreamableHTTPClientParams` to configure authorization headers:

```python
from agno.agent import Agent
from agno.models.openai import OpenAIChat
from agno.tools.mcp import MCPTools
from agno.tools.mcp.params import StreamableHTTPClientParams
from os import getenv

# Configure the MCP server connection with authorization
server_params = StreamableHTTPClientParams(
    url="<connection URL>",
    headers={
        "Authorization": f"Bearer {getenv('OBOT_API_KEY')}",
    },
)

async def main():
    async with MCPTools(
        transport="streamable-http",
        server_params=server_params
    ) as mcp_tools:
        agent = Agent(
            model=OpenAIChat(id="gpt-4o"),
            tools=[mcp_tools],
            markdown=True,
        )
        await agent.aprint_response("Your prompt here", stream=True)

if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
```

#### LangChain

[LangChain MCP Adapters](https://github.com/langchain-ai/langchain-mcp-adapters) enable connecting LangChain agents to MCP servers. Configure the `MultiServerMCPClient` with HTTP transport and authorization headers:

```python
from os import getenv
from langchain_mcp_adapters.client import MultiServerMCPClient
from langchain.agents import create_agent

# Configure the MCP client with authorization
client = MultiServerMCPClient(
    {
        "obot-server": {
            "transport": "http",
            "url": "<connection URL>",
            "headers": {
                "Authorization": f"Bearer {getenv('OBOT_API_KEY')}",
            },
        }
    }
)

tools = await client.get_tools()
agent = create_agent("openai:gpt-4.1", tools)
response = await agent.ainvoke({"messages": "your message here"})
```

## Managing API Keys

### Viewing Your Keys

Navigate to **Profile > API Keys** to see all your API keys. The table displays:

| Column | Description |
|--------|-------------|
| Name | The key's descriptive name |
| Description | Additional context about the key |
| Servers | Number of MCP servers the key can access |
| Created | When the key was created |
| Last Used | When the key was last used for authentication |
| Expires | When the key will expire (or "Never" if no expiration) |

### Deleting an API Key

1. Navigate to **Profile > API Keys**
2. Click the three-dot menu on the key you want to delete
3. Select **Delete**
4. Confirm the deletion

Deleted keys are immediately invalidated and cannot be recovered.

## MCP Server Access

When you create an API key with specific MCP servers, the key can only connect to those servers. If you select **All MCP Servers**, the key can access:

- All MCP servers you currently have access to
- Any servers you gain access to in the future

Access is still subject to your user permissions. If you lose access to an MCP server (for example, if it's removed from a registry you have access to), the API key will no longer be able to connect to that server, even if it was explicitly included when the key was created.

## Admin Management

Administrators can manage API keys across all users.

### Viewing All API Keys

1. Navigate to **User Management > API Keys** in the admin sidebar
2. View all API keys in the system with their associated users

The admin view includes the same information as the user view, plus a **User** column showing which user owns each key.

### Deleting Any API Key

Administrators can delete any user's API key:

1. Navigate to **User Management > API Keys**
2. Click the three-dot menu on the key
3. Select **Delete**
4. Confirm the deletion

## Security Best Practices

- **Use descriptive names**: Name keys based on their purpose (e.g., "CI/CD Pipeline", "Monitoring Script") to easily identify and manage them
- **Set expiration dates**: For temporary use cases, always set an expiration date
- **Scope to specific servers**: When possible, limit keys to only the MCP servers they need rather than using "All MCP Servers"
- **Rotate keys regularly**: Delete old keys and create new ones periodically
- **Never share keys**: Each integration should have its own API key
- **Delete unused keys**: Remove keys that are no longer needed
- **Store securely**: Treat API keys like passwords - never commit them to version control or share them in plain text
