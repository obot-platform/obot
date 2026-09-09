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
			config, _, err := ServerToServerConfig(server, nil, "user", "scope", "default", nil)
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

	config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil)
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

	_, missing, err := ServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", "test-scope", "test-catalog", nil)
	if err != nil {
		t.Fatalf("expected missing option to be reported without an error, got %v", err)
	}
	if !slices.Equal(missing, []string{"REGION"}) {
		t.Fatalf("expected REGION to be missing, got %v", missing)
	}

	_, _, err = ServerToServerConfig(server, server.ValidConnectURLs("http://localhost:8080"), "test-user-id", "test-scope", "test-catalog", map[string]string{"REGION": "stale"})
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
	config, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", stale)
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
	if _, _, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", stale); err == nil {
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
		nil,
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

	config, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", nil)
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
	ordinary, missing, err := ServerToServerConfig(server, nil, "user", "scope", "catalog", nil)
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

	config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil)
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
				{Key: "X-Tenant-ID", Usage: types.Header, UserAllowed: true, Required: true},
				{Key: "X-Account-ID", Usage: types.Header, UserAllowed: true},
			},
			expected: []string{"X-Tenant-ID", "X-Account-ID"},
		},
		{
			name: "per-user values are not shared",
			config: []types.MCPConfig{
				{Key: "X-Tenant-ID", Usage: types.Header, UserAllowed: true, Required: true, Value: "tenant"},
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", nil)
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
				{Usage: types.Header, Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "user-configurable header only",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "mixed static and user-configurable",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "Bearer static-token"},
				{Usage: types.Header, Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{"X-API-Key": "user-key"},
			expectedHeaders: []string{"Authorization=Bearer static-token", "X-API-Key=user-key"},
			expectedMissing: []string{},
		},
		{
			name: "missing required user-configurable header",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "Bearer static-token"},
				{Usage: types.Header, Key: "X-API-Key", Required: true},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{"X-API-Key"},
		},
		{
			name: "optional user-configurable header missing",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "Bearer static-token"},
				{Usage: types.Header, Key: "X-Optional", Required: false},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "static header overrides credential",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "Bearer static-token"},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer static-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty static value falls back to credential",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer user-token"},
			expectedHeaders: []string{"Authorization=Bearer user-token"},
			expectedMissing: []string{},
		},
		{
			name: "empty credential value is ignored",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "", Required: true},
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv)

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

	config, missing, err := ServerToServerConfig(mcpServer, nil, "test-user-id", "test-scope", "test-catalog", nil)
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
				{Usage: types.Header, Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "header with prefix not applied to static value",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Value: "static-token", Prefix: "Bearer "},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{"Authorization=static-token"},
			expectedMissing: []string{},
		},
		{
			name: "env var with Bearer prefix",
			env: []types.MCPConfig{
				{Usage: types.Env, Key: "API_KEY", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"API_KEY": "secret-key-123"},
			expectedEnv:     []string{"API_KEY=Bearer secret-key-123"},
			expectedMissing: []string{},
		},
		{
			name: "env var with sk- prefix (OpenAI style)",
			env: []types.MCPConfig{
				{Usage: types.Env, Key: "OPENAI_API_KEY", Prefix: "sk-", Required: true},
			},
			credEnv:         map[string]string{"OPENAI_API_KEY": "proj-abc123xyz"},
			expectedEnv:     []string{"OPENAI_API_KEY=sk-proj-abc123xyz"},
			expectedMissing: []string{},
		},
		{
			name: "multiple headers and env vars with different prefixes",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Prefix: "Bearer ", Required: true},
				{Usage: types.Header, Key: "X-API-Key", Prefix: "Key ", Required: true},
			},
			env: []types.MCPConfig{
				{Usage: types.Env, Key: "TOKEN", Prefix: "Token ", Required: true},
				{Usage: types.Env, Key: "SECRET", Required: true}, // No prefix
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
				{Usage: types.Header, Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{},
			expectedHeaders: []string{},
			expectedMissing: []string{"Authorization"},
		},
		{
			name: "prefix not duplicated when user already included it in header",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			credEnv:         map[string]string{"Authorization": "Bearer my-token"},
			expectedHeaders: []string{"Authorization=Bearer my-token"},
			expectedMissing: []string{},
		},
		{
			name: "prefix not duplicated when user already included it in env var",
			env: []types.MCPConfig{
				{Usage: types.Env, Key: "API_KEY", Prefix: "sk-", Required: true},
			},
			credEnv:         map[string]string{"API_KEY": "sk-proj-abc123"},
			expectedEnv:     []string{"API_KEY=sk-proj-abc123"},
			expectedMissing: []string{},
		},
		{
			name: "mixed - some with prefix already included, some without",
			headers: []types.MCPConfig{
				{Usage: types.Header, Key: "Authorization", Prefix: "Bearer ", Required: true},
			},
			env: []types.MCPConfig{
				{Usage: types.Env, Key: "API_KEY", Prefix: "sk-", Required: true},
				{Usage: types.Env, Key: "TOKEN", Prefix: "Token ", Required: true},
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv)

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
					{Usage: types.Header, Key: "Authorization", Value: "Bearer token-with-special!@#$%^&*()characters"},
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

			config, missing, err := ServerToServerConfig(mcpServer, mcpServer.ValidConnectURLs(baseURL), "test-user-id", "test-scope", "test-catalog", tt.credEnv)

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
