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
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config: []types.MCPConfig{{
			Key: "REGION", Name: "Region", Usage: types.Env, Required: true, Options: testConfigurationOptions()}},
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
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Config[0].Value = "us" },
			wantErr: "value and options are mutually exclusive",
		},
		{
			name: "secret binding",
			mutate: func(m *types.MCPServerCatalogEntryManifest) {
				m.Config[0].SecretBinding = &types.MCPSecretBinding{Name: "secret", Key: "region"}
			},
			wantErr: "secretBinding and options are mutually exclusive",
		},
		{
			name:    "blank name",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Config[0].Options[0].Name = " " },
			wantErr: "name cannot be empty",
		},
		{
			name:    "blank value",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Config[0].Options[0].Value = " " },
			wantErr: "value cannot be empty",
		},
		{
			name:    "duplicate value",
			mutate:  func(m *types.MCPServerCatalogEntryManifest) { m.Config[0].Options[1].Value = "us" },
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

func TestValidateCatalogEntryManifestConfigUsage(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPServerCatalogEntryManifest
		wantErr  string
	}{
		{
			name: "header is allowed for remote runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
				Config:       []types.MCPConfig{{Key: "Authorization", Usage: types.Header}},
			},
		},
		{
			name: "header is allowed for npx runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
				Config:    []types.MCPConfig{{Key: "Authorization", Usage: types.Header}},
			},
		},
		{
			name: "header is allowed for uvx runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "test-server"},
				Config:    []types.MCPConfig{{Key: "Authorization", Usage: types.Header}},
			},
		},
		{
			name: "header is allowed for containerized runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test-server:latest",
					Port:  8080,
					Path:  "/mcp",
				},
				Config: []types.MCPConfig{{Key: "Authorization", Usage: types.Header}},
			},
		},
		{
			name: "sensitive static header is allowed",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
				Config:       []types.MCPConfig{{Key: "Authorization", Usage: types.Header, Value: "Bearer token", Sensitive: true}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCatalogEntryManifest(t.Context(), tt.manifest, false, ValidationOptions{})
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSystemManifestsAllowSensitiveStaticValues(t *testing.T) {
	config := []types.MCPConfig{
		{
			Key:       "Authorization",
			Usage:     types.Header,
			Value:     "Bearer token",
			Sensitive: true,
		},
		{
			Key:       "API_KEY",
			Usage:     types.Env,
			Value:     "token",
			Sensitive: true,
		},
	}
	require.NoError(t, ValidateSystemMCPServerCatalogEntryManifest(t.Context(), types.SystemMCPServerCatalogEntryManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config:    config,
	}, ValidationOptions{}))
	require.NoError(t, ValidateSystemMCPServerManifest(t.Context(), types.SystemMCPServerManifest{
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config:    config,
	}, ValidationOptions{}))
}

func TestValidateConfiguredOptions(t *testing.T) {
	field := types.MCPConfig{Usage: types.Env, Key: "REGION", Required: true, Options: testConfigurationOptions()}

	missing, err := ValidateConfiguredOptions([]types.MCPConfig{field}, map[string]string{"REGION": "eu"})
	require.NoError(t, err)
	require.Empty(t, missing)

	missing, err = ValidateConfiguredOptions([]types.MCPConfig{field}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"REGION"}, missing)

	_, err = ValidateConfiguredOptions([]types.MCPConfig{field}, map[string]string{"REGION": "ap"})
	require.EqualError(t, err, `env "REGION" value "ap" is not one of the configured options`)

	field.Required = false
	missing, err = ValidateConfiguredOptions([]types.MCPConfig{field}, nil)
	require.NoError(t, err)
	require.Empty(t, missing)
	require.True(t, ConfigurationOptionValueValid(field.ToHeader(), nil))
	require.False(t, ConfigurationOptionValueValid(field.ToHeader(), map[string]string{"REGION": "stale"}))

	header := types.MCPConfig{Usage: types.Header, Key: "X-REGION", Required: true, Options: testConfigurationOptions()}
	missing, err = ValidateConfiguredOptions([]types.MCPConfig{header}, map[string]string{"X-REGION": "us"})
	require.NoError(t, err)
	require.Empty(t, missing)
	_, err = ValidateConfiguredOptions([]types.MCPConfig{header}, map[string]string{"X-REGION": "stale"})
	require.EqualError(t, err, `header "X-REGION" value "stale" is not one of the configured options`)
}

func TestValidateCatalogConfigurationConstraints(t *testing.T) {
	catalog := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{{
			Key: "REGION", Usage: types.Env, Prefix: "region-", Required: true, Sensitive: true, Options: testConfigurationOptions()}},
		RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${REGION}.example.com/mcp"},
	}
	server, err := types.MapCatalogEntryToServer(catalog, "", false)
	require.NoError(t, err)
	require.NoError(t, ValidateCatalogConfigurationConstraints(server, catalog))

	t.Run("option definition changed", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Config[0].Options = []types.MCPConfigurationOption{{Name: "Anything", Value: "anything"}}
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `config "REGION" configuration must match`)
	})

	t.Run("option semantics changed", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Config[0].Required = false
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `config "REGION" configuration must match`)
	})

	t.Run("option field injected", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.Config = append(changed.Config, types.MCPConfig{Usage: types.Env,
			Key: "INJECTED", Options: []types.MCPConfigurationOption{{Name: "Injected", Value: "injected"}}})
		require.ErrorContains(t, ValidateCatalogConfigurationConstraints(changed, catalog), `config "INJECTED" configuration must match`)
	})

	t.Run("unrelated remote fields are not constrained", func(t *testing.T) {
		changed := *server.DeepCopy()
		changed.RemoteConfig.URL = "https://changed.example.com/mcp"
		changed.RemoteConfig.URLTemplate = "https://changed.example.com/${REGION}"
		require.NoError(t, ValidateCatalogConfigurationConstraints(changed, catalog))
	})
}
