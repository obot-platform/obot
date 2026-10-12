---
title: "Kubernetes (production)"
---

# Kubernetes (production) {#kubernetes-deployment}

Deploy Obot on Kubernetes for production-grade reliability, scalability, and high availability.

:::info Helm Chart Reference
For a complete list of all available Helm chart configuration values, see [charts.obot.ai](https://charts.obot.ai/).
:::

## Prerequisites

- **Helm**
- **PostgreSQL 17+**
- **StorageClass** (if using persistent volumes)
- **Encryption provider** (AWS KMS, GCP KMS, or Azure Key Vault recommended)

### Minimum Cluster Requirements

- **Nodes**: 1+ nodes
- **CPU**: 2 cores
- **Memory**: 4GB

### Recommended Cluster Requirements

- **HA Cluster**
- **CPU**: 4 cores for Obot
- **Memory**: 8GB for Obot

## Helm Installation

Obot provides a Helm chart for easy deployment [here](https://charts.obot.ai).

The chart has sane defaults for a test cluster.

### Production Installation

Create a `values.yaml` file with your production configuration:

```yaml
# Optionally customize replica count for high availability
# replicaCount: 2

# Enable ingress or use a service of type loadbalancer to expose Obot
ingress:
  enabled: true
  hosts:
    - <your obot hostname>

# Disable local persistence only if no enabled feature needs files retained under /data
persistence:
  enabled: false

# In this example, we will be using AWS KMS for encryption.
# Sensitive values must be configured under secret.
secret:
  # this should have IAM permissions for KMS
  AWS_ACCESS_KEY_ID: <access key>
  AWS_SECRET_ACCESS_KEY: <secret key>

  # This should be set to avoid ratelimiting certain actions that interact with github, such as server sources
  GITHUB_AUTH_TOKEN: <PAT from github>

  # optional - this will be generated automatically if you do not set it
  OBOT_BOOTSTRAP_TOKEN: <some random value>

  # Point this to your postgres database
  OBOT_SERVER_DSN: postgres://<user>:<pass>@<host>/<db>

  # Setting these is optional, but you'll need to setup a model provider from the Admin UI before using the LLM Gateway.
  # You can set either, neither or both.
  OPENAI_API_KEY: <openai api key>
  ANTHROPIC_API_KEY: <anthropic api key>

# Non-sensitive values must be configured under config.
config:
  AWS_REGION: <aws region>

  # Enable encryption
  OBOT_SERVER_ENCRYPTION_PROVIDER: aws
  OBOT_AWS_KMS_KEY_ARN: <your kms arn>

  OBOT_SERVER_HOSTNAME: <your obot hostname>
```

### High Availability

To enable a high availability setup, uncomment the `replicaCount` line and set it to `2` or higher. Use an external PostgreSQL database with its own availability and recovery plan.

If enabled features require shared files under `/data`, configure a `ReadWriteMany` volume accessible to every replica. A shared `ReadWriteOnce` claim is not a multi-replica storage solution. See [Persistent Storage](./kubernetes-persistent-storage.md) and [High availability](../operations/high-availability.md).

For configuration options, see [Server Configuration](../configuration/server-configuration.md) and [Encryption Providers](../configuration/encryption-providers/aws-kms.md).

## Cloud-Specific Guides

For detailed cloud-specific deployment instructions:

- [Google Kubernetes Engine (GKE)](./reference-architectures/01-gcp-gke.md)
- [Amazon Elastic Kubernetes Service (EKS)](./reference-architectures/02-aws-eks.md)
- [Azure Kubernetes Service (AKS)](./reference-architectures/03-azure-aks.md)

## Security Configuration

### Network Policy for MCP Servers

For production deployments, ensure the NetworkPolicy is enabled to restrict network access from MCP server pods:

```yaml
mcpNamespace:
  networkPolicy:
    enabled: true # This is already enabled by default
    dnsNamespace: kube-system  # Adjust if your DNS is in a different namespace
```

When enabled, this policy:
- Restricts MCP servers to only communicate with Obot, DNS, and public internet
- Blocks access to private IP ranges and internal cluster resources
- Prevents potential lateral movement if an MCP server is compromised

For details, see [MCP Deployments in Kubernetes - Network Policy](../configuration/mcp-deployments-in-kubernetes.md#network-policy).

### Pod Security Admission for MCP Servers

Obot applies Pod Security Standards to the MCP namespace using Pod Security Admission (PSA). The default configuration uses the **restricted** policy level for maximum security:

```yaml
mcpNamespace:
  podSecurity:
    enforce: restricted  # Can be: privileged, baseline, or restricted
    enforceVersion: latest
    audit: restricted
    auditVersion: latest
    warn: restricted
    warnVersion: latest
```

The restricted policy follows current Pod hardening best practices and provides the highest level of security. If you need more permissive settings, you can change to **baseline** or **privileged** levels.

For details, see [MCP Deployments in Kubernetes - Pod Security Admission](../configuration/mcp-deployments-in-kubernetes.md#pod-security-admission).

### Restricting Obot Server Egress

The [Network Policy for MCP Servers](./kubernetes-deployment.md#network-policy-for-mcp-servers) above restricts the **MCP server pods**. It does not restrict the **Obot server** itself, which makes its own outbound connections as part of normal operation. In multi-tenant deployments or deployments with untrusted users, you may want to constrain which destinations the Obot server can reach, as a defense-in-depth measure.

If you want to constrain the Obot server's egress as a defense-in-depth measure, scope it tightly to your specific deployment — there is no safe blanket blocklist. Depending on your setup, the Obot server legitimately needs to reach private ranges (its database and in-cluster services such as a self-hosted model provider or the MCP namespace) and, with some cloud authentication methods, the cloud instance metadata endpoint (`169.254.169.254`), where it obtains credentials for integrations such as KMS.

## Next Steps

1. **Configure Authentication**: Set up [auth providers](../configuration/auth-providers.md)
2. **Add Model Providers**: Configure [model providers](../configuration/model-providers.md)
3. **Set Up MCP Servers**: Configure [MCP servers](../functionality/mcp-servers.md)
4. **Configure Monitoring**: Set up logging and metrics
5. **Review Security**: Enable authentication and encryption

## Related Documentation

- [Installation Overview](./overview.md)
- [Server Configuration](../configuration/server-configuration.md)
- [Settings for Hosted MCP Server Deployments](../configuration/mcp-deployments-in-kubernetes.md)
