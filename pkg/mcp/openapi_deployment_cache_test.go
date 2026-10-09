package mcp

import (
	"context"
	"encoding/json"
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
	"github.com/obot-platform/obot/apiclient/types"
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
				name           string
				cached         bool
				live           bool
				changed        bool
				requestChanged bool
				dynamicChanged bool
			}{
				{
					name:   "cache hit survives DNS outage",
					cached: true,
					live:   true,
				},
				{
					name:           "another user reuses deployment during DNS outage",
					cached:         true,
					live:           true,
					requestChanged: true,
				},
				{
					name: "running deployment survives empty cache and DNS outage",
					live: true,
				},
				{
					name:           "dynamic file updates still synchronize during DNS outage",
					cached:         true,
					live:           true,
					dynamicChanged: true,
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
					if backendName == "kubernetes" && (!test.cached && test.live || test.dynamicChanged) {
						t.Skip("Docker-specific deployment reuse and file synchronization")
					}
					config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret"})
					config.Env[0] = "OPENAPI_BASE_URL=https://destination.invalid/"
					config.StartupTimeout = time.Second
					if test.dynamicChanged {
						config.Files = append(config.Files, File{
							EnvKey:  "DYNAMIC_CONFIG",
							Data:    "original",
							Dynamic: true,
						})
					}
					var syncAttempted atomic.Bool
					var runtimeBackend backend
					if backendName == "docker" {
						ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							require.Equal(t, "/readyz", r.URL.Path)
							w.WriteHeader(http.StatusOK)
						}))
						t.Cleanup(ready.Close)
						config.ContainerPort = ready.Listener.Addr().(*net.TCPAddr).Port
						containerSummary := map[string]any{
							"Id":    "existing-container",
							"Names": []string{"/" + config.MCPServerName},
							"State": "running",
							"Labels": map[string]string{
								"mcp.config.hash":        serverID(config),
								"mcp.file.env.keys.hash": fileEnvKeysHash(config.Files),
							},
							"NetworkSettings": map[string]any{"Networks": map[string]any{"mcp": map[string]string{"IPAddress": "127.0.0.1"}}},
						}
						api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							if strings.HasSuffix(r.URL.Path, "/volumes/create") {
								syncAttempted.Store(true)
								w.WriteHeader(http.StatusInternalServerError)
								_, _ = io.WriteString(w, `{"message":"file sync unavailable"}`)
								return
							}
							require.True(t, strings.HasSuffix(r.URL.Path, "/json"), "unexpected Docker API request: %s", r.URL)
							if strings.HasSuffix(r.URL.Path, "/containers/json") {
								if test.live {
									require.NoError(t, json.NewEncoder(w).Encode([]any{containerSummary}))
								} else {
									_, _ = io.WriteString(w, `[]`)
								}
								return
							}
							if test.live {
								_, _ = io.WriteString(w, `{"Id":"existing-container","State":{"Running":true,"Status":"running"}}`)
							} else {
								w.WriteHeader(http.StatusNotFound)
								_, _ = io.WriteString(w, `{"message":"container missing"}`)
							}
						}))
						t.Cleanup(api.Close)
						cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, cli.Close()) })
						d := &dockerBackend{
							client:          cli,
							containerEnv:    true,
							network:         "mcp",
							deploymentCache: map[string]*dockerDeploymentCacheEntry{},
							syncedFilesHash: map[string]string{"existing-container": utils.Digest(config.Files)},
						}
						if test.cached {
							connection := config
							connection.Runtime = types.RuntimeRemote
							connection.URL = ready.URL + "/mcp"
							connection.Scope = "existing-container"
							connection.Headers = config.hostedConnectionHeaders()
							d.setDeploymentCache(config.MCPServerName, dockerDeploymentCacheEntry{
								hash:         utils.Digest(config),
								serverConfig: connection,
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
					if test.requestChanged {
						config.UserID = "another-user"
						config.AuditLogMetadata = map[string]string{"userID": "another-user"}
						config.PassthroughHeaderValues = []string{"Authorization=Bearer another-secret"}
						config.Webhooks = []Webhook{{URL: "https://hooks.example.com"}}
					}
					if test.dynamicChanged {
						config.Files[1].Data = "updated"
					}
					manager := SessionManager{backend: runtimeBackend}
					before := lookups.Load()
					connection, err := manager.LaunchServer(t.Context(), config)
					if test.dynamicChanged {
						require.ErrorContains(t, err, "file sync unavailable")
						require.True(t, syncAttempted.Load(), "file updates must not be hidden by the deployment cache")
						require.Equal(t, before, lookups.Load(), "dynamic file updates do not recreate the container")
					} else if test.live && !test.changed {
						require.NoError(t, err)
						require.Equal(t, before, lookups.Load(), "reusing a deployment must not resolve the API destination")
						require.Equal(t, types.RuntimeRemote, connection.Runtime)
						require.Equal(t, config.UserID, connection.UserID)
						require.Equal(t, config.AuditLogMetadata, connection.AuditLogMetadata)
						require.Equal(t, config.PassthroughHeaderValues, connection.PassthroughHeaderValues)
						require.Equal(t, config.Webhooks, connection.Webhooks)
						require.Equal(t, config.hostedConnectionHeaders(), connection.Headers)
					} else {
						require.ErrorContains(t, err, "DNS unavailable")
						require.Greater(t, lookups.Load(), before, "deployments must validate the API destination")
					}
				})
			}
		})
	}
}

func TestOpenAPICreatedContainerDestinationValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		url       string
		options   ValidationOptions
		wantErr   string
		wantStart bool
	}{
		{
			name:    "HTTP rejected after development mode disabled",
			url:     "http://93.184.216.34/",
			wantErr: "API destination must use HTTPS",
		},
		{
			name:    "private destination rejected after policy tightened",
			url:     "https://10.0.0.1/",
			wantErr: "blocked private",
		},
		{
			name:      "development HTTP still starts",
			url:       "http://93.184.216.34/",
			options:   ValidationOptions{DevMode: true},
			wantErr:   "failed to start container",
			wantStart: true,
		},
		{
			name: "explicitly allowed private destination still starts",
			url:  "https://10.0.0.1/",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{AllowPrivateIPMCP: true},
			},
			wantErr:   "failed to start container",
			wantStart: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret"})
			config.Env[0] = "OPENAPI_BASE_URL=" + test.url
			var startAttempted atomic.Bool
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					require.NoError(t, json.NewEncoder(w).Encode([]any{map[string]any{
						"Id":    "existing-container",
						"Names": []string{"/" + config.MCPServerName},
						"State": "created",
						"Image": "openapi:test",
						"Labels": map[string]string{
							"mcp.config.hash":        serverID(config),
							"mcp.file.env.keys.hash": fileEnvKeysHash(config.Files),
						},
						"NetworkSettings": map[string]any{"Networks": map[string]any{"mcp": map[string]string{"IPAddress": "127.0.0.1"}}},
					}}))
				case strings.HasSuffix(r.URL.Path, "/containers/existing-container/start"):
					startAttempted.Store(true)
					// Stop at the start call: readiness is covered by other tests.
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, `{"message":"start unavailable"}`)
				default:
					t.Errorf("unexpected Docker API request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(api.Close)
			cli, err := client.NewClientWithOpts(client.WithHost(api.URL), client.WithVersion("1.44"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cli.Close()) })
			d := &dockerBackend{
				client:            cli,
				network:           "mcp",
				openAPIImage:      "openapi:test",
				validationOptions: test.options,
			}
			_, err = d.ensureServerDeployment(t.Context(), config)
			require.ErrorContains(t, err, test.wantErr)
			require.Equal(t, test.wantStart, startAttempted.Load())
		})
	}
}
