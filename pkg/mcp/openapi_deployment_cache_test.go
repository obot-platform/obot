package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOpenAPIDestinationValidationDeploymentCache(t *testing.T) {
	var lookups atomic.Int64
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			lookups.Add(1)
			return nil, errors.New("DNS unavailable")
		},
	}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })

	for _, backendName := range []string{"docker", "kubernetes"} {
		t.Run(backendName, func(t *testing.T) {
			for _, test := range []struct {
				name    string
				cached  bool
				live    bool
				changed bool
			}{
				{
					name:   "cache hit survives DNS outage",
					cached: true,
					live:   true,
				},
				{
					name: "first deployment validates destination",
				},
				{
					name:    "changed configuration validates destination",
					cached:  true,
					live:    true,
					changed: true,
				},
				{
					name:   "missing deployment validates destination",
					cached: true,
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret"})
					config.Env[0] = "OPENAPI_BASE_URL=https://destination.invalid/"
					config.StartupTimeout = time.Second
					var runtimeBackend backend
					if backendName == "docker" {
						api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							require.True(t, strings.HasSuffix(r.URL.Path, "/json"), "unexpected Docker API request: %s", r.URL)
							w.Header().Set("Content-Type", "application/json")
							if test.live {
								_, _ = io.WriteString(w, `{"Id":"existing-container","State":{"Running":true}}`)
							} else {
								w.WriteHeader(http.StatusNotFound)
								_, _ = io.WriteString(w, `{"message":"container missing"}`)
							}
						}))
						t.Cleanup(api.Close)
						cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, cli.Close()) })
						d := &dockerBackend{client: cli, deploymentCache: map[string]*dockerDeploymentCacheEntry{}}
						if test.cached {
							d.setDeploymentCache(config.MCPServerName, dockerDeploymentCacheEntry{
								hash:         utils.Digest(config),
								serverConfig: config,
								containerIDs: map[string]string{config.MCPServerName: "existing-container"},
							})
						}
						runtimeBackend = d
					} else {
						scheme := runtime.NewScheme()
						require.NoError(t, corev1.AddToScheme(scheme))
						require.NoError(t, appsv1.AddToScheme(scheme))
						var objects []kclient.Object
						if test.live {
							objects = []kclient.Object{
								&appsv1.Deployment{
									Name:      config.MCPServerName,
									Namespace: "mcp",
									Status: appsv1.DeploymentStatus{
										Replicas:          1,
										UpdatedReplicas:   1,
										ReadyReplicas:     1,
										AvailableReplicas: 1,
									},
								},
								&corev1.Pod{
									Name:              "existing-pod",
									Namespace:         "mcp",
									CreationTimestamp: metav1.Now(),
									Labels:            map[string]string{"app": config.MCPServerName},
									Status:            corev1.PodStatus{Phase: corev1.PodRunning},
								},
							}
						}
						kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
						k := &kubernetesBackend{
							client:          kubeClient,
							cachedClient:    kubeClient,
							mcpNamespace:    "mcp",
							deploymentCache: map[string]*kubernetesDeploymentCacheEntry{},
						}
						if test.cached {
							k.setDeploymentCache(config.MCPServerName, kubernetesDeploymentCacheEntry{
								hash:    serverID(config) + utils.Digest(config.Files),
								podName: "existing-pod",
							})
						}
						runtimeBackend = k
					}
					if test.changed {
						config.Env[0] = "OPENAPI_BASE_URL=https://changed.invalid/"
					}
					manager := SessionManager{backend: runtimeBackend}
					before := lookups.Load()
					_, err := manager.LaunchServer(t.Context(), config)
					if test.cached && test.live && !test.changed {
						require.NoError(t, err)
						require.Equal(t, before, lookups.Load(), "cache hits must not resolve the API destination")
					} else {
						require.ErrorContains(t, err, "DNS unavailable")
						require.Greater(t, lookups.Load(), before, "deployments must validate the API destination")
					}
				})
			}
		})
	}
}
