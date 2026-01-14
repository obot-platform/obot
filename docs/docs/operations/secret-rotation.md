---
sidebar_position: 1
title: Cookie Secret Rotation
description: Operational runbook for rotating authentication provider cookie secrets with zero downtime
---

# Cookie Secret Rotation Runbook

## Overview

This runbook provides step-by-step procedures for rotating cookie secrets in Obot authentication providers with zero downtime. Cookie secrets are used to encrypt session data and must be rotated periodically for security compliance.

**Target Audience:** DevOps Engineers, SREs, Security Engineers
**Estimated Time:** 15-20 minutes (excluding grace period wait)
**Risk Level:** Low (zero-downtime procedure)

---

## Prerequisites

### Required Access
- [ ] Kubernetes cluster access (`kubectl` configured)
- [ ] Ability to modify provider ConfigMaps/Secrets
- [ ] Access to Prometheus metrics (for validation)
- [ ] Ability to restart provider pods

### Required Knowledge
- Basic Kubernetes concepts
- Understanding of cookie-based sessions
- Familiarity with base64 encoding

### Tools Required
- `kubectl` CLI
- `openssl` (for secret generation)
- `curl` or web browser (for testing)

---

## Secret Rotation Architecture

### How It Works

```
Current Secret (COOKIE_SECRET):
  - Used for ENCRYPTING new session cookies
  - Used for DECRYPTING existing cookies

Previous Secrets (PREVIOUS_COOKIE_SECRETS):
  - Used ONLY for DECRYPTING old cookies
  - Comma-separated list (oldest to newest)
  - Maintained during grace period
```

### Rotation States

**State 1: Single Secret (Pre-Rotation)**
```
COOKIE_SECRET: secret-A
PREVIOUS_COOKIE_SECRETS: (empty)
```
All cookies encrypted/decrypted with secret-A

**State 2: Dual Secrets (During Grace Period)**
```
COOKIE_SECRET: secret-B (NEW)
PREVIOUS_COOKIE_SECRETS: secret-A (OLD)
```
- New cookies encrypted with secret-B
- Old cookies (secret-A) still work
- Grace period: 7 days (recommended)

**State 3: Single Secret (Post-Rotation)**
```
COOKIE_SECRET: secret-B
PREVIOUS_COOKIE_SECRETS: (empty)
```
All cookies now use secret-B

---

## Procedure: Manual Cookie Secret Rotation

### Phase 1: Generate New Secret

**Step 1.1: Generate new 32-byte secret**
```bash
# Generate new secret
NEW_SECRET=$(openssl rand -base64 32)
echo "New secret generated: $NEW_SECRET"

# Save to secure location (e.g., password manager)
echo "$NEW_SECRET" > new-secret.txt
chmod 600 new-secret.txt
```

**Step 1.2: Validate secret entropy**
```bash
# Decode and check length (should be 32 bytes)
echo "$NEW_SECRET" | base64 -d | wc -c
# Expected output: 32
```

---

### Phase 2: Deploy Dual-Secret Configuration

**Step 2.1: Get current secret**
```bash
# For Entra ID provider
CURRENT_SECRET=$(kubectl get secret entra-auth-provider-secrets \
  -n auth-providers \
  -o jsonpath='{.data.OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET}' | base64 -d)

echo "Current secret: $CURRENT_SECRET"
```

**Step 2.2: Update configuration with dual secrets**

Create file `dual-secret-config.yaml`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: entra-auth-provider-secrets
  namespace: auth-providers
type: Opaque
stringData:
  # New secret (for encryption)
  OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET: "<NEW_SECRET>"

  # Old secret (for decryption of existing cookies)
  OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS: "<CURRENT_SECRET>"

  # Grace period: 7 days recommended (168 hours)
  OBOT_AUTH_PROVIDER_SECRET_GRACE_PERIOD: "168h"
```

**Step 2.3: Apply configuration**
```bash
# Apply the dual-secret configuration
kubectl apply -f dual-secret-config.yaml

# Restart provider pods to load new configuration
kubectl rollout restart deployment/entra-auth-provider -n auth-providers

# Wait for rollout to complete
kubectl rollout status deployment/entra-auth-provider -n auth-providers
```

**Step 2.4: Verify dual-secret mode**
```bash
# Check provider logs
kubectl logs -n auth-providers deployment/entra-auth-provider --tail=50 | grep "secret rotation"

# Expected log:
# INFO: secret rotation: version=2, 1 previous secret(s), age=0h, grace_period=168h
```

---

### Phase 3: Validation and Monitoring

**Step 3.1: Test new sessions (encrypted with new secret)**
```bash
# Create new session by logging in
# Navigate to: https://your-obot-instance.com/oauth2/start
# Complete login flow

# New session cookie should be encrypted with NEW_SECRET
```

**Step 3.2: Test old sessions (decrypted with old secret)**
```bash
# Use existing session from before rotation
# Should still work during grace period
# Cookie decrypted using PREVIOUS_COOKIE_SECRETS
```

**Step 3.3: Monitor Prometheus metrics**
```bash
# Check secret version
curl -s http://obot-server:8080/debug/metrics | grep obot_auth_cookie_secret_version
# Expected: obot_auth_cookie_secret_version{provider="entra-auth-provider"} 2

# Check rotation age
curl -s http://obot-server:8080/debug/metrics | grep obot_auth_secret_rotation_age
# Expected: obot_auth_secret_rotation_age_seconds{provider="entra-auth-provider"} 0

# Check decryption by version
curl -s http://obot-server:8080/debug/metrics | grep obot_auth_cookie_decrypt_by_version_total
# Expected output shows both current and previous-1 being used:
# obot_auth_cookie_decrypt_by_version_total{provider="entra-auth-provider",version="current"} 10
# obot_auth_cookie_decrypt_by_version_total{provider="entra-auth-provider",version="previous-1"} 5
```

**Step 3.4: Alert configuration (optional but recommended)**
```yaml
# Prometheus alert for old secret usage
groups:
  - name: auth_rotation
    rules:
      - alert: OldCookieSecretStillInUse
        expr: |
          obot_auth_cookie_decrypt_by_version_total{version=~"previous-.*"} > 0
        for: 8d  # 1 day after grace period ends
        annotations:
          summary: "Old cookie secrets still in use after grace period"
          description: "Provider {{ $labels.provider }} is still decrypting cookies with old secrets"
```

---

### Phase 4: Grace Period Wait

**Step 4.1: Wait for grace period (7 days recommended)**

The grace period ensures all old sessions expire naturally. Recommended duration:
- **Minimum:** Maximum session lifetime (e.g., 24 hours)
- **Recommended:** 7 days (covers edge cases, holidays, etc.)
- **Maximum:** Based on security policy requirements

**During grace period:**
- Both secrets remain active
- New sessions use NEW_SECRET
- Old sessions gradually expire
- Monitor metrics to track progress

**Step 4.2: Monitor old secret usage decline**
```bash
# Daily check: How many sessions still use old secret?
kubectl logs -n auth-providers deployment/entra-auth-provider | \
  grep "previous-1" | \
  tail -20

# Or query Prometheus:
# obot_auth_cookie_decrypt_by_version_total{version="previous-1"}
# Should decrease over time
```

**Step 4.3: Determine if safe to proceed**

Safe to proceed to Phase 5 when:
- [x] Grace period elapsed (7+ days)
- [x] Old secret usage dropped to zero (check metrics)
- [x] No active sessions older than grace period
- [x] All users have logged in at least once since rotation

---

### Phase 5: Remove Old Secret

**Step 5.1: Create single-secret configuration**

Create file `single-secret-config.yaml`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: entra-auth-provider-secrets
  namespace: auth-providers
type: Opaque
stringData:
  # Only new secret remains
  OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET: "<NEW_SECRET>"

  # Previous secrets removed
  # OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS: (removed)
```

**Step 5.2: Apply configuration**
```bash
# Apply single-secret configuration
kubectl apply -f single-secret-config.yaml

# Restart provider pods
kubectl rollout restart deployment/entra-auth-provider -n auth-providers

# Wait for rollout
kubectl rollout status deployment/entra-auth-provider -n auth-providers
```

**Step 5.3: Verify single-secret mode**
```bash
# Check provider logs
kubectl logs -n auth-providers deployment/entra-auth-provider --tail=50 | grep "secret rotation"

# Expected log:
# INFO: secret rotation: version=1, current secret only, age=168h
```

**Step 5.4: Final validation**
```bash
# Test login flow
# All new sessions should work normally

# Check metrics
curl -s http://obot-server:8080/debug/metrics | grep obot_auth_cookie_secret_version
# Expected: obot_auth_cookie_secret_version{provider="entra-auth-provider"} 1

# Verify no "previous-1" decryptions
curl -s http://obot-server:8080/debug/metrics | grep 'version="previous-1"'
# Expected: (no output - metric should not exist)
```

---

### Phase 6: Cleanup and Documentation

**Step 6.1: Secure old secret disposal**
```bash
# Securely delete old secret from local files
shred -u old-secret.txt

# Remove from password manager (if stored)
# Update rotation log (see template below)
```

**Step 6.2: Update rotation log**

Create/update `secret-rotation-log.md`:
```markdown
## Secret Rotation Log

### Rotation Event: 2026-01-13
- **Provider:** entra-auth-provider
- **Initiated By:** John Doe (john@example.com)
- **Start Date:** 2026-01-13 09:00 UTC
- **Grace Period End:** 2026-01-20 09:00 UTC
- **Completion Date:** 2026-01-20 10:00 UTC
- **Old Secret (last 8 chars):** ...abc123==
- **New Secret (last 8 chars):** ...xyz789==
- **Issues:** None
- **Rollback Required:** No
```

**Step 6.3: Schedule next rotation**
```bash
# Add calendar reminder for next rotation
# Recommended frequency: Every 90 days

# Example: Next rotation due 2026-04-13
```

---

## Automated Rotation Scripts

See `scripts/rotate-cookie-secret.sh` for automated rotation implementation.

---

## Troubleshooting

### Issue: Users getting "Session Invalid" errors after rotation

**Symptoms:**
- Users logged in before rotation cannot access Obot
- Error: "Session cookie decryption failed"

**Root Cause:**
- Old secret removed too soon (before grace period)
- Old secret not added to PREVIOUS_COOKIE_SECRETS

**Resolution:**
```bash
# Rollback: Re-add old secret to PREVIOUS_COOKIE_SECRETS
# Follow Phase 2 again with correct dual-secret configuration
```

---

### Issue: Metrics show high "previous-1" usage after grace period

**Symptoms:**
- After 7+ days, still significant old secret usage
- Metrics: `obot_auth_cookie_decrypt_by_version_total{version="previous-1"}` > 0

**Root Cause:**
- Long-lived sessions (exceed expected lifetime)
- Users with very infrequent access

**Resolution:**
```bash
# Option A: Extend grace period (recommended)
# Wait additional 3-7 days for sessions to expire

# Option B: Force logout all users (disruptive)
kubectl exec -n auth-providers deployment/entra-auth-provider -- \
  /path/to/force-logout-script.sh

# Option C: Accept minimal impact and proceed
# Document known affected users
```

---

### Issue: Rotation metrics not updating

**Symptoms:**
- Prometheus metrics show version=1 after deploying dual secrets
- `obot_auth_cookie_secret_version` not changing

**Root Cause:**
- Provider pods not restarted after configuration change
- Configuration not properly loaded

**Resolution:**
```bash
# Force restart provider pods
kubectl delete pods -n auth-providers -l app=entra-auth-provider

# Verify new pods picked up configuration
kubectl logs -n auth-providers deployment/entra-auth-provider --tail=100 | grep "secret rotation"
```

---

## Rollback Procedure

If issues arise during rotation, follow this rollback:

### Emergency Rollback (Any Phase)

**Step 1: Restore old secret as current**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: entra-auth-provider-secrets
  namespace: auth-providers
type: Opaque
stringData:
  # Restore old secret as current
  OBOT_ENTRA_AUTH_PROVIDER_COOKIE_SECRET: "<OLD_SECRET>"

  # Remove previous secrets
  # OBOT_ENTRA_AUTH_PROVIDER_PREVIOUS_COOKIE_SECRETS: (removed)
```

**Step 2: Restart provider**
```bash
kubectl apply -f rollback-config.yaml
kubectl rollout restart deployment/entra-auth-provider -n auth-providers
```

**Step 3: Verify rollback**
```bash
# All existing sessions should work immediately
# Check logs for confirmation
kubectl logs -n auth-providers deployment/entra-auth-provider --tail=50
```

---

## Best Practices

### Security
- ✅ Store secrets in Kubernetes Secrets (not ConfigMaps)
- ✅ Use secure secret generation (`openssl rand -base64 32`)
- ✅ Never commit secrets to version control
- ✅ Rotate every 90 days (security policy dependent)
- ✅ Maintain audit log of all rotations

### Operational
- ✅ Perform rotations during low-traffic windows
- ✅ Monitor metrics throughout grace period
- ✅ Test with non-production users first
- ✅ Have rollback plan ready
- ✅ Document all rotation events

### Compliance
- ✅ Follow organization's secret rotation policy
- ✅ Maintain rotation audit trail
- ✅ Alert on overdue rotations
- ✅ Automate rotation where possible

---

## Automation

See `scripts/rotate-cookie-secret.sh` for full automation of Phases 1-2.
See `scripts/check-rotation-status.sh` for grace period monitoring.
See `scripts/complete-rotation.sh` for Phase 5 automation.

---

*Last Updated: 2026-01-13*
*Maintained By: Platform Engineering Team*
*Review Frequency: Quarterly*
