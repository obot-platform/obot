---
title: MCP Hosting
---

# MCP Hosting

Obot deploys and manages hosted MCP server workloads on the underlying Kubernetes or Docker runtime.

## Runtime Types

- **[Node.js (npx)](../functionality/mcp-servers.md#npx-nodetypescript-based-mcp-servers)**: Run npm-packaged MCP servers via STDIO
- **[Python (uvx)](../functionality/mcp-servers.md#uvx-for-python-based-packages)**: Run PyPI-packaged MCP servers via STDIO
- **[Containerized](../functionality/mcp-servers.md#containerized-for-docker-based-deployments)**: Run Docker containers with HTTP/SSE transport

## Server Types

- **[Single-user](../functionality/mcp-servers.md#single-user-server)**: Each user gets their own isolated instance with separate credentials
- **[Multi-user](../functionality/mcp-servers.md#multi-user-server)**: A shared instance serves multiple users with shared or per-user credentials
- **[Remote](../functionality/mcp-servers.md#remote-server)**: External MCP servers accessed via HTTP, not hosted by Obot

## Adding a server {#mcp-servers-adding-a-server}

Navigate to **MCP Management > MCP Servers** in the MCP Platform, then select **Add MCP Server**.

Select the type of server you want to deploy.

![Alt text](/img/add-mcp-server-type-selector.png)


## Basic configuration {#mcp-servers-basic-configuration}

All server types require the same basic identifying information:

- **Name and description**: Provide a clear name and description to help users understand the server's purpose
- **Icon URL**: Optionally specify an icon URL to improve visual identification in the user interface
- **Categories/tags**: Add optional categorization to facilitate server discovery and filtering


## Runtime selection {#mcp-servers-runtime-selection}

Single-user and multi-user servers require runtime environment configuration. Remote servers skip this section since they connect to existing deployments.

Select the appropriate runtime environment based on your server's requirements:


### NPX: Node/Typescript Based MCP Servers {#mcp-servers-npx-nodetypescript-based-mcp-servers}

If you found an MCP server like Firecrawl and want to add it to the MCP Gateway you would do the following.

From the README.md:

```json
{
  "mcpServers": {
    "firecrawl-mcp": {
      "command": "npx",
      "args": ["-y", "firecrawl-mcp"],
      "env": {
        "FIRECRAWL_API_KEY": "YOUR-API-KEY"
      }
    }
  }
}
```

In the MCP Gateway

- You would select NPX from the drop down.
- Then put `firecrawl-mcp` in the package text box.

For single-user setup, you would add User supplied configuration

- Name: Firecrawl API Key
- Description: The api key for Firecrawl
- Key: FIRECRAWL_API_KEY

In this case you would select `required` and `sensitive` options as well.

For multi-user setup, you would follow the same steps but would be configuring this to **SHARE** a common API key with ALL users.


### UVX: For Python-based packages {#mcp-servers-uvx-for-python-based-packages}

If you found an MCP server like Duckduckgo and want it added to the gateway you would do the following.

From the README.md:

```json
{
    "mcpServers": {
        "ddg-search": {
            "command": "uvx",
            "args": ["duckduckgo-mcp-server"]
        }
    }
}
```

In the gateway you would:

- Select UVX from the drop down
- In the package field put in `duckduckgo-mcp-server`

If environment variables need to be configured, you would use the user or multi-user configuration to supply or prompt for the values.


### Containerized: For Docker-based deployments {#mcp-servers-containerized-for-docker-based-deployments}

If you want to provide a container to run your MCP server because you are running a non-TypeScript or Python MCP server you must configure it to run as either Streaming HTTP or SSE.

You will need to select the container option from the drop down. Then provide the following bits of info:

- Image: The uri of the OCI image. (ex. docker.elastic.co/mcp/elasticsearch)
- Port: port the MCP server will be listening on inside the container.
- Path: the URI path. (typically /MCP or /SSE)
- Command: primary command to execute
- Arguments: arguments to pass to the command.

You can also provide configuration through environment variables by filling in the configurations.


## Kubernetes Secret Bindings {#mcp-servers-kubernetes-secret-bindings}

MCP secret bindings let Admins select a key from an externally managed Kubernetes Secret as the value source for a multi-user MCP deployment configuration field.

Secret bindings are available only when Obot is using the Kubernetes MCP runtime backend.


### Required Kubernetes Secret Label {#mcp-servers-required-kubernetes-secret-label}

Secret binding selection in the admin UI is available for multi-user MCP deployments. The Kubernetes Secret must be in the Obot server's namespace and must have the [configured allowed secret-binding label](../configuration/server-configuration.md).

The label controls whether a Secret can be discovered and selected in the admin UI, and Obot also checks the label when resolving the binding at runtime. If the label is removed after an MCP server is already bound to that Secret, the binding is treated as unavailable. Required fields then appear as missing configuration until the label is restored or the binding is changed.

Secrets without data keys are not shown as bindable targets.


### Configure a Binding in the Admin UI {#mcp-servers-configure-a-binding-in-the-admin-ui}


#### New Catalog Entry {#mcp-servers-new-catalog-entry}
1. Go to **MCP Management > MCP Catalog**.
2. Use "Add Catalog Entry" to create a new Hosted Server (Multi-tenant)
3. Add a configuration value or header.
4. In **Value Source**, select **Kubernetes Secret**.
5. Select the Secret name and key.
6. Save the MCP server.


#### Git-ops Managed Template {#mcp-servers-git-ops-managed-template}
1. Go to **MCP Management > MCP Catalog**.
2. Locate a multi-user template created from a Git source
3. Click "Connect URL" to launch a new deployment
4. In **Value Source**, select **Kubernetes Secret**.
5. Select the Secret name and key.
6. Save the MCP server.


## Virtual MCPs (vMCPs)

Clients connect to new MCP endpoints through a virtual MCP (vMCP), which exposes tools from one or more catalog components through a single endpoint. A vMCP controls which tools users can access, while its backing servers use shared or per-user runtimes according to the component configuration.

vMCPs replace the legacy standalone connection and composite server models. Existing endpoints remain available for migration compatibility in this release; Obot will migrate them in a future release. See [Virtual MCPs](../functionality/virtual-mcps.md) for creation, access, configuration, and migration guidance.

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

- OAuth credentials encrypted at rest when an [encryption provider](../configuration/encryption-providers/overview.md) is configured; encryption is disabled by default
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

