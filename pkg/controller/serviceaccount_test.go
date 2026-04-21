package controller

import (
	"context"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/services"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway db: %v", err)
	}

	return gatewayclient.New(context.Background(), db, nil, nil, nil, nil, time.Minute, 10, 90)
}

func newRuntimeSecretClient() kclient.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		Build()
}

func TestReconcileServiceAccountKeyBootstrapsSecret(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	base := time.Now().UTC().Add(-time.Hour)
	controller := &Controller{
		services: &services.Services{
			GatewayClient:     gatewayClient,
			MCPRuntimeBackend: "kubernetes",
		},
		runtimeClient: newRuntimeSecretClient(),
		now:           func() time.Time { return base },
	}

	account, ok := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	if !ok {
		t.Fatal("expected hardcoded service account to exist")
	}

	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	keys, err := gatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		t.Fatalf("failed to list service account keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	secret := &corev1.Secret{}
	if err := controller.runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: controller.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err != nil {
		t.Fatalf("failed to read service account secret: %v", err)
	}

	if string(secret.Data[serviceaccounts.ServiceAccountNameKey]) != account.Name {
		t.Fatalf("expected secret serviceAccountName=%q, got %q", account.Name, secret.Data[serviceaccounts.ServiceAccountNameKey])
	}
	if _, err := gatewayClient.ValidateStorageServiceAccountToken(ctx, string(secret.Data[serviceaccounts.NetworkPolicySecretKey])); err != nil {
		t.Fatalf("expected secret token to validate, got %v", err)
	}
}

func TestReconcileServiceAccountKeyRotatesWithOverlap(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	base := time.Now().UTC().Add(-11 * time.Hour)
	controller := &Controller{
		services: &services.Services{
			GatewayClient:     gatewayClient,
			MCPRuntimeBackend: "kubernetes",
		},
		runtimeClient: newRuntimeSecretClient(),
		now:           func() time.Time { return base },
	}

	account, _ := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := controller.runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: controller.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err != nil {
		t.Fatalf("failed to read service account secret: %v", err)
	}
	oldToken := string(secret.Data[serviceaccounts.NetworkPolicySecretKey])

	controller.now = func() time.Time { return base.Add(11 * time.Hour) }
	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error on rotation: %v", err)
	}

	if err := controller.runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: controller.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err != nil {
		t.Fatalf("failed to read rotated service account secret: %v", err)
	}
	newToken := string(secret.Data[serviceaccounts.NetworkPolicySecretKey])
	if newToken == oldToken {
		t.Fatal("expected rotation to write a new token to the secret")
	}

	keys, err := gatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		t.Fatalf("failed to list service account keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 overlapping keys after rotation, got %d", len(keys))
	}

	if _, err := gatewayClient.ValidateStorageServiceAccountToken(ctx, oldToken); err != nil {
		t.Fatalf("expected previous token to remain valid during overlap, got %v", err)
	}
	if _, err := gatewayClient.ValidateStorageServiceAccountToken(ctx, newToken); err != nil {
		t.Fatalf("expected new token to validate, got %v", err)
	}

	var retiredCount int
	for _, key := range keys {
		if key.RetireAfter != nil {
			retiredCount++
		}
	}
	if retiredCount != 1 {
		t.Fatalf("expected 1 retired overlapping key, got %d", retiredCount)
	}
}

func TestReconcileServiceAccountKeyDeletesExpiredOverlapKeys(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	base := time.Now().UTC().Add(-11 * time.Hour)
	controller := &Controller{
		services: &services.Services{
			GatewayClient:     gatewayClient,
			MCPRuntimeBackend: "kubernetes",
		},
		runtimeClient: newRuntimeSecretClient(),
		now:           func() time.Time { return base },
	}

	account, _ := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	controller.now = func() time.Time { return base.Add(11 * time.Hour) }
	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error on rotation: %v", err)
	}

	controller.now = func() time.Time { return base.Add(12*time.Hour + time.Minute) }
	if err := controller.reconcileServiceAccountKey(ctx, account); err != nil {
		t.Fatalf("unexpected reconcile error on cleanup: %v", err)
	}

	keys, err := gatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		t.Fatalf("failed to list service account keys after cleanup: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected expired overlap keys to be removed, got %d keys", len(keys))
	}
}

func TestReconcileServiceAccountKeysSkipsWhenBackendNotKubernetes(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	base := time.Now().UTC()
	controller := &Controller{
		services: &services.Services{
			GatewayClient:     gatewayClient,
			MCPRuntimeBackend: "docker",
		},
		runtimeClient: newRuntimeSecretClient(),
		now:           func() time.Time { return base },
	}

	account, _ := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	if err := controller.reconcileServiceAccountKeys(ctx); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	keys, err := gatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		t.Fatalf("failed to list service account keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected no keys to be created for non-kubernetes backend, got %d", len(keys))
	}

	secret := &corev1.Secret{}
	if err := controller.runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: controller.runtimeNamespace(),
		Name:      account.SecretName,
	}, secret); err == nil {
		t.Fatal("expected no secret to be created for non-kubernetes backend")
	}
}

func TestReconcileServiceAccountKeysCleansUpWhenBackendDisabled(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	base := time.Now().UTC()
	account, _ := serviceaccounts.Get(serviceaccounts.NetworkPolicyProvider)
	runtimeClient := newRuntimeSecretClient()

	created, err := gatewayClient.CreateServiceAccountAPIKey(ctx, account.Name, base)
	if err != nil {
		t.Fatalf("failed to create service account key: %v", err)
	}

	secret := &corev1.Secret{}
	secret.Name = account.SecretName
	secret.Namespace = system.DefaultNamespace
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		serviceaccounts.NetworkPolicySecretKey: []byte(created.Token),
	}
	if err := runtimeClient.Create(ctx, secret); err != nil {
		t.Fatalf("failed to create secret fixture: %v", err)
	}

	controller := &Controller{
		services: &services.Services{
			GatewayClient:     gatewayClient,
			MCPRuntimeBackend: "docker",
		},
		runtimeClient: runtimeClient,
		now:           func() time.Time { return base },
	}

	if err := controller.reconcileServiceAccountKeys(ctx); err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}

	keys, err := gatewayClient.ListServiceAccountAPIKeys(ctx, account.Name)
	if err != nil {
		t.Fatalf("failed to list service account keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected disabled backend cleanup to remove keys, got %d", len(keys))
	}

	if err := runtimeClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      account.SecretName,
	}, &corev1.Secret{}); err == nil {
		t.Fatal("expected disabled backend cleanup to remove secret")
	}
}
