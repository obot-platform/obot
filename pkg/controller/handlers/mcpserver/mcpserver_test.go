package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConfigurationHasDrifted(t *testing.T) {
	tests := []struct {
		name           string
		serverManifest types.MCPServerManifest
		entryManifest  types.MCPServerCatalogEntryManifest
		expectedDrift  bool
		expectedError  bool
	}{
		{
			name: "no drift - identical UVX manifests",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Args:    []string{"arg1", "arg2"},
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Args:    []string{"arg1", "arg2"},
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - identical NPX manifests",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@test/package",
					Args:    []string{"--port", "3000"},
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@test/package",
					Args:    []string{"--port", "3000"},
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - identical containerized manifests",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:   "test/image:latest",
					Command: "start",
					Args:    []string{"--verbose"},
					Port:    8080,
					Path:    "/mcp",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:   "test/image:latest",
					Command: "start",
					Args:    []string{"--verbose"},
					Port:    8080,
					Path:    "/mcp",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - remote with fixed URL",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://api.example.com/mcp",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - remote with hostname constraint",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Hostname: "api.example.com",
					URL:      "https://api.example.com:8080/mcp/path",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.example.com",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different runtime types",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "test-package",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "no drift - different names",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "different-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different UVX packages",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Args:    []string{"arg1"},
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "different-package",
					Args:    []string{"arg1"},
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different UVX commands",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Command: "start",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Command: "run",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different UVX args",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Args:    []string{"arg1", "arg2"},
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
					Args:    []string{"arg2", "arg1"}, // Different order
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different containerized image",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test/image:v1",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test/image:v2",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different remote fixed URL",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://api.different.com/mcp",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - remote hostname mismatch",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.different.com",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "no drift - different env order (order doesn't matter)",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
				Env: []types.MCPEnv{
					{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}},
					{MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}},
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
				Env: []types.MCPEnv{
					{MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}},
					{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}},
				},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different env values",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}}},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "error - invalid URL in remote server config",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "://invalid-url",
				},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.example.com",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - missing runtime config",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig:   nil, // Missing config
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - unknown runtime type",
			serverManifest: types.MCPServerManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     "unknown",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Name:        "test-server",
				Description: "Test server",
				Runtime:     "unknown",
			},
			expectedDrift: false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drifted, err := configurationHasDrifted(tt.serverManifest, tt.entryManifest, false)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if drifted != tt.expectedDrift {
				t.Errorf("Expected drift=%v, got drift=%v", tt.expectedDrift, drifted)
			}
		})
	}
}

func TestRuntimeSpecificDriftFunctions(t *testing.T) {
	t.Run("uvxConfigHasDrifted", func(t *testing.T) {
		tests := []struct {
			name          string
			serverConfig  *types.UVXRuntimeConfig
			entryConfig   *types.UVXRuntimeConfig
			expectedDrift bool
		}{
			{
				name:          "both nil",
				serverConfig:  nil,
				entryConfig:   nil,
				expectedDrift: false,
			},
			{
				name:          "server nil, entry not nil",
				serverConfig:  nil,
				entryConfig:   &types.UVXRuntimeConfig{Package: "test"},
				expectedDrift: true,
			},
			{
				name:          "server not nil, entry nil",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test"},
				entryConfig:   nil,
				expectedDrift: true,
			},
			{
				name:          "identical configs",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test", Args: []string{"arg1"}},
				entryConfig:   &types.UVXRuntimeConfig{Package: "test", Args: []string{"arg1"}},
				expectedDrift: false,
			},
			{
				name:          "different packages",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test1"},
				entryConfig:   &types.UVXRuntimeConfig{Package: "test2"},
				expectedDrift: true,
			},
			{
				name:          "different args",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test", Args: []string{"arg1"}},
				entryConfig:   &types.UVXRuntimeConfig{Package: "test", Args: []string{"arg2"}},
				expectedDrift: true,
			},
			{
				name:          "different egress domains",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test", EgressDomains: []string{"api.example.com"}},
				entryConfig:   &types.UVXRuntimeConfig{Package: "test", EgressDomains: []string{"*.example.com"}},
				expectedDrift: true,
			},
			{
				name:          "different deny all egress",
				serverConfig:  &types.UVXRuntimeConfig{Package: "test", DenyAllEgress: new(false)},
				entryConfig:   &types.UVXRuntimeConfig{Package: "test", DenyAllEgress: new(true)},
				expectedDrift: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := uvxConfigHasDrifted(tt.serverConfig, tt.entryConfig, false)
				if result != tt.expectedDrift {
					t.Errorf("Expected drift=%v, got drift=%v", tt.expectedDrift, result)
				}
			})
		}
	})

	t.Run("npxConfigHasDrifted", func(t *testing.T) {
		result := npxConfigHasDrifted(
			&types.NPXRuntimeConfig{Package: "test", DenyAllEgress: new(false)},
			&types.NPXRuntimeConfig{Package: "test", DenyAllEgress: new(true)},
			false,
		)
		if !result {
			t.Errorf("Expected drift=true, got drift=%v", result)
		}
	})

	t.Run("containerizedConfigHasDrifted", func(t *testing.T) {
		result := containerizedConfigHasDrifted(
			&types.ContainerizedRuntimeConfig{Image: "img", Port: 8080, Path: "/mcp", DenyAllEgress: new(false)},
			&types.ContainerizedRuntimeConfig{Image: "img", Port: 8080, Path: "/mcp", DenyAllEgress: new(true)},
			false,
		)
		if !result {
			t.Errorf("Expected drift=true, got drift=%v", result)
		}
	})

	t.Run("default deny semantics are compared effectively", func(t *testing.T) {
		assert.False(t, uvxConfigHasDrifted(
			&types.UVXRuntimeConfig{Package: "test", EgressDomains: []string{"api.example.com"}},
			&types.UVXRuntimeConfig{Package: "test", EgressDomains: []string{"api.example.com"}, DenyAllEgress: new(false)},
			true,
		))

		assert.True(t, uvxConfigHasDrifted(
			&types.UVXRuntimeConfig{Package: "test"},
			&types.UVXRuntimeConfig{Package: "test", DenyAllEgress: new(false)},
			true,
		))
	})

	t.Run("remoteConfigHasDrifted", func(t *testing.T) {
		tests := []struct {
			name          string
			serverConfig  *types.RemoteRuntimeConfig
			entryConfig   *types.RemoteCatalogConfig
			expectedDrift bool
		}{
			{
				name:          "both nil",
				serverConfig:  nil,
				entryConfig:   nil,
				expectedDrift: false,
			},
			{
				name:          "fixed URL match",
				serverConfig:  &types.RemoteRuntimeConfig{URL: "https://api.example.com"},
				entryConfig:   &types.RemoteCatalogConfig{FixedURL: "https://api.example.com"},
				expectedDrift: false,
			},
			{
				name:          "fixed URL mismatch",
				serverConfig:  &types.RemoteRuntimeConfig{URL: "https://api.example.com"},
				entryConfig:   &types.RemoteCatalogConfig{FixedURL: "https://api.different.com"},
				expectedDrift: true,
			},
			{
				name:          "hostname missing",
				serverConfig:  &types.RemoteRuntimeConfig{},
				entryConfig:   &types.RemoteCatalogConfig{Hostname: "api.example.com"},
				expectedDrift: true,
			},
			{
				name:          "hostname mismatch",
				serverConfig:  &types.RemoteRuntimeConfig{Hostname: "api.example.com"},
				entryConfig:   &types.RemoteCatalogConfig{Hostname: "api2.example.com"},
				expectedDrift: true,
			},
			{
				name:          "hostname match",
				serverConfig:  &types.RemoteRuntimeConfig{Hostname: "api2.example.com"},
				entryConfig:   &types.RemoteCatalogConfig{Hostname: "api2.example.com"},
				expectedDrift: false,
			},
			{
				name: "headers match despite order",
				serverConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{
					{Key: "X-Second", Value: "second"},
					{Key: "X-First", Value: "first"},
				}},
				entryConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{
					{Key: "X-First", Value: "first"},
					{Key: "X-Second", Value: "second"},
				}},
				expectedDrift: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := remoteConfigHasDrifted(tt.serverConfig, tt.entryConfig)
				if result != tt.expectedDrift {
					t.Errorf("Expected drift=%v, got drift=%v", tt.expectedDrift, result)
				}
			})
		}
	})
}

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()

	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithIndex(&v1.MCPNetworkPolicy{}, "spec.mcpServerName", func(obj kclient.Object) []string {
			policy := obj.(*v1.MCPNetworkPolicy)
			if policy.Spec.MCPServerName == "" {
				return nil
			}
			return []string{policy.Spec.MCPServerName}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPServer(name string) *v1.MCPServer {
	return &v1.MCPServer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}

func TestShouldSyncOAuthMetadata(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		lastRequestTime metav1.Time
		lastSyncTime    metav1.Time
		expected        bool
	}{
		{
			name:     "skips server with no requests",
			expected: false,
		},
		{
			name:            "syncs after request with no previous sync",
			lastRequestTime: metav1.NewTime(now.Add(-5 * time.Minute)),
			expected:        true,
		},
		{
			name:            "skips when last request predates sync",
			lastRequestTime: metav1.NewTime(now.Add(-2 * time.Hour)),
			lastSyncTime:    metav1.NewTime(now.Add(-90 * time.Minute)),
			expected:        false,
		},
		{
			name:            "skips when sync interval has not elapsed",
			lastRequestTime: metav1.NewTime(now.Add(-10 * time.Minute)),
			lastSyncTime:    metav1.NewTime(now.Add(-30 * time.Minute)),
			expected:        false,
		},
		{
			name:            "syncs after interval and newer request",
			lastRequestTime: metav1.NewTime(now.Add(-10 * time.Minute)),
			lastSyncTime:    metav1.NewTime(now.Add(-2 * time.Hour)),
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newMCPServer("test-server")
			server.Status.LastRequestTime = tt.lastRequestTime
			server.Status.LastOAuthMetadataSync = tt.lastSyncTime

			assert.Equal(t, tt.expected, shouldSyncOAuthMetadata(server, now))
		})
	}
}

func TestShutdownIdleServersSetsLastRequestTimeForOlderServers(t *testing.T) {
	server := newMCPServer("older-server")
	server.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).ShutdownIdleServers(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServer
	require.NoError(t, client.Get(context.Background(), router.Key(server.Namespace, server.Name), &updated))
	assert.False(t, updated.Status.LastRequestTime.IsZero())
	assert.WithinDuration(t, time.Now(), updated.Status.LastRequestTime.Time, 5*time.Second)
}

func TestShutdownIdleServersSkipsRecentlyCreatedServersWithoutLastRequestTime(t *testing.T) {
	server := newMCPServer("new-server")
	server.CreationTimestamp = metav1.NewTime(time.Now().Add(-30 * time.Second))

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).ShutdownIdleServers(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServer
	require.NoError(t, client.Get(context.Background(), router.Key(server.Namespace, server.Name), &updated))
	assert.True(t, updated.Status.LastRequestTime.IsZero())
}

func TestShutdownIdleServersSchedulesRetryUsingServerSpecificInterval(t *testing.T) {
	server := newMCPServer("custom-interval")
	server.Spec.Manifest.IdleShutdownIntervalHours = 5
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
		multiUserIdleShutdownDelay:  20 * time.Hour,
		agentIdleShutdownDelay:      25 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)

	assert.InDelta(t, (3 * time.Hour).Seconds(), resp.Delay.Seconds(), 1)
}

func TestShutdownIdleServersUsesAgentDefaultIdleInterval(t *testing.T) {
	server := newMCPServer("agent-server")
	server.Spec.NanobotAgentID = "agent-1"
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
		multiUserIdleShutdownDelay:  20 * time.Hour,
		agentIdleShutdownDelay:      7 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)

	assert.InDelta(t, (5 * time.Hour).Seconds(), resp.Delay.Seconds(), 1)
}

func TestShutdownIdleServersUsesMultiUserDefaultIdleInterval(t *testing.T) {
	server := newMCPServer("shared-server")
	server.Spec.MCPCatalogID = "catalog-1"
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
		multiUserIdleShutdownDelay:  9 * time.Hour,
		agentIdleShutdownDelay:      25 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)

	assert.InDelta(t, (7 * time.Hour).Seconds(), resp.Delay.Seconds(), 1)
}

func TestShutdownIdleServersSkipsWhenShutdownDisabled(t *testing.T) {
	server := newMCPServer("disabled-shutdown")
	server.Spec.Manifest.IdleShutdownIntervalHours = -1
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-24 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)
	assert.Zero(t, resp.Delay)
}

func TestEnsureMCPNetworkPolicyCreatesPolicy(t *testing.T) {
	server := newMCPServer("egress-server")
	server.Spec.Manifest.Runtime = types.RuntimeNPX
	server.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{
		Package:       "@test/package",
		EgressDomains: []string{"api.example.com", "*.google.com"},
	}

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(context.Background(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Len(t, policies.Items, 1)
	policy := policies.Items[0]
	assert.True(t, strings.HasPrefix(policy.Name, "mnp1"))
	assert.Equal(t, server.Name, policy.Spec.MCPServerName)
	assert.Equal(t, map[string]string{"app": server.Name}, policy.Spec.PodSelector)
	assert.Equal(t, []string{"*.google.com", "api.example.com"}, policy.Spec.EgressDomains)
	assert.False(t, policy.Spec.DenyAllEgress)
}

func TestEnsureMCPNetworkPolicyCreatesDenyAllPolicy(t *testing.T) {
	server := newMCPServer("deny-all-server")
	server.Spec.Manifest.Runtime = types.RuntimeUVX
	server.Spec.Manifest.UVXConfig = &types.UVXRuntimeConfig{
		Package:       "test-package",
		DenyAllEgress: new(true),
	}

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(context.Background(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Len(t, policies.Items, 1)
	assert.Empty(t, policies.Items[0].Spec.EgressDomains)
	assert.True(t, policies.Items[0].Spec.DenyAllEgress)
}

func TestEnsureMCPNetworkPolicyDeletesPolicyWhenProviderDisabled(t *testing.T) {
	server := newMCPServer("no-provider-server")
	existing := &v1.MCPNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
		Spec: v1.MCPNetworkPolicySpec{
			MCPServerName: server.Name,
		},
	}

	client := newFakeClient(t, server, existing)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(context.Background(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Empty(t, policies.Items)
}

func TestEnsureMCPNetworkPolicySkipsNanobotAgentServer(t *testing.T) {
	server := newMCPServer("nanobot-agent-server")
	server.Spec.NanobotAgentID = "agent-1"
	server.Spec.Manifest.Runtime = types.RuntimeNPX
	server.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{
		Package:       "@test/package",
		EgressDomains: []string{"api.example.com"},
	}

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(context.Background(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Empty(t, policies.Items)
}

func TestEnsureMCPNetworkPolicyDeletesPolicyForUnsupportedRuntime(t *testing.T) {
	server := newMCPServer("remote-server")
	server.Spec.Manifest.Runtime = types.RuntimeRemote
	server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"}

	existing := &v1.MCPNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
		Spec: v1.MCPNetworkPolicySpec{
			MCPServerName: server.Name,
		},
	}

	client := newFakeClient(t, server, existing)
	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(context.Background(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Empty(t, policies.Items)
}
