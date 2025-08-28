---
title: Adding MCP Servers
description: Adding MCP servers to the Obot Gateway
---

## Overview

Managing MCP servers in the Obot gateway starts with adding them to the main catalog. Administrators can control which servers are available to users and how they are configured. Servers may be added individually through the UI, or managed via a Git repository.

The choice of server type depends on how the MCP server was developed. All servers that are not remote servers are deployed and managed by the Obot Gateway.

## Server types

The system supports three distinct server types, each designed for specific deployment scenarios:

### Single-user server

Single-user servers establish a one-to-one mapping between users and server instances. Each user has their own server instance deployed and provides their own individual credentials (such as personal API keys). Most `stdio` servers were designed with this model in mind. The intended use was to run on the individuals laptop.

This model provides maximum isolation and is ideal when:

- Users need to connect with their personal accounts (e.g., individual GitHub tokens)
- Security policies require user-level credential isolation
- Different users need different configurations or permissions

Keep in mind the gateway will deploy these servers in their own environment on a hosted platform. MCP servers that expect local access to the users filesystem, run local executables, or write output to the local disk will not work as expected.

### Multi-user server

Multi-user servers address organizational deployment patterns through two primary configurations:

1. **Shared credentials**: Organizations provide centralized credentials (e.g., a weather API key) that all users can leverage
2. **Self-authenticating servers**: Servers that handle OAuth or multi-tenancy internally, enabling secure multi-user access

This approach is optimal when:

- The organization owns shared service accounts or API keys
- You want to simplify user onboarding by eliminating individual setup
- The service supports organizational or tenant-based access
- Usage monitoring and control at the organizational level is important

Multi-User servers will still require the user to authenticate to the Obot Gateway's configured identity provider.

### Remote server

MCP Servers that are HTTP Streaming compatible should be configured this way. These servers can be provided by trusted 3rd party vendors. Remote servers also work for MCP servers deployed through existing CI/CD pipeline within the organization.

Choose this type when:

- You have MCP services deployed through traditional application deployment mechanisms
- External partners provide MCP endpoints and you just want to integrate
- You are building MCP servers through existing CI/CD workflows or SaaS services

Remote MCP servers that conform to the MCP spec authentication schema will work out of the box. Servers that do not conform to the spec may not work within the Gateway. Please open a GitHub issue if you run into issues with remote servers.

## Adding a server

Navigate to the Admin panel and access the MCP Servers section, then select **Add MCP Server** button.

Select the type of server you want to deploy.

![Alt text](/img/add-mcp-server-type-selector.png)

## Basic configuration

All server types require the same basic identifying information:

- **Name and description**: Provide a clear name and description to help users understand the server's purpose
- **Icon URL**: Optionally specify an icon URL to improve visual identification in the user interface
- **Categories/tags**: Add optional categorization to facilitate server discovery and filtering

## Runtime selection

Single-user and multi-user servers require runtime environment configuration. Remote servers skip this section since they connect to existing deployments.

Select the appropriate runtime environment based on your server's requirements:

- **npx**: For Node.js-based packages
- **uvx**: For Python-based packages  
- **Containerized**: For Docker-based deployments

Specify the command, package name, and any required arguments. The configuration should mirror the standard execution method used outside of Obot.

## Configuration parameters

How configuration is handled depends on the server type:

### Single-user servers

Define the configuration parameters that users must provide when enabling the server. Common examples include API keys and authentication tokens.

For each parameter, specify:

- **Label and description**: Clear identification of the parameter's purpose
- **Environment variable name**: The variable name expected by the server (e.g., `OPENAI_API_KEY`)
- **Required**: Whether the parameter is mandatory for server operation
- **Sensitive**: Whether the value should be masked in the user interface

User-provided values are passed as environment variables to the server process during initialization.

### Multi-user servers

Multi-user servers use pre-configured values that are deployed with the server instance. Configure any required API keys or environment variables in this section. These values will be set on the deployment automatically.

Unlike single-user servers, there are no user environment settings with multi-user servers since everything is handled in the configuration section. The server will be pre-deployed, and users simply connect to it without being prompted for any information.

When users authenticate, they use the built-in authentication or OAuth per the MCP specification.

### Remote servers

Configure the connection to your remote MCP server:

- **Remote URL**: The endpoint URL for the remote MCP server

Additional configuration options are available for specialized scenarios:

- **Connection restrictions**: Restrict connections to specific URLs if the provider requires unconventional configurations
- **Custom headers**: Set specific HTTP headers if required by the remote server
- **Configuration values**: Set configuration values that will be sent to the remote server

For most remote server configurations, you'll typically only need to specify the remote URL.


Select **Save** to deploy the persist the server configuration.

## Post-deployment management

After successfully adding a server:

- The server appears in the available servers list for authorized users
- Server entries can now be added to authorization groups for different teams
- Users can integrate the server into their clients to access tools in conversations and tasks
- Administrative monitoring of usage and auditing is available through the Admin panel