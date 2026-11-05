---
displayed_sidebar: sidebar
title: Agent Authorization Scopes
---

For the current task-based instructions, see [Credentials and encryption](../security/credentials.md). This page preserves existing bookmarks and specialized reference material.

Agent authorization scopes provide programmatic access to Obot from scripts, automation tools, external MCP clients, and compatible LLM clients. Instead of using interactive browser-based OAuth authentication, you can create scopes that generate API keys with only the capabilities each integration needs.

## Overview {#overview}


API keys for agent authorization scopes are designed for machine-to-machine access to Obot. Each scope:

- Can be scoped to specific MCP servers (or all servers)
- Can include optional capabilities for Obot API access, LLM proxy access, skill access, and device scan access
- Can have an optional expiration date
- Is still limited by the owning user's permissions and access policies

The API keys generated for a scope use the format `ok1-<userId>-<keyId>-<secret>` and are passed as Bearer tokens in the Authorization header.

## Creating an Agent Authorization Scope {#creating-an-agent-authorization-scope}

Continue to [Creating an Agent Authorization Scope](../security/credentials.md#agent-auth-scopes-creating-an-agent-authorization-scope).

## Using an API Key {#using-an-api-key}


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

### Testing an API Key {#testing-an-api-key}


To test an API key, you can use the `/api/me` endpoint:

```bash
curl -H "Authorization: Bearer <key>" <obot host>/api/me
```

If the key is valid, you should receive a response with your user information.

### Configuring MCP Clients {#configuring-mcp-clients}


Once you have an API key, you can configure various MCP clients to connect to your Obot MCP servers. The MCP endpoint URL follows this pattern:

```
https://<obot-host>/mcp-connect/<vmcp-id>
```

Copy the vMCP's **Connection URL** from Obot. A catalog entry ID is not a connection identifier.

#### VS Code {#vs-code}


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

#### Agno {#agno}


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

#### LangChain {#langchain}


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

## Managing Agent Authorization Scopes {#managing-agent-authorization-scopes}

Continue to [Managing Agent Authorization Scopes](../security/credentials.md#agent-auth-scopes-managing-agent-authorization-scopes).

### Viewing Your Agent Authorization Scopes {#viewing-your-agent-authorization-scopes}

Continue to [Viewing Your Agent Authorization Scopes](../security/credentials.md#agent-auth-scopes-viewing-your-agent-authorization-scopes).

### Deleting an Agent Authorization Scope {#deleting-an-agent-authorization-scope}

Continue to [Deleting an Agent Authorization Scope](../security/credentials.md#agent-auth-scopes-deleting-an-agent-authorization-scope).

## MCP Server Access {#mcp-server-access}


When you create an agent authorization scope with specific MCP servers, its API keys can connect only to those servers. If you select **All MCP Servers**, its API keys can access:

- All MCP servers you currently have access to
- Any servers you gain access to in the future

Access is still subject to your user permissions. If you lose access to an MCP server (for example, if it's removed from a registry you can access), API keys generated for the authorization scope can no longer connect to that server, even if it was explicitly included when the scope was created.

MCP server access is independent of the optional capabilities. For example, you can create an authorization scope that grants access only to MCP servers, only to the LLM proxy, or to MCP servers and other capabilities.

## Creating an Agent Authorization Scope with the CLI {#creating-an-agent-authorization-scope-with-the-cli}


The `obot login` command creates an agent authorization scope through the browser-based login flow and stores its API key. By default, it requests the `llm`, `skills`, and `device-scans` scopes. The generated key therefore cannot call general Obot API endpoints unless you request the `api` scope explicitly:

```bash
obot login --url https://obot.example.com
```

Use `--scope` to request narrower or additional capabilities:

```bash
obot login --url https://obot.example.com --scope llm --print-token
obot login --url https://obot.example.com --scope all-mcp --scope skills --print-token
```

Valid values for `--scope` are `api`, `llm`, `skills`, `device-scans`, and `all-mcp`. Passing `--scope` replaces the defaults rather than adding to them, so list every scope the key needs. You can also set a recognizable name and description for the generated authorization scope:

```bash
obot login \
  --url https://obot.example.com \
  --token-name "CI gateway scope" \
  --token-description "Used by nightly automation" \
  --scope llm \
  --print-token
```

## Admin Management {#admin-management}


Administrators can manage agent authorization scopes across all users.

### Viewing All Agent Authorization Scopes {#viewing-all-agent-authorization-scopes}


1. Navigate to **Identity & Access > Agents** in the admin sidebar.
2. View all agent authorization scopes in the system with their associated users.

The admin view includes the same information as the user view, plus a **Created By** column showing which user owns each scope.

### Deleting Any Agent Authorization Scope {#deleting-any-agent-authorization-scope}


Administrators can delete any user's agent authorization scope:

1. Navigate to **Identity & Access > Agents**.
2. Click the three-dot menu for the authorization scope.
3. Select **Delete**.
4. Confirm the deletion.

## Security Best Practices {#security-best-practices}


- **Use descriptive names**: Name authorization scopes based on their purpose (e.g., "CI/CD Pipeline", "Monitoring Script") to easily identify and manage them
- **Set expiration dates**: For temporary use cases, always set an expiration date
- **Use least privilege**: Enable only the capabilities each authorization scope needs
- **Scope to specific servers**: When possible, limit authorization scopes to only the MCP servers they need rather than using "All MCP Servers"
- **Rotate keys regularly**: Delete old keys and create new ones periodically
- **Never share keys**: Each integration should have its own API key
- **Delete unused keys**: Remove keys that are no longer needed
- **Store securely**: Treat API keys like passwords — never commit them to version control or share them in plain text
