# Virtual/Composite MCP Server (VMCP) Strawman

## Phase 1 - User Stories

As an **Obot Admin**, I want to be able to construct a catalog entry such that:

- It combines one or more distinct single, remote, and multi-user MCP catalog entries into a single, sharable entry
- When launched by users, a single connection URL is produced, which acts as a single, unified, remote MCP server to connected clients
- Only the tools, resources, resource templates, and prompts I choose from its component MCP servers are made available to clients
- I can modify the client-facing:
    - tool and tool parameter names and descriptions
    - prompt and prompt parameter names and descriptions
    - resource names, titles, and descriptions
    - resource template names, titles, and descriptions

## Notes:

### Creating a VMCP Catalog Entry

Walking through the admin’s frontend UX for creating a VMCP:

1. Click `Add MCP Server` on `/admin/mcp-servers`
2. Select `Virtual MCP Server` from the options displayed in the `Select Server Type` dialog
3. Enter a `Name` and optionally a `Description` and `Icon URL`
4. Click `+ MCP Server` to open the default catalog dialog (like adding to a project)
    1. Select a server
    2. Click `Add To Virtual MCP Server`
5. Added MCP server is in list
6. After MCP servers are selected, head down to the tool config section below
7. Click a button to auto-populate tools (required if you want to lock down tools)
    1. This runs through starting each MCP server added above (tool preview style) and populating the list
    2. Disable/enable/edit each available tool and parameter name/description as needed 
8. Resource/Resource Template/Prompt configuration will also happen here, but the details are TBD
9. Select `Create`

### Launching a VMCP Server

Walking through the consumer’s frontend UX for launching a VMCP server:

1. Select the VMCP server from the `Available Connectors` list on `/mcp-servers` or when adding a connector to a project
2. Click `Connect To Server` 
3. If optional/required configuration is present on any of the MCP servers that compose the VMCP, user is presented with a configuration dialog containing the combined config
    1. If adding to a project, OAuth prompt(s) are also surfaced

### Roundtrip Name Transforms

- Tool and tool parameter names need to be transformed:
    - Client → Server: To the native component MCP server names
    - Server → Client: To the VMCP configured names

### Level-2 Client Auth

- When an external client connects to a VMCP server, they must be sent through the OAuth flow for every “OAuth MCP server” in the VMCP
- Clients should be redirected to a page that allows them to `Authenticate` or `Skip` authentication for each component MCP server that requires OAuth
- Any clients that are skipped or that Oauth isn’t completed for should not be proxied to (i.e. Their tools/resources/prompts don’t aren’t returned when requested)
- When proxying requests to the component MCP servers, we need to ensure requests from Client → Server have the corresponding auth token attached

### Access Control

- User access to a Virtual MCP Server (VMCP) catalog entry  must be controlled via ACR (Access Control Rule), like any other entry
- Access to a VMCPs component servers:
    - is not required to use the VMCP
    - does not grant access to use the VMCP

### Updates

- When the definition of an MCP server’s entry changes, VMCP definitions that utilize it should update automatically when possible*
    - Actually, we want an admin to manually upgrade like we do with running servers
