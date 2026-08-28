package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
)

func TestStaticConfigurationSensitiveOnlyAndRedaction(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{
		Env: []types.MCPEnv{
			{Key: "SECRET", Value: "hidden", Sensitive: true},
			{Key: "PUBLIC", Value: "inline"},
		},
		RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Key: "Authorization", Value: "token", Sensitive: true}}},
	}

	secrets := ExtractStaticCatalogConfiguration(&manifest, nil, true)
	assert.Empty(t, manifest.Env[0].Value)
	assert.Equal(t, "inline", manifest.Env[1].Value)
	assert.Empty(t, manifest.RemoteConfig.Headers[0].Value)
	assert.Equal(t, "hidden", secrets["SECRET"])
	assert.Equal(t, "token", secrets["Authorization"])

	RedactStaticCatalogConfiguration(&manifest, secrets)
	assert.True(t, manifest.Env[0].ValueConfigured)
	assert.False(t, manifest.Env[1].ValueConfigured)
	assert.True(t, manifest.RemoteConfig.Headers[0].ValueConfigured)
	assert.Empty(t, manifest.Env[0].Value)
	assert.Equal(t, "inline", manifest.Env[1].Value)
}

func TestStaticConfigurationRedactsLegacyPlaintext(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{
		Env: []types.MCPEnv{{
			Key: "SECRET", Value: "legacy-plaintext", Sensitive: true,
		}},
	}

	RedactStaticCatalogConfiguration(&manifest, map[string]string{})
	assert.Empty(t, manifest.Env[0].Value)
	assert.True(t, manifest.Env[0].ValueConfigured)
}

func TestStaticConfigurationListRedactionPreservesConfiguredStatus(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
		Key: "SECRET", Value: "legacy-plaintext", Sensitive: true,
	}}}
	secrets := ExtractStaticCatalogConfiguration(&manifest, nil, false)

	RedactStaticCatalogConfiguration(&manifest, nil)

	assert.Equal(t, "legacy-plaintext", secrets["SECRET"])
	assert.Empty(t, manifest.Env[0].Value)
	assert.True(t, manifest.Env[0].ValueConfigured)
}

func TestStaticConfigurationDoesNotOwnSecretBoundOrOptionFields(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{
		{Key: "BOUND", Sensitive: true, SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "token"}},
		{Key: "OPTION", Sensitive: true, Options: []types.MCPConfigurationOption{{Name: "One", Value: "one"}}},
	}}

	secrets := ExtractStaticCatalogConfiguration(&manifest, nil, true)
	assert.Empty(t, secrets)
	assert.False(t, CatalogHasSensitiveStaticConfiguration(&manifest))
}

func TestStaticConfigurationEditablePreserveReplaceAndClear(t *testing.T) {
	manifest := types.MCPServerManifest{Env: []types.MCPEnv{{Key: "TOKEN", Value: "one", Sensitive: true}}}
	secrets := ExtractStaticServerConfiguration(&manifest, nil, true)

	manifest.Env[0].ValueConfigured = true
	secrets = ExtractStaticServerConfiguration(&manifest, secrets, true)
	assert.Equal(t, "one", secrets["TOKEN"])

	manifest.Env[0].Value = "two"
	secrets = ExtractStaticServerConfiguration(&manifest, secrets, true)
	assert.Equal(t, "two", secrets["TOKEN"])

	manifest.Env[0].ValueConfigured = false
	secrets = ExtractStaticServerConfiguration(&manifest, secrets, true)
	assert.Empty(t, secrets)
}

func TestStaticConfigurationAuthoritativeIgnoresValueConfigured(t *testing.T) {
	manifest := types.MCPServerManifest{Env: []types.MCPEnv{{Key: "TOKEN", Value: "one", Sensitive: true}}}
	secrets := ExtractStaticServerConfiguration(&manifest, nil, false)
	manifest.Env[0].ValueConfigured = true
	secrets = ExtractStaticServerConfiguration(&manifest, secrets, false)
	assert.Empty(t, secrets)
}

func TestStaticConfigurationMigrationDetectionStopsAfterExtraction(t *testing.T) {
	manifest := types.MCPServerManifest{
		Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "plaintext", Sensitive: true,
		}},
	}

	assert.True(t, ServerHasStaticConfigurationValues(&manifest))
	assert.True(t, ServerHasSensitiveStaticConfiguration(&manifest))
	ExtractStaticServerConfiguration(&manifest, nil, false)
	assert.False(t, ServerHasStaticConfigurationValues(&manifest))
	assert.True(t, ServerHasSensitiveStaticConfiguration(&manifest))
}

func TestStaticConfigurationCompositePathsDoNotCollide(t *testing.T) {
	component := func(value string) types.CatalogComponentServer {
		return types.CatalogComponentServer{CatalogEntryID: "repeated", Manifest: types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{Key: "TOKEN", Value: value, Sensitive: true}}}}
	}
	manifest := types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeComposite, CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{component("one"), component("two")}}}
	secrets := ExtractStaticCatalogConfiguration(&manifest, nil, false)
	assert.Equal(t, map[string]string{
		"component/repeated/0/TOKEN": "one",
		"component/repeated/1/TOKEN": "two",
	}, secrets)
	hydrated := HydrateStaticCatalogConfiguration(manifest, secrets)
	assert.Empty(t, manifest.CompositeConfig.ComponentServers[0].Manifest.Env[0].Value)
	assert.Empty(t, manifest.CompositeConfig.ComponentServers[1].Manifest.Env[0].Value)
	assert.Equal(t, "one", hydrated.CompositeConfig.ComponentServers[0].Manifest.Env[0].Value)
	assert.Equal(t, "two", hydrated.CompositeConfig.ComponentServers[1].Manifest.Env[0].Value)
}

func TestStaticCatalogConfigurationSurvivesComponentReordering(t *testing.T) {
	component := func(id, value string) types.CatalogComponentServer {
		return types.CatalogComponentServer{CatalogEntryID: id, Manifest: types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{Key: "TOKEN", Value: value, Sensitive: true}}}}
	}
	manifest := types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeComposite, CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{component("one", "secret-one"), component("two", "secret-two")}}}
	secrets := ExtractStaticCatalogConfiguration(&manifest, nil, true)
	RedactStaticCatalogConfiguration(&manifest, secrets)

	manifest.CompositeConfig.ComponentServers[0], manifest.CompositeConfig.ComponentServers[1] = manifest.CompositeConfig.ComponentServers[1], manifest.CompositeConfig.ComponentServers[0]
	secrets = ExtractStaticCatalogConfiguration(&manifest, secrets, true)
	manifest = HydrateStaticCatalogConfiguration(manifest, secrets)

	assert.Equal(t, "secret-two", manifest.CompositeConfig.ComponentServers[0].Manifest.Env[0].Value)
	assert.Equal(t, "secret-one", manifest.CompositeConfig.ComponentServers[1].Manifest.Env[0].Value)
}

func TestStaticServerConfigurationSurvivesComponentReordering(t *testing.T) {
	component := func(id, value string) types.ComponentServer {
		return types.ComponentServer{CatalogEntryID: id, Manifest: types.MCPServerManifest{Env: []types.MCPEnv{{Key: "TOKEN", Value: value, Sensitive: true}}}}
	}
	manifest := types.MCPServerManifest{Runtime: types.RuntimeComposite, CompositeConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{component("one", "secret-one"), component("two", "secret-two")}}}
	secrets := ExtractStaticServerConfiguration(&manifest, nil, true)
	RedactStaticServerConfiguration(&manifest, secrets)

	manifest.CompositeConfig.ComponentServers[0], manifest.CompositeConfig.ComponentServers[1] = manifest.CompositeConfig.ComponentServers[1], manifest.CompositeConfig.ComponentServers[0]
	secrets = ExtractStaticServerConfiguration(&manifest, secrets, true)
	manifest = HydrateStaticServerConfiguration(manifest, secrets)

	assert.Equal(t, "secret-two", manifest.CompositeConfig.ComponentServers[0].Manifest.Env[0].Value)
	assert.Equal(t, "secret-one", manifest.CompositeConfig.ComponentServers[1].Manifest.Env[0].Value)
}

func TestStaticConfigurationEnvAndHeaderShareUserStyleKey(t *testing.T) {
	manifest := types.MCPServerManifest{
		Env: []types.MCPEnv{{Key: "TOKEN", Value: "env", Sensitive: true}},
		RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{
			Key: "TOKEN", Value: "header", Sensitive: true,
		}}},
	}

	secrets := ExtractStaticServerConfiguration(&manifest, nil, false)
	assert.Equal(t, map[string]string{"TOKEN": "header"}, secrets)

	hydrated := HydrateStaticServerConfiguration(manifest, secrets)
	assert.Empty(t, manifest.Env[0].Value)
	assert.Empty(t, manifest.RemoteConfig.Headers[0].Value)
	assert.Equal(t, "header", hydrated.Env[0].Value)
	assert.Equal(t, "header", hydrated.RemoteConfig.Headers[0].Value)
}

func TestHydrateStaticSystemConfigurationDoesNotMutateSource(t *testing.T) {
	catalog := types.SystemMCPServerCatalogEntryManifest{Env: []types.MCPEnv{{Key: "TOKEN", Sensitive: true}}}
	hydratedCatalog := HydrateStaticSystemCatalogConfiguration(catalog, map[string]string{"TOKEN": "catalog"})
	assert.Empty(t, catalog.Env[0].Value)
	assert.Equal(t, "catalog", hydratedCatalog.Env[0].Value)

	server := types.SystemMCPServerManifest{Env: []types.MCPEnv{{Key: "TOKEN", Sensitive: true}}}
	hydratedServer := HydrateStaticSystemServerConfiguration(server, map[string]string{"TOKEN": "server"})
	assert.Empty(t, server.Env[0].Value)
	assert.Equal(t, "server", hydratedServer.Env[0].Value)
}

func TestMergeRuntimeConfigurationStaticOverridesUser(t *testing.T) {
	merged := MergeRuntimeConfiguration(
		map[string]string{"STATIC": "attack", "USER": "new"},
		map[string]string{"STATIC": "catalog"},
	)
	assert.Equal(t, map[string]string{"STATIC": "catalog", "USER": "new"}, merged)
}
