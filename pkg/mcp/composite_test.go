package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRuntimeIdentityDigestTracksRuntimeConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		base     types.MCPServerCatalogEntryManifest
		edited   types.MCPServerCatalogEntryManifest
		wantMove bool
	}{
		{
			name:     "npx package",
			base:     npxManifest("@example/server@1.0.0"),
			edited:   npxManifest("@example/server@2.0.0"),
			wantMove: true,
		},
		{
			name: "npx args",
			base: npxManifest("@example/server@1.0.0"),
			edited: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "@example/server@1.0.0", Args: []string{"--verbose"}},
			},
			wantMove: true,
		},
		{
			name: "containerized image",
			base: containerizedManifest("example/mcp:1.0.0"),
			edited: types.MCPServerCatalogEntryManifest{
				Runtime:             types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "example/mcp:2.0.0", Port: 8080, Path: "/mcp"},
			},
			wantMove: true,
		},
		{
			name: "uvx command",
			base: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "example", Command: "serve"},
			},
			edited: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "example", Command: "start"},
			},
			wantMove: true,
		},
		{
			name:     "remote fixed URL",
			base:     remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://one.example.com/mcp"}),
			edited:   remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://two.example.com/mcp"}),
			wantMove: true,
		},
		{
			name:     "remote URL template",
			base:     remoteManifest(&types.RemoteCatalogConfig{URLTemplate: "https://${WORKSPACE}.example.com/mcp"}),
			edited:   remoteManifest(&types.RemoteCatalogConfig{URLTemplate: "https://${WORKSPACE}.example.com/mcp/v2"}),
			wantMove: true,
		},
		{
			name:     "remote hostname",
			base:     remoteManifest(&types.RemoteCatalogConfig{Hostname: "*.one.example.com"}),
			edited:   remoteManifest(&types.RemoteCatalogConfig{Hostname: "*.two.example.com"}),
			wantMove: true,
		},
		{
			name:     "remote tunnel name",
			base:     remoteManifest(&types.RemoteCatalogConfig{Hostname: "api.example.com", TunnelName: "office"}),
			edited:   remoteManifest(&types.RemoteCatalogConfig{Hostname: "api.example.com", TunnelName: "datacenter"}),
			wantMove: true,
		},
		{
			name: "env key",
			base: withEnv(npxManifest("@example/server@1.0.0"), types.MCPEnv{Key: "TOKEN"}),
			edited: withEnv(npxManifest("@example/server@1.0.0"),
				types.MCPEnv{Key: "TOKEN"},
				types.MCPEnv{Key: "ORG"},
			),
			wantMove: true,
		},
		{
			name:     "remote header key",
			base:     remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp", Headers: []types.MCPHeader{{Key: "X-Api-Key"}}}),
			edited:   remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp", Headers: []types.MCPHeader{{Key: "X-Api-Token"}}}),
			wantMove: true,
		},
		{
			name: "name, icon, and descriptions",
			base: npxManifest("@example/server@1.0.0"),
			edited: types.MCPServerCatalogEntryManifest{
				Name:             "Renamed",
				Icon:             "https://example.com/icon.svg",
				ShortDescription: "short",
				Description:      "long",
				Runtime:          types.RuntimeNPX,
				NPXConfig:        &types.NPXRuntimeConfig{Package: "@example/server@1.0.0"},
			},
		},
		{
			name: "tool preview",
			base: npxManifest("@example/server@1.0.0"),
			edited: types.MCPServerCatalogEntryManifest{
				Runtime:     types.RuntimeNPX,
				NPXConfig:   &types.NPXRuntimeConfig{Package: "@example/server@1.0.0"},
				ToolPreview: []types.MCPServerTool{{Name: "create_issue", Description: "Create an issue"}},
			},
		},
		{
			name: "resource limits",
			base: npxManifest("@example/server@1.0.0"),
			edited: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "@example/server@1.0.0"},
				Resources: &types.MCPResourceRequirements{
					Limits: types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
				},
			},
		},
		{
			name: "env value and binding",
			base: withEnv(npxManifest("@example/server@1.0.0"), types.MCPEnv{Key: "TOKEN"}),
			edited: withEnv(npxManifest("@example/server@1.0.0"), types.MCPEnv{Key: "TOKEN",
				Name:          "Token",
				Value:         "a-value",
				Required:      true,
				SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "token", AdminAdded: true},
			}),
		},
		{
			name:   "remote header value",
			base:   remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp", Headers: []types.MCPHeader{{Key: "X-Api-Key"}}}),
			edited: remoteManifest(&types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp", Headers: []types.MCPHeader{{Key: "X-Api-Key", Name: "API Key", Value: "a-value", Required: true}}}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := RuntimeIdentityDigest(tt.base), RuntimeIdentityDigest(tt.edited)
			if tt.wantMove {
				assert.NotEqual(t, before, after, "editing %s must report the component's tool overrides stale", tt.name)
				return
			}
			assert.Equal(t, before, after, "editing %s cannot change which tools a server serves", tt.name)
		})
	}
}

func TestComponentToolOverridesStale(t *testing.T) {
	upstream := npxManifest("@example/server@2.0.0")
	currentDigest := RuntimeIdentityDigest(upstream)
	staleDigest := RuntimeIdentityDigest(npxManifest("@example/server@1.0.0"))
	overrides := []types.ToolOverride{{Name: "create_issue", Enabled: true}}

	tests := []struct {
		name      string
		component types.CatalogComponentServer
		resolved  ResolvedComponent
		want      bool
	}{
		{
			name:      "no overrides",
			component: types.CatalogComponentServer{CatalogEntryID: "entry", SourceDigest: staleDigest},
			resolved:  ResolvedComponent{Manifest: upstream},
		},
		{
			name:      "no captured digest",
			component: types.CatalogComponentServer{CatalogEntryID: "entry", ToolOverrides: overrides},
			resolved:  ResolvedComponent{Manifest: upstream},
		},
		{
			name:      "upstream missing",
			component: types.CatalogComponentServer{CatalogEntryID: "entry", ToolOverrides: overrides, SourceDigest: staleDigest},
			resolved:  ResolvedComponent{Missing: true},
		},
		{
			name:      "digest matches",
			component: types.CatalogComponentServer{CatalogEntryID: "entry", ToolOverrides: overrides, SourceDigest: currentDigest},
			resolved:  ResolvedComponent{Manifest: upstream},
		},
		{
			name:      "digest differs",
			component: types.CatalogComponentServer{CatalogEntryID: "entry", ToolOverrides: overrides, SourceDigest: staleDigest},
			resolved:  ResolvedComponent{Manifest: upstream},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ComponentToolOverridesStale(tt.component, tt.resolved))
		})
	}
}

func TestResolveCompositeComponentRef(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name: "component-entry", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{Manifest: npxManifest("@example/server@1.0.0")},
	}
	entry.Spec.Manifest.Name = "Component"
	sharedServer := &v1.MCPServer{
		Name: "shared-server", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "default",
			Manifest: types.MCPServerManifest{
				Name:            "Shared Server",
				Runtime:         types.RuntimeContainerized,
				MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "API_KEY"}}},
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "example/shared:1.0.0",
					Port:  8080,
					Path:  "/mcp",
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, sharedServer).Build()

	t.Run("catalog entry", func(t *testing.T) {
		resolved, err := ResolveCompositeComponentRef(t.Context(), c, "component-entry", "")
		require.NoError(t, err)
		assert.False(t, resolved.Missing)
		assert.Equal(t, "Component", resolved.Manifest.Name)
		require.NotNil(t, resolved.Manifest.NPXConfig)
		assert.Equal(t, "@example/server@1.0.0", resolved.Manifest.NPXConfig.Package)
	})

	t.Run("multi-user server in catalog entry form", func(t *testing.T) {
		resolved, err := ResolveCompositeComponentRef(t.Context(), c, "", "shared-server")
		require.NoError(t, err)
		assert.False(t, resolved.Missing)
		assert.Equal(t, sharedServer.Spec.Manifest.ConvertToCatalogEntry(), resolved.Manifest)
		require.NotNil(t, resolved.Manifest.ContainerizedConfig)
		assert.Equal(t, "example/shared:1.0.0", resolved.Manifest.ContainerizedConfig.Image)
		assert.NotNil(t, resolved.Manifest.MultiUserConfig)
	})

	t.Run("missing objects are reported, not errors", func(t *testing.T) {
		for _, tt := range []struct{ catalogEntryID, mcpServerID string }{
			{catalogEntryID: "deleted-entry"},
			{mcpServerID: "deleted-server"},
			{},
		} {
			resolved, err := ResolveCompositeComponentRef(t.Context(), c, tt.catalogEntryID, tt.mcpServerID)
			require.NoError(t, err)
			assert.True(t, resolved.Missing)
			assert.Zero(t, resolved.Manifest)
		}
	})
}

func npxManifest(pkg string) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: pkg},
	}
}

func containerizedManifest(image string) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Runtime:             types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: image, Port: 8080, Path: "/mcp"},
	}
}

func remoteManifest(config *types.RemoteCatalogConfig) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: config,
	}
}

func withEnv(manifest types.MCPServerCatalogEntryManifest, env ...types.MCPEnv) types.MCPServerCatalogEntryManifest {
	manifest.Env = env
	return manifest
}
