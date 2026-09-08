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
