# Slack MCP Server

Slack has an [official remote MCP server](https://docs.slack.dev/ai/slack-mcp-server/) that Obot can connect to. This guide explains how to set that up.

### 1. Configure a Slack app

1. Go to [https://api.slack.com/apps](https://api.slack.com/apps) and click Create New App.
2. Select `From a manifest`
3. Select your Slack workspace
4. Switch to the YAML tab and paste in the following manifest, then fill in the `<YOUR APP NAME>` and `<YOUR OBOT HOST>` placeholders with the name you want for this Slack app, and the host where your Obot instance is running

```yaml
display_information:
  name: <YOUR APP NAME>
oauth_config:
  redirect_urls:
    - https://<YOUR OBOT HOST>/oauth/mcp/callback
  scopes:
    user:
      - search:read.mpim
      - search:read.private
      - search:read.public
      - search:read.im
      - search:read.files
      - files:read
      - emoji:read
      - search:read.users
      - chat:write
      - channels:history
      - groups:history
      - mpim:history
      - im:history
      - channels:write
      - groups:write
      - im:write
      - mpim:write
      - reactions:write
      - canvases:read
      - canvases:write
      - users:read
      - users:read.email
      - channels:read
      - groups:read
      - mpim:read
      - reactions:read
  pkce_enabled: true
settings:
  org_deploy_enabled: false
  socket_mode_enabled: false
  token_rotation_enabled: false
  is_mcp_enabled: true
```

5. Take a look at the User Scopes if you would like (these are the permissions that the Slack MCP server will use on behalf of users that connect to it), then click `Create`
6. Note the Client ID and Client Secret for your app, as these will be needed next
7. In the sidebar, under `Settings`, click `Install App`
8. Click the green button to install your app into your workspace, then click `Allow`

### 2. Configure the server in Obot

:::note
Obot's default MCP catalog includes a catalog entry for Slack. You can use that instead of creating your own, if you are using the default catalog.
:::

1. On Obot's MCP Catalog page, click the `Add Catalog Entry` button and select `Remote Server`
2. Name the server and write a description if you would like
3. In the URL field, enter `https://mcp.slack.com/mcp`
4. Click on `Advanced Configuration` and enable Static OAuth
5. Click `Save`
6. In the Configure Static OAuth box, enter the Client ID and Client Secret from your Slack app, and click `Save`

After this, you will see the connection URL. Your users can now connect to the Slack MCP server. Note that this server will only work with the Slack workspace that you installed the Slack app into.
