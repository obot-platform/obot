package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
func TestK8sObjects_NanobotAgentExcludesAuditLogConfig(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "nanobot-agent-server",
		MCPServerDisplayName: "Nanobot Agent Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/nanobot-ai/nanobot:latest",
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

	configSecret := findSecret(t, objs, name.SafeConcatName("nanobot-agent-server", "config"))
	assertNoAuditLogEnv(t, configSecret.StringData)
}

func TestK8sObjects_NonAgentShimKeepsAuditLogConfig(t *testing.T) {
	k := newTestKubernetesBackend(t)

	objs, err := k.k8sObjects(context.Background(), ServerConfig{
		Runtime:              types.RuntimeContainerized,
		MCPServerName:        "standard-server",
		MCPServerDisplayName: "Standard Server",
		UserID:               "user-1",
		OwnerUserID:          "user-2",
		ContainerImage:       "ghcr.io/obot-platform/mcp-images/phat:main",
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

	shimConfigSecret := findSecret(t, objs, name.SafeConcatName("standard-server", "config", "shim"))
	assertHasAuditLogEnv(t, shimConfigSecret.StringData)
}

func newTestKubernetesBackend(t *testing.T) *kubernetesBackend {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	return &kubernetesBackend{
		baseImage:            "ghcr.io/obot-platform/mcp-images/phat:main",
		httpWebhookBaseImage: "ghcr.io/obot-platform/http-webhook:main",
		remoteShimBaseImage:  "ghcr.io/obot-platform/remote-shim:main",
		mcpNamespace:         "obot-mcp",
		obotClient:           fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
}

func findSecret(t *testing.T, objs []kclient.Object, secretName string) *corev1.Secret {
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

func assertNoAuditLogEnv(t *testing.T, env map[string]string) {
	t.Helper()

	for key := range env {
		if strings.HasPrefix(key, "NANOBOT_RUN_AUDIT_LOG_") {
			t.Fatalf("unexpected audit log env %q present", key)
		}
	}
}

func assertHasAuditLogEnv(t *testing.T, env map[string]string) {
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
