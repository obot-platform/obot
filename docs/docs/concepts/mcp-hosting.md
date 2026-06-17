---
title: MCP Hosting
---

# MCP Hosting

The MCP Hosting layer runs and manages MCP servers directly within Obot. It handles deployment, lifecycle management, and runtime isolation for MCP servers.

## Runtime Types

- **[Node.js (npx)](../functionality/mcp-servers.md#npx-node-typescript-based-mcp-servers)**: Run npm-packaged MCP servers via STDIO
- **[Python (uvx)](../functionality/mcp-servers.md#uvx-for-python-based-packages)**: Run PyPI-packaged MCP servers via STDIO
- **[Containerized](../functionality/mcp-servers.md#containerized-for-docker-based-deployments)**: Run Docker containers with HTTP/SSE transport

## Server Types

- **[Single-user](../functionality/mcp-servers.md#single-user-server)**: Each user gets their own isolated instance with separate credentials
- **[Multi-user](../functionality/mcp-servers.md#multi-user-server)**: A shared instance serves multiple users with shared or per-user credentials
- **[Remote](../functionality/mcp-servers.md#remote-server)**: External MCP servers accessed via HTTP, not hosted by Obot
- **[Composite](../functionality/mcp-servers.md#composite-server)**: Combines multiple servers into a single virtual server with curated tools

## Deployment Environments

### Docker

When running Obot with Docker, MCP servers are deployed as sibling containers:

- Obot communicates with the Docker daemon to manage containers
- Servers run alongside the Obot container
- Suitable for development and small deployments
- See [Docker Deployment](../installation/docker-deployment.md) for setup details

### Kubernetes

For production deployments, Obot can deploy MCP servers to Kubernetes:

- Servers run as pods in the cluster
- Supports resource limits, network policies, and scaling
- See [MCP Deployments in Kubernetes](../configuration/mcp-deployments-in-kubernetes.md) for configuration details

## Authentication

Obot handles OAuth 2.1 flows for MCP servers that require authentication:

- OAuth credentials stored securely with encryption at rest
- Automatic token refresh
- Per-user credential isolation
- Supports custom OAuth configurations

See [MCP Server OAuth Configuration](../configuration/mcp-server-oauth-configuration.md) for details on configuring OAuth for MCP servers.

## Security and Isolation

Adding an MCP server causes Obot to run code on the hosting backend: `npx` and `uvx` servers execute the requested npm/PyPI package, and **containerized** servers run an arbitrary OCI image with a user-supplied command. The [Power User and Power User+ roles](../configuration/user-roles.md#security-model) can deploy servers, so granting those roles is, by design, granting the ability to run code on your infrastructure.

How well that code is contained depends on the deployment environment:

- **Docker** runs MCP servers as sibling containers through the host Docker socket, which provides little isolation from the host. Use it for development or single-tenant, trusted use only.
- **Kubernetes** runs each MCP server in its own pod and supports the restricted Pod Security Admission policy, a NetworkPolicy, and sandboxed container runtimes (gVisor, Kata Containers) for stronger isolation. Use it for multi-tenant or untrusted workloads.

See [User Roles — Security Model](../configuration/user-roles.md#security-model) and [MCP Deployments in Kubernetes](../configuration/mcp-deployments-in-kubernetes.md) for details.

## Learn More

- [MCP Servers](../functionality/mcp-servers.md) - Adding and configuring MCP servers
- [Installation](../installation/overview.md) - Deployment environments and setup
