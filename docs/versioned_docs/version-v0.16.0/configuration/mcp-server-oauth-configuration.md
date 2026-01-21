# MCP Server OAuth Configuration

Some MCP Servers do not have dynamic OAuth, and require pre-registering an OAuth client with the provider. One such example of this is the GitHub remote MCP server. The following steps are required to successfully use the remote hosted GitHub MCP server.


1. Go to github.com/settings/developers and create a new OAuth app. The callback URL needs to be `<obot host>/oauth/mcp/callback`. You'll need the client ID and secret from this setup.
2. Get a token you can use to interact with the Obot API: `OBOT_BASE_URL=<obot host>/api obot token`. If Obot is running at `localhost:8080`, then you can leave off the `OBOT_BASE_URL` part.
3.Run the following curl command:
```
curl -H 'Authorization: Bearer <obot token>' \
<obot host>/api/oauth-apps \
-d '{"clientID":"<CLIENT ID>","clientSecret":"<CLIENT SECRET>","type":"github","alias":"github","authorizationServerURL":"https://github.com/login/oauth"}'
```

After this you should be able to add `https://api.githubcopilot.com/mcp` as a remote MCP server.