package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp/attribution"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestObotHeadersCannotBeConfiguredAsStaticHeaders(t *testing.T) {
	config := ServerConfig{
		Headers:                 []string{"x-obot-api-key-name=static-spoof", "X-Obot-User-Id=static-spoof", "X-Obot-IDP-User-Id=spoof", "x-obot-idp-groups=[\"spoof\"]", "X-Tenant=tenant"},
		PassthroughHeaderNames:  []string{"X-OBOT-USER-EMAIL", "X-Obot-IDP-Groups"},
		PassthroughHeaderValues: []string{"spoof@example.com", "[\"spoof\"]"},
	}
	headers := attribution.Configured(config.Headers, config.PassthroughHeaderNames, config.PassthroughHeaderValues)
	require.Equal(t, http.Header{"X-Tenant": {"tenant"}}, headers)
	wrapper := MMMCPConfig(config, nil)
	require.Equal(t, map[string]string{"X-Tenant": "tenant"}, wrapper.Servers[0].Headers)
	require.Empty(t, wrapper.Servers[0].PassthroughHeaders)
}

func TestReadinessProbesExcludeConfiguredAttribution(t *testing.T) {
	for _, tt := range []struct {
		name       string
		fallback   bool
		lastMethod string
	}{
		{
			name:       "streamable POST and cleanup",
			lastMethod: http.MethodDelete,
		},
		{
			name:       "SSE fallback",
			fallback:   true,
			lastMethod: http.MethodGet,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type probe struct {
				method  string
				headers http.Header
			}
			received := make(chan probe, 2)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- probe{method: r.Method, headers: r.Header.Clone()}
				if tt.fallback && r.Method == http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte("event: endpoint\ndata: /messages\n\n"))
					return
				}
				w.Header().Set("Mcp-Session-Id", "probe-session")
			}))
			defer backend.Close()
			config := ServerConfig{
				PassthroughHeaderNames:  []string{"X-Tenant"},
				PassthroughHeaderValues: []string{"tenant"},
			}
			for _, name := range types.ObotHeaders() {
				config.PassthroughHeaderNames = append(config.PassthroughHeaderNames, string(name))
				config.PassthroughHeaderValues = append(config.PassthroughHeaderValues, "spoof")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			require.NoError(t, ensureServerReady(ctx, backend.URL, config))
			for _, method := range []string{http.MethodPost, tt.lastMethod} {
				select {
				case request := <-received:
					require.Equal(t, method, request.method)
					require.Equal(t, "tenant", request.headers.Get("X-Tenant"))
					for _, name := range types.ObotHeaders() {
						require.Empty(t, request.headers.Get(string(name)))
					}
				case <-ctx.Done():
					t.Fatalf("missing %s probe", method)
				}
			}
		})
	}
}

func TestHTTPClientsCannotRestoreConfiguredObotHeaders(t *testing.T) {
	received := make(chan http.Header, 3)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	manager := &SessionManager{
		backend: &kubernetesBackend{},
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{
			AllowLocalhostMCP: true,
			AllowPrivateIPMCP: true,
		},
	}
	config := ServerConfig{
		Headers: []string{"X-Obot-Api-Key-Name=spoof", "X-Obot-User-Id=999", "X-Tenant=tenant"},
	}
	client, err := manager.HTTPClientForServer(config, HTTPClientOptions{DirectConnect: true})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, backend.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Obot-Api-Key-Name", "authenticated-writer")
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	headers := <-received
	require.Equal(t, "authenticated-writer", headers.Get("X-Obot-Api-Key-Name"))
	require.Empty(t, headers.Get("X-Obot-User-Id"))
	require.Equal(t, "tenant", headers.Get("X-Tenant"))

	// Background requests do not inherit the previous caller's headers.
	req, err = http.NewRequestWithContext(t.Context(), http.MethodPost, backend.URL, nil)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	headers = <-received
	require.Empty(t, headers.Get("X-Obot-Api-Key-Name"))
	require.Empty(t, headers.Get("X-Obot-User-Id"))

	oauthClient, err := manager.OAuthClientForServer(config)
	require.NoError(t, err)
	resp, err = oauthClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	headers = <-received
	require.Empty(t, headers.Get("X-Obot-Api-Key-Name"))
	require.Empty(t, headers.Get("X-Tenant"))
}

func TestHostedConnectionsDiscardObotHeaderSelections(t *testing.T) {
	server := ServerConfig{
		Runtime:        types.RuntimeContainerized,
		MCPServerName:  "jira",
		ContainerPort:  8080,
		StartupTimeout: time.Second,
	}
	server.InjectObotHeaders = []types.ObotHeader{types.ObotHeaderAPIKeyName}
	t.Run("docker", func(t *testing.T) {
		connection, err := (&dockerBackend{}).buildServerConfig(server, &container.Summary{
			ID: "container",
			Ports: []container.Port{
				{
					PrivatePort: 8080,
					PublicPort:  18080,
				},
			},
		}, 8080, false)
		require.NoError(t, err)
		require.Empty(t, connection.InjectObotHeaders, "hosted connections must not inherit legacy attribution selections")
	})
	t.Run("kubernetes cached deployment", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, appsv1.AddToScheme(scheme))
		require.NoError(t, corev1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&appsv1.Deployment{
				Name:      server.MCPServerName,
				Namespace: "mcp",
				Status: appsv1.DeploymentStatus{
					Replicas:          1,
					UpdatedReplicas:   1,
					ReadyReplicas:     1,
					AvailableReplicas: 1,
				},
			},
			&corev1.Pod{
				Name:              "running-pod",
				Namespace:         "mcp",
				CreationTimestamp: metav1.Now(),
				Labels:            map[string]string{"app": server.MCPServerName},
				Status:            corev1.PodStatus{Phase: corev1.PodRunning},
			},
		).Build()
		backend := &kubernetesBackend{
			client:          client,
			cachedClient:    client,
			mcpNamespace:    "mcp",
			deploymentCache: map[string]*kubernetesDeploymentCacheEntry{},
		}
		backend.setDeploymentCache(server.MCPServerName, kubernetesDeploymentCacheEntry{
			hash:    serverID(server) + utils.Digest(server.Files),
			podName: "running-pod",
		})
		connection, err := backend.ensureServerDeployment(t.Context(), server)
		require.NoError(t, err)
		require.Empty(t, connection.InjectObotHeaders, "hosted connections must not inherit legacy attribution selections")
	})
}
