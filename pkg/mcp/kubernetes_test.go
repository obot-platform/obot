package mcp

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReplaceHostWithServiceFQDN(t *testing.T) {
	tests := []struct {
		name        string
		serviceFQDN string
		inputURL    string
		expectedURL string
	}{
		{
			name:        "replace localhost with service FQDN",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "http://localhost:8080/oauth/token",
			expectedURL: "http://obot.obot-system.svc.cluster.local/oauth/token",
		},
		{
			name:        "replace external domain with service FQDN",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "https://obot.example.com/oauth/token",
			expectedURL: "http://obot.obot-system.svc.cluster.local/oauth/token",
		},
		{
			name:        "preserve path with multiple segments",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "http://localhost:8080/api/v1/oauth/token",
			expectedURL: "http://obot.obot-system.svc.cluster.local/api/v1/oauth/token",
		},
		{
			name:        "handle URL with no path",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "http://localhost:8080",
			expectedURL: "http://obot.obot-system.svc.cluster.local",
		},
		{
			name:        "handle URL with query string",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "http://localhost:8080/oauth/token?foo=bar",
			expectedURL: "http://obot.obot-system.svc.cluster.local/oauth/token?foo=bar",
		},
		{
			name:        "empty service FQDN returns original URL",
			serviceFQDN: "",
			inputURL:    "http://localhost:8080/oauth/token",
			expectedURL: "http://localhost:8080/oauth/token",
		},
		{
			name:        "empty URL returns empty string",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "",
			expectedURL: "",
		},
		{
			name:        "malformed URL without scheme returns original",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "localhost:8080/oauth/token",
			expectedURL: "localhost:8080/oauth/token",
		},
		{
			name:        "custom cluster domain",
			serviceFQDN: "obot.obot-system.svc.custom.domain",
			inputURL:    "http://localhost:8080/oauth/token",
			expectedURL: "http://obot.obot-system.svc.custom.domain/oauth/token",
		},
		{
			name:        "handle root path",
			serviceFQDN: "obot.obot-system.svc.cluster.local",
			inputURL:    "http://localhost:8080/",
			expectedURL: "http://obot.obot-system.svc.cluster.local/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &kubernetesBackend{
				serviceFQDN: tt.serviceFQDN,
			}
			result := k.transformObotHostname(tt.inputURL)
			if result != tt.expectedURL {
				t.Errorf("replaceHostWithServiceFQDN() = %v, want %v", result, tt.expectedURL)
			}
		})
	}
}

func TestNewKubernetesBackend_ServiceFQDN(t *testing.T) {
	tests := []struct {
		name             string
		serviceName      string
		serviceNamespace string
		clusterDomain    string
		expectedFQDN     string
	}{
		{
			name:             "constructs FQDN with all values",
			serviceName:      "obot",
			serviceNamespace: "obot-system",
			clusterDomain:    "cluster.local",
			expectedFQDN:     "obot.obot-system.svc.cluster.local",
		},
		{
			name:             "custom cluster domain",
			serviceName:      "obot",
			serviceNamespace: "default",
			clusterDomain:    "my-cluster.local",
			expectedFQDN:     "obot.default.svc.my-cluster.local",
		},
		{
			name:             "empty service name results in empty FQDN",
			serviceName:      "",
			serviceNamespace: "obot-system",
			clusterDomain:    "cluster.local",
			expectedFQDN:     "",
		},
		{
			name:             "empty service namespace results in empty FQDN",
			serviceName:      "obot",
			serviceNamespace: "",
			clusterDomain:    "cluster.local",
			expectedFQDN:     "",
		},
		{
			name:             "both empty results in empty FQDN",
			serviceName:      "",
			serviceNamespace: "",
			clusterDomain:    "cluster.local",
			expectedFQDN:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newKubernetesBackend(nil, nil, nil, Options{ServiceName: tt.serviceName, ServiceNamespace: tt.serviceNamespace, MCPClusterDomain: tt.clusterDomain})
			k := backend.(*kubernetesBackend)
			if k.serviceFQDN != tt.expectedFQDN {
				t.Errorf("newKubernetesBackend() serviceFQDN = %v, want %v", k.serviceFQDN, tt.expectedFQDN)
			}
		})
	}
}

func TestGetDeploymentMCPContainerImage(t *testing.T) {
	tests := []struct {
		name       string
		containers []corev1.Container
		expected   string
	}{
		{
			name: "prefers mcp container",
			containers: []corev1.Container{
				{Name: "shim", Image: "example.com/shim:v1"},
				{Name: "mcp", Image: "example.com/mcp:v2"},
			},
			expected: "example.com/mcp:v2",
		},
		{
			name: "falls back to first container",
			containers: []corev1.Container{
				{Name: "shim", Image: "example.com/shim:v1"},
			},
			expected: "example.com/shim:v1",
		},
		{
			name:       "returns empty for no containers",
			containers: nil,
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{Containers: tt.containers},
					},
				},
			}

			gotName, gotImage := getDeploymentMCPContainerImage(deployment)
			if gotImage != tt.expected {
				t.Fatalf("getDeploymentMCPContainerImage() image = %q, want %q", gotImage, tt.expected)
			}
			if len(tt.containers) > 0 && gotName == "" {
				t.Fatal("expected container name to be returned")
			}
		})
	}
}

func TestPatchDeploymentMCPContainerImage(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "server-1", Namespace: "obot"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mcp", Image: "example.com/old:v1"}}},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
	backend := &kubernetesBackend{client: client}

	if err := backend.patchDeploymentMCPContainerImage(t.Context(), deployment, "mcp", "example.com/new:v2"); err != nil {
		t.Fatalf("patchDeploymentMCPContainerImage() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(deployment), &updated); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	_, got := getDeploymentMCPContainerImage(&updated)
	if got != "example.com/new:v2" {
		t.Fatalf("patched image = %q, want %q", got, "example.com/new:v2")
	}

	if updated.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("expected restartedAt annotation to be set on pod template")
	}
}

func TestPatchDeploymentMCPContainerImageFallsBackToFirstContainerName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "server-2", Namespace: "obot"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "primary", Image: "example.com/old:v1"}}},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
	backend := &kubernetesBackend{client: client}

	containerName, _ := getDeploymentMCPContainerImage(deployment)
	if err := backend.patchDeploymentMCPContainerImage(t.Context(), deployment, containerName, "example.com/new:v2"); err != nil {
		t.Fatalf("patchDeploymentMCPContainerImage() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(deployment), &updated); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	if len(updated.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected exactly 1 container, got %d", len(updated.Spec.Template.Spec.Containers))
	}
	if updated.Spec.Template.Spec.Containers[0].Name != "primary" {
		t.Fatalf("patched container name = %q, want %q", updated.Spec.Template.Spec.Containers[0].Name, "primary")
	}
	if updated.Spec.Template.Spec.Containers[0].Image != "example.com/new:v2" {
		t.Fatalf("patched image = %q, want %q", updated.Spec.Template.Spec.Containers[0].Image, "example.com/new:v2")
	}
}

func TestRestartServerPatchesImageWhenDifferent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "server-1", Namespace: "obot"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mcp", Image: "example.com/old:v1"}}},
			},
		},
	}
	k8sSettings := &v1.K8sSettings{
		ObjectMeta: metav1.ObjectMeta{Name: system.K8sSettingsName, Namespace: system.DefaultNamespace},
		Spec: v1.K8sSettingsSpec{
			PodSecurityAdmission: &v1.PodSecurityAdmissionSettings{Enabled: true, Enforce: "privileged"},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, k8sSettings).Build()
	backend := &kubernetesBackend{client: client, obotClient: client, mcpNamespace: "obot"}

	err := backend.restartServer(t.Context(), ServerConfig{MCPServerName: "server-1", ContainerImage: "example.com/new:v2"})
	if err != nil {
		t.Fatalf("restartServer() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(deployment), &updated); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}

	_, got := getDeploymentMCPContainerImage(&updated)
	if got != "example.com/new:v2" {
		t.Fatalf("deployment image after restart = %q, want %q", got, "example.com/new:v2")
	}

	if updated.Annotations["obot.ai/k8s-settings-hash"] == "" {
		t.Fatal("expected K8s settings hash annotation to be patched onto deployment")
	}
}
