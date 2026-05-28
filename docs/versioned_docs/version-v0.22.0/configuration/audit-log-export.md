# MCP Audit Log Exports

Obot can export audit logs to various cloud storage providers for long-term retention. This feature supports both one-time exports and scheduled recurring exports.

## Overview

The audit log export feature enables you to:

- Export audit logs to Amazon S3, Google Cloud Storage, Azure Blob Storage, or custom S3-compatible storage
- Create one-time exports for specific date ranges and filters
- Schedule recurring exports (hourly, daily, weekly, monthly)
- Apply filters to export only relevant logs

## Supported Storage Providers

### Amazon S3

**Requirements:**

- AWS account with S3 access
- S3 bucket created in your desired region
- IAM credentials with appropriate permissions

**Required IAM Permissions:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject"],
      "Resource": [
        "arn:aws:s3:::your-audit-logs-bucket",
        "arn:aws:s3:::your-audit-logs-bucket/*"
      ]
    }
  ]
}
```

**Configuration:**

- **Region**: AWS region where your S3 bucket is located (e.g., `us-east-1`)
- **Access Key ID**: AWS access key ID with S3 permissions. Optional if you are using workload identity.
- **Secret Access Key**: AWS secret access key. Optional if you are using workload identity.

**Encryption:**

- You can enable server-side encryption (SSE) for your bucket to encrypt your audit logs at rest, using either SSE-S3 or SSE-KMS. For more information, see [Server-Side Encryption](https://docs.aws.amazon.com/AmazonS3/latest/userguide/serv-side-encryption.html). If you are using SSE-KMS, you need to create a KMS key and grant the necessary permissions to the KMS key to the identity associated with your Obot deployment.

### Google Cloud Storage (GCS)

Google Cloud Storage provides reliable and scalable storage for audit logs.

**Requirements:**

- Google Cloud project with Cloud Storage API enabled
- Storage bucket created in your desired region
- Service account with appropriate permissions

**Required Role:**

- `roles/storage.objectCreator` Allows users to create objects. Does not give permission to view, delete, or overwrite objects.

**Configuration:**

- **Service Account JSON**: You will need to create a service account and grant the role above to the service account. Download the JSON key file for the service account and provide it in the configuration. For more information, see [Service Accounts](https://cloud.google.com/iam/docs/service-accounts). Optional if you are using workload identity.

**Encryption:**

- Enabled by default using Google-managed keys. You can also use customer-managed encryption keys (CMEK) to encrypt your audit logs. For more information, see [Customer-Managed Encryption Keys](https://cloud.google.com/storage/docs/encryption/customer-managed-keys).

### Azure Blob Storage

Azure Blob Storage integration uses service principal authentication for secure access.

**Requirements:**

- Azure subscription with Storage Account
- Blob container created in your storage account
- Azure Active Directory application (service principal)

**Required Permissions:**

- `Storage Blob Data Contributor` role on the storage account or container

**Configuration:**

- **Storage Account**: Name of your Azure storage account
- **Client ID**: Application (client) ID of your Azure AD application. Optional if you are using workload identity.
- **Tenant ID**: Directory (tenant) ID of your Azure AD tenant. Optional if you are using workload identity.
- **Client Secret**: Client secret for your Azure AD application. Optional if you are using workload identity.

### Custom S3 Compatible Storage

Support for S3-compatible storage providers like MinIO, DigitalOcean Spaces, Wasabi, R2, etc.

**Requirements:**

- S3-compatible storage service
- Bucket/container created
- Access credentials

**Configuration:**

- **Endpoint**: Full URL to your S3-compatible service
- **Region**: Region identifier (provider-specific)
- **Access Key ID**: Access key for authentication
- **Secret Access Key**: Secret key for authentication

## Storage Configuration

### Initial Setup

1. **Navigate to Audit Log Exports**:

   - Go to Admin → Audit Logs → Manage Exports
   - Click "Configure Storage"

2. **Select Provider**:

   - Choose your preferred storage provider from the dropdown
   - Configure authentication method (if applicable)

3. **Enter Credentials**:

   - Provide the required credentials based on your chosen provider
   - For cloud providers, you can choose between manual credentials or workload identity

4. **Test Connection**:

   - Use the "Test Connection" button to verify your configuration
   - This ensures Obot can successfully connect to your storage

5. **Save Configuration**:
   - Click "Save Credentials" to store your configuration securely

### Authentication Methods

#### Manual Credentials

Provide explicit access keys, service account files, or client secrets. This method gives you full control over the credentials used.

#### Workload Identity (Cloud Providers Only)

Use the identity associated with your Obot deployment. This method is more secure as it doesn't require storing explicit credentials.

**Supported for:**

- Amazon S3 (when running on AWS EKS). See [Workload Identity for AWS](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html) for more information.
- Google Cloud Storage (when running on Google GKE). See [Workload Identity for Google Cloud](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) for more information.
- Azure Blob Storage (when running on Azure AKS). See [Workload Identity for Azure](https://learn.microsoft.com/en-us/azure/aks/workload-identity-deploy-cluster) for more information.

## Creating Exports

### One-Time Exports

One-time exports allow you to export audit logs for a specific time range with optional filters.

1. **Navigate to Audit Logs**:

   - Go to Admin → Audit Logs
   - Apply any desired filters (user, MCP server, call type, etc.)

2. **Create Export**:

   - Click "Create Export" → "Create One-time Export"
   - If filters are applied, you'll be asked whether to include them

3. **Configure Export**:

   - **Name**: Descriptive name for the export
   - **Bucket**: Storage bucket name where exports will be saved
   - **Key Prefix**: Path prefix within the bucket. If empty, defaults to "mcp-audit-logs/YYYY/MM/DD/" format based on current date.
   - **Time Range**: Start and end dates/times
   - **Filters**: Additional filters to apply

4. **Submit Export**:
   - Click "Create Export" to start the process
   - Monitor progress in the exports list

### Scheduled Exports

Scheduled exports run automatically at specified intervals.

1. **Create Schedule**:

   - Click "Create Export" → "Create Export Schedule"
   - Configure the same options as one-time exports

2. **Schedule Configuration**:

   - **Frequency**: Hourly, Daily, Weekly, or Monthly
   - **Time**: Specific time to run (for daily/weekly/monthly)
   - **Day**: Day of week (weekly) or month (monthly)
   - **Bucket**: Storage bucket name where exports will be saved
   - **Key Prefix**: Path prefix within the bucket. If empty, defaults to "mcp-audit-logs/YYYY/MM/DD/" format based on current date.

3. **Manage Schedules**:
   - View and manage schedules in the "Export Schedules" tab
   - Enable/disable schedules as needed
   - Edit schedule configuration

## Export Format

### JSON Lines (JSONL)

All audit logs are exported in JSON Lines format, where each line contains a complete JSON object representing one audit log entry.

**Example:**

```jsonl
{"timestamp":"2024-01-15T10:30:00Z","user_id":"user123","mcp_server":"github","call_type":"tools/call","response_status":"success"}
{"timestamp":"2024-01-15T10:31:00Z","user_id":"user456","mcp_server":"slack","call_type":"resources/read","response_status":"success"}
```

### File Structure

Exported files are organized with the following structure by default:

```
mcp-audit-logs/
├── <year>/<month>/<day>/
│   │   └── <export-name>-<timestamp>.jsonl
```

You can customize the key prefix to store the exports in a different location.
