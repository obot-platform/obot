# Kubernetes Deployment

Deploy Obot on Kubernetes for production-grade reliability, scalability, and high availability.

:::info Helm Chart Reference
For a complete list of all available Helm chart configuration values, see [charts.obot.ai](https://charts.obot.ai/).
:::

## Prerequisites

- **Helm**
- **PostgreSQL 17+** with pgvector extension
- **Object storage or a persistent volume for published workflows** (for production)
- **StorageClass** (for production)
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

  # Store published workflows in external object storage
  # Options are s3, azure, gcs, and custom
  OBOT_ARTIFACT_STORAGE_PROVIDER: s3
  OBOT_ARTIFACT_STORAGE_BUCKET: <artifact bucket name>
  OBOT_ARTIFACT_S3_REGION: <aws region>

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

For published workflow storage in HA, use one of these:

- External object storage such as S3, GCS, Azure Blob Storage, or an S3-compatible service
- The `persistence` PVC with `ReadWriteMany` access so all replicas can share `/data`

For detailed configuration options, see:

- **[Server Configuration](/configuration/server-configuration/)** - All available environment variables
- **[Workspace Provider](/configuration/workspace-provider/)** - S3 storage configuration
- **[Workflow Sharing](../functionality/workflow-sharing.md)** - How shared workflows work and how to configure their storage
- **[Encryption Providers](/configuration/encryption-providers/aws-kms/)** - KMS encryption setup

## Cloud-Specific Guides

For detailed cloud-specific deployment instructions:

- [Google Kubernetes Engine (GKE)](/installation/reference-architectures/gcp-gke/)
- [Amazon Elastic Kubernetes Service (EKS)](/installation/reference-architectures/aws-eks/)
- [Azure Kubernetes Service (AKS)](/installation/reference-architectures/azure-aks/)

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

## Agent Persistence

By default, Obot Agent uses storage inside its pod, which means all agent state is lost if the pod restarts. For production deployments, configure a persistent `StorageClass`.

For complete guidance and examples (including AWS EBS, GCP Hyperdisk, and `nfs-subdir-external-provisioner`), see [Persistent Storage in Kubernetes](/installation/kubernetes-persistent-storage.md).

## Workflow Sharing in Kubernetes

If `OBOT_ARTIFACT_STORAGE_PROVIDER` is unset, Obot stores published workflows on local disk at `/data/.local/share/obot/published-artifacts`.

The Helm chart's `persistence` PVC mounts at `/data`, so it covers that path.

### Use the Existing Persistence Claim

Use this when you do not want S3, GCS, Azure Blob Storage, or another object store for published workflows.

If you disable `persistence`, published workflow artifacts remain on pod-local disk and will be lost when the pod is replaced.

For a single Obot replica:

- Enable `persistence`
- Use `ReadWriteOnce` as the access mode
- This is appropriate for `replicaCount: 1`
- A block-storage-backed `StorageClass` such as EBS, PD, or Azure Disk is typically fine

For multiple Obot replicas:

- Enable `persistence`
- Use `ReadWriteMany` as the access mode
- All replicas must mount the same `/data` volume concurrently
- This requires a shared filesystem-backed `StorageClass`, such as NFS
- A single shared `ReadWriteOnce` claim is not a valid multi-replica setup

Example using dynamic provisioning for a single replica:

```yaml
replicaCount: 1

config:
  OBOT_ARTIFACT_STORAGE_PROVIDER: ""
  OBOT_ARTIFACT_STORAGE_BUCKET: ""

persistence:
  enabled: true
  storageClass: gp3
  accessModes:
    - ReadWriteOnce
  size: 10Gi
```

Example using an existing RWX claim for multiple replicas:

```yaml
replicaCount: 2

config:
  OBOT_ARTIFACT_STORAGE_PROVIDER: ""
  OBOT_ARTIFACT_STORAGE_BUCKET: ""

persistence:
  enabled: true
  existingClaim: obot-data-rwx
```

The `obot-data-rwx` claim itself must be provisioned with `ReadWriteMany`.

For more examples and storage-class guidance, see [Persistent Storage in Kubernetes](/installation/kubernetes-persistent-storage.md).

## Next Steps

1. **Configure Authentication**: Set up [auth providers](/configuration/auth-providers/)
2. **Add Model Providers**: Configure [model providers](/configuration/model-providers/)
3. **Set Up MCP Servers**: Configure [MCP servers](/functionality/mcp-servers/)
4. **Configure Monitoring**: Set up logging and metrics
5. **Review Security**: Enable authentication and encryption

## Related Documentation

- [Installation Overview](/installation/overview/)
- [Server Configuration](/configuration/server-configuration/)
- [Settings for Hosted MCP Server Deployments](/configuration/mcp-deployments-in-kubernetes/)
