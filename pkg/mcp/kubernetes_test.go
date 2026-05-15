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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestK8sObjects_NanobotAgentExcludesAuditLogConfig(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
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
		AuditLogToken:        "audit-token",
		AuditLogEndpoint:     "https://obot.example.com/api/mcp-audit-logs",
		AuditLogMetadata:     "mcpID=server-1",
	}, nil)
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	configSecret := findSecret(t, objs, name.SafeConcatName("nanobot-agent-server", "mcp", "config"))
	assertNoAuditLogEnv(t, configSecret.Data)
}

func TestK8sObjects_NonAgentShimKeepsAuditLogConfig(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
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
		AuditLogToken:        "audit-token",
		AuditLogEndpoint:     "https://obot.example.com/api/mcp-audit-logs",
		AuditLogMetadata:     "mcpID=server-1",
	}, nil)
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	shimConfigSecret := findSecret(t, objs, name.SafeConcatName("standard-server", "mcp", "config", "shim"))
	assertHasAuditLogEnv(t, shimConfigSecret.Data)
}

func TestK8sObjects_ServicePorts(t *testing.T) {
	tests := []struct {
		name                   string
		nanobotAgentName       string
		expectedHTTPPortTarget intstr.IntOrString
		expectedStrategy       appsv1.DeploymentStrategyType
	}{
		{
			name:                   "standard containerized server routes http service port to shim",
			expectedHTTPPortTarget: intstr.FromString("http"),
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
			objs, err := k.k8sObjects(context.Background(), ServerConfig{
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
			}, nil)
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

func TestAnalyzePodStatus(t *testing.T) {
	tests := []struct {
		name            string
		pod             corev1.Pod
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
			retryable, err := analyzePodStatus(&tt.pod)
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
		mcpNamespace: "obot-mcp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
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
	}, nil)
	if err != nil {
		t.Fatalf("k8sObjects() error = %v", err)
	}

	dep := findDeployment(t, objs, "test-server")
	assertImagePullSecrets(t, dep, []string{"managed-a", "managed-b"})

	expectedHash := ComputeK8sSettingsHash(v1.K8sSettingsSpec{}, []string{"managed-b", "managed-a"})
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

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
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
	}, nil)
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

	objs, err := k.k8sObjects(context.Background(), server, nil)
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

	if err := k.obotClient.Create(context.Background(), &v1.ImagePullSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: system.DefaultNamespace},
		Spec:       v1.ImagePullSecretSpec{Enabled: true},
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := k.restartServer(context.Background(), server); err != nil {
		t.Fatalf("restartServer() error = %v", err)
	}

	var updated appsv1.Deployment
	if err := k.client.Get(context.Background(), client.ObjectKey{Name: "test-server", Namespace: "obot-mcp"}, &updated); err != nil {
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
		baseImage:           "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		remoteShimBaseImage: "ghcr.io/obot-platform/remote-shim:main",
		mcpNamespace:        "obot-mcp",
		obotClient:          clientBuilder.Build(),
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

func assertHasAuditLogEnv(t *testing.T, env map[string][]byte) {
	t.Helper()

	expected := []string{
		"NANOBOT_RUN_AUDIT_LOG_TOKEN",
		"NANOBOT_RUN_AUDIT_LOG_SEND_URL",
		"NANOBOT_RUN_AUDIT_LOG_BATCH_SIZE",
		"NANOBOT_RUN_AUDIT_LOG_FLUSH_INTERVAL_SECONDS",
		"NANOBOT_RUN_AUDIT_LOG_METADATA",
	}

	for _, key := range expected {
		if _, ok := env[key]; !ok {
			t.Fatalf("expected audit log env %q to be present", key)
		}
	}
}
