#!/usr/bin/env bash
#
# Complete Cookie Secret Rotation (Phase 5)
# Removes old secret after grace period
#
# Usage: ./complete-rotation.sh <provider> <namespace>
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

if [ "$#" -ne 2 ]; then
    log_error "Usage: $0 <provider> <namespace>"
    exit 1
fi

PROVIDER="$1"
NAMESPACE="$2"
SECRET_NAME="${PROVIDER}-secrets"
PROVIDER_UPPER=$(echo "$PROVIDER" | tr '[:lower:]' '[:upper:]' | tr '-' '_')

log_info "=== Complete Cookie Secret Rotation ==="
log_info "Provider: $PROVIDER"
log_info "Namespace: $NAMESPACE"
log_info ""

# Check if grace period elapsed
log_warn "⚠️  This will remove old secrets. Ensure:"
log_warn "  1. Grace period elapsed (7+ days)"
log_warn "  2. No active sessions using old secret"
log_warn "  3. Metrics show zero 'previous-1' usage"
echo ""
read -p "Have you verified all conditions? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    log_info "Completion cancelled. Run: ./check-rotation-status.sh $PROVIDER $NAMESPACE"
    exit 0
fi

# Get current secret (will become the only secret)
CURRENT_SECRET=$(kubectl get secret "$SECRET_NAME" \
    -n "$NAMESPACE" \
    -o jsonpath="{.data.OBOT_${PROVIDER_UPPER}_COOKIE_SECRET}" | base64 -d)

if [ -z "$CURRENT_SECRET" ]; then
    log_error "Could not retrieve current secret"
    exit 1
fi

log_info "✓ Current secret retrieved"

# Create single-secret configuration
TEMP_YAML="/tmp/single-secret-config-$(date +%Y%m%d-%H%M%S).yaml"

cat > "$TEMP_YAML" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $SECRET_NAME
  namespace: $NAMESPACE
type: Opaque
stringData:
  # Only current secret remains
  OBOT_${PROVIDER_UPPER}_COOKIE_SECRET: "$CURRENT_SECRET"
EOF

log_info "✓ Single-secret configuration created"

# Apply configuration
log_info "Applying configuration..."
if kubectl apply -f "$TEMP_YAML"; then
    log_info "✓ Configuration applied"
else
    log_error "Failed to apply configuration"
    exit 1
fi

# Restart provider
log_info "Restarting provider..."
kubectl rollout restart "deployment/$PROVIDER" -n "$NAMESPACE"
kubectl rollout status "deployment/$PROVIDER" -n "$NAMESPACE" --timeout=5m

log_info "✓ Rollout completed"

# Cleanup
rm -f "$TEMP_YAML"

log_info ""
log_info "✅ Rotation Complete!"
log_info ""
log_info "Old secret has been removed. All sessions now use new secret."
log_info "Update docs/secret-rotation-log.md with completion date."
log_info ""
