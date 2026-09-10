package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestInstanceUserURLConfiguration(t *testing.T) {
	parent := v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{
			ID: "github",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{Hostname: "github.example.com"},
			}},
		}},
	}}}
	require.False(t, IsMultiUser(parent.Spec.Manifest))
	components := ComponentsForInstance(parent, v1.VMCPInstance{})
	require.Empty(t, parent.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config)
	require.Equal(t, []string{ConfigurationKey("github", "__url")}, MissingRequiredConfiguration(components[0], nil, true))
	manifest := parent.Spec.Manifest
	manifest.Components = components
	configuration := types.VMCPConfiguration{Components: map[string]map[string]string{
		"github": {"__url": "https://github.example.com/mcp"},
	}}
	encoded, err := ValidateAndEncodeUserConfiguration(manifest, configuration)
	require.NoError(t, err)
	require.Equal(t, "https://github.example.com/mcp", encoded[ConfigurationKey("github", "__url")])
	require.Empty(t, MissingRequiredConfiguration(components[0], encoded, true))
	configuration.Components["github"]["__url"] = "https://other.example.com/mcp"
	_, err = ValidateAndEncodeUserConfiguration(manifest, configuration)
	require.ErrorContains(t, err, "does not match required hostname")
}
