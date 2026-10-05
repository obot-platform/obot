---
title: "Add remote MCP servers"
---

Register an existing HTTP MCP endpoint when the server runs outside Obot. Obot proxies the connection; it does not deploy or sandbox that remote service.

1. Open **MCP Servers**, select **Add MCP Server**, and choose **Remote Server**.
2. Enter a name, short description, and the server's MCP URL.
3. Configure required headers or user-supplied values. A remote service must be reachable from Obot, or through a configured [MCP tunnel](./filters-tunnels.md#mcp-tunnels-create-an-mcp-tunnel).
4. If the provider requires a pre-registered OAuth application, enable **Static OAuth** and configure its client credentials using [MCP Server OAuth Configuration](../configuration/mcp-server-oauth-configuration.md).
5. Save the entry and review catalog access policies.
6. Add the entry to a [vMCP](./server-types.md), configure its tools and profiles, and test the resulting connection.

For a complete provider-specific setup, see the [Slack MCP tutorial](../configuration/tutorials/slack-mcp-server.md).

Obot blocks localhost, private-IP, and link-local remote destinations by default. Use a tunnel for private services when appropriate. Do not disable these protections globally just to make an arbitrary endpoint connect. See [Network isolation](../security/isolation.md) for the separate gateway and workload boundaries.

## Remote server {#mcp-servers-remote-server}

MCP Servers that are HTTP Streaming compatible should be configured this way. These servers can be provided by trusted 3rd party vendors. Remote servers also work for MCP servers deployed through existing CI/CD pipeline within the organization.

Choose this type when:

- You have MCP services deployed through traditional application deployment mechanisms
- External partners provide MCP endpoints and you just want to integrate
- You are building MCP servers through existing CI/CD workflows or SaaS services

Remote MCP servers that conform to the MCP spec authentication schema will work out of the box. Servers that do not conform to the spec may not work within the gateway. Please open a GitHub issue if you run into issues with remote servers.

**Configuration**: Specify the remote URL endpoint. Additional options include connection restrictions for unconventional configurations, custom HTTP headers, and configuration values to send to the remote server.

If Obot cannot directly reach a remote server, use an [MCP tunnel](./filters-tunnels.md#mcp-tunnels-create-an-mcp-tunnel) to route requests through a machine on the server's network. Keep the remote server's real HTTP or HTTPS URL and select the tunnel separately in **Advanced Configuration**.
