package mcp

import (
	"testing"
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
