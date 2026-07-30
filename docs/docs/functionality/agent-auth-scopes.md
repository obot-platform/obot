---
title: Agent Authorization Scopes
---

# Agent Authorization Scopes

Agent authorization scopes provide programmatic access to Obot from scripts, automation tools, external MCP clients, and compatible LLM clients. Instead of using interactive browser-based OAuth authentication, you can create scopes that generate API keys with only the capabilities each integration needs.

## Overview

API keys for agent authorization scopes are designed for machine-to-machine access to Obot. Each scope:

- Can be scoped to specific MCP servers (or all servers)
- Can include optional capabilities for Obot API access, LLM proxy access, skill access, and device scan access
- Can have an optional expiration date
- Is still limited by the owning user's permissions and access policies

The API keys generated for a scope use the format `ok1-<userId>-<keyId>-<secret>` and are passed as Bearer tokens in the Authorization header.

## Creating an Agent Authorization Scope

1. Select **Agent Auth Scopes** in the sidebar.
2. Click **Create Auth Scope**.
3. Fill in the required information:
   - **Name** (required): A descriptive name that identifies the authorization scope's purpose
   - **Description** (optional): Additional context about how the authorization scope is used
   - **Expiration Date** (optional): When the authorization scope should automatically expire. Authorization scopes without an expiration date remain valid until deleted.
   - **MCP Servers**: Select which MCP servers this authorization scope can access. You can:
     - Select **All MCP Servers** to grant access to all servers you currently have access to, including any servers you gain access to in the future
     - Select individual servers to restrict the authorization scope to only those specific servers
     - Leave this empty when you are creating a capability-only scope
   - **API Scopes**: Select any non-MCP capabilities this authorization scope should allow:
     - **LLM proxy access**: Call LLM proxy endpoints
     - **Skill access**: Discover and download skills
     - **Device scan access**: Submit and read device scans
4. Click **Save**.

After creation, you'll see a dialog displaying an initial new API key to use. **Copy and save this key immediately**—it will only be shown once and cannot be retrieved later.

## Using an API Key

Include the API key in the Authorization header when connecting to MCP servers or Obot API endpoints:

```bash
Authorization: Bearer ok1-123-456-abcdefghijklmnopqrstuvwxyz
```

API keys can grant access to:

| Capability | Access granted |
|------------|----------------|
| Selected MCP servers | MCP server connections via the `/mcp-connect/` endpoints |
| API access | Obot API endpoints allowed by your user role |
| LLM proxy access | LLM proxy endpoints such as `/api/llm-proxy/openai` and `/api/llm-proxy/anthropic` |
| Skill access | Skill discovery and downloads |
| Device scan access | Device scan submission and retrieval |

All keys can use `/api/me` to verify authentication.

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

## Managing Agent Authorization Scopes

### Viewing Your Agent Authorization Scopes

Navigate to **Agent Auth Scopes** in the sidebar to see all your agent authorization scopes. The table displays:

| Column | Description |
|--------|-------------|
| Name | The authorization scope's descriptive name |
| Capabilities | Capabilities enabled for the scope; a **Servers** badge indicates MCP server access |
| Last Used | When an API key generated for the scope was last used |
| Expires | When the scope will expire (or "Never" if it has no expiration date) |

### Deleting an Agent Authorization Scope

1. Navigate to **Agent Auth Scopes** in the sidebar
2. Click the three-dot menu on the authorization scope you want to delete
3. Select **Delete**
4. Confirm the deletion

Deleting an agent authorization scope immediately invalidates its API keys. This action cannot be undone.

## MCP Server Access

When you create an agent authorization scope with specific MCP servers, its API keys can connect only to those servers. If you select **All MCP Servers**, its API keys can access:

- All MCP servers you currently have access to
- Any servers you gain access to in the future

Access is still subject to your user permissions. If you lose access to an MCP server (for example, if it's removed from a registry you can access), API keys generated for the authorization scope can no longer connect to that server, even if it was explicitly included when the scope was created.

MCP server access is independent of the optional capabilities. For example, you can create an authorization scope that grants access only to MCP servers, only to the LLM proxy, or to MCP servers and other capabilities.

## Creating an Agent Authorization Scope with the CLI

The `obot login` command creates an agent authorization scope through the browser-based login flow and stores its API key. By default, it requests API access:

```bash
obot login --url https://obot.example.com
```

Use `--scope` to request narrower or additional capabilities:

```bash
obot login --url https://obot.example.com --scope llm --print-token
obot login --url https://obot.example.com --scope all-mcp --scope skills --print-token
```

Valid values for `--scope` are `api`, `llm`, `all-mcp`, and `skills`. You can also set a recognizable name and description for the generated authorization scope:

```bash
obot login \
  --url https://obot.example.com \
  --token-name "CI gateway scope" \
  --token-description "Used by nightly automation" \
  --scope llm \
  --print-token
```

## Admin Management

Administrators can manage agent authorization scopes across all users.

### Viewing All Agent Authorization Scopes

1. Navigate to **Administration > Auth Management > Agent Auth Scopes** in the admin sidebar.
2. View all agent authorization scopes in the system with their associated users.

The admin view includes the same information as the user view, plus a **Created By** column showing which user owns each scope.

### Deleting Any Agent Authorization Scope

Administrators can delete any user's agent authorization scope:

1. Navigate to **Administration > Auth Management > Agent Auth Scopes**.
2. Click the three-dot menu for the authorization scope.
3. Select **Delete**.
4. Confirm the deletion.

## Security Best Practices

- **Use descriptive names**: Name authorization scopes based on their purpose (e.g., "CI/CD Pipeline", "Monitoring Script") to easily identify and manage them
- **Set expiration dates**: For temporary use cases, always set an expiration date
- **Use least privilege**: Enable only the capabilities each authorization scope needs
- **Scope to specific servers**: When possible, limit authorization scopes to only the MCP servers they need rather than using "All MCP Servers"
- **Rotate keys regularly**: Delete old keys and create new ones periodically
- **Never share keys**: Each integration should have its own API key
- **Delete unused keys**: Remove keys that are no longer needed
- **Store securely**: Treat API keys like passwords — never commit them to version control or share them in plain text
