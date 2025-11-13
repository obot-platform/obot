# IDE/Client Integration

This guide covers integrating Obot MCP servers with various IDEs and client applications through the my-connections interface.

## Setting Up MCP Server Connections

### Step 1: Access My Connections

Navigate to the **my-connections** page in your Obot interface by clicking on the user avatar in the upper right of the UI. This page is where you manage MCP server connections.

### Step 2: Connect to an MCP Server

1. Select an MCP server from the available list
2. Click the **"Connect to Server"** button
3. Fill out any necessary connection information (credentials, endpoints, etc.)
4. Save your configuration

### Step 3: Get Client Integration Code

Once your MCP server is connected:

1. Click the **"Chat"** button next to your connected server, Or..
2. You'll see integration snippets for popular clients:
   - **Claude Desktop**
   - **VS Code** with MCP extension
   - **Cursor** IDE

## Managing Multiple Connections

### My Connectors Page

In the **my-connectors** page, you can:

- View all **Enabled Connectors**
- Get individual configuration for each connector
- Click **"Generate Configuration"** to create a complete config file with all enabled connectors

### Supported Clients

Obot supports clients that supports the current version of the spec. If there is an issue connecting a client, please file an issue on GitHub.

#### Clients with Direct Remote Support

For clients that support remote MCP connections (like VS Code, Cursor):

```json
{
  "servers": {
    "server-name": {
      "url": "http://localhost:8080/mcp-connect/your-server-id"
    }
  }
}
```

Or for Cursor IDE specifically:

```json
{
  "mcpServers": {
    "server-name": {
      "url": "http://localhost:8080/mcp-connect/your-server-id"
    }
  }
}
```

#### Clients Requiring mcp-remote

For clients that don't support remote connections directly (like Claude Desktop):

```json
{
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "http://localhost:8080/mcp-connect/your-server-id"
      ]
    }
  }
}
```

#### Specific Client Examples

**VS Code MCP Extension:**

```json
{
  "servers": {
    "Asana": {
      "url": "http://localhost:8080/mcp-connect/default-asana-877addce"
    }
  }
}
```

**Claude Desktop:**

```json
{
  "mcpServers": {
    "Asana": {
      "command": "npx",
      "args": [
        "mcp-remote@latest",
        "http://localhost:8080/mcp-connect/default-asana-877addce"
      ]
    }
  }
}
```

**Cursor IDE:**

```json
{
  "mcpServers": {
    "Brave Search": {
      "url": "http://localhost:8080/mcp-connect/default-brave-search-523e4405"
    },
    "Firecrawl": {
      "url": "http://localhost:8080/mcp-connect/default-firecrawl-c9e4d337"
    },
    "Asana": {
      "url": "http://localhost:8080/mcp-connect/default-asana-877addce"
    }
  }
}
```

## Configuration Management

### Individual Connector Config

For each enabled connector, you can:

- Edit the configuration
- View connection status
- Update credentials or settings
- Disable/enable as needed