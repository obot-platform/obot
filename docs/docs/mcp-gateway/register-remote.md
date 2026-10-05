---
title: "Add remote MCP servers"
---

## Remote server {#mcp-servers-remote-server}

MCP Servers that are HTTP Streaming compatible should be configured this way. These servers can be provided by trusted 3rd party vendors. Remote servers also work for MCP servers deployed through existing CI/CD pipeline within the organization.

Choose this type when:

- You have MCP services deployed through traditional application deployment mechanisms
- External partners provide MCP endpoints and you just want to integrate
- You are building MCP servers through existing CI/CD workflows or SaaS services

Remote MCP servers that conform to the MCP spec authentication schema will work out of the box. Servers that do not conform to the spec may not work within the gateway. Please open a GitHub issue if you run into issues with remote servers.

**Configuration**: Specify the remote URL endpoint. Additional options include connection restrictions for unconventional configurations, custom HTTP headers, and configuration values to send to the remote server.

If Obot cannot directly reach a remote server, use an [MCP Tunnel](../functionality/mcp-tunnels.md) to route requests through a machine on the server's network. Keep the remote server's real HTTP or HTTPS URL and select the tunnel separately in **Advanced Configuration**.

