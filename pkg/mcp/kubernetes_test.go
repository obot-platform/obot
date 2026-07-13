package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestComputeK8sSettingsHashUsesServerSpecificResources(t *testing.T) {
	baseSettings := v1.K8sSettingsSpec{
		RuntimeClassName: ptr.To("runtime-class"),
	}
	resourceSettings := *baseSettings.DeepCopy()
	resourceSettings.Resources = &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	nanobotSettings := *resourceSettings.DeepCopy()
	nanobotSettings.NanobotWorkspaceSize = "10Gi"
	nanobotSettings.NanobotAgentResources = &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	baseHash := ComputeK8sSettingsHash(baseSettings, nil, types.RuntimeNPX, false, ResourceMaximums{}, nil)
	if got := ComputeK8sSettingsHash(resourceSettings, nil, types.RuntimeNPX, false, ResourceMaximums{}, nil); got == baseHash {
		t.Fatalf("regular server hash = %s, want it to differ when default resources are set", got)
	}

	remoteBaseHash := ComputeK8sSettingsHash(baseSettings, nil, types.RuntimeRemote, false, ResourceMaximums{}, nil)
	if got := ComputeK8sSettingsHash(resourceSettings, nil, types.RuntimeRemote, false, ResourceMaximums{}, nil); got != remoteBaseHash {
		t.Fatalf("remote server hash = %s, want %s", got, remoteBaseHash)
	}

	compositeBaseHash := ComputeK8sSettingsHash(baseSettings, nil, types.RuntimeComposite, false, ResourceMaximums{}, nil)
	if got := ComputeK8sSettingsHash(resourceSettings, nil, types.RuntimeComposite, false, ResourceMaximums{}, nil); got != compositeBaseHash {
		t.Fatalf("composite server hash = %s, want %s", got, compositeBaseHash)
	}
	if compositeBaseHash != remoteBaseHash {
		t.Fatalf("composite base hash = %s, want remote base hash %s", compositeBaseHash, remoteBaseHash)
	}

	nanobotBaseHash := ComputeK8sSettingsHash(baseSettings, nil, types.RuntimeNPX, true, ResourceMaximums{}, nil)
	if got := ComputeK8sSettingsHash(resourceSettings, nil, types.RuntimeNPX, true, ResourceMaximums{}, nil); got != nanobotBaseHash {
		t.Fatalf("nanobot agent server hash = %s, want %s before nanobot-only settings are set", got, nanobotBaseHash)
	}
	if got := ComputeK8sSettingsHash(nanobotSettings, nil, types.RuntimeNPX, false, ResourceMaximums{}, nil); got != ComputeK8sSettingsHash(resourceSettings, nil, types.RuntimeNPX, false, ResourceMaximums{}, nil) {
		t.Fatalf("non-nanobot hash = %s, want nanobot-only settings ignored", got)
	}
	if got := ComputeK8sSettingsHash(nanobotSettings, nil, types.RuntimeNPX, true, ResourceMaximums{}, nil); got == nanobotBaseHash {
		t.Fatalf("nanobot hash = %s, want it to differ when nanobot-only settings are set", got)
	}

	serverResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("768Mi"),
		},
	}
	if got := ComputeK8sSettingsHash(baseSettings, serverResources, types.RuntimeNPX, false, ResourceMaximums{}, nil); got == baseHash {
		t.Fatalf("server-specific resources hash = %s, want it to differ from default resource hash", got)
	}
}

func TestComputeK8sSettingsHashUsesImplicitResourceMaximums(t *testing.T) {
	baseHash := ComputeK8sSettingsHash(v1.K8sSettingsSpec{}, nil, types.RuntimeNPX, false, ResourceMaximums{}, nil)
	cappedHash := ComputeK8sSettingsHash(v1.K8sSettingsSpec{}, nil, types.RuntimeNPX, false, ResourceMaximums{
		CPURequest:    new(resource.MustParse("5m")),
		MemoryRequest: new(resource.MustParse("128Mi")),
	}, nil)
	if cappedHash == baseHash {
		t.Fatalf("capped hash = %s, want it to differ from uncapped implicit defaults", cappedHash)
	}
}

func TestMCPContainerResourcesAppliesServerOverridesWithRequestDefaults(t *testing.T) {
	serverResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("1"),
		},
	}
	resources := mcpContainerResources(serverResources, types.RuntimeNPX, false, v1.K8sSettingsSpec{})

	if got, want := resources.Requests[corev1.ResourceMemory], resource.MustParse("512Mi"); got.Cmp(want) != 0 {
		t.Fatalf("memory request = %s, want %s", got.String(), want.String())
	}
	if got, want := resources.Requests[corev1.ResourceCPU], defaultCPURequest; got.Cmp(want) != 0 {
		t.Fatalf("cpu request = %s, want %s", got.String(), want.String())
	}
	if got, want := resources.Limits[corev1.ResourceCPU], resource.MustParse("1"); got.Cmp(want) != 0 {
		t.Fatalf("cpu limit = %s, want %s", got.String(), want.String())
	}
}

func TestEffectiveDefaultMCPResourceRequirements(t *testing.T) {
	t.Run("uses built-in defaults when no K8s resources are configured", func(t *testing.T) {
		defaults := EffectiveDefaultMCPResourceRequirements(v1.K8sSettingsSpec{})

		if got, want := defaults.Requests[corev1.ResourceCPU], defaultCPURequest; got.Cmp(want) != 0 {
			t.Fatalf("cpu request = %s, want %s", got.String(), want.String())
		}
		if got, want := defaults.Requests[corev1.ResourceMemory], defaultMCPMemoryRequest; got.Cmp(want) != 0 {
			t.Fatalf("memory request = %s, want %s", got.String(), want.String())
		}
		if len(defaults.Limits) != 0 {
			t.Fatalf("limits = %#v, want none", defaults.Limits)
		}
	})

	t.Run("uses configured MCP resources and adds missing CPU request", func(t *testing.T) {
		defaults := EffectiveDefaultMCPResourceRequirements(v1.K8sSettingsSpec{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		})

		if got, want := defaults.Requests[corev1.ResourceCPU], defaultCPURequest; got.Cmp(want) != 0 {
			t.Fatalf("cpu request = %s, want %s", got.String(), want.String())
		}
		if got, want := defaults.Requests[corev1.ResourceMemory], resource.MustParse("512Mi"); got.Cmp(want) != 0 {
			t.Fatalf("memory request = %s, want %s", got.String(), want.String())
		}
		if got, want := defaults.Limits[corev1.ResourceMemory], resource.MustParse("1Gi"); got.Cmp(want) != 0 {
			t.Fatalf("memory limit = %s, want %s", got.String(), want.String())
		}
	})
}

func TestMCPContainerResourcesAppliesServerCPURequestWithMemoryDefault(t *testing.T) {
	serverResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("250m"),
		},
	}
	resources := mcpContainerResources(serverResources, types.RuntimeNPX, false, v1.K8sSettingsSpec{})

	if got, want := resources.Requests[corev1.ResourceCPU], resource.MustParse("250m"); got.Cmp(want) != 0 {
		t.Fatalf("cpu request = %s, want %s", got.String(), want.String())
	}
	if got, want := resources.Requests[corev1.ResourceMemory], defaultMCPMemoryRequest; got.Cmp(want) != 0 {
		t.Fatalf("memory request = %s, want %s", got.String(), want.String())
	}
}

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
			backend := newKubernetesBackend(true, nil, nil, nil, nil, Options{ServiceName: tt.serviceName, ServiceNamespace: tt.serviceNamespace, MCPClusterDomain: tt.clusterDomain}, ResourceMaximums{})
			k := backend.(*kubernetesBackend)
			if k.serviceFQDN != tt.expectedFQDN {
				t.Errorf("newKubernetesBackend() serviceFQDN = %v, want %v", k.serviceFQDN, tt.expectedFQDN)
			}
		})
	}
}

func TestK8sObjects_NanobotAgentExcludesAuditLogConfig(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "nanobot-agent-server",
		MCPServerDisplayName: "Nanobot Agent Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/nanobot:latest",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "nanobot",
		Args:                 []string{"run"},
		NanobotAgentName:     "agent-1",
		AuditLogMetadata:     map[string]string{"mcpID": "server-1"},
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	configSecret := findSecret(t, objs, name.SafeConcatName("nanobot-agent-server", "mcp", "config"))
	assertNoAuditLogEnv(t, configSecret.Data)
}

func TestK8sObjects_DoesNotCreateShimContainer(t *testing.T) {
	k := newTestKubernetesBackend(t, &v1.K8sSettings{
		ObjectMeta: metav1.ObjectMeta{Name: system.K8sSettingsName, Namespace: system.DefaultNamespace},
		Spec: v1.K8sSettingsSpec{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		},
	})

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "standard-server",
		MCPServerDisplayName: "Standard Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "server",
		Args:                 []string{"run"},
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	dep := findDeployment(t, objs, "standard-server")
	if len(dep.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("container count = %d, want 1", len(dep.Spec.Template.Spec.Containers))
	}
	if dep.Spec.Template.Spec.Containers[0].Name != "mcp" {
		t.Fatalf("container name = %q, want mcp", dep.Spec.Template.Spec.Containers[0].Name)
	}
}

func TestK8sObjects_RemoteCreatesNoObjects(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:              types.RuntimeRemote,
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}
	if len(objs) != 0 {
		t.Fatalf("object count = %d, want 0", len(objs))
	}
}

func TestK8sObjects_CompositeCreatesNanobotDeployment(t *testing.T) {
	k := newTestKubernetesBackend(t)
	k.serviceFQDN = "obot.obot-system.svc.cluster.local"
	k.authEnabled = true

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:                   types.RuntimeComposite,
		MCPServerName:             "test-server",
		MCPServerDisplayName:      "Test Server",
		UserID:                    "user-1",
		OwnerUserID:               "user-2",
		Issuer:                    "https://obot.example.com",
		Audiences:                 []string{"https://obot.example.com/mcp-connect/test-server"},
		AuthorizeEndpoint:         "https://obot.example.com/oauth/authorize",
		TokenExchangeEndpoint:     "https://obot.example.com/oauth/token",
		JWKSEndpoint:              "https://obot.example.com/oauth/jwks.json",
		TokenExchangeClientID:     "client-id",
		TokenExchangeClientSecret: "client-secret",
		Components: []ComponentServer{
			{Name: "component", URL: "https://example.com/mcp"},
		},
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	dep := findDeployment(t, objs, "test-server")
	if len(dep.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("container count = %d, want 1", len(dep.Spec.Template.Spec.Containers))
	}
	container := dep.Spec.Template.Spec.Containers[0]
	if container.Name != "mcp" {
		t.Fatalf("container name = %q, want mcp", container.Name)
	}
	if container.Image != "ghcr.io/obot-platform/nanobot:main" {
		t.Fatalf("container image = %q, want composite image", container.Image)
	}
	if container.ReadinessProbe == nil {
		t.Fatal("expected composite container readiness probe")
	}

	configSecret := findSecret(t, objs, name.SafeConcatName("test-server", "mcp", "config"))
	if got := string(configSecret.Data["NANOBOT_DISABLE_HEALTH_CHECKER"]); got != "true" {
		t.Fatalf("NANOBOT_DISABLE_HEALTH_CHECKER = %q, want true", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_OAUTH_TOKEN_URL"]); got != "http://obot.obot-system.svc.cluster.local/oauth/token" {
		t.Fatalf("NANOBOT_RUN_OAUTH_TOKEN_URL = %q, want token exchange endpoint", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_OAUTH_AUTHORIZE_URL"]); got != "http://obot.obot-system.svc.cluster.local/oauth/authorize" {
		t.Fatalf("NANOBOT_RUN_OAUTH_AUTHORIZE_URL = %q, want authorize endpoint", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_OAUTH_JWKSURL"]); got != "http://obot.obot-system.svc.cluster.local/oauth/jwks.json" {
		t.Fatalf("NANOBOT_RUN_OAUTH_JWKSURL = %q, want JWKS endpoint", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_OAUTH_CLIENT_ID"]); got != "client-id" {
		t.Fatalf("NANOBOT_RUN_OAUTH_CLIENT_ID = %q, want client-id", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_OAUTH_CLIENT_SECRET"]); got != "client-secret" {
		t.Fatalf("NANOBOT_RUN_OAUTH_CLIENT_SECRET = %q, want client-secret", got)
	}
	if got := string(configSecret.Data["NANOBOT_RUN_TRUSTED_ISSUER"]); got != "https://obot.example.com" {
		t.Fatalf("NANOBOT_RUN_TRUSTED_ISSUER = %q, want issuer", got)
	}
}

func TestK8sObjects_UVXAndNPXPassNanobotHealthEnv(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeUVX, types.RuntimeNPX} {
		t.Run(string(runtime), func(t *testing.T) {
			k := newTestKubernetesBackend(t)

			objs, err := k.k8sObjects(t.Context(), ServerConfig{
				Runtime:              runtime,
				MCPServerName:        "test-server",
				MCPServerDisplayName: "Test Server",
				UserID:               "user-1",
				OwnerUserID:          "user-2",
				Command:              strings.ToLower(string(runtime)),
				Args:                 []string{"example"},
			})
			if err != nil {
				t.Fatalf("k8sObjects() error = %v", err)
			}

			configSecret := findSecret(t, objs, name.SafeConcatName("test-server", "mcp", "config"))
			if got := string(configSecret.Data["NANOBOT_RUN_HEALTHZ_PATH"]); got != "/healthz" {
				t.Fatalf("NANOBOT_RUN_HEALTHZ_PATH = %q, want /healthz", got)
			}
			if got := string(configSecret.Data["NANOBOT_RUN_FORCE_FETCH_TOOL_LIST"]); got != "true" {
				t.Fatalf("NANOBOT_RUN_FORCE_FETCH_TOOL_LIST = %q, want true", got)
			}
		})
	}
}

func TestK8sObjects_ServicePorts(t *testing.T) {
	tests := []struct {
		name                   string
		nanobotAgentName       string
		expectedHTTPPortTarget intstr.IntOrString
		expectedStrategy       appsv1.DeploymentStrategyType
	}{
		{
			name:                   "standard containerized server routes http service port to mcp container",
			expectedHTTPPortTarget: intstr.FromString("mcp"),
		},
		{
			name:                   "nanobot agent routes http service port to mcp container",
			nanobotAgentName:       "agent-1",
			expectedHTTPPortTarget: intstr.FromString("mcp"),
			expectedStrategy:       appsv1.RecreateDeploymentStrategyType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTestKubernetesBackend(t)
			objs, err := k.k8sObjects(t.Context(), ServerConfig{
				Runtime:              types.RuntimeContainerized,
				MCPServerName:        "test-server",
				MCPServerDisplayName: "Test Server",
				UserID:               "user-1",
				OwnerUserID:          "user-2",
				ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
				ContainerPort:        8080,
				ContainerPath:        "/mcp",
				Command:              "server",
				Args:                 []string{"run"},
				NanobotAgentName:     tt.nanobotAgentName,
			})
			if err != nil {
				t.Fatalf("k8sObjects() error = %v", err)
			}

			service := findService(t, objs, "test-server")
			assertServicePort(t, service, "http", 80, tt.expectedHTTPPortTarget)
			assertServicePort(t, service, "mcp", 8080, intstr.FromString("mcp"))

			dep := findDeployment(t, objs, "test-server")
			if dep.Spec.Strategy.Type != tt.expectedStrategy {
				t.Fatalf("deployment strategy = %q, want %q", dep.Spec.Strategy.Type, tt.expectedStrategy)
			}
		})
	}
}

func TestK8sObjects_MCPContainerResources(t *testing.T) {
	tests := []struct {
		name              string
		server            ServerConfig
		settings          *v1.K8sSettings
		resourceMaximums  ResourceMaximums
		wantCPURequest    string
		wantMemoryRequest string
		wantMemoryLimit   string
	}{
		{
			name: "non-agent default requests 200Mi memory",
			server: ServerConfig{
				Runtime: types.RuntimeContainerized,
			},
			wantMemoryRequest: "200Mi",
		},
		{
			name: "non-agent implicit defaults are capped by maximums",
			server: ServerConfig{
				Runtime: types.RuntimeContainerized,
			},
			resourceMaximums: ResourceMaximums{
				CPURequest:    new(resource.MustParse("5m")),
				MemoryRequest: new(resource.MustParse("128Mi")),
			},
			wantCPURequest:    "5m",
			wantMemoryRequest: "128Mi",
		},
		{
			name: "nanobot agent default requests 400Mi memory",
			server: ServerConfig{
				Runtime:          types.RuntimeContainerized,
				NanobotAgentName: "agent-1",
			},
			wantMemoryRequest: "400Mi",
		},
		{
			name: "nanobot agent implicit defaults are capped by maximums",
			server: ServerConfig{
				Runtime:          types.RuntimeContainerized,
				NanobotAgentName: "agent-1",
			},
			resourceMaximums: ResourceMaximums{
				CPURequest:    new(resource.MustParse("5m")),
				MemoryRequest: new(resource.MustParse("256Mi")),
			},
			wantCPURequest:    "5m",
			wantMemoryRequest: "256Mi",
		},
		{
			name: "nanobot agent uses dedicated resources",
			server: ServerConfig{
				Runtime:          types.RuntimeContainerized,
				NanobotAgentName: "agent-1",
			},
			settings: &v1.K8sSettings{
				ObjectMeta: metav1.ObjectMeta{Name: system.K8sSettingsName, Namespace: system.DefaultNamespace},
				Spec: v1.K8sSettingsSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("250Mi")},
					},
					NanobotAgentResources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
					},
				},
			},
			wantMemoryRequest: "512Mi",
			wantMemoryLimit:   "1Gi",
		},
		{
			name: "uvx runtime uses standard MCP resources",
			server: ServerConfig{
				Runtime: types.RuntimeUVX,
			},
			wantMemoryRequest: "200Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTestKubernetesBackend(t)
			k.resourceMaximums = tt.resourceMaximums
			if tt.settings != nil {
				scheme := runtime.NewScheme()
				if err := v1.AddToScheme(scheme); err != nil {
					t.Fatalf("AddToScheme() error = %v", err)
				}
				k.obotClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.settings).Build()
			}

			server := tt.server
			server.MCPServerName = "test-server"
			server.MCPServerDisplayName = "Test Server"
			server.UserID = "user-1"
			server.OwnerUserID = "user-2"
			server.ContainerImage = "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main"
			server.ContainerPort = 8080
			server.ContainerPath = "/mcp"
			server.Command = "server"
			server.Args = []string{"run"}

			objs, err := k.k8sObjects(t.Context(), server)
			if err != nil {
				t.Fatalf("k8sObjects() error = %v", err)
			}

			container := findContainer(t, findDeployment(t, objs, "test-server"), "mcp")
			memoryRequest := container.Resources.Requests[corev1.ResourceMemory]
			if memoryRequest.String() != tt.wantMemoryRequest {
				t.Fatalf("memory request = %q, want %q", memoryRequest.String(), tt.wantMemoryRequest)
			}
			cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
			wantCPURequest := tt.wantCPURequest
			if wantCPURequest == "" {
				wantCPURequest = "10m"
			}
			if cpuRequest.String() != wantCPURequest {
				t.Fatalf("CPU request = %q, want %q", cpuRequest.String(), wantCPURequest)
			}
			memoryLimit, hasMemoryLimit := container.Resources.Limits[corev1.ResourceMemory]
			if tt.wantMemoryLimit == "" && hasMemoryLimit {
				t.Fatalf("unexpected memory limit: %s", memoryLimit.String())
			}
			if tt.wantMemoryLimit != "" && (!hasMemoryLimit || memoryLimit.String() != tt.wantMemoryLimit) {
				t.Fatalf("memory limit = %q, want %q", memoryLimit.String(), tt.wantMemoryLimit)
			}
		})
	}
}

func TestK8sObjectsAllowsSystemServerResourcesAboveMaximum(t *testing.T) {
	k := newTestKubernetesBackend(t)
	k.resourceMaximums = ResourceMaximums{CPURequest: new(resource.MustParse("100m"))}

	server := testK8sServerConfig()
	server.SystemMCPServer = true
	server.Resources = &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("250m"),
		},
	}

	if _, err := k.k8sObjects(t.Context(), server); err != nil {
		t.Fatalf("expected system MCP server resources to bypass maximums: %v", err)
	}
}

func TestAnalyzePodStatus(t *testing.T) {
	tests := []struct {
		name            string
		pod             corev1.Pod
		isAgent         bool
		wantRetryable   bool
		wantErr         error
		wantErrContains string
	}{
		{
			name: "running mcp container remains retryable",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase:             corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{{Name: "mcp"}},
				},
			},
			wantRetryable:   true,
			wantErrContains: "pod in phase Running",
		},
		{
			name: "image pull backoff is retryable image pull",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{{
						Name: "mcp",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
						},
					}},
				},
			},
			wantRetryable:   true,
			wantErr:         ErrImagePullFailed,
			wantErrContains: "ImagePullBackOff",
		},
		{
			name: "unschedulable pod remains retryable under pull/scheduling budget",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: corev1.PodReasonUnschedulable,
					}},
				},
			},
			wantRetryable:   true,
			wantErr:         ErrPodSchedulingFailed,
			wantErrContains: "unschedulable",
		},
		{
			name: "crash loop fails permanently",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{{
						Name: "mcp",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off restarting failed container"},
						},
					}},
				},
			},
			wantErr:         ErrPodCrashLoopBackOff,
			wantErrContains: "back-off restarting failed container",
		},
		{
			name: "failed phase fails health check timeout",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase:   corev1.PodFailed,
					Message: "pod failed",
				},
			},
			wantErr:         ErrHealthCheckTimeout,
			wantErrContains: "pod failed",
		},
		{
			name: "succeeded phase fails health check timeout for non-agent",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			wantErr:         ErrHealthCheckTimeout,
			wantErrContains: "pod succeeded and exited",
		},
		{
			name: "succeeded phase remains retryable for agent",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			isAgent:         true,
			wantRetryable:   true,
			wantErr:         ErrHealthCheckTimeout,
			wantErrContains: "pod succeeded and exited",
		},
		{
			name: "repeated terminated errors fail crash loop",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:         "mcp",
						RestartCount: 4,
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"},
						},
					}},
				},
			},
			wantErr:         ErrPodCrashLoopBackOff,
			wantErrContains: "repeatedly crashing",
		},
		{
			name: "evicted pod fails scheduling",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase:   corev1.PodPending,
					Reason:  "Evicted",
					Message: "node had disk pressure",
				},
			},
			wantErr:         ErrPodSchedulingFailed,
			wantErrContains: "node had disk pressure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, err := analyzePodStatus(&tt.pod, tt.isAgent)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("analyzePodStatus() error = %v, want %v", err, tt.wantErr)
				}
			} else if tt.wantErrContains == "" && err != nil {
				t.Fatalf("analyzePodStatus() error = %v, want nil", err)
			}
			if retryable != tt.wantRetryable {
				t.Fatalf("analyzePodStatus() retryable = %v, want %v", retryable, tt.wantRetryable)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Fatalf("analyzePodStatus() error = %q, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

type fakeWithWatch struct {
	client.Client // controller-runtime fake for Get/List/Create etc.
	watcher       *watch.FakeWatcher
}

func (f *fakeWithWatch) Watch(_ context.Context, _ client.ObjectList, _ ...client.ListOption) (watch.Interface, error) {
	return f.watcher, nil
}

func TestUpdatedMCPPodName_SucceededPodAgentRetryBehavior(t *testing.T) {
	tests := []struct {
		name            string
		nanobotAgent    string
		wantErrContains string
	}{
		{
			name:            "non-agent succeeded pod fails immediately",
			wantErrContains: "pod succeeded and exited",
		},
		{
			name:            "agent succeeded pod remains retryable",
			nanobotAgent:    "agent-1",
			wantErrContains: "watch retries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := appsv1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme(appsv1) error = %v", err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme(corev1) error = %v", err)
			}

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "obot-mcp",
				},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    1,
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-server-pod",
					Namespace:         "obot-mcp",
					CreationTimestamp: metav1.Now(),
					Labels: map[string]string{
						"app": "test-server",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			}

			watcher := watch.NewFake()
			go func() {
				watcher.Add(deployment.DeepCopy())
				watcher.Stop()
			}()

			client := &fakeWithWatch{
				Client:  fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, pod).Build(),
				watcher: watcher,
			}

			k := &kubernetesBackend{
				client:       client,
				cachedClient: client,
				mcpNamespace: "obot-mcp",
			}

			_, err := k.updatedMCPPodName(t.Context(), "http://mcp.example.com", "test-server", ServerConfig{
				Runtime:          types.RuntimeRemote,
				NanobotAgentName: tt.nanobotAgent,
				StartupTimeout:   time.Second,
			}, "")
			if !errors.Is(err, ErrHealthCheckTimeout) {
				t.Fatalf("updatedMCPPodName() error = %v, want %v", err, ErrHealthCheckTimeout)
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("updatedMCPPodName() error = %q, want to contain %q", err, tt.wantErrContains)
			}
		})
	}
}

func TestUpdatedMCPPodName_ContainerStartupDeadlineExceeded(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(appsv1) error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}

	now := time.Now()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "obot-mcp",
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			UpdatedReplicas:    1,
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-server-pod",
			Namespace:         "obot-mcp",
			CreationTimestamp: metav1.NewTime(now.Add(-time.Minute)),
			Labels: map[string]string{
				"app": "test-server",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "mcp",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-2 * time.Second))},
				},
			}},
		},
	}

	watcher := watch.NewFake()

	go func() {
		watcher.Add(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "obot-mcp"},
		})
		watcher.Stop()
	}()

	client := &fakeWithWatch{
		Client:  fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, pod).Build(),
		watcher: watcher,
	}

	k := &kubernetesBackend{
		client:       client,
		cachedClient: client,
		mcpNamespace: "obot-mcp",
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	_, err := k.updatedMCPPodName(ctx, "http://mcp.example.com", "test-server", ServerConfig{
		Runtime:        types.RuntimeRemote,
		StartupTimeout: time.Second,
	}, "")
	if !errors.Is(err, ErrHealthCheckTimeout) {
		t.Fatalf("updatedMCPPodName() error = %v, want %v", err, ErrHealthCheckTimeout)
	}
	if err.Error() != "timed out waiting for MCP server to be ready after 5 watch retries: timeout waiting for Deployment test-server to meet condition" {
		t.Fatalf("updatedMCPPodName() error = %q, want deployment timeout message", err)
	}
}

func TestK8sObjects_ManagedImagePullSecrets(t *testing.T) {
	managedSecrets := []v1.ImagePullSecret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "managed-b", Namespace: system.DefaultNamespace},
			Spec:       v1.ImagePullSecretSpec{Enabled: true},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "disabled", Namespace: system.DefaultNamespace},
			Spec:       v1.ImagePullSecretSpec{Enabled: false},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "managed-a", Namespace: system.DefaultNamespace},
			Spec:       v1.ImagePullSecretSpec{Enabled: true},
		},
	}

	objs := make([]client.Object, 0, len(managedSecrets))
	for i := range managedSecrets {
		objs = append(objs, &managedSecrets[i])
	}
	k := newTestKubernetesBackend(t, objs...)

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "server",
		Args:                 []string{"run"},
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	dep := findDeployment(t, objs, "test-server")
	assertImagePullSecrets(t, dep, []string{"managed-a", "managed-b"})

	expectedHash := ComputeK8sSettingsHash(v1.K8sSettingsSpec{}, nil, types.RuntimeContainerized, false, ResourceMaximums{}, []string{"managed-b", "managed-a"})
	if dep.Annotations["obot.ai/k8s-settings-hash"] != expectedHash {
		t.Fatalf("k8s settings hash = %q, want %q", dep.Annotations["obot.ai/k8s-settings-hash"], expectedHash)
	}
}

func TestK8sObjects_StaticImagePullSecretsOverrideManaged(t *testing.T) {
	k := newTestKubernetesBackend(t,
		&v1.ImagePullSecret{
			ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: system.DefaultNamespace},
			Spec:       v1.ImagePullSecretSpec{Enabled: true},
		},
	)
	k.imagePullSecrets = []string{"static-b", "static-a", "static-a"}

	objs, err := k.k8sObjects(t.Context(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "server",
		Args:                 []string{"run"},
	})
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	dep := findDeployment(t, objs, "test-server")
	assertImagePullSecrets(t, dep, []string{"static-a", "static-b"})
}

func TestRestartServerAddsManagedImagePullSecretsToFreshDeployment(t *testing.T) {
	k := newTestKubernetesBackend(t,
		&v1.K8sSettings{
			ObjectMeta: metav1.ObjectMeta{Name: system.K8sSettingsName, Namespace: system.DefaultNamespace},
			Spec:       v1.K8sSettingsSpec{},
		},
	)
	server := ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "server",
		Args:                 []string{"run"},
	}

	objs, err := k.k8sObjects(t.Context(), server)
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}
	dep := findDeployment(t, objs, "test-server")

	runtimeScheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(runtimeScheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(runtimeScheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	k.client = fake.NewClientBuilder().WithScheme(runtimeScheme).WithObjects(dep).Build()

	if err := k.obotClient.Create(t.Context(), &v1.ImagePullSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: system.DefaultNamespace},
		Spec:       v1.ImagePullSecretSpec{Enabled: true},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := k.restartServer(t.Context(), server); err != nil {
		t.Fatalf("restartServer() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := k.client.Get(t.Context(), client.ObjectKey{Name: "test-server", Namespace: "obot-mcp"}, &updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertImagePullSecrets(t, &updated, []string{"managed"})
}

func TestStrategicMergePatchReplacesImagePullSecrets(t *testing.T) {
	dep := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "old-secret"}},
				},
			},
		},
	}
	original, err := json.Marshal(dep)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"imagePullSecrets": []map[string]any{
						{"$patch": "replace"},
						{"name": "new-secret"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	patched, err := strategicpatch.StrategicMergePatch(original, patch, appsv1.Deployment{})
	if err != nil {
		t.Fatalf("StrategicMergePatch() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := json.Unmarshal(patched, &updated); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertImagePullSecrets(t, &updated, []string{"new-secret"})
}

func TestResourcesMatchIgnoresExtraActualKeys(t *testing.T) {
	actual := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("1"),
			corev1.ResourceMemory:           resource.MustParse("1Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("500m"),
			corev1.ResourceMemory:           resource.MustParse("512Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
		},
	}
	desired := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	if !resourcesMatch(actual, desired) {
		t.Fatal("expected resources with extra actual ephemeral-storage keys to match")
	}

	desired.Requests[corev1.ResourceMemory] = resource.MustParse("1Gi")
	if resourcesMatch(actual, desired) {
		t.Fatal("expected differing desired memory request to fail")
	}
}

func TestTolerationsMatchIgnoresExtraActualTolerations(t *testing.T) {
	actual := []corev1.Toleration{
		{
			Key:      "kubernetes.io/arch",
			Operator: corev1.TolerationOpEqual,
			Value:    "amd64",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	if !tolerationsMatch(actual, nil) {
		t.Fatal("expected extra actual tolerations to match empty desired tolerations")
	}

	desired := []corev1.Toleration{
		{
			Key:      "workload",
			Operator: corev1.TolerationOpEqual,
			Value:    "mcp",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
	if tolerationsMatch(actual, desired) {
		t.Fatal("expected missing desired toleration to fail")
	}

	actual = append(actual, desired[0])
	if !tolerationsMatch(actual, desired) {
		t.Fatal("expected desired toleration plus extra actual toleration to match")
	}
}

func newTestKubernetesBackend(t *testing.T, objs ...client.Object) *kubernetesBackend {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		clientBuilder = clientBuilder.WithObjects(objs...)
	}

	return &kubernetesBackend{
		baseImage:          "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		compositeBaseImage: "ghcr.io/obot-platform/nanobot:main",
		mcpNamespace:       "obot-mcp",
		obotClient:         clientBuilder.Build(),
	}
}

func testK8sServerConfig() ServerConfig {
	return ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		ContainerPort:        8080,
		ContainerPath:        "/mcp",
		Command:              "server",
		Args:                 []string{"run"},
	}
}

func findSecret(t *testing.T, objs []client.Object, secretName string) *corev1.Secret {
	t.Helper()

	for _, obj := range objs {
		secret, ok := obj.(*corev1.Secret)
		if ok && secret.Name == secretName {
			return secret
		}
	}

	t.Fatalf("secret %q not found", secretName)
	return nil
}

func findService(t *testing.T, objs []client.Object, serviceName string) *corev1.Service {
	t.Helper()

	for _, obj := range objs {
		service, ok := obj.(*corev1.Service)
		if ok && service.Name == serviceName {
			return service
		}
	}

	t.Fatalf("service %q not found", serviceName)
	return nil
}

func findDeployment(t *testing.T, objs []client.Object, deploymentName string) *appsv1.Deployment {
	t.Helper()

	for _, obj := range objs {
		dep, ok := obj.(*appsv1.Deployment)
		if ok && dep.Name == deploymentName {
			return dep
		}
	}

	t.Fatalf("deployment %q not found", deploymentName)
	return nil
}

func findContainer(t *testing.T, deployment *appsv1.Deployment, containerName string) corev1.Container {
	t.Helper()

	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			return container
		}
	}

	t.Fatalf("container %q not found", containerName)
	return corev1.Container{}
}

func assertServicePort(t *testing.T, service *corev1.Service, portName string, port int32, targetPort intstr.IntOrString) {
	t.Helper()

	for _, servicePort := range service.Spec.Ports {
		if servicePort.Name == portName {
			if servicePort.Port != port {
				t.Fatalf("service port %q port = %d, want %d", portName, servicePort.Port, port)
			}
			if servicePort.TargetPort != targetPort {
				t.Fatalf("service port %q targetPort = %v, want %v", portName, servicePort.TargetPort, targetPort)
			}
			return
		}
	}

	t.Fatalf("service port %q not found", portName)
}

func assertImagePullSecrets(t *testing.T, dep *appsv1.Deployment, expected []string) {
	t.Helper()

	actual := make([]string, 0, len(dep.Spec.Template.Spec.ImagePullSecrets))
	for _, ref := range dep.Spec.Template.Spec.ImagePullSecrets {
		actual = append(actual, ref.Name)
	}

	if strings.Join(actual, ",") != strings.Join(expected, ",") {
		t.Fatalf("image pull secrets = %v, want %v", actual, expected)
	}
}

func assertNoAuditLogEnv(t *testing.T, env map[string][]byte) {
	t.Helper()

	for key := range env {
		if strings.HasPrefix(key, "NANOBOT_RUN_AUDIT_LOG_") {
			t.Fatalf("unexpected audit log env %q present", key)
		}
	}
}
