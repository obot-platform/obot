package controller

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceAccountRotationInterval = 10 * time.Hour
	serviceAccountOverlapWindow    = time.Hour
	serviceAccountRotationPeriod   = time.Minute
	serviceAccountKeyIDAnnotation  = "obot.obot.ai/key-id"
)

var errRuntimeK8sConfigUnavailable = errors.New("runtime Kubernetes config is not configured")

func (c *Controller) runServiceAccountKeyRotation(ctx context.Context) {
	if err := c.reconcileServiceAccountKeys(ctx); err != nil {
		log.Errorf("failed to reconcile service account keys: %v", err)
	}

	ticker := time.NewTicker(serviceAccountRotationPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.reconcileServiceAccountKeys(ctx); err != nil {
				log.Errorf("failed to reconcile service account keys: %v", err)
			}
		}
	}
}

func (c *Controller) reconcileServiceAccountKeys(ctx context.Context) error {
	var errs []error
	for _, account := range serviceaccounts.All() {
		if !serviceaccounts.Enabled(account, c.services.MCPRuntimeBackend, c.services.MCPNetworkPolicyEnabled) {
			if err := c.cleanupServiceAccountKey(ctx, account); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := c.reconcileServiceAccountKey(ctx, account); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// reconcileServiceAccountSecretChange restores the managed provider token secret
// after manual changes or deletion, while preserving disabled-state cleanup.
func (c *Controller) reconcileServiceAccountSecretChange(req router.Request, _ router.Response) error {
	account, ok := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	if !ok {
		return nil
	}
	if !serviceaccounts.Enabled(account, c.services.MCPRuntimeBackend, c.services.MCPNetworkPolicyEnabled) {
		return c.cleanupServiceAccountKey(req.Ctx, account)
	}
	return c.reconcileServiceAccountKey(req.Ctx, account)
}

func (c *Controller) cleanupServiceAccountKey(ctx context.Context, account serviceaccounts.Account) error {
	if err := c.services.GatewayClient.DeleteAllServiceAccountAPIKeys(ctx, account.Name); err != nil {
		return fmt.Errorf("failed to delete disabled keys for %s: %w", account.Name, err)
	}

	if account.SecretManaged {
		if err := c.deleteServiceAccountSecret(ctx, account); err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) reconcileServiceAccountKey(ctx context.Context, account serviceaccounts.Account) error {
	now := c.now().UTC()
	keys, err := c.services.GatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		return fmt.Errorf("failed to list keys for %s: %w", account.Name, err)
	}

	latestActive := latestActiveServiceAccountAPIKey(keys, now)
	hasExpiredKey := hasExpiredServiceAccountAPIKey(keys, now)
	beforeRotationOverlap := latestActive != nil && now.Sub(latestActive.CreatedAt) < serviceAccountRotationInterval-serviceAccountOverlapWindow
	if latestActive != nil && !account.SecretManaged && !hasExpiredKey && beforeRotationOverlap {
		return nil
	}

	if err := c.services.GatewayClient.DeleteExpiredServiceAccountAPIKeys(ctx, account.Name, now); err != nil {
		return fmt.Errorf("failed to delete expired keys for %s: %w", account.Name, err)
	}

	secretToken, existingSecret, err := c.getServiceAccountSecretToken(ctx, account)
	if err != nil {
		return err
	}

	secretCurrent := false
	if secretToken != "" && latestActive != nil && existingSecret != nil && existingSecret.Annotations[serviceAccountKeyIDAnnotation] == strconv.FormatUint(uint64(latestActive.ID), 10) {
		secretCurrent = true
	} else if secretToken != "" {
		existingKey, validateErr := c.services.GatewayClient.ValidateStorageServiceAccountToken(ctx, secretToken)
		if validateErr == nil && existingKey.ServiceAccountName == account.Name && latestActive != nil && existingKey.ID == latestActive.ID {
			secretCurrent = true
		}
	}

	if latestActive != nil && account.SecretManaged && !hasExpiredKey && beforeRotationOverlap && secretCurrent {
		return nil
	}

	needsRotation := latestActive == nil || (account.SecretManaged && !secretCurrent) || now.Sub(latestActive.CreatedAt) >= serviceAccountRotationInterval
	if !needsRotation {
		return nil
	}

	newKey, err := c.services.GatewayClient.CreateServiceAccountAPIKey(ctx, account.Name, now)
	if err != nil {
		return fmt.Errorf("failed to create key for %s: %w", account.Name, err)
	}

	if account.SecretManaged {
		if err := c.writeServiceAccountSecret(ctx, account, existingSecret, newKey.PlaintextToken(), newKey.ID, newKey.CreatedAt, newKey.CreatedAt.Add(serviceAccountRotationInterval)); err != nil {
			if deleteErr := c.services.GatewayClient.DeleteServiceAccountAPIKeyByID(ctx, newKey.ID); deleteErr != nil {
				return errors.Join(err, fmt.Errorf("failed to roll back new key %d: %w", newKey.ID, deleteErr))
			}
			return err
		}
	}

	if err := c.services.GatewayClient.RetireOtherServiceAccountAPIKeys(ctx, account.Name, newKey.ID, now.Add(serviceAccountOverlapWindow)); err != nil {
		return fmt.Errorf("failed to retire older keys for %s: %w", account.Name, err)
	}

	return nil
}

func latestActiveServiceAccountAPIKey(keys []types.ServiceAccountAPIKey, now time.Time) *types.ServiceAccountAPIKey {
	var latestActive *types.ServiceAccountAPIKey
	for i := range keys {
		key := &keys[i]
		if key.ValidAfter.After(now) {
			continue
		}
		if key.RetireAfter != nil && !key.RetireAfter.After(now) {
			continue
		}
		if latestActive == nil || key.CreatedAt.After(latestActive.CreatedAt) {
			latestActive = key
		}
	}
	return latestActive
}

func hasExpiredServiceAccountAPIKey(keys []types.ServiceAccountAPIKey, now time.Time) bool {
	for _, key := range keys {
		if key.RetireAfter != nil && !key.RetireAfter.After(now) {
			return true
		}
	}
	return false
}

func (c *Controller) getServiceAccountSecretToken(ctx context.Context, account serviceaccounts.Account) (string, *corev1.Secret, error) {
	if !account.SecretManaged {
		return "", nil, nil
	}

	runtimeClient, err := c.runtimeK8sClient()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build runtime client: %w", err)
	}

	ns, err := c.runtimeNamespace()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get runtime namespace: %w", err)
	}

	secret := &corev1.Secret{}
	if err := runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: ns,
		Name:      account.SecretName,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", secret, nil
		}
		return "", nil, fmt.Errorf("failed to read secret %s/%s: %w", ns, account.SecretName, err)
	}

	return string(secret.Data[account.SecretKey]), secret, nil
}

func (c *Controller) writeServiceAccountSecret(ctx context.Context, account serviceaccounts.Account, existing *corev1.Secret, token string, keyID uint, rotatedAt, expiresAt time.Time) error {
	runtimeClient, err := c.runtimeK8sClient()
	if err != nil {
		return fmt.Errorf("failed to build runtime client: %w", err)
	}

	create := existing == nil || existing.Name == ""
	secret := existing
	if secret == nil {
		secret = &corev1.Secret{}
	}

	ns, err := c.runtimeNamespace()
	if err != nil {
		return fmt.Errorf("failed to get runtime namespace: %w", err)
	}

	secret.Name = account.SecretName
	secret.Namespace = ns
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels["app.kubernetes.io/name"] = "obot"
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[serviceAccountKeyIDAnnotation] = strconv.FormatUint(uint64(keyID), 10)
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		account.SecretKey:                     []byte(token),
		serviceaccounts.ServiceAccountNameKey: []byte(account.Name),
		serviceaccounts.RotatedAtKey:          []byte(rotatedAt.UTC().Format(time.RFC3339)),
		serviceaccounts.ExpiresAtKey:          []byte(expiresAt.UTC().Format(time.RFC3339)),
	}

	if create {
		return runtimeClient.Create(ctx, secret)
	}
	return runtimeClient.Update(ctx, secret)
}

func (c *Controller) deleteServiceAccountSecret(ctx context.Context, account serviceaccounts.Account) error {
	runtimeClient, err := c.runtimeK8sClient()
	if errors.Is(err, errRuntimeK8sConfigUnavailable) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to build runtime client: %w", err)
	}

	ns, err := c.runtimeNamespace()
	if err != nil {
		return fmt.Errorf("failed to get runtime namespace: %w", err)
	}

	secret := &corev1.Secret{}
	if err := runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: ns,
		Name:      account.SecretName,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to read secret %s/%s for deletion: %w", ns, account.SecretName, err)
	}

	if err := runtimeClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s/%s: %w", ns, account.SecretName, err)
	}
	return nil
}

func (c *Controller) runtimeNamespace() (string, error) {
	if c.services.ServiceNamespace != "" {
		return c.services.ServiceNamespace, nil
	}
	return "", errors.New("could not determine runtime namespace: service namespace not configured")
}

func (c *Controller) runtimeK8sClient() (kclient.Client, error) {
	if c.runtimeClient == nil {
		return nil, errRuntimeK8sConfigUnavailable
	}
	return c.runtimeClient, nil
}
