package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceAccountRotationInterval = 10 * time.Hour
	serviceAccountOverlapWindow    = time.Hour
	serviceAccountRotationPeriod   = time.Minute
)

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
	for _, account := range serviceaccounts.All() {
		if !serviceaccounts.Enabled(account, c.services.MCPRuntimeBackend) {
			if err := c.cleanupServiceAccountKey(ctx, account); err != nil {
				return err
			}
			continue
		}
		if err := c.reconcileServiceAccountKey(ctx, account); err != nil {
			return err
		}
	}
	return nil
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
	if err := c.services.GatewayClient.DeleteExpiredServiceAccountAPIKeys(ctx, account.Name, now); err != nil {
		return fmt.Errorf("failed to delete expired keys for %s: %w", account.Name, err)
	}

	keys, err := c.services.GatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		return fmt.Errorf("failed to list keys for %s: %w", account.Name, err)
	}

	var latestActive *types.ServiceAccountAPIKey
	for i := range keys {
		key := &keys[i]
		if key.ValidAfter.After(now) {
			continue
		}
		if key.RetireAfter != nil && key.RetireAfter.Before(now) {
			continue
		}
		if latestActive == nil || key.CreatedAt.After(latestActive.CreatedAt) {
			latestActive = key
		}
	}

	secretToken, secretKey, err := c.getServiceAccountSecretToken(ctx, account)
	if err != nil {
		return err
	}

	secretCurrent := false
	if secretToken != "" {
		existingKey, validateErr := c.services.GatewayClient.ValidateStorageServiceAccountToken(ctx, secretToken)
		if validateErr == nil && existingKey.ServiceAccountName == account.Name && latestActive != nil && existingKey.ID == latestActive.ID {
			secretCurrent = true
		}
	}

	needsRotation := latestActive == nil || !secretCurrent || now.Sub(latestActive.CreatedAt) >= serviceAccountRotationInterval
	if !needsRotation {
		return nil
	}

	newKey, err := c.services.GatewayClient.CreateServiceAccountAPIKey(ctx, account.Name, now)
	if err != nil {
		return fmt.Errorf("failed to create key for %s: %w", account.Name, err)
	}

	if account.SecretManaged {
		if err := c.writeServiceAccountSecret(ctx, account, secretKey, newKey.Token, newKey.CreatedAt, newKey.CreatedAt.Add(serviceAccountRotationInterval)); err != nil {
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

func (c *Controller) getServiceAccountSecretToken(ctx context.Context, account serviceaccounts.Account) (string, *corev1.Secret, error) {
	if !account.SecretManaged {
		return "", nil, nil
	}

	runtimeClient, err := c.runtimeK8sClient(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build runtime client: %w", err)
	}

	secret := &corev1.Secret{}
	if err := runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: c.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", secret, nil
		}
		return "", nil, fmt.Errorf("failed to read secret %s/%s: %w", c.runtimeNamespace(), account.SecretName, err)
	}

	return string(secret.Data[serviceaccounts.NetworkPolicySecretKey]), secret, nil
}

func (c *Controller) writeServiceAccountSecret(ctx context.Context, account serviceaccounts.Account, existing *corev1.Secret, token string, rotatedAt, expiresAt time.Time) error {
	runtimeClient, err := c.runtimeK8sClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to build runtime client: %w", err)
	}

	create := existing == nil || existing.Name == ""
	secret := existing
	if secret == nil {
		secret = &corev1.Secret{}
	}

	secret.ObjectMeta.Name = account.SecretName
	secret.ObjectMeta.Namespace = c.runtimeNamespace()
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels["app.kubernetes.io/name"] = "obot"
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		serviceaccounts.NetworkPolicySecretKey: []byte(token),
		serviceaccounts.ServiceAccountNameKey:  []byte(account.Name),
		serviceaccounts.RotatedAtKey:           []byte(rotatedAt.UTC().Format(time.RFC3339)),
		serviceaccounts.ExpiresAtKey:           []byte(expiresAt.UTC().Format(time.RFC3339)),
	}

	if create {
		return runtimeClient.Create(ctx, secret)
	}
	return runtimeClient.Update(ctx, secret)
}

func (c *Controller) deleteServiceAccountSecret(ctx context.Context, account serviceaccounts.Account) error {
	runtimeClient, err := c.runtimeK8sClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to build runtime client: %w", err)
	}

	secret := &corev1.Secret{}
	if err := runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: c.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to read secret %s/%s for deletion: %w", c.runtimeNamespace(), account.SecretName, err)
	}

	if err := runtimeClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s/%s: %w", c.runtimeNamespace(), account.SecretName, err)
	}
	return nil
}

func (c *Controller) runtimeNamespace() string {
	if namespace := os.Getenv("POD_NAMESPACE"); namespace != "" {
		return namespace
	}
	return system.DefaultNamespace
}

func (c *Controller) runtimeK8sClient(ctx context.Context) (kclient.Client, error) {
	if c.runtimeClient != nil {
		return c.runtimeClient, nil
	}

	cfg, err := services.BuildLocalK8sConfig()
	if err != nil {
		return nil, err
	}

	client, err := kclient.New(cfg, kclient.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return nil, err
	}

	c.runtimeClient = client
	return client, nil
}
