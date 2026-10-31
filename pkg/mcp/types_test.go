package mcp

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestServerToServerConfigComponentAuditLogs(t *testing.T) {
	for _, test := range []struct {
		name      string
		spec      v1.MCPServerSpec
		component bool
	}{
		{
			name: "standalone",
		},
		{
			name: "shared vMCP component",
			spec: v1.MCPServerSpec{
				VMCPID:          "vmcp1shared",
				VMCPComponentID: "component",
			},
			component: true,
		},
		{
			name: "dedicated vMCP component",
			spec: v1.MCPServerSpec{
				VMCPInstanceID:  "vmcpi1user",
				VMCPComponentID: "component",
			},
			component: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := v1.MCPServer{
				Name: "ms1server",
				Spec: test.spec,
			}
			server.Spec.Manifest = types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
			}
			config, _, err := ServerToServerConfig(server, nil, "user", "scope", "default", nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if config.ComponentMCPServer != test.component {
				t.Fatalf("ComponentMCPServer = %v, want %v", config.ComponentMCPServer, test.component)
			}
			if ignored := config.AuditLogMetadata[AuditLogIgnore] == "true"; ignored != test.component {
				t.Fatalf("audit ignore = %v, want %v", ignored, test.component)
			}
			if test.component {
				if len(config.AuditLogMetadata) != 1 {
					t.Fatalf("unexpected component audit attribution: %#v", config.AuditLogMetadata)
				}
			} else if config.AuditLogMetadata["mcpID"] != server.Name {
				t.Fatalf("missing server audit attribution: %#v", config.AuditLogMetadata)
			}
		})
	}
}

func TestCoreResourceRequirements(t *testing.T) {
	t.Run("nil resources returns nil", func(t *testing.T) {
		result, err := CoreResourceRequirements(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil result, got %#v", result)
		}
	})

	t.Run("empty resources returns non nil empty requirements", func(t *testing.T) {
		result, err := CoreResourceRequirements(&types.MCPResourceRequirements{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Requests) != 0 || len(result.Limits) != 0 {
			t.Fatalf("expected empty requirements, got %#v", result)
		}
	})

	t.Run("valid resources are converted", func(t *testing.T) {
		result, err := CoreResourceRequirements(&types.MCPResourceRequirements{
			Requests: types.MCPResourceRequests{CPU: "250m", Memory: "512Mi"},
			Limits:   types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertQuantityEqual(t, result.Requests[corev1.ResourceCPU], resource.MustParse("250m"), "cpu request")
		assertQuantityEqual(t, result.Requests[corev1.ResourceMemory], resource.MustParse("512Mi"), "memory request")
		assertQuantityEqual(t, result.Limits[corev1.ResourceCPU], resource.MustParse("1"), "cpu limit")
		assertQuantityEqual(t, result.Limits[corev1.ResourceMemory], resource.MustParse("1Gi"), "memory limit")
	})

	t.Run("invalid quantity returns contextual error", func(t *testing.T) {
		_, err := CoreResourceRequirements(&types.MCPResourceRequirements{
			Requests: types.MCPResourceRequests{CPU: "not-a-quantity"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `invalid CPU request "not-a-quantity"`) {
			t.Fatalf("expected contextual error, got %v", err)
		}
	})
}

func assertQuantityEqual(t *testing.T, got, want resource.Quantity, name string) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Fatalf("%s = %s, want %s", name, got.String(), want.String())
	}
}

func TestServerToServerConfig_ContainerizedHealthzPath(t *testing.T) {
	baseURL := "http://localhost:8080"
	mcpServer := v1.MCPServer{
		Name: "test-server",
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:       "test-image",
					Port:        8080,
					Path:        "/mcp",
					HealthzPath: "/healthz",
				},
			},
		},
	}

	config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("expected no missing config, got %v", missing)
	}
	if config.HealthzPath != "/healthz" {
		t.Fatalf("expected healthz path /healthz, got %q", config.HealthzPath)
	}
}

func TestServerToServerConfig_ConfigurationOptions(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "example-server"},
		Config: []types.MCPConfig{{Usage: types.Env,
			Key:      "REGION",
			Required: true,
			Options:  []types.MCPConfigurationOption{{Name: "US", Value: "us"}}}},
	}
	server := v1.MCPServer{Name: "test-server", Spec: v1.MCPServerSpec{Manifest: manifest}}

	_, missing, err := ServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", "test-scope", "test-catalog", nil, false)
	if err != nil {
		t.Fatalf("expected missing option to be reported without an error, got %v", err)
	}
	if !slices.Equal(missing, []string{"REGION"}) {
		t.Fatalf("expected REGION to be missing, got %v", missing)
	}

	_, _, err = ServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", "test-scope", "test-catalog", map[string]string{"REGION": "stale"}, false)
	if err == nil || !strings.Contains(err.Error(), `env "REGION" value "stale" is not one of the configured options`) {
		t.Fatalf("expected invalid option error, got %v", err)
	}
}

func TestServerToServerConfigIgnoresStalePerUserOptionValues(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
		Config: []types.MCPConfig{
			{
				Key:         "X-REGION",
				Usage:       types.Header,
				UserAllowed: true,
				Required:    true,
				Options:     []types.MCPConfigurationOption{{Name: "US", Value: "us"}},
			},
		},
	}
	stale := map[string]string{"X-REGION": "stale"}
	server := v1.MCPServer{Spec: v1.MCPServerSpec{Manifest: manifest}}
	config, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", stale, false)
	if err != nil || len(missing) != 0 || len(config.Headers) != 0 || !slices.Equal(config.PassthroughHeaderNames, []string{"X-REGION"}) {
		t.Fatalf("unexpected shared configuration: config=%#v missing=%v err=%v", config, missing, err)
	}
	instance := v1.MCPServerInstance{Spec: v1.MCPServerInstanceSpec{Config: manifest.UserConfig()}}
	names, values, missing := serverInstanceHeaders(instance, stale)
	if len(names) != 0 || len(values) != 0 || !slices.Equal(missing, []string{"X-REGION"}) {
		t.Fatalf("invalid user selection was accepted: names=%v values=%v missing=%v", names, values, missing)
	}
	names, values, missing = serverInstanceHeaders(instance, map[string]string{"X-REGION": "us"})
	if !slices.Equal(names, []string{"X-REGION"}) || !slices.Equal(values, []string{"us"}) || len(missing) != 0 {
		t.Fatalf("valid user selection was rejected: names=%v values=%v missing=%v", names, values, missing)
	}
	server.Spec.Manifest.Config[0].UserAllowed = false
	if _, _, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", stale, false); err == nil {
		t.Fatal("expected stale server-owned option value to be rejected")
	}
}

func TestSystemServerToServerConfig_ConfigurationOptions(t *testing.T) {
	manifest := types.SystemMCPServerManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "example-server"},
		Config: []types.MCPConfig{{Usage: types.Env,
			Key:      "REGION",
			Required: true,
			Options:  []types.MCPConfigurationOption{{Name: "US", Value: "us"}}}},
	}
	server := v1.SystemMCPServer{Name: "test-system-server", Spec: v1.SystemMCPServerSpec{Manifest: manifest}}

	_, missing, err := SystemServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", nil)
	if err != nil {
		t.Fatalf("expected missing option to be reported without an error, got %v", err)
	}
	if !slices.Equal(missing, []string{"REGION"}) {
		t.Fatalf("expected REGION to be missing, got %v", missing)
	}

	_, _, err = SystemServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", map[string]string{"REGION": "stale"})
	if err == nil || !strings.Contains(err.Error(), `env "REGION" value "stale" is not one of the configured options`) {
		t.Fatalf("expected invalid option error, got %v", err)
	}
}

func TestServerToServerConfig_UsesStaticCatalogEnvValue(t *testing.T) {
	baseURL := "http://localhost:8080"
	mcpServer := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "example-server",
					Args:    []string{"--token=${CATALOG_TOKEN}"},
				},
				Config: []types.MCPConfig{{Usage: types.Env,
					Key:      "CATALOG_TOKEN",
					Value:    "catalog-value",
					Prefix:   "Bearer ",
					Required: true,
				}},
			},
		},

		Name: "test-server",
	}

	config, missing, err := ServerToServerConfig(
		mcpServer,
		mcpServer.ValidConnectURLs(baseURL),
		"test-user-id",
		"test-scope",
		"test-catalog",
		nil, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("expected no missing config, got %v", missing)
	}
	if !slices.Equal(config.Env, []string{"CATALOG_TOKEN=catalog-value"}) {
		t.Fatalf("expected static env value, got %v", config.Env)
	}
	if !slices.Equal(config.Args, []string{"example-server", "--token=catalog-value"}) {
		t.Fatalf("expected static env value to expand runtime arguments, got %v", config.Args)
	}
}

func TestServerToServerConfig_InterpolatedValueIsNotInjected(t *testing.T) {
	server := v1.MCPServer{
		Name: "test-server",
		Spec: v1.MCPServerSpec{Manifest: types.MCPServerManifest{
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "example-server", Args: []string{"--tag=${TAG}"}},
			Config:    []types.MCPConfig{{Key: "TAG", Value: "v1", Usage: types.Interpolated}},
		}},
	}

	config, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 || len(config.Env) != 0 || !slices.Equal(config.Args, []string{"example-server", "--tag=v1"}) {
		t.Fatalf("config = %#v, missing = %v", config, missing)
	}
}

func TestFlattenedServerConfigurationUsages(t *testing.T) {
	fields := []types.MCPConfig{
		{
			Key:   "TOKEN",
			Usage: types.Env,
			Value: "token",
		},
		{
			Key:   "TAG",
			Usage: types.Interpolated,
			Value: "v1",
		},
		{
			Key:   "STATIC_FILE",
			Usage: types.File,
			Value: "static contents",
		},
		{
			Key:   "DYNAMIC_FILE",
			Usage: types.DynamicFile,
			Value: "dynamic contents",
		},
	}
	runtime := &types.NPXRuntimeConfig{Package: "example-server", Args: []string{"--tag=${TAG}", "--token=${TOKEN}"}}
	server := v1.MCPServer{Spec: v1.MCPServerSpec{Manifest: types.MCPServerManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: runtime,
		Config:    fields,
	}}}
	ordinary, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", nil, false)
	if err != nil || len(missing) != 0 {
		t.Fatalf("server configuration: missing=%v err=%v", missing, err)
	}
	systemServer := v1.SystemMCPServer{Spec: v1.SystemMCPServerSpec{Manifest: types.SystemMCPServerManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: runtime,
		Config:    fields,
	}}}
	systemConfig, missing, err := SystemServerToServerConfig(systemServer, nil, "user", nil)
	if err != nil || len(missing) != 0 {
		t.Fatalf("system configuration: missing=%v err=%v", missing, err)
	}
	for _, config := range []ServerConfig{ordinary, systemConfig} {
		if !slices.Equal(config.Env, []string{"TOKEN=token"}) || !slices.Equal(config.Args, []string{"example-server", "--tag=v1", "--token=token"}) {
			t.Fatalf("unexpected flattened configuration: env=%v args=%v", config.Env, config.Args)
		}
		if len(config.Files) != 2 || config.Files[0].EnvKey != "STATIC_FILE" || config.Files[0].Dynamic || config.Files[0].Data != "static contents" || config.Files[1].EnvKey != "DYNAMIC_FILE" || !config.Files[1].Dynamic || config.Files[1].Data != "dynamic contents" {
			t.Fatalf("unexpected file configuration: %#v", config.Files)
		}
	}
}

func TestServerToServerConfig_StartupTimeoutFromRuntimeConfig(t *testing.T) {
	baseURL := "http://localhost:8080"
	mcpServer := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:                 "test-image",
					Port:                  8080,
					Path:                  "/mcp",
					StartupTimeoutSeconds: 90,
				},
			},
		},

		Name: "test-server"}

	config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("expected no missing config, got %v", missing)
	}
	if config.StartupTimeout != 90*time.Second {
		t.Fatalf("expected startup timeout 90s, got %s", config.StartupTimeout)
	}
}

func TestServerToServerConfig_MultiUserPassthroughHeaders(t *testing.T) {
	baseURL := "http://localhost:8080"
	tests := []struct {
		name     string
		config   []types.MCPConfig
		expected []string
	}{
		{
			name:     "no multi-user config",
			expected: nil,
		},
		{
			name: "user-defined headers",
			config: []types.MCPConfig{
				{
					Key:         "X-Tenant-ID",
					Usage:       types.Header,
					UserAllowed: true,
					Required:    true,
				},
				{
					Key:         "X-Account-ID",
					Usage:       types.Header,
					UserAllowed: true,
				},
			},
			expected: []string{"X-Tenant-ID", "X-Account-ID"},
		},
		{
			name: "per-user values are not shared",
			config: []types.MCPConfig{
				{
					Key:         "X-Tenant-ID",
					Usage:       types.Header,
					UserAllowed: true,
					Required:    true,
					Value:       "tenant",
				},
			},
			expected: []string{"X-Tenant-ID"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{
							URL: "https://example.com/mcp",
						},
						Config: tt.config,
					},
				},

				Name: "test-server"}

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(missing) > 0 {
				t.Fatalf("expected no missing config, got %v", missing)
			}
			if len(config.Headers) != 0 || len(config.Env) != 0 {
				t.Fatalf("per-user headers must not be shared: headers=%v env=%v", config.Headers, config.Env)
			}

			if !slices.Equal(config.PassthroughHeaderNames, tt.expected) {
				t.Fatalf("expected passthrough header names %v, got %v", tt.expected, config.PassthroughHeaderNames)
			}
		})
	}
}

func TestServerToServerConfig_StaticHeaders_Remote(t *testing.T) {
	baseURL := "http://localhost:8080"
	tests := []struct {
		name            string
		headers         []types.MCPConfig
		credEnv         map[string]string
		expectedHeaders []string
		expectedMissing []string
	}{
		{
			name: "static header only",
			headers: []types.MCPConfig{
				{
					Usage: types.Header,
					Key:   "Authorization",
					Value: "Bearer static-token",
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "user-configurable header only",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "X-API-Key",
					Required: true,
				},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "mixed static and user-configurable",
			headers: []types.MCPConfig{
				{
					Usage: types.Header,
					Key:   "Authorization",
					Value: "Bearer static-token",
				},
				{
					Usage:    types.Header,
					Key:      "X-API-Key",
					Required: true,
				},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"Authorization=Bearer static-token", "X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "missing required user-configurable header",
			headers: []types.MCPConfig{
				{
					Usage: types.Header,
					Key:   "Authorization",
					Value: "Bearer static-token",
				},
				{
					Usage:    types.Header,
					Key:      "X-API-Key",
					Required: true,
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{"X-API-Key"},
		},
		{
			name: "optional user-configurable header missing",
			headers: []types.MCPConfig{
				{
					Usage: types.Header,
					Key:   "Authorization",
					Value: "Bearer static-token",
				},
				{
					Usage:    types.Header,
					Key:      "X-Optional",
					Required: false,
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "static header overrides credential",
			headers: []types.MCPConfig{
				{
					Usage: types.Header,
					Key:   "Authorization",
					Value: "Bearer static-token",
				},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty static value falls back to credential",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Value:    "",
					Required: true,
				},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer user-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty credential value is ignored",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Value:    "",
					Required: true,
				},
			},
			credEnv:         map[string]string{"Authorization": ""},
			expectedHeaders: []string{},
			expectedMissing: []string{"Authorization"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{
							URL: "https://example.com/mcp",
						},
						Config: tt.headers,
					},
				},

				Name: "test-server"}

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv, false)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare missing headers
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing headers, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing header %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}

			// Verify the URL was set correctly
			if config.URL != "https://example.com/mcp" {
				t.Errorf("expected URL https://example.com/mcp, got %s", config.URL)
			}

			// Verify the runtime was set correctly
			if config.Runtime != types.RuntimeRemote {
				t.Errorf("expected runtime %v, got %v", types.RuntimeRemote, config.Runtime)
			}

			// Verify the audiences were set correctly
			expectedAudiences := mcpServer.ValidConnectURLs(baseURL)
			if len(config.Audiences) != len(expectedAudiences) {
				t.Errorf("expected %d audiences, got %d: expected %v, got %v", len(expectedAudiences), len(config.Audiences), expectedAudiences, config.Audiences)
			} else {
				for i, expected := range expectedAudiences {
					if config.Audiences[i] != expected {
						t.Errorf("audience %d: expected %s, got %s", i, expected, config.Audiences[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_RemoteTunnelName(t *testing.T) {
	mcpServer := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:        "http://127.0.0.1:8080/mcp",
					TunnelName: "mcptunnel-office",
				},
			},
		},

		Name: "test-server"}

	config, missing, err := ServerToServerConfig(mcpServer, nil, "test-user-id", "test-scope", "test-catalog", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected no missing config, got %v", missing)
	}
	if config.URL != mcpServer.Spec.Manifest.RemoteConfig.URL {
		t.Fatalf("URL = %q, want %q", config.URL, mcpServer.Spec.Manifest.RemoteConfig.URL)
	}
	if config.TunnelName != mcpServer.Spec.Manifest.RemoteConfig.TunnelName {
		t.Fatalf("TunnelName = %q, want %q", config.TunnelName, mcpServer.Spec.Manifest.RemoteConfig.TunnelName)
	}
}

func TestServerToServerConfig_WithPrefix(t *testing.T) {
	baseURL := "http://localhost:8080"
	tests := []struct {
		name            string
		headers         []types.MCPConfig
		env             []types.MCPConfig
		credEnv         map[string]string
		expectedHeaders []string
		expectedEnv     []string
		expectedMissing []string
	}{
		{
			name: "header with prefix applied to user value",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Prefix:   "Bearer ",
					Required: true,
				},
			},
			credEnv:         map[string]string{"Authorization": "my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "header with prefix not applied to static value",
			headers: []types.MCPConfig{
				{
					Usage:  types.Header,
					Key:    "Authorization",
					Value:  "static-token",
					Prefix: "Bearer ",
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=static-token"},
			expectedMissing: []string{},
		},
		{
			name: "env var with Bearer prefix",
			env: []types.MCPConfig{
				{
					Usage:    types.Env,
					Key:      "API_KEY",
					Prefix:   "Bearer ",
					Required: true,
				},
			},
			credEnv:         map[string]string{"API_KEY": "secret-key-123"},
			expectedEnv:     []string{"API_KEY=Bearer secret-key-123"},
			expectedMissing: []string{},
		},
		{
			name: "env var with sk- prefix (OpenAI style)",
			env: []types.MCPConfig{
				{
					Usage:    types.Env,
					Key:      "OPENAI_API_KEY",
					Prefix:   "sk-",
					Required: true,
				},
			},
			credEnv:         map[string]string{"OPENAI_API_KEY": "proj-abc123xyz"},
			expectedEnv:     []string{"OPENAI_API_KEY=sk-proj-abc123xyz"},
			expectedMissing: []string{},
		},
		{
			name: "multiple headers and env vars with different prefixes",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Prefix:   "Bearer ",
					Required: true,
				},
				{
					Usage:    types.Header,
					Key:      "X-API-Key",
					Prefix:   "Key ",
					Required: true,
				},
			},
			env: []types.MCPConfig{
				{
					Usage:    types.Env,
					Key:      "TOKEN",
					Prefix:   "Token ",
					Required: true,
				},
				{
					Usage:    types.Env,
					Key:      "SECRET",
					Required: true,
				}, // No prefix
			},
			credEnv: map[string]string{
				"Authorization": "auth-token",
				"X-API-Key":     "api-key-value",
				"TOKEN":         "token-value",
				"SECRET":        "secret-value",
			},
			expectedHeaders: []string{"Authorization=Bearer auth-token", "X-API-Key=Key api-key-value"},
			expectedEnv:     []string{"TOKEN=Token token-value", "SECRET=secret-value"},
			expectedMissing: []string{},
		},
		{
			name: "prefix not applied when value is empty",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Prefix:   "Bearer ",
					Required: true,
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{},
			expectedMissing: []string{"Authorization"},
		},
		{
			name: "prefix not duplicated when user already included it in header",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Prefix:   "Bearer ",
					Required: true,
				},
			},
			credEnv:         map[string]string{"Authorization": "Bearer my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "prefix not duplicated when user already included it in env var",
			env: []types.MCPConfig{
				{
					Usage:    types.Env,
					Key:      "API_KEY",
					Prefix:   "sk-",
					Required: true,
				},
			},
			credEnv:         map[string]string{"API_KEY": "sk-proj-abc123"},
			expectedEnv:     []string{"API_KEY=sk-proj-abc123"},
			expectedMissing: []string{},
		},
		{
			name: "mixed - some with prefix already included, some without",
			headers: []types.MCPConfig{
				{
					Usage:    types.Header,
					Key:      "Authorization",
					Prefix:   "Bearer ",
					Required: true,
				},
			},
			env: []types.MCPConfig{
				{
					Usage:    types.Env,
					Key:      "API_KEY",
					Prefix:   "sk-",
					Required: true,
				},
				{
					Usage:    types.Env,
					Key:      "TOKEN",
					Prefix:   "Token ",
					Required: true,
				},
			},
			credEnv: map[string]string{
				"Authorization": "Bearer already-has-it",
				"API_KEY":       "proj-needs-it",
				"TOKEN":         "Token already-has-it",
			},
			expectedHeaders: []string{"Authorization=Bearer already-has-it"},
			expectedEnv:     []string{"API_KEY=sk-proj-needs-it", "TOKEN=Token already-has-it"},
			expectedMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{
							URL: "https://example.com/mcp",
						},
						Config: append(tt.env, tt.headers...),
					},
				},

				Name: "test-server"}

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv, false)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare env vars
			if len(config.Env) != len(tt.expectedEnv) {
				t.Errorf("expected %d env vars, got %d: expected %v, got %v", len(tt.expectedEnv), len(config.Env), tt.expectedEnv, config.Env)
			} else {
				for i, expected := range tt.expectedEnv {
					if config.Env[i] != expected {
						t.Errorf("env var %d: expected %s, got %s", i, expected, config.Env[i])
					}
				}
			}

			// Compare missing
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing items, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing item %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_StaticHeaders_EdgeCases(t *testing.T) {
	baseURL := "http://localhost:8080"
	tests := []struct {
		name            string
		manifest        types.MCPServerManifest
		credEnv         map[string]string
		expectedHeaders []string
		expectedMissing []string
		expectError     bool
	}{
		{
			name: "header with special characters in value",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{
						Usage: types.Header,
						Key:   "Authorization",
						Value: "Bearer token-with-special!@#$%^&*()characters",
					},
				},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer token-with-special!@#$%^&*()characters"},
			expectedMissing: []string{},
			expectError:     false,
		},
		{
			name: "nil remote config should return error",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: nil,
			},
			credEnv:     map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Spec: v1.MCPServerSpec{
					Manifest: tt.manifest,
				},

				Name: "test-server"}

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv, false)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Compare headers
			if len(config.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d: expected %v, got %v", len(tt.expectedHeaders), len(config.Headers), tt.expectedHeaders, config.Headers)
			} else {
				for i, expected := range tt.expectedHeaders {
					if config.Headers[i] != expected {
						t.Errorf("header %d: expected %s, got %s", i, expected, config.Headers[i])
					}
				}
			}

			// Compare missing headers
			if len(missing) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing headers, got %d: expected %v, got %v", len(tt.expectedMissing), len(missing), tt.expectedMissing, missing)
			} else {
				for i, expected := range tt.expectedMissing {
					if missing[i] != expected {
						t.Errorf("missing header %d: expected %s, got %s", i, expected, missing[i])
					}
				}
			}
		})
	}
}

func TestServerToServerConfig_RemoteURLTemplate(t *testing.T) {
	tests := []struct {
		name         string
		remoteConfig *types.RemoteRuntimeConfig
		config       []types.MCPConfig
		credEnv      map[string]string
		expectedURL  string
		expectedMiss []string
	}{
		{
			name: "template expanded from configuration",
			remoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URLTemplate: "https://${MIXPANEL_HOST}/mcp",
			},
			config: []types.MCPConfig{
				{
					Key:      "MIXPANEL_HOST",
					Name:     "Data Region",
					Usage:    types.Env,
					Required: true,
				},
			},
			credEnv:     map[string]string{"MIXPANEL_HOST": "mcp.mixpanel.com"},
			expectedURL: "https://mcp.mixpanel.com/mcp",
		},
		{
			name: "template expanded from a static catalog value",
			remoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URLTemplate: "https://${MIXPANEL_HOST}/mcp",
			},
			config: []types.MCPConfig{
				{
					Key:   "MIXPANEL_HOST",
					Name:  "Data Region",
					Usage: types.Env,
					Value: "mcp.eu.mixpanel.com",
				},
			},
			expectedURL: "https://mcp.eu.mixpanel.com/mcp",
		},
		{
			name: "unconfigured references are reported once as missing",
			remoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URLTemplate: "https://${MIXPANEL_HOST}/mcp/${MIXPANEL_HOST}",
			},
			config: []types.MCPConfig{
				{
					Key:      "MIXPANEL_HOST",
					Name:     "Data Region",
					Usage:    types.Env,
					Required: true,
				},
			},
			expectedMiss: []string{"MIXPANEL_HOST"},
		},
		{
			name: "persisted URL wins over the template",
			remoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URL:         "https://mcp.mixpanel.com/mcp",
				URLTemplate: "https://${MIXPANEL_HOST}/mcp",
			},
			config: []types.MCPConfig{
				{
					Key:   "MIXPANEL_HOST",
					Name:  "Data Region",
					Usage: types.Env,
				},
			},
			credEnv:     map[string]string{"MIXPANEL_HOST": "mcp.eu.mixpanel.com"},
			expectedURL: "https://mcp.mixpanel.com/mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpServer := v1.MCPServer{
				Name: "test-server",
				Spec: v1.MCPServerSpec{
					VMCPComponentID: "component-1",
					Manifest: types.MCPServerManifest{
						Runtime:      types.RuntimeRemote,
						Config:       tt.config,
						RemoteConfig: tt.remoteConfig,
					},
				},
			}

			config, missing, err := ServerToServerConfig(mcpServer, nil, "test-user-id", "test-scope", "test-catalog", tt.credEnv, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(missing, tt.expectedMiss) {
				t.Fatalf("missing = %v, want %v", missing, tt.expectedMiss)
			}
			if config.URL != tt.expectedURL {
				t.Fatalf("URL = %q, want %q", config.URL, tt.expectedURL)
			}
		})
	}
}

func TestExpandURLTemplatePinsTemplateHost(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		values      map[string]string
		expectedURL string
		expectError bool
	}{
		{
			name:        "host label",
			template:    "https://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "us"},
			expectedURL: "https://us.api.example.com/mcp",
		},
		{
			name:        "multi-label host value stays under the admin's domain",
			template:    "https://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "attacker.example"},
			expectedURL: "https://attacker.example.api.example.com/mcp",
		},
		{
			name:        "port",
			template:    "https://api.example.com:${PORT}/mcp",
			values:      map[string]string{"PORT": "8443"},
			expectedURL: "https://api.example.com:8443/mcp",
		},
		{
			// The admin delegated the whole host to the user.
			name:        "whole host",
			template:    "https://${HOST}/mcp",
			values:      map[string]string{"HOST": "anything.example"},
			expectedURL: "https://anything.example/mcp",
		},
		{
			// The user already chooses the whole host, so a value that brings its own path is
			// not a host change.
			name:        "whole host value with a path",
			template:    "https://${SERVER}",
			values:      map[string]string{"SERVER": "tenant.example.com/api/mcp"},
			expectedURL: "https://tenant.example.com/api/mcp",
		},
		{
			name:        "whole host value with userinfo",
			template:    "https://${SERVER}",
			values:      map[string]string{"SERVER": "user@tenant.example.com"},
			expectedURL: "https://user@tenant.example.com",
		},
		{
			// HOST may be admin-fixed, so a userinfo value must not end the authority early
			// even though the host part is a single variable.
			name:        "userinfo value moves a one-variable host",
			template:    "https://${TOKEN}@${HOST}/mcp",
			values:      map[string]string{"TOKEN": "x@evil.example/", "HOST": "api.example.com"},
			expectError: true,
		},
		{
			name:        "userinfo value before a one-variable host",
			template:    "https://${TOKEN}@${HOST}/mcp",
			values:      map[string]string{"TOKEN": "p@ss", "HOST": "api.example.com"},
			expectedURL: "https://p@ss@api.example.com/mcp",
		},
		{
			// Literal text next to the variable (here the port) means the host part is pinned.
			name:        "host variable with literal port moves the host",
			template:    "https://${HOST}:8443/mcp",
			values:      map[string]string{"HOST": "anything.example/x?"},
			expectError: true,
		},
		{
			// With no literal domain after the variable, a dotted value becomes the domain.
			name:        "value at the end of the hostname moves the domain",
			template:    "http://api.lab.${TLD}:9000/mcp",
			values:      map[string]string{"TLD": "attacker.lab.test"},
			expectError: true,
		},
		{
			name:        "suffix value with a port moves the domain",
			template:    "http://us.api.lab.test${SUFFIX}",
			values:      map[string]string{"SUFFIX": ".attacker.lab.test:9000"},
			expectError: true,
		},
		{
			// The literal ".com" is only a public suffix, so X alone names the domain.
			name:        "value forms the registrable domain",
			template:    "https://api.${X}.com/mcp",
			values:      map[string]string{"X": "evil"},
			expectError: true,
		},
		{
			// Provider suffixes don't count: the admin chose the platform, and the user picks
			// a tenant on it.
			name:        "value picks a tenant under a provider suffix",
			template:    "https://${APP}.herokuapp.com/mcp",
			values:      map[string]string{"APP": "tenant"},
			expectedURL: "https://tenant.herokuapp.com/mcp",
		},
		{
			name:        "value forms the domain before a multi-label ICANN suffix",
			template:    "https://api.${X}.co.uk/mcp",
			values:      map[string]string{"X": "evil"},
			expectError: true,
		},
		{
			name:        "value next to literal domain text stays under it",
			template:    "https://api${X}example.com/mcp",
			values:      map[string]string{"X": "-eu."},
			expectedURL: "https://api-eu.example.com/mcp",
		},
		{
			// Literal "example.com" must be whole labels, not the end of "evilexample.com".
			name:        "value joins literal domain text into a new domain",
			template:    "https://api${X}example.com/mcp",
			values:      map[string]string{"X": "x.evil"},
			expectError: true,
		},
		{
			name:        "port variable after a literal hostname",
			template:    "http://localhost:${PORT}/mcp",
			values:      map[string]string{"PORT": "8080"},
			expectedURL: "http://localhost:8080/mcp",
		},
		{
			name:        "host variable with literal port",
			template:    "https://${HOST}:8443/mcp",
			values:      map[string]string{"HOST": "anything.example"},
			expectedURL: "https://anything.example:8443/mcp",
		},
		{
			// Without literal octets in front, the user would choose the whole address.
			name:        "IP address made only of values",
			template:    "http://${A}.${B}:8080/mcp",
			values:      map[string]string{"A": "203.0.113", "B": "7"},
			expectError: true,
		},
		{
			name:        "fully qualified hostname with a trailing dot",
			template:    "https://${R}.api.example.com./mcp",
			values:      map[string]string{"R": "us"},
			expectedURL: "https://us.api.example.com./mcp",
		},
		{
			name:        "IP address octet",
			template:    "http://10.0.0.${N}:8080/mcp",
			values:      map[string]string{"N": "5"},
			expectedURL: "http://10.0.0.5:8080/mcp",
		},
		{
			name:        "single-label hostname",
			template:    "http://mcp-${ID}:8080/mcp",
			values:      map[string]string{"ID": "a"},
			expectedURL: "http://mcp-a:8080/mcp",
		},
		{
			// net/http converts these full-width dots to '.' before dialing.
			name:        "ideographic full stops in a single-label hostname move the domain",
			template:    "https://mcp-${ID}/mcp",
			values:      map[string]string{"ID": "x。attacker。example"},
			expectError: true,
		},
		{
			name:        "fullwidth full stops in a single-label hostname move the domain",
			template:    "https://mcp-${ID}/mcp",
			values:      map[string]string{"ID": "x．attacker．example"},
			expectError: true,
		},
		{
			name:        "halfwidth ideographic full stops in a single-label hostname move the domain",
			template:    "https://mcp-${ID}/mcp",
			values:      map[string]string{"ID": "x｡attacker｡example"},
			expectError: true,
		},
		{
			name:        "single-label hostname without a port",
			template:    "https://mcp-${ID}/mcp",
			values:      map[string]string{"ID": "abc"},
			expectedURL: "https://mcp-abc/mcp",
		},
		{
			name:        "non-ASCII literal domain",
			template:    "https://${R}.münchen.de/mcp",
			values:      map[string]string{"R": "api"},
			expectedURL: "https://api.münchen.de/mcp",
		},
		{
			name:        "dotted value in a single-label hostname moves the domain",
			template:    "http://mcp-${ID}:8080/mcp",
			values:      map[string]string{"ID": "a.attacker.example"},
			expectError: true,
		},
		{
			name:        "scheme variable",
			template:    "${SCHEME}://api.example.com/mcp",
			values:      map[string]string{"SCHEME": "http"},
			expectedURL: "http://api.example.com/mcp",
		},
		{
			name:        "path value may contain URL delimiters",
			template:    "https://api.example.com/${TENANT}/mcp",
			values:      map[string]string{"TENANT": "a/b?c#d"},
			expectedURL: "https://api.example.com/a/b?c#d/mcp",
		},
		{
			// Without a literal "://", the user's value decides where the host begins.
			name:        "no literal scheme separator",
			template:    "${BASE_URL}/mcp",
			values:      map[string]string{"BASE_URL": "https://anything.example"},
			expectedURL: "https://anything.example/mcp",
		},
		{
			name:        "scheme separator in the query is not the template's",
			template:    "${BASE}/r?u=https://x.example",
			values:      map[string]string{"BASE": "https://anything.example"},
			expectedURL: "https://anything.example/r?u=https://x.example",
		},
		{
			name:        "slash in host label moves the host",
			template:    "https://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "attacker.example/x?"},
			expectError: true,
		},
		{
			name:        "query in host label moves the host",
			template:    "https://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "attacker.example?"},
			expectError: true,
		},
		{
			name:        "fragment in host label moves the host",
			template:    "https://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "attacker.example#"},
			expectError: true,
		},
		{
			name:        "userinfo in host label moves the host",
			template:    "https://${REGION}api.example.com/mcp",
			values:      map[string]string{"REGION": "x@attacker.example/"},
			expectError: true,
		},
		{
			name:        "admin-written userinfo",
			template:    "https://${USER}@api.example.com/mcp",
			values:      map[string]string{"USER": "x"},
			expectedURL: "https://x@api.example.com/mcp",
		},
		{
			name:        "uppercase template scheme",
			template:    "HTTPS://${REGION}.api.example.com/mcp",
			values:      map[string]string{"REGION": "us"},
			expectedURL: "HTTPS://us.api.example.com/mcp",
		},
		{
			// The admin's literal '@' is still the URL's last one, so the host is unchanged.
			name:        "at sign in admin-written userinfo value",
			template:    "https://user:${PW}@api.example.com/mcp",
			values:      map[string]string{"PW": "p@ss"},
			expectedURL: "https://user:p@ss@api.example.com/mcp",
		},
		{
			name:        "at sign in host value after admin-written userinfo",
			template:    "https://${USER}@api-${REGION}.example.com/mcp",
			values:      map[string]string{"USER": "u", "REGION": "x@attacker"},
			expectError: true,
		},
		{
			// Documented limit: with no literal '/', '?' or '#' after the host, the variable
			// counts as part of the host, so a value that starts a path is rejected.
			name:        "path variable directly after host",
			template:    "https://api.example.com${PATH}",
			values:      map[string]string{"PATH": "/mcp"},
			expectError: true,
		},
		{
			name:        "slash in admin-written userinfo moves the host",
			template:    "https://${USER}@api.example.com/mcp",
			values:      map[string]string{"USER": "attacker.example/"},
			expectError: true,
		},
		{
			name:        "userinfo skips the admin host prefix",
			template:    "https://api-${REGION}.example.com/mcp",
			values:      map[string]string{"REGION": "x@attacker"},
			expectError: true,
		},
		{
			name:        "slash in port moves the host",
			template:    "https://api.example.com:${PORT}/mcp",
			values:      map[string]string{"PORT": "443@attacker.example/"},
			expectError: true,
		},
		{
			name:        "scheme value adds its own host",
			template:    "${SCHEME}://api.example.com/mcp",
			values:      map[string]string{"SCHEME": "https://attacker.example/?x="},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, missing, err := expandURLTemplate(tt.template, tt.values, nil)
			if len(missing) > 0 {
				t.Fatalf("unexpected missing references: %v", missing)
			}
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected an error, got URL %q", expanded)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if expanded != tt.expectedURL {
				t.Fatalf("URL = %q, want %q", expanded, tt.expectedURL)
			}
		})
	}
}

func TestResolveURLTemplateTreatsAdminValuesAsLiteral(t *testing.T) {
	ddSiteOptions := []types.MCPConfigurationOption{{Value: "datadoghq.com"}, {Value: "us3.datadoghq.com"}}
	tests := []struct {
		name          string
		template      string
		config        []types.MCPConfig
		configuration map[string]string
		adminKeys     []string
		expectedURL   string
		expectError   bool
	}{
		{
			name:          "user value forms the domain",
			template:      "https://mcp.${DD_SITE}/api/mcp",
			configuration: map[string]string{"DD_SITE": "datadoghq.eu"},
			expectError:   true,
		},
		{
			name:        "static value forms the domain",
			template:    "https://mcp.${DD_SITE}/api/mcp",
			config:      []types.MCPConfig{{Key: "DD_SITE", Value: "datadoghq.eu"}},
			expectedURL: "https://mcp.datadoghq.eu/api/mcp",
		},
		{
			name:          "static value overrides a user value",
			template:      "https://mcp.${DD_SITE}/api/mcp",
			config:        []types.MCPConfig{{Key: "DD_SITE", Value: "datadoghq.eu"}},
			configuration: map[string]string{"DD_SITE": "attacker.example"},
			expectedURL:   "https://mcp.datadoghq.eu/api/mcp",
		},
		{
			name:          "chosen option forms the domain",
			template:      "https://mcp.${DD_SITE}/api/mcp",
			config:        []types.MCPConfig{{Key: "DD_SITE", Options: ddSiteOptions}},
			configuration: map[string]string{"DD_SITE": "us3.datadoghq.com"},
			expectedURL:   "https://mcp.us3.datadoghq.com/api/mcp",
		},
		{
			// Having options isn't enough: the value must be one of them.
			name:          "value outside the options forms the domain",
			template:      "https://mcp.${DD_SITE}/api/mcp",
			config:        []types.MCPConfig{{Key: "DD_SITE", Options: ddSiteOptions}},
			configuration: map[string]string{"DD_SITE": "attacker.example"},
			expectError:   true,
		},
		{
			name:          "admin key forms the domain",
			template:      "https://mcp.${DD_SITE}/api/mcp",
			configuration: map[string]string{"DD_SITE": "datadoghq.eu"},
			adminKeys:     []string{"DD_SITE"},
			expectedURL:   "https://mcp.datadoghq.eu/api/mcp",
		},
		{
			name:          "user label before an admin domain",
			template:      "https://${TENANT}.mcp.${DD_SITE}/api/mcp",
			configuration: map[string]string{"TENANT": "acme", "DD_SITE": "datadoghq.eu"},
			adminKeys:     []string{"DD_SITE"},
			expectedURL:   "https://acme.mcp.datadoghq.eu/api/mcp",
		},
		{
			name:          "user label before an admin domain moves the host",
			template:      "https://${TENANT}.mcp.${DD_SITE}/api/mcp",
			configuration: map[string]string{"TENANT": "attacker.example/", "DD_SITE": "datadoghq.eu"},
			adminKeys:     []string{"DD_SITE"},
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, missing, err := ResolveURLTemplate(tt.template, tt.config, tt.configuration, tt.adminKeys)
			if len(missing) > 0 {
				t.Fatalf("unexpected missing references: %v", missing)
			}
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected an error, got URL %q", resolved)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved != tt.expectedURL {
				t.Fatalf("expected %q, got %q", tt.expectedURL, resolved)
			}
		})
	}
}

func TestValidateURLTemplateHostErrorOmitsValues(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
	}{
		{
			name:     "host mismatch",
			template: "https://${REGION}.api.example.com/mcp",
			values:   map[string]string{"REGION": "secret-region.example/x?"},
		},
		{
			// url.Parse errors quote the whole URL, so this covers the parse-failure path.
			name:     "unparseable port",
			template: "https://api.example.com:${PORT}/mcp",
			values:   map[string]string{"PORT": "secret-port"},
		},
		{
			// The invalid escape is in a path value, so the error must name it even though it is
			// outside the scheme and authority.
			name:     "invalid escape in path value",
			template: "https://${REGION}.api.example.com/${TENANT}",
			values:   map[string]string{"REGION": "us", "TENANT": "secret-tenant%zz"},
		},
		{
			name:     "userinfo password that moves the host",
			template: "https://user:${PW}@api.example.com/mcp",
			values:   map[string]string{"PW": "secret-password/x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := expandURLTemplate(tt.template, tt.values, nil)
			if err == nil {
				t.Fatal("expected an error")
			}
			for key, value := range tt.values {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error %q contains the value of %s", err, key)
				}
				if !strings.Contains(err.Error(), key) {
					t.Fatalf("error %q does not name %s", err, key)
				}
			}
		})
	}
}
func TestServerToServerConfig_Builtin(t *testing.T) {
	tests := []struct {
		name            string
		config          *types.BuiltinRuntimeConfig
		wantBuiltinName string
		wantErr         bool
	}{
		{
			name:            "names the built-in server",
			config:          &types.BuiltinRuntimeConfig{Name: "obot-admin"},
			wantBuiltinName: "obot-admin",
		},
		{
			name:    "requires a built-in configuration",
			wantErr: true,
		},
		{
			name:    "requires a built-in server name",
			config:  &types.BuiltinRuntimeConfig{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, _, err := ServerToServerConfig(v1.MCPServer{
				Name: "test-server",
				Spec: v1.MCPServerSpec{
					Manifest: types.MCPServerManifest{
						Runtime:       types.RuntimeBuiltin,
						BuiltinConfig: tt.config,
					},
				},
			}, nil, "test-user-id", "test-scope", "test-catalog", nil, false)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if config.BuiltinName != tt.wantBuiltinName {
				t.Fatalf("BuiltinName = %q, want %q", config.BuiltinName, tt.wantBuiltinName)
			}
			if config.URL != "" || config.Command != "" {
				t.Fatalf("built-in server has a URL %q or command %q", config.URL, config.Command)
			}
		})
	}
}

func TestDeploysNothing(t *testing.T) {
	for runtime, want := range map[types.Runtime]bool{
		types.RuntimeRemote:        true,
		types.RuntimeVMCP:          true,
		types.RuntimeBuiltin:       true,
		types.RuntimeNPX:           false,
		types.RuntimeUVX:           false,
		types.RuntimeContainerized: false,
	} {
		if got := deploysNothing(runtime); got != want {
			t.Errorf("deploysNothing(%q) = %v, want %v", runtime, got, want)
		}
	}
}
