#!/usr/bin/env bash
#
# Cookie Secret Rotation Script
# Automates Phases 1-2 of secret rotation with zero downtime
#
# Usage: ./rotate-cookie-secret.sh <provider> <namespace>
# Example: ./rotate-cookie-secret.sh entra-auth-provider auth-providers
#

set -euo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl."
        exit 1
    fi

    if ! command -v openssl &> /dev/null; then
        log_error "openssl not found. Please install openssl."
        exit 1
    fi

    if ! command -v base64 &> /dev/null; then
        log_error "base64 not found. Please install base64."
        exit 1
    fi

    log_info "All prerequisites met."
}

# Validate arguments
if [ "$#" -ne 2 ]; then
    log_error "Usage: $0 <provider> <namespace>"
    log_error "Example: $0 entra-auth-provider auth-providers"
    exit 1
fi

PROVIDER="$1"
NAMESPACE="$2"
SECRET_NAME="${PROVIDER}-secrets"
PROVIDER_UPPER=$(echo "$PROVIDER" | tr '[:lower:]' '[:upper:]' | tr '-' '_')

log_info "=== Cookie Secret Rotation ==="
log_info "Provider: $PROVIDER"
log_info "Namespace: $NAMESPACE"
log_info "Secret: $SECRET_NAME"
log_info ""

check_prerequisites

# Confirmation
echo -e "${YELLOW}⚠️  This will rotate the cookie secret for $PROVIDER.${NC}"
echo -e "${YELLOW}⚠️  Users with active sessions will continue to work during the grace period.${NC}"
echo ""
read -p "Continue? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    log_info "Rotation cancelled."
    exit 0
fi

# Phase 1: Generate new secret
log_info "Phase 1: Generating new secret..."

NEW_SECRET=$(openssl rand -base64 32)
NEW_SECRET_LENGTH=$(echo "$NEW_SECRET" | base64 -d | wc -c | tr -d ' ')

if [ "$NEW_SECRET_LENGTH" -ne 32 ]; then
    log_error "Generated secret has incorrect length: $NEW_SECRET_LENGTH bytes (expected 32)"
    exit 1
fi

log_info "✓ New secret generated (32 bytes)"
log_info "  Last 8 characters: ...${NEW_SECRET: -8}"

# Save new secret to temporary file (for disaster recovery)
BACKUP_FILE="/tmp/cookie-secret-rotation-$(date +%Y%m%d-%H%M%S).txt"
echo "$NEW_SECRET" > "$BACKUP_FILE"
chmod 600 "$BACKUP_FILE"
log_info "✓ Secret backed up to: $BACKUP_FILE"

# Phase 2: Get current secret
log_info ""
log_info "Phase 2: Retrieving current secret..."

# Try provider-specific secret first
CURRENT_SECRET_KEY="OBOT_${PROVIDER_UPPER}_COOKIE_SECRET"
CURRENT_SECRET=$(kubectl get secret "$SECRET_NAME" \
    -n "$NAMESPACE" \
    -o jsonpath="{.data.$CURRENT_SECRET_KEY}" 2>/dev/null | base64 -d || echo "")

# Fallback to shared secret
if [ -z "$CURRENT_SECRET" ]; then
    log_warn "Provider-specific secret not found, trying shared secret..."
    CURRENT_SECRET=$(kubectl get secret "$SECRET_NAME" \
        -n "$NAMESPACE" \
        -o jsonpath='{.data.OBOT_AUTH_PROVIDER_COOKIE_SECRET}' 2>/dev/null | base64 -d || echo "")
fi

if [ -z "$CURRENT_SECRET" ]; then
    log_error "Could not retrieve current secret from Kubernetes."
    log_error "Secret: $SECRET_NAME in namespace: $NAMESPACE"
    exit 1
fi

CURRENT_SECRET_LENGTH=$(echo "$CURRENT_SECRET" | base64 -d | wc -c | tr -d ' ')
log_info "✓ Current secret retrieved ($CURRENT_SECRET_LENGTH bytes)"
log_info "  Last 8 characters: ...${CURRENT_SECRET: -8}"

# Phase 3: Create dual-secret configuration
log_info ""
log_info "Phase 3: Creating dual-secret configuration..."

# Create temporary YAML file
TEMP_YAML="/tmp/dual-secret-config-$(date +%Y%m%d-%H%M%S).yaml"

cat > "$TEMP_YAML" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $SECRET_NAME
  namespace: $NAMESPACE
type: Opaque
stringData:
  # New secret (for encryption)
  OBOT_${PROVIDER_UPPER}_COOKIE_SECRET: "$NEW_SECRET"

  # Old secret (for decryption of existing cookies)
  OBOT_${PROVIDER_UPPER}_PREVIOUS_COOKIE_SECRETS: "$CURRENT_SECRET"

  # Grace period: 7 days (168 hours)
  OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD: "168h"
EOF

log_info "✓ Configuration file created: $TEMP_YAML"

# Phase 4: Apply configuration
log_info ""
log_info "Phase 4: Applying dual-secret configuration..."

if kubectl apply -f "$TEMP_YAML"; then
    log_info "✓ Configuration applied successfully"
else
    log_error "Failed to apply configuration"
    exit 1
fi

# Phase 5: Restart provider
log_info ""
log_info "Phase 5: Restarting provider pods..."

if kubectl rollout restart "deployment/$PROVIDER" -n "$NAMESPACE"; then
    log_info "✓ Rollout initiated"
else
    log_error "Failed to restart deployment"
    exit 1
fi

# Wait for rollout
log_info "Waiting for rollout to complete..."
if kubectl rollout status "deployment/$PROVIDER" -n "$NAMESPACE" --timeout=5m; then
    log_info "✓ Rollout completed successfully"
else
    log_error "Rollout failed or timed out"
    exit 1
fi

# Phase 6: Verification
log_info ""
log_info "Phase 6: Verifying rotation..."

sleep 5  # Give pods time to start

# Check logs for rotation confirmation
log_info "Checking provider logs..."
LOG_OUTPUT=$(kubectl logs -n "$NAMESPACE" "deployment/$PROVIDER" --tail=50 2>/dev/null | grep "secret rotation" || echo "")

if [ -n "$LOG_OUTPUT" ]; then
    log_info "✓ Rotation confirmed in logs:"
    echo "$LOG_OUTPUT" | head -3
else
    log_warn "Could not verify rotation in logs (provider may still be starting)"
fi

# Cleanup temporary files
rm -f "$TEMP_YAML"
log_info "✓ Temporary configuration file cleaned up"

# Summary
log_info ""
log_info "=== Rotation Complete ==="
log_info ""
log_info "✅ New secret deployed successfully"
log_info "✅ Dual-secret mode activated"
log_info "✅ Grace period: 7 days (until $(date -d '+7 days' +%Y-%m-%d 2>/dev/null || date -v+7d +%Y-%m-%d 2>/dev/null || echo 'unknown'))"
log_info ""
log_info "Next Steps:"
log_info "1. Monitor Prometheus metrics for 7 days:"
log_info "   curl http://obot-server:8080/debug/metrics | grep obot_auth_cookie_decrypt_by_version"
log_info ""
log_info "2. After grace period (7+ days), complete rotation:"
log_info "   ./scripts/complete-rotation.sh $PROVIDER $NAMESPACE"
log_info ""
log_info "3. Update rotation log in docs/secret-rotation-log.md"
log_info ""
log_info "Backup files (delete after grace period):"
log_info "  - $BACKUP_FILE"
log_info ""
log_info "For troubleshooting, see: docs/SECRET_ROTATION_RUNBOOK.md"
log_info ""
