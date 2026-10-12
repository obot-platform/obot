---
title: "Add hosted MCP servers"
---

# Add hosted MCP servers {#hosted-mcp-servers}

Obot deploys and manages hosted MCP server workloads on Kubernetes or Docker. For production workloads, prefer a container image that packages the server and its dependencies together. Pin the image by digest to deploy the same artifact consistently across environments; rebuild and update it deliberately when dependencies change.

## Runtime Types

- **[Node.js (npx)](./mcp-hosting.md#mcp-servers-npx-nodetypescript-based-mcp-servers)**: Run npm-packaged MCP servers via STDIO
- **[Python (uvx)](./mcp-hosting.md#mcp-servers-uvx-for-python-based-packages)**: Run PyPI-packaged MCP servers via STDIO
- **[Containerized](./mcp-hosting.md#mcp-servers-containerized-for-docker-based-deployments)**: Run OCI container images with Streamable HTTP (recommended for production)

## Adding a server {#mcp-servers-adding-a-server}

Open **MCP Servers**, then select **Add MCP Server**.

Choose **Hosted Server**, then select its runtime and launch settings. Define the configuration fields, save the catalog entry, then add it to a vMCP to choose how the values are supplied.

![Catalog entry choices: Hosted Server and Remote Server](/img/add-mcp-hosted-or-remote.png)

## Basic configuration {#mcp-servers-basic-configuration}

Hosted server entries require the same basic identifying information:

- **Name and description**: Provide a clear name and description to help users understand the server's purpose
- **Icon URL**: Optionally specify an icon URL to improve visual identification in the user interface
- **Categories/tags**: Add optional categorization to facilitate server discovery and filtering

## Runtime selection {#mcp-servers-runtime-selection}

Hosted servers require a runtime and launch settings.

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

Declare a configuration field with these settings:

- Name: Firecrawl API Key
- Description: The API key for Firecrawl
- Key: `FIRECRAWL_API_KEY`
- Enable **Required** and **Sensitive**.

After saving the entry, add it to a vMCP. Choose **Preconfigured** to share one API key with that vMCP's connections, or **Provided at connection** to ask each user for their own key.

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

Declare any environment variables as configuration fields. Choose how their values are supplied when adding the entry to a vMCP.

### Containerized: Recommended for production {#mcp-servers-containerized-for-docker-based-deployments}

Use a container image for production servers, regardless of implementation language. Packaging dependencies into a tested image makes the runtime predictable and avoids resolving npm or PyPI packages at startup. Use an immutable image digest rather than a mutable tag when repeatable deployments are required. Containerized servers run on either the Docker or Kubernetes backend.

Configure the server to expose **Streamable HTTP**. Legacy **HTTP+SSE** is [deprecated in the MCP specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#backwards-compatibility). This deprecation concerns the older transport with separate SSE and POST endpoints; Streamable HTTP can still use SSE for streaming responses. Use the Streamable HTTP endpoint for hosted servers in a vMCP; a legacy SSE-only endpoint is not sufficient for this workflow.

Select the container runtime and provide:

- **Image**: The OCI image reference, preferably including its digest.
- **Port**: The port the MCP server listens on inside the container.
- **Path**: The server's Streamable HTTP endpoint, commonly `/mcp`. Use the actual path implemented by the server.
- **Command and arguments**: Any overrides needed to start the server.

Declare environment variables and other required configuration fields, then choose how unresolved values are supplied when adding the entry to a vMCP.

## Kubernetes Secret Bindings {#mcp-servers-kubernetes-secret-bindings}

Kubernetes Secret bindings let administrators supply a catalog configuration field from a Secret managed outside Obot. A vMCP inherits that binding when the catalog entry is added as a component.

Secret bindings are available only when Obot is using the Kubernetes MCP runtime backend.

### Required Kubernetes Secret Label {#mcp-servers-required-kubernetes-secret-label}

The Kubernetes Secret must be in the Obot server's namespace and carry the configured allowed secret-binding label, `obot.obot.ai/allow-secret-binding` by default. See [Server configuration](../configuration/server-configuration.md) to change the required label key.

The label controls whether a Secret can be discovered and selected in the admin UI, and Obot also checks the label when resolving the binding at runtime. If the label is removed after an MCP server is already bound to that Secret, the binding is treated as unavailable. Required fields then appear as missing configuration until the label is restored or the binding is changed.

Secrets without data keys are not shown as bindable targets.

### Configure a Binding in the Admin UI {#mcp-servers-configure-a-binding-in-the-admin-ui}

#### New Catalog Entry {#mcp-servers-new-catalog-entry}

1. Open **MCP Servers**.
2. Select **Add MCP Server**, then **Hosted Server**.
3. Add a configuration value or header.
4. Set the field's **Value** to **Static** to expose the value-source controls.
5. In **Value Source**, select **Kubernetes Secret**.
6. Select the Secret name and key.
7. Save the MCP server.

This binds the catalog field to an existing Secret. The vMCP's **Preconfigured** and **Provided at connection** choices apply only to fields that do not already have a static value or Secret binding.

#### Using a Git-managed catalog entry {#mcp-servers-git-ops-managed-template}

For an entry synchronized from Git, define its `secretBinding` in the source catalog and synchronize the entry. See the [catalog schema](../configuration/mcp-server-gitops.md) for configuration fields. Create the referenced Secret in the Obot namespace and apply the required label before using the entry.

Add the entry to a vMCP as a component. The saved component includes the Secret name and key, and Obot resolves the value from Kubernetes when preparing the server configuration. Users connecting to the vMCP are not prompted for that field, and the vMCP configuration dialog does not offer a replacement value for it. The binding works with shared or per-user component deployments; it is not a separate server type.

For a vMCP managed in Obot, changing the Secret reference in the catalog requires an explicit [vMCP update](../mcp-gateway/publish.md#virtual-mcps-snapshots-and-updates) before existing components use that reference. The component stores a reference, not a frozen copy of the Secret's value. Do not assume an already-running process immediately reloads a rotated environment variable; verify the workload after rotating its credentials.

For a Git-managed vMCP, catalog sync refreshes the component definition. Its YAML can also put a `secretBinding` on a `fixed` component configuration policy when the catalog field is not static. This vMCP-level binding is configured in Git, not the UI. See [vMCP definitions](../configuration/mcp-server-gitops.md#vmcp-definitions) for the constraints and an example.

## Configuration and sharing {#server-types}

Add a **Hosted Server** catalog entry to define its runtime and configuration fields. When you add the entry to a vMCP, choose **Preconfigured** for values shared by its connections or **Provided at connection** for values each user supplies. Obot determines shared or per-user deployment behavior from that component configuration.

See [Configure servers in a vMCP](../mcp-gateway/server-types.md) for the workflow. A **[Remote Server](../mcp-gateway/register-remote.md#mcp-servers-remote-server)** catalog entry instead describes an existing HTTP endpoint; Obot does not host that server.

## Virtual MCPs (vMCPs)

Clients connect to new MCP endpoints through a virtual MCP (vMCP), which exposes tools from one or more catalog components through a single endpoint. A vMCP controls which tools users can access, while its backing servers use shared or per-user runtimes according to the component configuration.

vMCPs replace the legacy standalone connection and composite server models. Obot migrates existing composite servers during upgrade. Other legacy endpoints remain available for compatibility. See [Virtual MCPs](../mcp-gateway/server-types.md) for creation, access, configuration, and migration guidance.

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

<a id="authentication" name="authentication" />

## Security and Isolation

Adding an MCP server causes Obot to run code on the hosting backend: `npx` and `uvx` servers execute the requested npm/PyPI package, and **containerized** servers run an arbitrary OCI image with a user-supplied command. The [Power User and Power User+ roles](../security/policy-coverage.md#user-roles-security-model) can deploy servers, so granting those roles is, by design, granting the ability to run code on your infrastructure.

How well that code is contained depends on the deployment environment:

- **Docker** runs MCP servers as sibling containers through the host Docker socket, which provides little isolation from the host. Use it for development or single-tenant, trusted use only.
- **Kubernetes** runs each MCP server in its own pod and supports the restricted Pod Security Admission policy, a NetworkPolicy, and sandboxed container runtimes (gVisor, Kata Containers) for stronger isolation. Use it for multi-tenant or untrusted workloads.

See [User Roles — Security Model](../security/policy-coverage.md#user-roles-security-model) and [MCP Deployments in Kubernetes](../configuration/mcp-deployments-in-kubernetes.md) for details.

## Learn More

- [Legacy MCP server reference](../functionality/mcp-servers.md) - Existing standalone deployments and compatibility terminology
- [Installation](../installation/overview.md) - Deployment environments and setup
