package obothelmvalues

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMaskedValuesFromSecret(t *testing.T) {
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
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	values, err := MaskedValuesFromSecret(context.Background(), fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), "obot", "obot-obot")
	if err != nil {
		t.Fatalf("MaskedValuesFromSecret() error = %v", err)
	}
	if values["replicaCount"] != float64(2) && values["replicaCount"] != int64(2) {
		t.Fatalf("replicaCount = %v, want 2", values["replicaCount"])
	}
}

func TestMaskedValuesFromSecretMasksSensitiveValues(t *testing.T) {
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
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	values, err := MaskedValuesFromSecret(context.Background(), fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), "obot", "obot-obot")
	if err != nil {
		t.Fatalf("MaskedValuesFromSecret() error = %v", err)
	}

	config, ok := values["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want map", values["config"])
	}
	if config["OPENAI_API_KEY"] != MaskedValue {
		t.Fatalf("OPENAI_API_KEY = %v, want masked value", config["OPENAI_API_KEY"])
	}
}

func TestMaskedValuesFromSecretUnavailableWithoutSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	values, err := MaskedValuesFromSecret(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build(), "obot", "obot-obot")
	if err != nil {
		t.Fatalf("MaskedValuesFromSecret() error = %v", err)
	}
	if values != nil {
		t.Fatalf("values = %#v, want nil without secret", values)
	}
}

func TestMaskedValuesFromSecretEmptyYAML(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-obot-helm-values",
			Namespace: "obot",
		},
		Data: map[string][]byte{
			secretKeyValuesYAML: []byte("   \n"),
		},
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	values, err := MaskedValuesFromSecret(context.Background(), fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(), "obot", "obot-obot")
	if err != nil {
		t.Fatalf("MaskedValuesFromSecret() error = %v", err)
	}
	if values != nil {
		t.Fatalf("values = %#v, want nil for empty snapshot", values)
	}
}

func TestSecretName(t *testing.T) {
	if got := SecretName("obot-obot"); got != "obot-obot-helm-values" {
		t.Fatalf("SecretName() = %q, want obot-obot-helm-values", got)
	}
}
