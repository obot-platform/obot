package mcpserver

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNanobotImageDrift(t *testing.T) {
	tests := []struct {
		name          string
		manifestImage string
		desiredImage  string
		expectDrift   bool
	}{
		{
			name:          "no drift when deployed image matches desired override",
			manifestImage: "ghcr.io/nanobot-ai/nanobot:v1",
			desiredImage:  "ghcr.io/nanobot-ai/nanobot:v1",
			expectDrift:   false,
		},
		{
			name:          "drift when desired override differs from deployed image",
			manifestImage: "ghcr.io/nanobot-ai/nanobot:v1",
			desiredImage:  "ghcr.io/nanobot-ai/nanobot:v2",
			expectDrift:   true,
		},
		{
			name:          "no drift when desired override empty and manifest used",
			manifestImage: "ghcr.io/nanobot-ai/nanobot:v1",
			desiredImage:  "",
			expectDrift:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &v1.MCPServer{
				Spec: v1.MCPServerSpec{
					NanobotAgentID:       "nba1test",
					DesiredImageOverride: tt.desiredImage,
					Manifest: types.MCPServerManifest{
						Runtime: types.RuntimeContainerized,
						ContainerizedConfig: &types.ContainerizedRuntimeConfig{
							Image: tt.manifestImage,
						},
					},
				},
			}

			drifted := nanobotImageHasDrifted(server)
			if drifted != tt.expectDrift {
				t.Fatalf("drift = %v, want %v", drifted, tt.expectDrift)
			}
		})
	}
}

func TestDetectDriftUpdatesNanobotStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1nba1test",
			Namespace: "default",
		},
		Spec: v1.MCPServerSpec{
			NanobotAgentID:       "nba1test",
			DesiredImageOverride: "ghcr.io/nanobot-ai/nanobot:v2",
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "ghcr.io/nanobot-ai/nanobot:v1",
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(server).
		Build()

	req := router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server.DeepCopy(),
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	if err := New(nil, nil, "").DetectDrift(req, nil); err != nil {
		t.Fatalf("DetectDrift() error = %v", err)
	}

	var updated v1.MCPServer
	if err := client.Get(context.Background(), kclient.ObjectKeyFromObject(server), &updated); err != nil {
		t.Fatalf("failed to get updated server: %v", err)
	}

	if !updated.Status.NeedsUpdate {
		t.Fatalf("expected NeedsUpdate to be true")
	}

	req.Object = updated.DeepCopy()
	updated.Spec.Manifest.ContainerizedConfig.Image = updated.Spec.DesiredImageOverride
	updated.Status.NeedsUpdate = true
	if err := client.Update(context.Background(), &updated); err != nil {
		t.Fatalf("failed to update server spec: %v", err)
	}
	if err := client.Status().Update(context.Background(), &updated); err != nil {
		t.Fatalf("failed to update server status: %v", err)
	}

	req.Object = updated.DeepCopy()
	if err := New(nil, nil, "").DetectDrift(req, nil); err != nil {
		t.Fatalf("DetectDrift() second call error = %v", err)
	}

	if err := client.Get(context.Background(), kclient.ObjectKeyFromObject(server), &updated); err != nil {
		t.Fatalf("failed to get updated server after second drift check: %v", err)
	}

	if updated.Status.NeedsUpdate {
		t.Fatalf("expected NeedsUpdate to be false after deployed image catches up")
	}
}

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
			drifted, err := configurationHasDrifted(tt.serverManifest, tt.entryManifest)

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
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := uvxConfigHasDrifted(tt.serverConfig, tt.entryConfig)
				if result != tt.expectedDrift {
					t.Errorf("Expected drift=%v, got drift=%v", tt.expectedDrift, result)
				}
			})
		}
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
