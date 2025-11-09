# Deploying Obot to Google Cloud Run

This guide will help you deploy the Obot application to Google Cloud Run.

## Prerequisites

1. **Google Cloud Account**: You need a Google Cloud account with billing enabled
2. **gcloud CLI**: Install the [Google Cloud SDK](https://cloud.google.com/sdk/docs/install)
3. **Authentication**: Authenticate with `gcloud auth login`
4. **Docker**: The deployment uses the pre-built image from `ghcr.io/obot-platform/obot:latest`

## Quick Start

### 1. Configure Your .env File

The deployment script automatically reads from your `.env` file in the root directory. Make sure your `.env` file is configured with your values (you can copy from `.env.example` if needed):

```bash
# Your .env file should contain:
OBOT_PORT=8080
OBOT_SERVER_ENABLE_AUTHENTICATION=true
OBOT_SERVER_HOSTNAME=https://your-domain.com  # Will be auto-updated if not set
OBOT_BOOTSTRAP_TOKEN=your-secret-token
OPENAI_API_KEY=sk-...  # Your OpenAI API key
ANTHROPIC_API_KEY=sk-ant-...  # Optional
GITHUB_AUTH_TOKEN=ghp_...  # Optional
```

**Note**: The script will automatically:
- Load all variables from `.env`
- Create secrets in Secret Manager for sensitive values (API keys, tokens)
- Use the values for Cloud Run deployment

### 2. Set GCP-Specific Variables (Optional)

You can set GCP-specific variables as environment variables or in your `.env` file:

```bash
export GCP_PROJECT_ID="your-project-id"  # Or set in .env
export GCP_REGION="us-central1"  # Or set in .env
```

### 3. Run the Deployment Script

Make the script executable and run it:

```bash
chmod +x deploy-cloud-run.sh
./deploy-cloud-run.sh
```

The script will:
- Enable required Google Cloud APIs
- Create a Cloud SQL PostgreSQL instance (if it doesn't exist)
- Create necessary secrets in Secret Manager
- Deploy the service to Cloud Run
- Configure the service account with proper permissions

### 3. Access Your Service

After deployment, the script will output your service URL. Access it in your browser:

```
https://your-service-url.run.app
```

## Manual Deployment

If you prefer to deploy manually, you can use the `cloud-run-service.yaml` file:

1. **Update the YAML file** with your project details:
   - Replace `PROJECT_ID` with your GCP project ID
   - Replace `REGION` with your desired region
   - Replace `INSTANCE_NAME` with your Cloud SQL instance name
   - Update database credentials
   - Update the service URL in `OBOT_SERVER_HOSTNAME`

2. **Create Cloud SQL instance**:
   ```bash
   gcloud sql instances create obot-db \
     --database-version=POSTGRES_15 \
     --tier=db-f1-micro \
     --region=us-central1
   ```

3. **Create database and user**:
   ```bash
   gcloud sql databases create obot --instance=obot-db
   gcloud sql users create obot --instance=obot-db --password=YOUR_PASSWORD
   ```

4. **Create secrets**:
   ```bash
   echo -n "your-api-key" | gcloud secrets create openai-api-key --data-file=-
   echo -n "your-api-key" | gcloud secrets create anthropic-api-key --data-file=-
   ```

5. **Deploy to Cloud Run**:
   ```bash
   gcloud run services replace cloud-run-service.yaml
   ```

## Configuration

### Environment Variables

Key environment variables you can configure:

| Variable | Description | Required |
|----------|-------------|----------|
| `OBOT_SERVER_DSN` | PostgreSQL connection string | Yes (auto-configured) |
| `OBOT_SERVER_HOSTNAME` | Your Cloud Run service URL | Yes |
| `OPENAI_API_KEY` | OpenAI API key | Recommended |
| `ANTHROPIC_API_KEY` | Anthropic API key | Optional |
| `GITHUB_AUTH_TOKEN` | GitHub token for rate limiting | Optional |
| `OBOT_SERVER_ENCRYPTION_PROVIDER` | Set to "gcp" for GCP KMS | Optional |
| `OBOT_SERVER_ENABLE_AUTHENTICATION` | Enable authentication | Optional |
| `OBOT_BOOTSTRAP_TOKEN` | Bootstrap token for authentication | Optional |
| `OBOT_PORT` | Port to listen on (default: 8080) | Optional |

### Updating Environment Variables

**Recommended**: Update your `.env` file and run the deployment script again. The script will automatically:
- Update secrets in Secret Manager
- Update the Cloud Run service with new environment variables

Alternatively, you can update manually:

```bash
gcloud run services update obot \
  --region=us-central1 \
  --update-env-vars="OBOT_SERVER_HOSTNAME=https://your-url.run.app"
```

### Updating Secrets

To update a secret:

```bash
echo -n "new-value" | gcloud secrets versions add secret-name --data-file=-
```

Then update the service to use the new version:

```bash
gcloud run services update obot \
  --region=us-central1 \
  --update-secrets="OPENAI_API_KEY=openai-api-key:latest"
```

## Important Notes

### Cloud SQL Connection

The deployment uses Cloud SQL Proxy via Unix socket. The connection string format is:
```
postgresql://USER:PASSWORD@/DATABASE?host=/cloudsql/PROJECT_ID:REGION:INSTANCE_NAME
```

### Docker Socket Limitation

Cloud Run does **not** support mounting the Docker socket (`/var/run/docker.sock`). This means:
- Features that require Docker-in-Docker will not work
- MCP servers that need to run in containers may have limitations
- Consider using Cloud Run Jobs or other GCP services for containerized workloads

### Persistent Storage

Cloud Run is stateless. For persistent data:
- Use Cloud SQL for the database (already configured)
- Use Cloud Storage for file storage if needed
- Consider using Cloud Run with Cloud Storage volumes (if available in your region)

### Scaling

The default configuration:
- **Min instances**: 1 (always running)
- **Max instances**: 10
- **CPU**: 2 vCPU
- **Memory**: 4 GiB

Adjust these based on your needs:

```bash
gcloud run services update obot \
  --region=us-central1 \
  --min-instances=0 \
  --max-instances=20 \
  --memory=8Gi \
  --cpu=4
```

### Custom Domain

To use a custom domain:

1. Map your domain in Cloud Run:
   ```bash
   gcloud run domain-mappings create \
     --service=obot \
     --domain=your-domain.com \
     --region=us-central1
   ```

2. Update DNS records as instructed
3. Update `OBOT_SERVER_HOSTNAME` to your custom domain

## Troubleshooting

### View Logs

```bash
gcloud run services logs read obot --region=us-central1
```

### Check Service Status

```bash
gcloud run services describe obot --region=us-central1
```

### Test Database Connection

```bash
gcloud sql connect obot-db --user=obot --database=obot
```

### Common Issues

1. **Service won't start**: Check logs for database connection issues
2. **Database connection errors**: Verify Cloud SQL instance is running and connection name is correct
3. **Secret access denied**: Ensure service account has `roles/secretmanager.secretAccessor` role
4. **Out of memory**: Increase memory allocation with `--memory` flag

## Cost Estimation

Approximate monthly costs (varies by usage):
- **Cloud Run**: ~$25-50 (with 1 always-on instance)
- **Cloud SQL (db-f1-micro)**: ~$7-10
- **Secret Manager**: Minimal (free tier covers most use cases)
- **Networking**: Varies by traffic

Total: ~$35-65/month for basic setup

## Security Best Practices

1. **Use Secret Manager** for all sensitive data (API keys, passwords)
2. **Enable authentication** if exposing publicly
3. **Use IAM** to restrict access to Cloud SQL
4. **Enable VPC** for private networking if needed
5. **Regular updates**: Keep the container image updated
6. **Monitor logs** for suspicious activity

## Next Steps

- Configure OAuth authentication (see [Obot documentation](https://docs.obot.ai))
- Set up monitoring and alerting
- Configure custom domain
- Set up CI/CD for automated deployments
- Review and adjust resource limits based on usage

## Support

For issues specific to:
- **Obot**: Check [Obot documentation](https://docs.obot.ai)
- **Cloud Run**: Check [Cloud Run documentation](https://cloud.google.com/run/docs)
- **Cloud SQL**: Check [Cloud SQL documentation](https://cloud.google.com/sql/docs)

