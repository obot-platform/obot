# MCP Gateway Overview

The MCP Gateway allows Obot to expose MCP servers only to authorized users. The gateway acts as a bridge between your MCP client and MCP servers that are running remotely or managed by Obot. The Gateway can be used with Obot Chat or with an external MCP client.

## What is MCP?

The Model Context Protocol (MCP) is an open standard that enables secure, standardized connections between AI agents and external resources. MCP allows agents to:

- **Access Tools**: Execute functions in external applications
- **Read Data**: Query databases, APIs, and file systems  
- **Authenticate**: Securely access protected resources
- **Stream Events**: Receive real-time updates from external systems

## MCP Gateway Architecture

The obot MCP Gateway provides several key components:

### MCP Server Management
- **Server Registry**: Catalog of available MCP servers and their capabilities
- **Session Management**: Handle user sessions and authentication contexts
- **Health Monitoring**: Track server availability and performance

### Authentication & Security
- **OAuth 2.1 Flows**: Secure authentication with external services
- **Token Management**: Store and refresh access tokens automatically
- **Credential Isolation**: Separate credentials per user and thread

## Core Concepts

### MCP Servers

### MCP Server Catalogs
MCP server catalogs are collections of available MCP servers:
- **Public Catalogs**: Community-maintained MCP server registries
- **Private Catalogs**: Organization-specific and custom MCP server collections
- **Verified Tools**: Recommended MCP servers for common use cases

### Authentication Flows
The MCP Gateway supports various authentication methods:
- **OAuth 2.1**: For services like Google, Microsoft, GitHub, etc
- **API Keys**: For simple token-based authentication
- **Basic Auth**: Username/password combinations
- **Custom Auth**: Specialized authentication schemes

## Gateway Capabilities

### Dynamic Tool Loading
- **Runtime Discovery**: Find and connect to new MCP servers automatically
- **Hot Swapping**: Add or remove tools without restarting
- **Dependency Resolution**: Manage tool dependencies and conflicts

### Monitoring & Observability
- **Request Tracking**: Monitor all MCP interactions
- **Performance Metrics**: Track response times and success rates
- **Error Logging**: Detailed error reporting and debugging
- **Audit Trails**: Complete record of tool usage for compliance

## Security Considerations

### Credential Management
- **Secure Storage**: Encrypted credential storage
- **Scope Limitation**: Minimal required permissions
- **Regular Rotation**: Automatic token refresh and rotation
- **Audit Logging**: Track all credential usage

### Network Security
- **HTTPS Only to the Gateway**: All external MCP communication over encrypted channels
- **Rate Limiting**: Prevent abuse and DoS attacks

### Access Control
- **User Isolation**: Separate credentials per user
- **Role-Based Access**: Control which tools users can access
- **Audit Compliance**: Meet security or regulatory requirements
