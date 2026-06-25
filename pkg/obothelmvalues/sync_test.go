package obothelmvalues

import (
	"context"
	"strings"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncFromSecretCreatesObotHelmValues(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-obot-helm-values",
			Namespace: "obot",
		},
		Data: map[string][]byte{
			secretKeyValuesYAML: []byte("replicaCount: 2\n"),
		},
	}

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	storageClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	localK8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	if err := SyncFromSecret(context.Background(), storageClient, localK8sClient, "obot", "obot-obot"); err != nil {
		t.Fatalf("SyncFromSecret() error = %v", err)
	}

	var helmValues v1.ObotHelmValues
	if err := storageClient.Get(context.Background(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotHelmValuesName,
	}, &helmValues); err != nil {
		t.Fatalf("get obot helm values: %v", err)
	}
	if helmValues.Spec.ValuesYAML != "replicaCount: 2" {
		t.Fatalf("valuesYAML = %q, want stored snapshot", helmValues.Spec.ValuesYAML)
	}
}

func TestSyncFromSecretMasksSensitiveValues(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-obot-helm-values",
			Namespace: "obot",
		},
		Data: map[string][]byte{
			secretKeyValuesYAML: []byte(`config:
  OPENAI_API_KEY: sk-test
replicaCount: 2
`),
		},
	}

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	storageClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	localK8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	if err := SyncFromSecret(context.Background(), storageClient, localK8sClient, "obot", "obot-obot"); err != nil {
		t.Fatalf("SyncFromSecret() error = %v", err)
	}

	var helmValues v1.ObotHelmValues
	if err := storageClient.Get(context.Background(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotHelmValuesName,
	}, &helmValues); err != nil {
		t.Fatalf("get obot helm values: %v", err)
	}
	if strings.Contains(helmValues.Spec.ValuesYAML, "sk-test") {
		t.Fatalf("stored values should be masked: %q", helmValues.Spec.ValuesYAML)
	}
	if !strings.Contains(helmValues.Spec.ValuesYAML, MaskedValue) {
		t.Fatalf("stored values = %q, want masked config", helmValues.Spec.ValuesYAML)
	}
}

func TestSyncFromSecretUpdatesExistingValues(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-obot-helm-values",
			Namespace: "obot",
		},
		Data: map[string][]byte{
			secretKeyValuesYAML: []byte("replicaCount: 3\n"),
		},
	}
	existing := &v1.ObotHelmValues{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ObotHelmValuesName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ObotHelmValuesSpec{
			ValuesYAML: "replicaCount: 2",
		},
	}

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	storageClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	localK8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	if err := SyncFromSecret(context.Background(), storageClient, localK8sClient, "obot", "obot-obot"); err != nil {
		t.Fatalf("SyncFromSecret() error = %v", err)
	}

	var helmValues v1.ObotHelmValues
	if err := storageClient.Get(context.Background(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotHelmValuesName,
	}, &helmValues); err != nil {
		t.Fatalf("get obot helm values: %v", err)
	}
	if helmValues.Spec.ValuesYAML != "replicaCount: 3" {
		t.Fatalf("valuesYAML = %q, want updated snapshot", helmValues.Spec.ValuesYAML)
	}
}
