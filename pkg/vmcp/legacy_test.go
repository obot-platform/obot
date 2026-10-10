package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestComponentsForInstance(t *testing.T) {
	component := types.VMCPComponent{
		ID:         "component1",
		ToolPrefix: "shared_",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "current-package",
				},
			},
		},
	}
	parent := v1.VMCP{
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{component},
			},
		},
	}
	legacy := *component.DeepCopy()
	legacy.SourceDigest = utils.Digest([]any{component, parent.Spec.ComponentStaticConfigurationHashes[component.ID]})
	legacy.CatalogEntry.Manifest.NPXConfig.Package = "connection-package"
	legacy.ToolPrefix = "connection_"
	legacy.ToolOverrides = []types.ToolOverride{{Name: "echo", Enabled: true}}
	instance := v1.VMCPInstance{
		Spec: v1.VMCPInstanceSpec{
			LegacyComponents: []types.VMCPComponent{legacy},
		},
	}

	t.Run("retains connection snapshot and tool choices", func(t *testing.T) {
		original := parent.DeepCopy()
		components := ComponentsForInstance(parent, instance)
		require.Equal(t, []types.VMCPComponent{legacy}, components)
		require.Equal(t, original, &parent)
	})

	t.Run("uses upgraded parent snapshot", func(t *testing.T) {
		upgraded := parent.DeepCopy()
		upgraded.Spec.Manifest.Components[0].CatalogEntry.Manifest.NPXConfig.Package = "upgraded-package"
		components := ComponentsForInstance(*upgraded, instance)
		require.Equal(t, upgraded.Spec.Manifest.Components, components)
		components[0].CatalogEntry.Manifest.NPXConfig.Package = "mutated-result"
		require.Equal(t, "upgraded-package", upgraded.Spec.Manifest.Components[0].CatalogEntry.Manifest.NPXConfig.Package)
	})

	t.Run("does not restore removed component", func(t *testing.T) {
		removed := parent.DeepCopy()
		removed.Spec.Manifest.Components = nil
		require.Empty(t, ComponentsForInstance(*removed, instance))
	})

	t.Run("disabled components expose neither tools nor other capabilities", func(t *testing.T) {
		disabled := instance.DeepCopy()
		disabled.Spec.LegacyDisabledComponents = []string{component.ID}
		require.Empty(t, ComponentsForInstance(parent, *disabled))
	})

	t.Run("policy edit retires connection overrides", func(t *testing.T) {
		changed := parent.DeepCopy()
		changed.Spec.Manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{
			Key:    "TOKEN",
			Policy: types.VMCPConfigurationPolicyProhibited,
		}}
		require.Equal(t, changed.Spec.Manifest.Components, ComponentsForInstance(*changed, instance))
	})

	t.Run("fixed configuration rotation retires connection overrides", func(t *testing.T) {
		changed := parent.DeepCopy()
		changed.Spec.ComponentStaticConfigurationHashes = map[string]string{component.ID: "rotated"}
		require.Equal(t, changed.Spec.Manifest.Components, ComponentsForInstance(*changed, instance))
	})

	t.Run("ordinary instance uses independent parent copy", func(t *testing.T) {
		components := ComponentsForInstance(parent, v1.VMCPInstance{})
		require.Equal(t, parent.Spec.Manifest.Components, components)
		components[0].CatalogEntry.Manifest.NPXConfig.Package = "mutated-result"
		require.Equal(t, "current-package", parent.Spec.Manifest.Components[0].CatalogEntry.Manifest.NPXConfig.Package)
	})
}

func TestStaticConfigurationRotationRetainsUnrelatedLegacyComponents(t *testing.T) {
	parent := v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{
				ID:            "one",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
			},
			{
				ID:            "two",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
			},
		},
	}}}
	configuration := map[string]string{
		ConfigurationKey("one", "TOKEN"): "first",
		ConfigurationKey("two", "TOKEN"): "second",
	}
	SetStaticConfigurationHashes(&parent, configuration)
	originalHash := parent.Spec.StaticConfigurationHash
	instance := v1.VMCPInstance{}
	for _, component := range parent.Spec.Manifest.Components {
		legacy := *component.DeepCopy()
		legacy.SourceDigest = utils.Digest([]any{component, parent.Spec.ComponentStaticConfigurationHashes[component.ID]})
		legacy.ToolPrefix = "legacy_"
		legacy.ToolOverrides = []types.ToolOverride{{Name: "echo", Enabled: true}}
		instance.Spec.LegacyComponents = append(instance.Spec.LegacyComponents, legacy)
	}
	require.Equal(t, instance.Spec.LegacyComponents, ComponentsForInstance(parent, instance))

	configuration[ConfigurationKey("one", "TOKEN")] = "rotated"
	SetStaticConfigurationHashes(&parent, configuration)
	require.NotEqual(t, originalHash, parent.Spec.StaticConfigurationHash)
	require.Equal(t, []types.VMCPComponent{
		parent.Spec.Manifest.Components[0],
		instance.Spec.LegacyComponents[1],
	}, ComponentsForInstance(parent, instance))

	parent.Spec.Manifest.Components = parent.Spec.Manifest.Components[:1]
	SetStaticConfigurationHashes(&parent, configuration)
	require.NotContains(t, parent.Spec.ComponentStaticConfigurationHashes, "two")
}

func TestLegacyComponentNeedsUpdate(t *testing.T) {
	npx := func(pkg string) types.VMCPComponent {
		return types.VMCPComponent{
			ID: "component1",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "Server",
					Runtime: types.RuntimeNPX,
					NPXConfig: &types.NPXRuntimeConfig{
						Package: pkg,
					},
					Config: []types.MCPConfig{
						{
							Key:         "TOKEN",
							Required:    true,
							Sensitive:   true,
							UserAllowed: true,
						},
					},
				},
			},
		}
	}
	remote := func(config types.RemoteCatalogConfig) types.VMCPComponent {
		return types.VMCPComponent{
			ID: "component1",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:         "Server",
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &config,
				},
			},
		}
	}

	for _, tc := range []struct {
		name      string
		component types.VMCPComponent
		legacy    func(types.VMCPComponent) types.VMCPComponent
		want      bool
	}{
		{
			name:      "same server",
			component: npx("current"),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				return legacy
			},
		},
		{
			name:      "connection configuration and catalog copy",
			component: npx("current"),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.Config[0].Value = "secret"
				legacy.CatalogEntry.Manifest.Config[0].UserAllowed = false
				legacy.CatalogEntry.Manifest.Description = "older description"
				legacy.CatalogEntry.Manifest.StaticConfigurationRevision = "server-revision"
				legacy.ToolPrefix = "mine_"
				return legacy
			},
		},
		{
			name:      "older package",
			component: npx("current"),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.NPXConfig.Package = "older"
				return legacy
			},
			want: true,
		},
		{
			name:      "configuration schema changed",
			component: npx("current"),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.Config = nil
				return legacy
			},
			want: true,
		},
		{
			name:      "connection URL for hostname-constrained remote",
			component: remote(types.RemoteCatalogConfig{Hostname: "example.com"}),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.RemoteConfig.FixedURL = "https://example.com/private"
				return legacy
			},
		},
		{
			name:      "rendered URL for templated remote",
			component: remote(types.RemoteCatalogConfig{URLTemplate: "https://${TENANT}.example.com/mcp"}),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.RemoteConfig.FixedURL = "https://tenant.example.com/mcp"
				return legacy
			},
		},
		{
			name:      "changed fixed URL",
			component: remote(types.RemoteCatalogConfig{FixedURL: "https://example.com/v2"}),
			legacy: func(legacy types.VMCPComponent) types.VMCPComponent {
				legacy.CatalogEntry.Manifest.RemoteConfig.FixedURL = "https://example.com/v1"
				return legacy
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := *tc.component.DeepCopy()
			legacy := tc.legacy(*tc.component.DeepCopy())
			require.Equal(t, tc.want, LegacyComponentNeedsUpdate(legacy, tc.component))
			require.Equal(t, original, tc.component)
		})
	}
}

func TestLegacySnapshotOmitsConnectionConfiguration(t *testing.T) {
	component := types.VMCPComponent{
		ID: "component1",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{Hostname: "example.com"},
				Config: []types.MCPConfig{
					{
						Key:         "TOKEN",
						UserAllowed: true,
					},
				},
			},
		},
	}
	legacy := *component.DeepCopy()
	legacy.CatalogEntry.Manifest.RemoteConfig.FixedURL = "https://example.com/private"
	legacy.CatalogEntry.Manifest.Config[0].Value = "secret"
	legacy.CatalogEntry.Manifest.Config[0].UserAllowed = false
	legacy.CatalogEntry.Manifest.Config = append(legacy.CatalogEntry.Manifest.Config, types.MCPConfig{
		Key:   "OLD",
		Value: "old-secret",
	})

	snapshot := LegacySnapshot(legacy, component)
	require.Empty(t, snapshot.Manifest.RemoteConfig.FixedURL)
	require.Equal(t, "example.com", snapshot.Manifest.RemoteConfig.Hostname)
	require.Equal(t, []types.MCPConfig{
		{
			Key:         "TOKEN",
			UserAllowed: true,
		},
		{
			Key: "OLD",
		},
	}, snapshot.Manifest.Config)
	require.Equal(t, "https://example.com/private", legacy.CatalogEntry.Manifest.RemoteConfig.FixedURL, "the retained snapshot is unchanged")
	require.Equal(t, "secret", legacy.CatalogEntry.Manifest.Config[0].Value, "the retained snapshot is unchanged")
}
