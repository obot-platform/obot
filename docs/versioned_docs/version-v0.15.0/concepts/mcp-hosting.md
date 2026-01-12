---
title: MCP Hosting
---

# MCP Hosting

The MCP Hosting layer runs and manages MCP servers directly within Obot. It handles deployment, lifecycle management, and runtime isolation for MCP servers.

## Runtime Types

- **[Node.js (npx)](/functionality/mcp-servers#npx-nodetypescript-based-mcp-servers)**: Run npm-packaged MCP servers via STDIO
- **[Python (uvx)](/functionality/mcp-servers#uvx-for-python-based-packages)**: Run PyPI-packaged MCP servers via STDIO
- **[Containerized](/functionality/mcp-servers#containerized-for-docker-based-deployments)**: Run Docker containers with HTTP/SSE transport

## Server Types

- **[Single-user](/functionality/mcp-servers#single-user-server)**: Each user gets their own isolated instance with separate credentials
- **[Multi-user](/functionality/mcp-servers#multi-user-server)**: A shared instance serves multiple users with shared or per-user credentials
- **[Remote](/functionality/mcp-servers#remote-server)**: External MCP servers accessed via HTTP, not hosted by Obot
- **[Composite](/functionality/mcp-servers#composite-server)**: Combines multiple servers into a single virtual server with curated tools

## Deployment Environments

### Docker

When running Obot with Docker, MCP servers are deployed as sibling containers:

- Obot communicates with the Docker daemon to manage containers
- Servers run alongside the Obot container
- Suitable for development and small deployments
- See [Docker Deployment](/installation/docker-deployment) for setup details

### Kubernetes

For production deployments, Obot can deploy MCP servers to Kubernetes:

- Servers run as pods in the cluster
- Supports resource limits, network policies, and scaling
- See [MCP Deployments in Kubernetes](/configuration/mcp-deployments-in-kubernetes) for configuration details

## Authentication

Obot handles OAuth 2.1 flows for MCP servers that require authentication:

- OAuth credentials stored securely with encryption at rest
- Automatic token refresh
- Per-user credential isolation
- Supports custom OAuth configurations

See [MCP Server OAuth Configuration](/configuration/mcp-server-oauth-configuration) for details on configuring OAuth for MCP servers.

## Learn More

- [MCP Servers](/functionality/mcp-servers) - Adding and configuring MCP servers
- [Installation](/installation/overview) - Deployment environments and setup
