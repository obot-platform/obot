# FAQ

## Onboarding & Setup

### Should I use Docker or Kubernetes for production?

Docker is suitable for local testing and small-scale deployments. For production, especially in enterprise settings, Kubernetes is recommended for high availability, resource management, and upgrades.

### Why can’t I see the User Management section?

User Management is only visible when authentication is enabled. Make sure you start Obot with `OBOT_SERVER_ENABLE_AUTHENTICATION=true`. If you don't see the bootstrap token prompt, the environment variable may not be set correctly. Follow the [installation guide](/installation/enabling-authentication/).

### How do I assign roles to users before they log in?

Currently, users must log in at least once before roles can be assigned. To pre-assign admin roles, set the `OBOT_SERVER_AUTH_ADMIN_EMAILS` environment variable during deployment. 

### What are the differences between the open source and enterprise versions of Obot?

Both use the same core codebase, but the enterprise version includes additional closed-source plugins for:

- enterprise authentication (Entra, Okta) 
- model providers (Azure OpenAI, Amazon Bedrock)

## Integration & Troubleshooting

### How do I connect my IDE or MCP client to an Obot hosted MCP server?

<details>
<summary>Step-by-step instructions</summary>

1. Go to the **MCP Servers** page in the left navigation
2. Click on the server you want to connect to
3. Click **Connect to Server**
4. If the server requires configuration or authentication, fill that out and click **Launch**
5. A connection modal will appear with the connection URL and configuration snippets for popular MCP clients

</details>

### Why does my IDE/client (e.g., Cline) fail to connect to Obot with a "Session ID is required" error?

Some clients do not support the required OAuth flows. As a workaround, use the `mcp-remote` package as a proxy, or check for client updates that add OAuth support.

## Enterprise Access

### How do I request an enterprise trial or proof-of-concept?

- Contact the Obot team directly on Discord, Website, or email.

## Miscellaneous

### How do I pass user-specific parameters (e.g., Jira PAT tokens) to a remote MCP server?

As an admin, you can configure the server in the MCP Servers section. See [MCP Servers](/functionality/mcp-servers/) for details.

### How do I get started with AKS/GKE/AWS deployment?

See the reference architecture guide for your cloud provider, and follow the Kubernetes installation.

- [AKS](/installation/reference-architectures/azure-aks/)
- [GKE](/installation/reference-architectures/gcp-gke/)
- [AWS](/installation/reference-architectures/aws-eks/)
