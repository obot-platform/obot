package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	storedOpenAPISchema = `{"openapi":"3.1.0","info":{"title":"Test API","version":"1"},"servers":[{"url":"https://api.example.com/v1"}],"paths":{}}`
)

type openAPILifecycleBackend struct {
	backend
	configured ServerConfig
}

func testOpenAPISchema(raw string) *types.OpenAPISchema {
	return &types.OpenAPISchema{Raw: json.RawMessage(raw)}
}

func openAPITestServer() v1.MCPServer {
	return v1.MCPServer{
		Name: "openapi-test",
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeOpenAPI,
				OpenAPIConfig: &types.OpenAPIRuntimeConfig{
					Source: types.OpenAPISource{URL: "https://must-not-fetch.invalid/openapi.json"},
					Schema: testOpenAPISchema(storedOpenAPISchema),
				},
				Config: []types.MCPConfig{{
					Key:       "Authorization",
					Usage:     types.Header,
					Prefix:    "Bearer ",
					Required:  true,
					Sensitive: true,
				}},
			},
		},
	}
}

func openAPITestConfig(t *testing.T, server v1.MCPServer, credentials map[string]string) ServerConfig {
	t.Helper()
	config, missing, err := ServerToServerConfig(server, nil, "user-1", "test", "default", credentials)
	require.NoError(t, err)
	require.Empty(t, missing)
	return config
}

func TestOpenAPISnapshotDeployment(t *testing.T) {
	server := openAPITestServer()
	config := openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})

	require.Equal(t, types.RuntimeOpenAPI, config.Runtime)
	require.Equal(t, types.RuntimeOpenAPI, server.Spec.Manifest.Runtime)
	require.Equal(t, 8080, config.ContainerPort)
	require.Equal(t, "/mcp", config.ContainerPath)
	require.Equal(t, "/readyz", config.HealthzPath)
	require.Equal(t, []string{"Authorization=Bearer secret-key"}, config.Headers)
	require.Equal(t, []File{{
		EnvKey:  "OPENAPI_SPEC_FILE",
		Data:    storedOpenAPISchema,
		Dynamic: false,
	}}, config.Files)
	require.Equal(t, []string{
		"OPENAPI_BASE_URL=https://api.example.com/v1/",
		"OPENAPI_CREDENTIAL_HEADERS=Authorization",
	}, config.Env)
	require.NotContains(t, strings.Join(config.Env, "\n"), "secret-key")
	require.NotContains(t, config.Files[0].Data, "secret-key")

	require.Empty(t, config.ContainerImage)

	// Catalog/source changes cannot update a deployed snapshot; changing the
	// snapshot or its settings produces a new deployment identity.
	originalID := serverID(config)
	server.Spec.Manifest.OpenAPIConfig.Source.URL = "https://new-source.invalid/schema"
	same := openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})
	require.Equal(t, originalID, serverID(same))

	server.Spec.Manifest.OpenAPIConfig.Schema = testOpenAPISchema(strings.ReplaceAll(storedOpenAPISchema, "Test API", "Updated API"))
	changed := openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})
	require.NotEqual(t, originalID, serverID(changed))

	server.Spec.Manifest.OpenAPIConfig.Schema = testOpenAPISchema(strings.ReplaceAll(storedOpenAPISchema, "api.example.com", "other.example.com"))
	changed = openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})
	require.Equal(t, "OPENAPI_BASE_URL=https://other.example.com/v1/", changed.Env[0])
	require.NotEqual(t, originalID, serverID(changed))

	server.Spec.Manifest.OpenAPIConfig.Schema = testOpenAPISchema(storedOpenAPISchema)
	server.Spec.Manifest.OpenAPIConfig.BaseURL = "https://other.example.com"
	changed = openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})
	require.NotEqual(t, originalID, serverID(changed))

	server.Spec.Manifest.OpenAPIConfig.BaseURL = ""
	rollback := openAPITestConfig(t, server, map[string]string{"Authorization": "secret-key"})
	require.Equal(t, originalID, serverID(rollback))
}

func TestOpenAPIDeploymentReadiness(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
	}{
		{
			name:   "conversion succeeded",
			status: http.StatusOK,
		},
		{
			name:   "conversion failed",
			status: http.StatusServiceUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrapper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/healthz":
					w.WriteHeader(http.StatusOK)
				case "/readyz":
					if test.status != http.StatusOK {
						http.Error(w, "OpenAPI conversion failed", test.status)
					}
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(wrapper.Close)
			config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "key"})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			err := ensureServerReady(ctx, wrapper.URL, config)
			if test.status == http.StatusOK {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrHealthCheckFailed)
				require.ErrorContains(t, err, "OpenAPI conversion failed")
			}
		})
	}
}

func TestOpenAPICredentialPrefixesAndMissing(t *testing.T) {
	for _, value := range []string{"key", "Bearer key"} {
		config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": value})
		require.Equal(t, []string{"Authorization=Bearer key"}, config.Headers)
	}
	server := openAPITestServer()
	server.Spec.Manifest.Config[0].Value = "Bearer static-key"
	config := openAPITestConfig(t, server, map[string]string{"Authorization": "ignored-key"})
	require.Equal(t, []string{"Authorization=Bearer static-key"}, config.Headers)
	server.Spec.Manifest.Config[0].Value = ""
	config, missing, err := ServerToServerConfig(server, nil, "user-1", "test", "default", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"Authorization"}, missing)
	require.Empty(t, config.Headers)
}

func TestOpenAPIManifestValidation(t *testing.T) {
	manifest := openAPITestServer().Spec.Manifest
	manifest.OpenAPIConfig.BaseURL = "https://93.184.216.34"
	require.NoError(t, ValidateServerManifest(t.Context(), manifest, false, ValidationOptions{}))
	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), manifest.ConvertToCatalogEntry(), false, ValidationOptions{}))
}

func TestOpenAPISourceShapeValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		source  types.OpenAPISource
		wantErr bool
	}{
		{
			name:    "missing source",
			wantErr: true,
		},
		{
			name: "both fields",
			source: types.OpenAPISource{
				URL:     "https://must-not-fetch.invalid/schema",
				Content: "must not parse source content",
			},
			wantErr: true,
		},
		{
			name: "URL only",
			source: types.OpenAPISource{
				URL: "https://must-not-fetch.invalid/schema",
			},
		},
		{
			name: "content only",
			source: types.OpenAPISource{
				Content: "must not parse source content",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := openAPITestServer().Spec.Manifest
			manifest.OpenAPIConfig.BaseURL = "https://93.184.216.34"
			manifest.OpenAPIConfig.Source = test.source
			for _, err := range []error{
				ValidateServerManifest(t.Context(), manifest, false, ValidationOptions{}),
				ValidateCatalogEntryManifest(t.Context(), manifest.ConvertToCatalogEntry(), false, ValidationOptions{}),
			} {
				if test.wantErr {
					var validationErr types.RuntimeValidationError
					require.ErrorAs(t, err, &validationErr)
					require.Equal(t, types.RuntimeOpenAPI, validationErr.Runtime)
					require.Equal(t, "openAPIConfig.source", validationErr.Field)
					require.Equal(t, "exactly one OpenAPI source URL or content is required", validationErr.Message)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestOpenAPIInvalidConfig(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*types.MCPServerManifest)
	}{
		{
			name:   "missing config",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig = nil },
		},
		{
			name:   "missing snapshot does not fetch source",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.Schema = nil },
		},
		{
			name:   "invalid JSON",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.Schema = testOpenAPISchema("not JSON") },
		},
		{
			name:   "null snapshot",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.Schema = testOpenAPISchema("null") },
		},
		{
			name:   "empty object snapshot",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.Schema = testOpenAPISchema("{}") },
		},
		{
			name: "unsupported OpenAPI version",
			mutate: func(m *types.MCPServerManifest) {
				m.OpenAPIConfig.Schema = testOpenAPISchema(strings.Replace(storedOpenAPISchema, "3.1.0", "2.0", 1))
			},
		},
		{
			name: "external reference",
			mutate: func(m *types.MCPServerManifest) {
				m.OpenAPIConfig.Schema = testOpenAPISchema(strings.Replace(storedOpenAPISchema, `"paths":{}`, `"paths":{"/x":{"$ref":"https://example.com/path"}}`, 1))
			},
		},
		{
			name:   "YAML snapshot",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.Schema = testOpenAPISchema("openapi: 3.1.0\n") },
		},
		{
			name: "schema limit",
			mutate: func(m *types.MCPServerManifest) {
				m.OpenAPIConfig.Schema = testOpenAPISchema(strings.Repeat(" ", openapi.MaxSchemaBytes+1))
			},
		},
		{
			name:   "credential env not allowed",
			mutate: func(m *types.MCPServerManifest) { m.Config[0].Usage = types.Env },
		},
		{
			name:   "credential file not allowed",
			mutate: func(m *types.MCPServerManifest) { m.Config[0].Usage = types.File },
		},
		{
			name:   "edited credential header is invalid",
			mutate: func(m *types.MCPServerManifest) { m.Config[0].Key = "Host" },
		},
		{
			name: "duplicate credential headers",
			mutate: func(m *types.MCPServerManifest) {
				m.Config = append(m.Config, types.MCPConfig{
					Key:   "authorization",
					Usage: types.Header,
				})
			},
		},
		{
			name: "missing schema credential header",
			mutate: func(m *types.MCPServerManifest) {
				m.OpenAPIConfig.Schema = testOpenAPISchema(strings.Replace(storedOpenAPISchema, `"paths":{}`, `"paths":{},"components":{"securitySchemes":{"key":{"type":"apiKey","in":"header","name":"X-API-Key"}}}`, 1))
			},
		},
		{
			name:   "credentials require HTTPS",
			mutate: func(m *types.MCPServerManifest) { m.OpenAPIConfig.BaseURL = "http://api.example.com" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := openAPITestServer()
			test.mutate(&server.Spec.Manifest)
			t.Run("deployment", func(t *testing.T) {
				_, _, err := ServerToServerConfig(server, nil, "user-1", "test", "default", nil)
				require.Error(t, err)
			})
			t.Run("server manifest", func(t *testing.T) {
				err := (OpenAPIValidator{}).ValidateConfig(t.Context(), server.Spec.Manifest)
				var validationErr types.RuntimeValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, types.RuntimeOpenAPI, validationErr.Runtime)
			})
			t.Run("catalog manifest", func(t *testing.T) {
				err := (OpenAPIValidator{}).ValidateCatalogConfig(t.Context(), server.Spec.Manifest.ConvertToCatalogEntry())
				var validationErr types.RuntimeValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, types.RuntimeOpenAPI, validationErr.Runtime)
			})
		})
	}
}

func TestOpenAPISharedHeadersAreRequestScoped(t *testing.T) {
	server := openAPITestServer()
	server.Spec.Manifest.Config[0].UserAllowed = true
	config := openAPITestConfig(t, server, map[string]string{"Authorization": "stale-shared-key"})
	require.Empty(t, config.Headers)
	require.NotContains(t, strings.Join(config.Env, "\n"), "stale-shared-key")
	component := types.VMCPComponent{
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: server.Spec.Manifest.ConvertToCatalogEntry()},
		Configuration: []types.VMCPConfigurationPolicy{{
			Key:    "Authorization",
			Policy: types.VMCPConfigurationPolicyUserAllowed,
		}},
	}
	require.True(t, vmcpaccess.IsMultiUser(component))
	instance := v1.MCPServerInstance{Spec: v1.MCPServerInstanceSpec{Config: server.Spec.Manifest.Config}}
	first, second := config, config
	var missing []string
	first.PassthroughHeaderNames, first.PassthroughHeaderValues, missing = serverInstanceHeaders(instance, map[string]string{"Authorization": "first-key"})
	require.Empty(t, missing)
	second.PassthroughHeaderNames, second.PassthroughHeaderValues, missing = serverInstanceHeaders(instance, map[string]string{"Authorization": "Bearer second-key"})
	require.Empty(t, missing)
	require.Equal(t, serverID(first), serverID(second))
	require.NotEqual(t, clientID(first, ""), clientID(second, ""))
	_, values, missing := serverInstanceHeaders(instance, nil)
	require.Empty(t, values)
	require.Equal(t, []string{"Authorization"}, missing)

	// Exercise the same HTTP transport used by the gateway concurrently, with
	// independent per-user configs and no shared client-header mutations.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Header.Get("Authorization"))
	}))
	t.Cleanup(upstream.Close)
	manager := SessionManager{backend: &dockerBackend{}}
	var wg sync.WaitGroup
	for _, test := range []struct {
		config ServerConfig
		want   string
	}{
		{
			config: first,
			want:   "Bearer first-key",
		},
		{
			config: second,
			want:   "Bearer second-key",
		},
	} {
		client, err := manager.HTTPClientForServer(test.config, HTTPClientOptions{})
		require.NoError(t, err)
		wg.Go(func() {
			for range 10 {
				resp, err := client.Get(upstream.URL)
				if err != nil {
					t.Error(err)
					return
				}
				body, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil || string(body) != test.want {
					t.Errorf("unexpected per-user header: %q (%v)", body, err)
				}
			}
		})
	}
	wg.Wait()
}

func TestOpenAPIKubernetesFiles(t *testing.T) {
	config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret-key"})
	backend := newTestKubernetesBackend(t)
	backend.openAPIImage = "openapi-mcp:test"
	objects, err := backend.k8sObjects(context.Background(), config)
	require.NoError(t, err)
	files := findSecret(t, objects, "openapi-test-mcp-files")
	require.Equal(t, storedOpenAPISchema, string(files.Data["openapi-test-OPENAPI_SPEC_FILE"]))
	deployment := findDeployment(t, objects, "openapi-test")
	require.Equal(t, "openapi-mcp:test", deployment.Spec.Template.Spec.Containers[0].Image)
	require.Empty(t, deployment.Spec.Template.Spec.Containers[0].Command)
	require.Empty(t, deployment.Spec.Template.Spec.Containers[0].Args)
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		require.NotEqual(t, "run-file", volume.Name, "OpenAPI does not use the stdio wrapper")
	}
	require.EqualValues(t, 8080, deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	all, err := json.Marshal(objects)
	require.NoError(t, err)
	require.NotContains(t, string(all), "secret-key")
	for _, obj := range objects {
		if secret, ok := obj.(*corev1.Secret); ok {
			for _, value := range secret.Data {
				require.NotContains(t, string(value), "secret-key")
			}
			if path, ok := secret.Data["OPENAPI_SPEC_FILE"]; ok {
				require.Equal(t, "/files/openapi-test-OPENAPI_SPEC_FILE", string(path))
			}
		}
	}
	config.Files = append(config.Files, File{
		EnvKey: "SECOND_FILE",
		Data:   strings.Repeat("x", corev1.MaxSecretSize-len(storedOpenAPISchema)),
	})
	_, err = backend.k8sObjects(context.Background(), config)
	require.NoError(t, err, "combined files at the limit are valid")
	config.Files[1].Data += "x"
	_, err = backend.k8sObjects(context.Background(), config)
	require.ErrorContains(t, err, "combined mounted files")
}

func TestOpenAPIKubernetesMaximumSchema(t *testing.T) {
	const marker = `"paths":{}`
	const withPadding = `"x-padding":"","paths":{}`
	padding := strings.Repeat("x", openapi.MaxSchemaBytes-len(storedOpenAPISchema)-(len(withPadding)-len(marker)))
	schema := strings.Replace(storedOpenAPISchema, marker, `"x-padding":"`+padding+`","paths":{}`, 1)
	require.Len(t, schema, openapi.MaxSchemaBytes)

	server := openAPITestServer()
	server.Spec.Manifest.OpenAPIConfig.Schema = testOpenAPISchema(schema)
	config := openAPITestConfig(t, server, map[string]string{"Authorization": "key"})

	backend := newTestKubernetesBackend(t)
	backend.openAPIImage = "openapi-mcp:test"
	objects, err := backend.k8sObjects(t.Context(), config)
	require.NoError(t, err)

	files := findSecret(t, objects, "openapi-test-mcp-files")
	require.Equal(t, schema, string(files.Data["openapi-test-OPENAPI_SPEC_FILE"]))
	require.Len(t, files.Data["openapi-test-OPENAPI_SPEC_FILE"], corev1.MaxSecretSize)
}

func TestOpenAPIBackendImageSelection(t *testing.T) {
	config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "key"})
	// A manifest cannot override the operator-owned runtime image.
	config.ContainerImage = "ignored:latest"
	docker := dockerBackend{
		containerizedBaseImage: "stdio:test",
		openAPIImage:           "openapi:test",
	}
	require.Equal(t, "openapi:test", docker.deploymentImage(config))
	docker.openAPIImage = "openapi:updated"
	require.Equal(t, "openapi:updated", docker.deploymentImage(config))

	kubernetes := newTestKubernetesBackend(t)
	kubernetes.openAPIImage = "openapi:test"
	objects, err := kubernetes.k8sObjects(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, "openapi:test", findDeployment(t, objects, config.MCPServerName).Spec.Template.Spec.Containers[0].Image)
	kubernetes.openAPIImage = "openapi:updated"
	objects, err = kubernetes.k8sObjects(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, "openapi:updated", findDeployment(t, objects, config.MCPServerName).Spec.Template.Spec.Containers[0].Image)
}

func TestOpenAPIDockerConnectionHeaders(t *testing.T) {
	config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret-key"})
	backend := dockerBackend{}
	containerSummary := &container.Summary{
		ID: "container-id",
		Ports: []container.Port{{
			PrivatePort: 8080,
			PublicPort:  12345,
		}},
	}
	connection, err := backend.buildServerConfig(config, containerSummary, 8080, false)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:12345/mcp", connection.URL)
	require.Equal(t, config.Headers, connection.Headers)
	require.Empty(t, connection.Env)
	require.Empty(t, connection.Files)

	config.Runtime = types.RuntimeContainerized
	connection, err = backend.buildServerConfig(config, containerSummary, 8080, false)
	require.NoError(t, err)
	require.Empty(t, connection.Headers)
}

func TestOpenAPIKubernetesConnectionHeaders(t *testing.T) {
	config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "secret-key"})
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
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
	).Build()
	deployedConfig := config
	deployedConfig.ContainerImage = "example.com/openapi:v1"
	backend := kubernetesBackend{
		client:           client,
		cachedClient:     client,
		openAPIImage:     "example.com/openapi:v1",
		mcpNamespace:     "mcp",
		mcpClusterDomain: "cluster.local",
		deploymentCache: map[string]*kubernetesDeploymentCacheEntry{
			config.MCPServerName: {
				hash:    serverID(deployedConfig) + utils.Digest(config.Files),
				podName: "existing-pod",
			},
		},
	}
	connection, err := backend.ensureServerDeployment(t.Context(), config)
	require.NoError(t, err)
	require.Equal(t, "http://openapi-test.mcp.svc.cluster.local/mcp", connection.URL)
	require.Equal(t, config.Headers, connection.Headers)

	config.Runtime = types.RuntimeContainerized
	backend.setDeploymentCache(config.MCPServerName, kubernetesDeploymentCacheEntry{
		hash:    serverID(config) + utils.Digest(config.Files),
		podName: "existing-pod",
	})
	connection, err = backend.ensureServerDeployment(t.Context(), config)
	require.NoError(t, err)
	require.Empty(t, connection.Headers)
}

func (b *openAPILifecycleBackend) ensureServerDeployment(_ context.Context, config ServerConfig) (ServerConfig, error) {
	b.configured = config
	return config, nil
}

func (b *openAPILifecycleBackend) deployServer(_ context.Context, config ServerConfig) error {
	b.configured = config
	return nil
}

func (b *openAPILifecycleBackend) restartServer(_ context.Context, config ServerConfig) error {
	b.configured = config
	return nil
}

func (b *openAPILifecycleBackend) getServerDetails(_ context.Context, _ string) (types.MCPServerDetails, error) {
	if b.configured.Runtime == "" {
		return types.MCPServerDetails{}, ErrServerNotRunning
	}
	return types.MCPServerDetails{}, nil
}

func (*openAPILifecycleBackend) streamServerLogs(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func TestOpenAPILifecyclePreservesRuntime(t *testing.T) {
	for _, action := range []string{"launch", "restart", "details", "logs"} {
		t.Run(action, func(t *testing.T) {
			backend := &openAPILifecycleBackend{}
			manager := SessionManager{
				backend:                   backend,
				remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true, AllowPrivateIPMCP: true, AllowLinkLocalMCP: true},
			}
			config := openAPITestConfig(t, openAPITestServer(), map[string]string{"Authorization": "key"})
			var err error
			switch action {
			case "launch":
				_, err = manager.LaunchServer(t.Context(), config)
			case "restart":
				err = manager.RestartServerDeployment(t.Context(), config)
			case "details":
				_, err = manager.GetServerDetails(t.Context(), config)
			case "logs":
				var logs io.ReadCloser
				logs, err = manager.StreamServerLogs(t.Context(), config)
				if logs != nil {
					require.NoError(t, logs.Close())
				}
			}
			require.NoError(t, err)
			require.Equal(t, types.RuntimeOpenAPI, backend.configured.Runtime)
			require.Empty(t, backend.configured.ContainerImage)
			require.Equal(t, config.Files, backend.configured.Files)
		})
	}
}

func TestOpenAPIDestinationPolicyAtDeployment(t *testing.T) {
	for _, action := range []string{"launch", "restart", "details", "logs"} {
		t.Run(action, func(t *testing.T) {
			for _, test := range []struct {
				name    string
				url     string
				devMode bool
				allow   RemoteMCPURLValidationConfig
				wantErr string
			}{
				{
					name:    "private destination blocked",
					url:     "https://10.0.0.1",
					wantErr: "blocked private",
				},
				{
					name:    "HTTP blocked in production without credentials",
					url:     "http://93.184.216.34",
					wantErr: "API destination must use HTTPS",
				},
				{
					name:    "development honors network restrictions",
					url:     "http://127.0.0.1:9999",
					devMode: true,
					wantErr: "blocked loopback",
				},
				{
					name:    "development with localhost allowed",
					url:     "http://127.0.0.1:9999",
					devMode: true,
					allow:   RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
				},
				{
					name:  "private destination explicitly allowed",
					url:   "https://10.0.0.1",
					allow: RemoteMCPURLValidationConfig{AllowPrivateIPMCP: true},
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					backend := &openAPILifecycleBackend{}
					manager := SessionManager{
						backend:                   backend,
						devMode:                   test.devMode,
						remoteURLValidationConfig: test.allow,
					}
					server := openAPITestServer()
					server.Spec.Manifest.Config = nil
					server.Spec.Manifest.OpenAPIConfig.BaseURL = test.url
					config := openAPITestConfig(t, server, nil)
					var err error
					switch action {
					case "launch":
						_, err = manager.LaunchServer(t.Context(), config)
					case "restart":
						err = manager.RestartServerDeployment(t.Context(), config)
					case "details":
						_, err = manager.GetServerDetails(t.Context(), config)
					case "logs":
						var logs io.ReadCloser
						logs, err = manager.StreamServerLogs(t.Context(), config)
						if logs != nil {
							require.NoError(t, logs.Close())
						}
					}
					if test.wantErr != "" {
						require.ErrorContains(t, err, test.wantErr)
						require.Empty(t, backend.configured.Runtime, "backend must not deploy a rejected destination")
					} else {
						require.NoError(t, err)
						require.Equal(t, types.RuntimeOpenAPI, backend.configured.Runtime)
					}

					// Both manifest entry points receive the same startup settings.
					options := manager.ValidationOptions()
					for _, validationErr := range []error{
						ValidateServerManifest(t.Context(), server.Spec.Manifest, false, options),
						ValidateCatalogEntryManifest(t.Context(), server.Spec.Manifest.ConvertToCatalogEntry(), false, options),
					} {
						if test.wantErr != "" {
							require.ErrorContains(t, validationErr, test.wantErr)
						} else {
							require.NoError(t, validationErr)
						}
					}
				})
			}
		})
	}
}

func TestOpenAPICredentialsMustBeRequired(t *testing.T) {
	for _, test := range []struct {
		name        string
		supplied    bool
		value       string
		userAllowed bool
	}{
		{
			name: "missing value",
		},
		{
			name:     "supplied value",
			supplied: true,
		},
		{
			name:  "static value",
			value: "Bearer static-key",
		},
		{
			name:        "per-user",
			userAllowed: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := openAPITestServer()
			server.Spec.Manifest.OpenAPIConfig.BaseURL = "https://93.184.216.34"

			header := &server.Spec.Manifest.Config[0]
			header.Required = false
			header.Value = test.value
			header.UserAllowed = test.userAllowed

			credentials := map[string]string{}
			if test.supplied {
				credentials["Authorization"] = "supplied-key"
			}

			_, _, deploymentErr := ServerToServerConfig(server, nil, "user-1", "test", "default", credentials)
			for _, err := range []error{
				ValidateServerManifest(t.Context(), server.Spec.Manifest, test.userAllowed, ValidationOptions{}),
				ValidateCatalogEntryManifest(t.Context(), server.Spec.Manifest.ConvertToCatalogEntry(), test.userAllowed, ValidationOptions{}),
				deploymentErr,
			} {
				require.ErrorContains(t, err, "credential header Authorization must be marked required")
			}
		})
	}
}
