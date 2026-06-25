package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/obothelmvalues"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildObotK8sSettingsFromStoredHelmValues(t *testing.T) {
	valuesYAML := `
replicaCount: 2
updateStrategy: RollingUpdate
runtimeClassName: gvisor
dev:
  useEmbeddedDb: false
image:
  repository: ghcr.io/obot-platform/obot
  tag: main
  pullPolicy: IfNotPresent
config:
  existingSecret: custom-secret
  OBOT_SERVER_ENABLE_AUTHENTICATION: true
  OPENAI_API_KEY: secret-key
service:
  type: ClusterIP
  port: 80
  annotations:
    example.com/setting: enabled
mcpNamespace:
  name: custom-mcp
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
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
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	helmValues := &v1.ObotHelmValues{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.ObotHelmValuesName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ObotHelmValuesSpec{
			ValuesYAML: mustMaskValuesYAML(t, valuesYAML),
		},
	}

	handler := &K8sSettingsHandler{
		mcpRuntimeBackend: "kubernetes",
	}

	settings, err := handler.buildObotK8sSettings(context.Background(), fake.NewClientBuilder().WithScheme(scheme).WithObjects(helmValues).Build())
	if err != nil {
		t.Fatalf("buildObotK8sSettings() error = %v", err)
	}
	if !settings.Available {
		t.Fatal("expected available settings")
	}
	if settings.ReplicaCount == nil || *settings.ReplicaCount != 2 {
		t.Fatalf("replicaCount = %v, want 2", settings.ReplicaCount)
	}
	if settings.RuntimeClassName != "gvisor" {
		t.Fatalf("runtimeClassName = %q, want gvisor", settings.RuntimeClassName)
	}
	if settings.Image == "" || !strings.Contains(settings.Image, "ghcr.io/obot-platform/obot") {
		t.Fatalf("image = %q, want repository in YAML", settings.Image)
	}
	if settings.Config == "" {
		t.Fatal("expected config YAML")
	}
	if strings.Contains(settings.Config, "secret-key") {
		t.Fatalf("config should not contain secret values: %q", settings.Config)
	}
	if !strings.Contains(settings.Config, obothelmvalues.MaskedValue) {
		t.Fatalf("config = %q, want masked config values", settings.Config)
	}
	if settings.MCPNamespace == "" || !strings.Contains(settings.MCPNamespace, "custom-mcp") {
		t.Fatalf("mcpNamespace = %q, want custom namespace", settings.MCPNamespace)
	}
	if settings.Tolerations == "" || !strings.Contains(settings.Tolerations, "dedicated") {
		t.Fatalf("tolerations = %q, want toleration YAML", settings.Tolerations)
	}
}

func TestBuildObotK8sSettingsFromHelmValuesMasksConfig(t *testing.T) {
	settings, err := buildObotK8sSettingsFromHelmValues(obothelmvalues.MaskValues(map[string]any{
		"config": map[string]any{
			"existingSecret":                    "custom-secret",
			"OBOT_SERVER_ENABLE_AUTHENTICATION": false,
			"OPENAI_API_KEY":                    "sk-test",
		},
	}))
	if err != nil {
		t.Fatalf("buildObotK8sSettingsFromHelmValues() error = %v", err)
	}

	if settings.Config == "" {
		t.Fatal("expected config YAML")
	}
	for _, key := range []string{"existingSecret", "OBOT_SERVER_ENABLE_AUTHENTICATION", "OPENAI_API_KEY"} {
		if !strings.Contains(settings.Config, key) {
			t.Fatalf("config = %q, want key %q", settings.Config, key)
		}
	}
	if strings.Contains(settings.Config, "custom-secret") || strings.Contains(settings.Config, "sk-test") {
		t.Fatalf("config should not contain raw values: %q", settings.Config)
	}
}

func TestBuildObotK8sSettingsUnavailableWithoutStoredValues(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	handler := &K8sSettingsHandler{
		mcpRuntimeBackend: "kubernetes",
	}

	settings, err := handler.buildObotK8sSettings(context.Background(), fake.NewClientBuilder().WithScheme(scheme).Build())
	if err != nil {
		t.Fatalf("buildObotK8sSettings() error = %v", err)
	}
	if settings.Available {
		t.Fatal("expected unavailable settings without stored helm values")
	}
}

func TestBuildObotK8sSettingsFromHelmValuesOmitsEmptySections(t *testing.T) {
	settings, err := buildObotK8sSettingsFromHelmValues(map[string]any{
		"replicaCount": 1,
		"affinity":     map[string]any{},
		"resources":    []any{},
	})
	if err != nil {
		t.Fatalf("buildObotK8sSettingsFromHelmValues() error = %v", err)
	}

	if settings.Affinity != "" {
		t.Fatalf("affinity = %q, want empty", settings.Affinity)
	}
	if settings.Resources != "" {
		t.Fatalf("resources = %q, want empty", settings.Resources)
	}
}

func TestBuildObotK8sSettingsNonKubernetesBackend(t *testing.T) {
	handler := &K8sSettingsHandler{
		mcpRuntimeBackend: "docker",
	}

	settings, err := handler.buildObotK8sSettings(context.Background(), fake.NewClientBuilder().Build())
	if err != nil {
		t.Fatalf("buildObotK8sSettings() error = %v", err)
	}
	if settings != (types.ObotK8sSettings{Available: false}) {
		t.Fatalf("settings = %#v, want unavailable", settings)
	}
}

func mustMaskValuesYAML(t *testing.T, valuesYAML string) string {
	t.Helper()
	maskedYAML, err := obothelmvalues.MaskValuesYAML(valuesYAML)
	if err != nil {
		t.Fatalf("MaskValuesYAML() error = %v", err)
	}
	return maskedYAML
}
