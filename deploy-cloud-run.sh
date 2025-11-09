#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Load environment variables from .env file if it exists
ENV_FILE="$(dirname "$0")/.env"
if [ -f "$ENV_FILE" ]; then
    echo -e "${GREEN}Loading environment variables from .env file...${NC}"
    # Export variables from .env file, ignoring comments and empty lines
    # This handles KEY=VALUE format properly, even with special characters
    # Strips surrounding quotes from values
    set -a
    while IFS= read -r line || [ -n "$line" ]; do
        # Skip comments and empty lines
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "${line// }" ]] && continue
        
        # Extract key and value
        if [[ "$line" =~ ^[[:space:]]*([^=]+)=(.*)$ ]]; then
            key="${BASH_REMATCH[1]// /}"
            value="${BASH_REMATCH[2]}"
            
            # Remove leading/trailing whitespace
            value="${value#"${value%%[![:space:]]*}"}"
            value="${value%"${value##*[![:space:]]}"}"
            
            # Strip surrounding quotes (single or double)
            if [[ "$value" =~ ^\".*\"$ ]] || [[ "$value" =~ ^\'.*\'$ ]]; then
                value="${value:1:-1}"
            fi
            
            # Export the variable
            export "$key=$value" 2>/dev/null || true
        fi
    done < "$ENV_FILE"
    set +a
    echo -e "${GREEN}Loaded environment variables from .env${NC}"
else
    echo -e "${YELLOW}Warning: .env file not found at $ENV_FILE${NC}"
    echo -e "${YELLOW}You can create one from .env.example${NC}"
fi

# Configuration
# Note: PROJECT_ID and REGION will be set from .env file (GCP_PROJECT_ID and GCP_REGION)
SERVICE_NAME="obot"
IMAGE_NAME="ghcr.io/obot-platform/obot:latest"

# Check if gcloud is installed
if ! command -v gcloud &> /dev/null; then
    echo -e "${RED}Error: gcloud CLI is not installed.${NC}"
    echo "Please install it from: https://cloud.google.com/sdk/docs/install"
    exit 1
fi

# Check if user is authenticated
ACTIVE_ACCOUNT=$(gcloud auth list --filter=status:ACTIVE --format="value(account)" 2>/dev/null | head -1)

if [ -z "$ACTIVE_ACCOUNT" ]; then
    echo -e "${RED}Error: Not authenticated with gcloud.${NC}"
    echo "Please run: gcloud auth login"
    exit 1
fi

# Check if authenticated as compute service account (which has limited permissions)
if echo "$ACTIVE_ACCOUNT" | grep -q "compute@developer.gserviceaccount.com"; then
    echo -e "${RED}Error: Authenticated as compute service account: $ACTIVE_ACCOUNT${NC}"
    echo -e "${YELLOW}This account has insufficient permissions for Cloud Run deployment.${NC}"
    echo ""
    echo -e "${YELLOW}To fix this, you have two options:${NC}"
    echo ""
    echo -e "${GREEN}Option 1: Authenticate with your personal Google account${NC}"
    echo "  Run: gcloud auth login"
    echo "  (You may need to do this from your local machine if on a GCE VM)"
    echo ""
    echo -e "${GREEN}Option 2: Use a service account key file${NC}"
    echo "  1. Create a service account in GCP Console with these roles:"
    echo "     - Cloud Run Admin"
    echo "     - Cloud SQL Admin"
    echo "     - Secret Manager Admin"
    echo "     - Service Account User"
    echo "  2. Download the key file"
    echo "  3. Run: gcloud auth activate-service-account --key-file=KEY_FILE.json"
    echo ""
    echo -e "${YELLOW}Current active account: $ACTIVE_ACCOUNT${NC}"
    exit 1
fi

echo -e "${GREEN}Authenticated as: $ACTIVE_ACCOUNT${NC}"

# Validate required environment variables from .env
if [ -z "$OBOT_SERVER_DSN" ]; then
    echo -e "${RED}Error: OBOT_SERVER_DSN is required but not set in .env file.${NC}"
    echo "Please add OBOT_SERVER_DSN to your .env file."
    exit 1
fi

if [ -z "$GCP_PROJECT_ID" ]; then
    echo -e "${RED}Error: GCP_PROJECT_ID is required but not set in .env file.${NC}"
    echo "Please add GCP_PROJECT_ID to your .env file."
    exit 1
fi

if [ -z "$GCP_REGION" ]; then
    echo -e "${RED}Error: GCP_REGION is required but not set in .env file.${NC}"
    echo "Please add GCP_REGION to your .env file."
    exit 1
fi

# Use GCP_PROJECT_ID and GCP_REGION from .env
PROJECT_ID="$GCP_PROJECT_ID"
REGION="$GCP_REGION"

echo -e "${GREEN}Using GCP_PROJECT_ID from .env: $PROJECT_ID${NC}"
echo -e "${GREEN}Using GCP_REGION from .env: $REGION${NC}"
echo -e "${GREEN}Using OBOT_SERVER_DSN from .env${NC}"

# Set the project
gcloud config set project "$PROJECT_ID" 2>/dev/null || {
    echo -e "${YELLOW}Warning: Could not set project. Continuing with current project.${NC}"
}

# Enable required APIs
echo -e "${YELLOW}Enabling required Google Cloud APIs...${NC}"
gcloud services enable \
    run.googleapis.com \
    secretmanager.googleapis.com \
    cloudbuild.googleapis.com \
    --project="$PROJECT_ID"

# Create service account if it doesn't exist
SERVICE_ACCOUNT="${SERVICE_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
if ! gcloud iam service-accounts describe "$SERVICE_ACCOUNT" --project="$PROJECT_ID" &>/dev/null; then
    echo -e "${YELLOW}Creating service account...${NC}"
    gcloud iam service-accounts create "${SERVICE_NAME}" \
        --display-name="Obot Cloud Run Service Account" \
        --project="$PROJECT_ID"
    
    # Grant necessary permissions
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:$SERVICE_ACCOUNT" \
        --role="roles/secretmanager.secretAccessor"
fi

# Create secrets if they don't exist
create_secret_if_not_exists() {
    local secret_name=$1
    local secret_value=$2
    
    if [ -z "$secret_value" ]; then
        return 0
    fi
    
    if ! gcloud secrets describe "$secret_name" --project="$PROJECT_ID" &>/dev/null; then
        echo -e "${YELLOW}Creating secret: $secret_name${NC}"
        echo -n "$secret_value" | gcloud secrets create "$secret_name" \
            --data-file=- \
            --replication-policy="automatic" \
            --project="$PROJECT_ID"
    else
        echo -e "${GREEN}Secret $secret_name already exists. Updating...${NC}"
        echo -n "$secret_value" | gcloud secrets versions add "$secret_name" \
            --data-file=- \
            --project="$PROJECT_ID"
    fi
    
    # Grant access to service account
    gcloud secrets add-iam-policy-binding "$secret_name" \
        --member="serviceAccount:$SERVICE_ACCOUNT" \
        --role="roles/secretmanager.secretAccessor" \
        --project="$PROJECT_ID" \
        --quiet &>/dev/null || true
}

# Create or update secrets from .env file
echo -e "${YELLOW}Setting up secrets from .env file...${NC}"

if [ -n "$OPENAI_API_KEY" ] && [ "$OPENAI_API_KEY" != "your-openai-api-key-here" ]; then
    create_secret_if_not_exists "openai-api-key" "$OPENAI_API_KEY"
else
    echo -e "${YELLOW}OPENAI_API_KEY not set or using placeholder. Skipping secret creation.${NC}"
fi

if [ -n "$ANTHROPIC_API_KEY" ] && [ "$ANTHROPIC_API_KEY" != "your-anthropic-api-key-here" ]; then
    create_secret_if_not_exists "anthropic-api-key" "$ANTHROPIC_API_KEY"
else
    echo -e "${YELLOW}ANTHROPIC_API_KEY not set. Skipping secret creation.${NC}"
fi

if [ -n "$GITHUB_AUTH_TOKEN" ] && [ "$GITHUB_AUTH_TOKEN" != "your-github-token-here" ]; then
    create_secret_if_not_exists "github-auth-token" "$GITHUB_AUTH_TOKEN"
else
    echo -e "${YELLOW}GITHUB_AUTH_TOKEN not set. Skipping secret creation.${NC}"
fi

if [ -n "$OBOT_BOOTSTRAP_TOKEN" ] && [ "$OBOT_BOOTSTRAP_TOKEN" != "my-very-secret-token" ]; then
    create_secret_if_not_exists "obot-bootstrap-token" "$OBOT_BOOTSTRAP_TOKEN"
else
    echo -e "${YELLOW}OBOT_BOOTSTRAP_TOKEN not set or using placeholder. Skipping secret creation.${NC}"
fi

# Deploy to Cloud Run
echo -e "${YELLOW}Deploying to Cloud Run...${NC}"

# Build environment variables from .env file
# Use OBOT_SERVER_DSN directly from .env file
ENV_VARS="OBOT_SERVER_DSN=$OBOT_SERVER_DSN"

# Add OBOT_SERVER_HOSTNAME if set (will be updated after first deployment if not set)
if [ -n "$OBOT_SERVER_HOSTNAME" ] && [ "$OBOT_SERVER_HOSTNAME" != "https://my-mcp-catalog-domain.ai" ]; then
    ENV_VARS="$ENV_VARS,OBOT_SERVER_HOSTNAME=$OBOT_SERVER_HOSTNAME"
fi

# Add OBOT_SERVER_ENABLE_AUTHENTICATION if set
if [ -n "$OBOT_SERVER_ENABLE_AUTHENTICATION" ]; then
    ENV_VARS="$ENV_VARS,OBOT_SERVER_ENABLE_AUTHENTICATION=$OBOT_SERVER_ENABLE_AUTHENTICATION"
fi

# Add OBOT_SERVER_RETENTION_POLICY_HOURS if set
if [ -n "$OBOT_SERVER_RETENTION_POLICY_HOURS" ]; then
    ENV_VARS="$ENV_VARS,OBOT_SERVER_RETENTION_POLICY_HOURS=$OBOT_SERVER_RETENTION_POLICY_HOURS"
fi

# Add OBOT_SERVER_ENCRYPTION_PROVIDER if set (default to gcp for Cloud Run)
if [ -n "$OBOT_SERVER_ENCRYPTION_PROVIDER" ]; then
    ENV_VARS="$ENV_VARS,OBOT_SERVER_ENCRYPTION_PROVIDER=$OBOT_SERVER_ENCRYPTION_PROVIDER"
else
    ENV_VARS="$ENV_VARS,OBOT_SERVER_ENCRYPTION_PROVIDER=gcp"
fi

# Build secrets list (only include secrets that exist)
SECRETS_LIST=""
if gcloud secrets describe "openai-api-key" --project="$PROJECT_ID" &>/dev/null; then
    SECRETS_LIST="OPENAI_API_KEY=openai-api-key:latest"
fi
if gcloud secrets describe "anthropic-api-key" --project="$PROJECT_ID" &>/dev/null; then
    if [ -n "$SECRETS_LIST" ]; then
        SECRETS_LIST="$SECRETS_LIST,ANTHROPIC_API_KEY=anthropic-api-key:latest"
    else
        SECRETS_LIST="ANTHROPIC_API_KEY=anthropic-api-key:latest"
    fi
fi
if gcloud secrets describe "github-auth-token" --project="$PROJECT_ID" &>/dev/null; then
    if [ -n "$SECRETS_LIST" ]; then
        SECRETS_LIST="$SECRETS_LIST,GITHUB_AUTH_TOKEN=github-auth-token:latest"
    else
        SECRETS_LIST="GITHUB_AUTH_TOKEN=github-auth-token:latest"
    fi
fi
if gcloud secrets describe "obot-bootstrap-token" --project="$PROJECT_ID" &>/dev/null; then
    if [ -n "$SECRETS_LIST" ]; then
        SECRETS_LIST="$SECRETS_LIST,OBOT_BOOTSTRAP_TOKEN=obot-bootstrap-token:latest"
    else
        SECRETS_LIST="OBOT_BOOTSTRAP_TOKEN=obot-bootstrap-token:latest"
    fi
fi

# Build the deployment command
DEPLOY_CMD="gcloud run deploy $SERVICE_NAME \
    --image=$IMAGE_NAME \
    --platform=managed \
    --region=$REGION \
    --service-account=$SERVICE_ACCOUNT \
    --set-env-vars=\"$ENV_VARS\" \
    --memory=4Gi \
    --cpu=2 \
    --min-instances=1 \
    --max-instances=10 \
    --timeout=300 \
    --port=${OBOT_PORT:-8080} \
    --allow-unauthenticated \
    --project=$PROJECT_ID"

# Add secrets if any exist
if [ -n "$SECRETS_LIST" ]; then
    DEPLOY_CMD="$DEPLOY_CMD --set-secrets=\"$SECRETS_LIST\""
fi

# Execute deployment
eval $DEPLOY_CMD

# Get the service URL
SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" --region="$REGION" --format="value(status.url)" --project="$PROJECT_ID")

echo -e "${GREEN}Deployment complete!${NC}"
echo -e "${GREEN}Service URL: $SERVICE_URL${NC}"
echo ""

# Update hostname if it wasn't set or was using placeholder
if [ -z "$OBOT_SERVER_HOSTNAME" ] || [ "$OBOT_SERVER_HOSTNAME" = "https://my-mcp-catalog-domain.ai" ]; then
    echo -e "${YELLOW}Updating OBOT_SERVER_HOSTNAME to the Cloud Run service URL...${NC}"
    
    # Rebuild ENV_VARS with the service URL
    UPDATED_ENV_VARS="OBOT_SERVER_DSN=$OBOT_SERVER_DSN"
    UPDATED_ENV_VARS="$UPDATED_ENV_VARS,OBOT_SERVER_HOSTNAME=$SERVICE_URL"
    
    # Add other environment variables if they were set
    if [ -n "$OBOT_SERVER_ENABLE_AUTHENTICATION" ]; then
        UPDATED_ENV_VARS="$UPDATED_ENV_VARS,OBOT_SERVER_ENABLE_AUTHENTICATION=$OBOT_SERVER_ENABLE_AUTHENTICATION"
    fi
    if [ -n "$OBOT_SERVER_RETENTION_POLICY_HOURS" ]; then
        UPDATED_ENV_VARS="$UPDATED_ENV_VARS,OBOT_SERVER_RETENTION_POLICY_HOURS=$OBOT_SERVER_RETENTION_POLICY_HOURS"
    fi
    if [ -n "$OBOT_SERVER_ENCRYPTION_PROVIDER" ]; then
        UPDATED_ENV_VARS="$UPDATED_ENV_VARS,OBOT_SERVER_ENCRYPTION_PROVIDER=$OBOT_SERVER_ENCRYPTION_PROVIDER"
    else
        UPDATED_ENV_VARS="$UPDATED_ENV_VARS,OBOT_SERVER_ENCRYPTION_PROVIDER=gcp"
    fi
    
    gcloud run services update "$SERVICE_NAME" \
        --region="$REGION" \
        --update-env-vars="$UPDATED_ENV_VARS" \
        --project="$PROJECT_ID" \
        --quiet
    
    echo -e "${GREEN}Updated OBOT_SERVER_HOSTNAME to: $SERVICE_URL${NC}"
    echo ""
    echo -e "${YELLOW}Note: If you want to use a custom domain, update your .env file and redeploy.${NC}"
else
    echo -e "${GREEN}OBOT_SERVER_HOSTNAME is already set to: $OBOT_SERVER_HOSTNAME${NC}"
fi

echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Access your service at: $SERVICE_URL"
echo "2. Configure authentication if needed (already enabled: ${OBOT_SERVER_ENABLE_AUTHENTICATION:-false})"
echo "3. Set up custom domain (optional)"
echo ""
echo "To update environment variables, edit your .env file and run this script again."

