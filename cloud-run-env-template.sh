#!/bin/bash
# Environment variables template for Cloud Run deployment
# 
# NOTE: The deploy-cloud-run.sh script automatically reads from .env file.
# This file is optional - you can either:
# 1. Use your existing .env file (recommended)
# 2. Set these as environment variables before running the script
# 3. Source this file if you want to override .env values

# Required: GCP Project ID
export GCP_PROJECT_ID="your-project-id"

# Required: GCP Region (e.g., us-central1, europe-west1)
export GCP_REGION="us-central1"

# Required: Database password (will be generated if not set)
export DB_PASSWORD="your-secure-password"

# Required: Database root password for Cloud SQL (will be generated if not set)
export DB_ROOT_PASSWORD="your-secure-root-password"

# Optional: API Keys (can also be set via Secret Manager after deployment)
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GITHUB_AUTH_TOKEN="ghp_..."

# Optional: Server hostname (will be set automatically after first deployment)
export OBOT_SERVER_HOSTNAME="https://your-service-url.run.app"

# Optional: Other Obot configuration
export OBOT_SERVER_ENCRYPTION_PROVIDER="gcp"
export OBOT_SERVER_ENABLE_AUTHENTICATION="true"
export OBOT_SERVER_RETENTION_POLICY_HOURS="2160"

