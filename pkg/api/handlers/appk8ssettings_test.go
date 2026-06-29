package handlers

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildAppK8sSettingsFromHelmValuesSecret(t *testing.T) {
	valuesYAML := `
runtimeClassName: gvisor
resources:
  requests:
    cpu: 500m
    memory: 512Mi
tolerations:
  - key: dedicated
    operator: Equal
    value: obot
    effect: NoSchedule
`

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obot-config",
			Namespace: "obot",
		},
		Data: map[string][]byte{
			"OBOT_APP_K8S_SETTINGS_YAML": []byte(valuesYAML),
		},
	}

	handler := NewAppK8sSettingsHandler(
		"obot-config",
		"obot",
		fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	)

	settings, err := handler.buildAppK8sSettings(context.Background())
	if err != nil {
		t.Fatalf("buildAppK8sSettings() error = %v", err)
	}
	if !settings.Available {
		t.Fatal("expected available settings")
	}
	if settings.RuntimeClassName != "gvisor" {
		t.Fatalf("runtimeClassName = %q, want gvisor", settings.RuntimeClassName)
	}
	if settings.Tolerations == "" || !strings.Contains(settings.Tolerations, "dedicated") {
		t.Fatalf("tolerations = %q, want toleration YAML", settings.Tolerations)
	}
}

func TestBuildAppK8sSettingsUnavailableWithoutSecret(t *testing.T) {
	handler := NewAppK8sSettingsHandler("obot-config", "obot", nil)

	settings, err := handler.buildAppK8sSettings(context.Background())
	if err != nil {
		t.Fatalf("buildAppK8sSettings() error = %v", err)
	}
	if settings.Available {
		t.Fatal("expected unavailable settings without k8s client")
	}
}
