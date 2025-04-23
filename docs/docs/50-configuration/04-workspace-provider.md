# Workspace Provider

Workspaces are where files are stored and manipulated in Obot. The default installation uses a local disk directory to provide workspaces. For production deployments, we recommend using cloud object stores like AWS S3 or Azure Blob Storage to ensure adequate storage capacity and high availability.

#### Ensure the bucket is configured to keep the objects private. You do not need public access to the bucket.

This section describes the configuration of the workspace provider.

## Provider Type Configuration

| Environment Variable | Description | Available Options |
|----------------------|-------------|-------------------|
| `OBOT_WORKSPACE_PROVIDER_TYPE` | The type of provider to use | `directory`, `s3`, `azure` |

> **Note**: The `s3` provider is compatible with S3-compatible services like CloudFlare R2. The `azure` provider is for Azure Blob Storage.

## Directory Provider Configuration

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `WORKSPACE_PROVIDER_DATA_HOME` | Directory where workspaces are nested | `$XDG_CONFIG_HOME/obot/workspace-provider` |

## S3 Provider Configuration

To use the S3 provider create an S3 bucket or the equivalent in a compatible provider. Ensure the bucket configuration does not allow public access.

| Environment Variable | Required | Description |
|----------------------|----------|-------------|
| `AWS_ACCESS_KEY_ID` | Yes | AWS access key ID |
| `AWS_SECRET_ACCESS_KEY` | Yes | AWS secret access key |
| `AWS_REGION` | Yes | AWS region |
| `WORKSPACE_PROVIDER_S3_BUCKET` | Yes | S3 bucket name |
| `WORKSPACE_PROVIDER_S3_BASE_ENDPOINT` | Optional* | Base endpoint URL for S3-compatible services |

> *Required when using S3-compatible services like CloudFlare R2

## Azure Provider Configuration

To use the Azure provider, create a storage container. Ensure the container is private to disallow anonymous access.

| Environment Variable | Required | Description |
|----------------------|----------|-------------|
| `WORKSPACE_PROVIDER_AZURE_CONTAINER` | Yes | Azure Blob Storage container name |
| `WORKSPACE_PROVIDER_AZURE_CONNECTION_STRING` | Yes | Azure Blob Storage connection string |
