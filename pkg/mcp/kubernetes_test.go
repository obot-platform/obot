package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func TestKubernetesBackendGetPullPolicy(t *testing.T) {
	tests := []struct {
		name            string
		configuredValue corev1.PullPolicy
		expected        corev1.PullPolicy
	}{
		{
			name:            "defaults empty value to always",
			configuredValue: "",
			expected:        corev1.PullAlways,
		},
		{
			name:            "keeps explicit if-not-present",
			configuredValue: corev1.PullIfNotPresent,
			expected:        corev1.PullIfNotPresent,
		},
		{
			name:            "keeps explicit never",
			configuredValue: corev1.PullNever,
			expected:        corev1.PullNever,
		},
		{
			name:            "keeps explicit always",
			configuredValue: corev1.PullAlways,
			expected:        corev1.PullAlways,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &kubernetesBackend{imagePullPolicy: tt.configuredValue}

			if got := k.getPullPolicy(); got != tt.expected {
				t.Fatalf("getPullPolicy() = %q, want %q", got, tt.expected)
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

func newTestKubernetesBackend(t *testing.T) *kubernetesBackend {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	return &kubernetesBackend{
		baseImage:           "ghcr.io/obot-platform/mcp-images/stdio-wrapper:main",
		remoteShimBaseImage: "ghcr.io/obot-platform/remote-shim:main",
		mcpNamespace:        "obot-mcp",
		obotClient:          fake.NewClientBuilder().WithScheme(scheme).Build(),
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
