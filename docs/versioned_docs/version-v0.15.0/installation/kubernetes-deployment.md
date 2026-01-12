# Kubernetes Deployment

Deploy Obot on Kubernetes for production-grade reliability, scalability, and high availability.

:::info Helm Chart Reference
For a complete list of all available Helm chart configuration values, see [charts.obot.ai](https://charts.obot.ai/).
:::

## Prerequisites

- **Helm**
- **PostgreSQL 17+** with pgvector extension
- **S3-compatible storage** (for production)
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

# This can be turned off because we are persisting data externally in postgres and S3
persistence:
  enabled: false

# In this example, we will be using S3 and AWS KMS for encryption
config:
  # this should have IAM permissions for S3 and KMS
  AWS_ACCESS_KEY_ID: <access key>
  AWS_SECRET_ACCESS_KEY: <secret key>
  AWS_REGION: <aws region>

  # This should be set to avoid ratelimiting certain actions that interact with github, such as server sources
  GITHUB_AUTH_TOKEN: <PAT from github>

  # Enable encryption
  OBOT_SERVER_ENCRYPTION_PROVIDER: aws
  OBOT_AWS_KMS_KEY_ARN: <your kms arn>

  # Enable S3 workspace provider
  OBOT_WORKSPACE_PROVIDER_TYPE: s3
  WORKSPACE_PROVIDER_S3_BUCKET: <s3 bucket name>

  # optional - this will be generated automatically if you do not set it
  OBOT_BOOTSTRAP_TOKEN: <some random value>

  # Point this to your postgres database
  OBOT_SERVER_DSN: postgres://<user>:<pass>@<host>/<db>

  OBOT_SERVER_HOSTNAME: <your obot hostname>
  # Setting these is optional, but you'll need to setup a model provider from the Admin UI before using chat.
  # You can set either, neither or both.
  OPENAI_API_KEY: <openai api key>
  ANTHROPIC_API_KEY: <anthropic api key>
```

### High Availability

To enable a high availability setup, uncomment the `replicaCount` line and set it to `2` or higher. An external PostgreSQL database and a workspace provider are required for HA.

For detailed configuration options, see:

- **[Server Configuration](/configuration/server-configuration)** - All available environment variables
- **[Workspace Provider](/configuration/workspace-provider)** - S3 storage configuration
- **[Encryption Providers](/configuration/encryption-providers/aws-kms)** - KMS encryption setup

## Cloud-Specific Guides

For detailed cloud-specific deployment instructions:

- [Google Kubernetes Engine (GKE)](/installation/reference-architectures/gcp-gke/)
- [Amazon Elastic Kubernetes Service (EKS)](/installation/reference-architectures/aws-eks/)
- [Azure Kubernetes Service (AKS)](/installation/reference-architectures/azure-aks/)

## Next Steps

1. **Configure Authentication**: Set up [auth providers](/configuration/auth-providers)
2. **Add Model Providers**: Configure [model providers](/configuration/model-providers)
3. **Set Up MCP Servers**: Configure [MCP servers](/functionality/mcp-servers)
4. **Configure Monitoring**: Set up logging and metrics
5. **Review Security**: Enable authentication and encryption

## Related Documentation

- [Installation Overview](/installation/overview/)
- [Server Configuration](/configuration/server-configuration)
- [Settings for Hosted MCP Server Deployments](/configuration/mcp-deployments-in-kubernetes)
