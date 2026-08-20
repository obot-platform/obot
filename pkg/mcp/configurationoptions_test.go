package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func testConfigurationOptions() []types.MCPConfigurationOption {
	return []types.MCPConfigurationOption{
		{Name: "United States", Value: "us", Description: "US endpoint"},
		{Name: "Europe", Value: "eu"},
	}
}

func TestValidateCatalogEntryManifestConfigurationOptions(t *testing.T) {
	base := types.MCPServerCatalogEntryManifest{
		ServerUserType: types.ServerUserTypeSingleUser,
		Runtime:        types.RuntimeNPX,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "test-server"},
		Env: []types.MCPEnv{{
			Key: "REGION", Name: "Region", Required: true, Options: testConfigurationOptions()}},
	}

	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), base, true, ValidationOptions{}))
	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), base, false, ValidationOptions{}))

	tests := []struct {
		name    string
		mutate  func(*types.MCPServerCatalogEntryManifest)
		wantErr string
	}{
		{
			name:    "static value",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Env[0].Value = "us" },
			wantErr: "value and options are mutually exclusive",
		},
		{
			name: "secret binding",
			mutate: func(m *types.MCPServerCatalogEntryManifest) {
				m.Env[0].SecretBinding = &types.MCPSecretBinding{Name: "secret", Key: "region"}
			},
			wantErr: "secretBinding and options are mutually exclusive",
		},
		{
			name:    "blank name",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Env[0].Options[0].Name = " " },
			wantErr: "name cannot be empty",
		},
		{
			name:    "blank value",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Env[0].Options[0].Value = " " },
			wantErr: "value cannot be empty",
		},
		{
			name:    "duplicate value",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Env[0].Options[1].Value = "us" },
			wantErr: "duplicate value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := *base.DeepCopy()
			tt.mutate(&manifest)
			require.ErrorContains(t, ValidateCatalogEntryManifest(t.Context(), manifest, false, ValidationOptions{}), tt.wantErr)
		})
	}
}

func TestValidateConfiguredOptions(t *testing.T) {
	field := types.MCPEnv{Key: "REGION", Required: true, Options: testConfigurationOptions()}

	missing, err := ValidateConfiguredOptions([]types.MCPEnv{field}, nil, map[string]string{"REGION": "eu"})
	require.NoError(t, err)
	require.Empty(t, missing)

	missing, err = ValidateConfiguredOptions([]types.MCPEnv{field}, nil, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"REGION"}, missing)

	_, err = ValidateConfiguredOptions([]types.MCPEnv{field}, nil, map[string]string{"REGION": "ap"})
	require.EqualError(t, err, `env "REGION" value "ap" is not one of the configured options`)

	field.Required = false
	missing, err = ValidateConfiguredOptions([]types.MCPEnv{field}, nil, nil)
	require.NoError(t, err)
	require.Empty(t, missing)
	require.True(t, ConfigurationOptionValueValid(field.MCPHeader, nil))
	require.False(t, ConfigurationOptionValueValid(field.MCPHeader, map[string]string{"REGION": "stale"}))

	header := types.MCPHeader{Key: "X-REGION", Required: true, Options: testConfigurationOptions()}
	missing, err = ValidateConfiguredOptions(nil, []types.MCPHeader{header}, map[string]string{"X-REGION": "us"})
	require.NoError(t, err)
	require.Empty(t, missing)
	_, err = ValidateConfiguredOptions(nil, []types.MCPHeader{header}, map[string]string{"X-REGION": "stale"})
	require.EqualError(t, err, `header "X-REGION" value "stale" is not one of the configured options`)
}

func TestValidateCatalogConfigurationConstraints(t *testing.T) {
	catalog := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{
			Key: "REGION", Prefix: "region-", Required: true, Sensitive: true, Options: testConfigurationOptions()}},
		RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${REGION}.example.com/mcp"},
	}
	server, err := types.MapCatalogEntryToServer(catalog, "", false)
	require.NoError(t, err)
	require.NoError(t, ValidateCatalogConfigurationConstraints(server, catalog))

	t.Run("option definition changed", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Env[0].Options = []types.MCPConfigurationOption{{Name: "Anything", Value: "anything"}}
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `env "REGION" configuration must match`)
	})

	t.Run("option semantics changed", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Env[0].Required = false
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `env "REGION" configuration must match`)
	})

	t.Run("option field injected", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Env = append(changed.Env, types.MCPEnv{
			Key: "INJECTED", Options: []types.MCPConfigurationOption{{Name: "Injected", Value: "injected"}}})
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `env "INJECTED" configuration must match`)
	})

	t.Run("unrelated remote fields are not constrained", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.RemoteConfig.URL = "https://changed.example.com/mcp"
		changed.RemoteConfig.URLTemplate = "https://changed.example.com/${REGION}"
		require.NoError(t, ValidateCatalogConfigurationConstraints(changed, catalog))
	})
}

func TestValidateCatalogConfigurationConstraintsCompositeOptions(t *testing.T) {
	componentCatalog := types.MCPServerCatalogEntryManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "component"},
		Env:       []types.MCPEnv{{Key: "REGION", Options: testConfigurationOptions()}},
	}
	matchingComponent, err := types.MapCatalogEntryToServer(componentCatalog, "", false)
	require.NoError(t, err)
	compositeCatalog := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{{
			CatalogEntryID: "component", Manifest: componentCatalog,
		}}},
	}
	matchingComposite := types.MCPServerManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{{
			CatalogEntryID: "component", Manifest: matchingComponent,
		}}},
	}
	require.NoError(t, ValidateCatalogConfigurationConstraints(matchingComposite, compositeCatalog))

	missingComponent := matchingComposite
	missingComponent.CompositeConfig = nil
	require.ErrorContains(t, ValidateCatalogConfigurationConstraints(missingComponent, compositeCatalog), `component "component" catalog-owned configuration options are missing`)

	injected := types.MCPServerManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{{
			CatalogEntryID: "injected",
			Manifest: types.MCPServerManifest{Env: []types.MCPEnv{{
				Key: "REGION", Options: testConfigurationOptions()}}},
		}}},
	}
	require.ErrorContains(t, ValidateCatalogConfigurationConstraints(injected, types.MCPServerCatalogEntryManifest{}), `component "injected" configuration options must be defined by the source catalog entry`)

	injected.CompositeConfig.ComponentServers[0].Manifest.Env[0].Options = nil
	require.NoError(t, ValidateCatalogConfigurationConstraints(injected, types.MCPServerCatalogEntryManifest{}))
}
