# MCP Server OAuth Configuration

Some remote MCP servers require OAuth authentication with pre-registered client credentials. Unlike servers that support dynamic OAuth registration, these servers need administrators to configure a static set of OAuth credentials (Client ID and Client Secret) that all users share.

## Overview

Static OAuth allows you to:

- Connect to remote MCP servers that require pre-registered OAuth applications
- Configure a single set of OAuth credentials that all users share
- Manage OAuth settings through the Obot admin interface

When static OAuth is configured for a remote MCP server:

1. Administrators register an OAuth application with the provider and enter the credentials in Obot
2. Users can add the MCP server to their projects without needing their own OAuth apps
3. Each user still authenticates individually through the OAuth flow using the shared client credentials

## Configuring static OAuth

### Step 1: Register an OAuth application with the provider

Before configuring Obot, you need to register an OAuth application with the service provider. The specific steps vary by provider, but generally:

1. Go to the provider's developer settings or OAuth application management page
2. Create a new OAuth application
3. Set the callback/redirect URL to: `https://<your-obot-host>/oauth/mcp/callback`
4. Note the **Client ID** and **Client Secret** provided by the service

### Step 2: Create or edit a remote MCP server

1. Navigate to **MCP Management > MCP Servers** in the Obot admin interface
2. Click **Add MCP Server** and select **Remote Server**, or edit an existing remote server
3. Enter the remote server URL
4. Click **Advanced Configuration** to reveal additional options

### Step 3: Enable static OAuth

1. In the advanced configuration section, toggle **Static OAuth** to enabled
2. Click **Save** to create or update the remote MCP server

### Step 4: Configure OAuth credentials

After saving the remote MCP server with static OAuth enabled:

1. Click **Configure OAuth Credentials** in the Static OAuth section
2. Enter the following information:
   - **Client ID**: The client ID from your registered OAuth application
   - **Client Secret**: The client secret from your registered OAuth application
3. Click **Save**

Once configured, the MCP server becomes available to users.

## Managing OAuth credentials

### Viewing credential status

The remote MCP server shows whether OAuth credentials are configured. When viewing a remote MCP server with static OAuth:

- If credentials are configured, you'll see the Client ID
- The Client Secret is never displayed after initial configuration

### Changing client credentials

To change the Client ID and Client Secret:

1. Click **Configure OAuth Credentials**
2. Click **Clear Credentials**
3. Confirm the deletion
4. Re-enter OAuth credentials with the new values

Clearing credentials temporarily makes the MCP server unavailable to users until new credentials are configured.

## Example: GitHub MCP Server

This example demonstrates configuring the GitHub remote MCP server.

### Register a GitHub OAuth application

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Click **OAuth Apps** > **New OAuth App**
3. Fill in the application details:
   - **Application name**: A descriptive name (e.g., "Obot MCP Integration")
   - **Homepage URL**: Your Obot instance URL
   - **Authorization callback URL**: `https://<your-obot-host>/oauth/mcp/callback`
4. Click **Register application**
5. Copy the **Client ID**
6. Click **Generate a new client secret** and copy the secret

### Configure the remote MCP server in Obot

1. Navigate to **MCP Management > MCP Servers**
2. Click **Add MCP Server** > **Remote Server**
3. Enter the server details:
   - **Name**: GitHub MCP
   - **Description**: Access GitHub repositories and features
4. Enter the URL: `https://api.githubcopilot.com/mcp`
5. Click **Advanced Configuration**
6. Toggle **Static OAuth** to enabled
7. Click **Save**

### Add OAuth credentials

1. Click **Configure OAuth Credentials**
2. Enter:
   - **Client ID**: Your GitHub OAuth app client ID
   - **Client Secret**: Your GitHub OAuth app client secret
3. Click **Save**

Users can now add the GitHub MCP server to their projects and authenticate with their GitHub accounts.

## Visibility and access control

Remote MCP servers that require static OAuth but don't have credentials configured are hidden from non-admin users. This prevents users from seeing MCP servers they cannot actually use.

Once an administrator configures the OAuth credentials, the server becomes visible to all users with appropriate access permissions.

## Limitations

- **Composite servers**: Remote servers with static OAuth cannot be included as components in composite servers. This will be addressed in a future release.
