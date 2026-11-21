package vmcp

import (
	"encoding/json"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
	"github.com/stretchr/testify/require"
)

func TestComponentConfigCopiesOnlyConfiguration(t *testing.T) {
	component := types.VMCPComponent{
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			OpenAPIConfig: &types.OpenAPIRuntimeConfig{Schema: &types.OpenAPISchema{
				Raw: make(json.RawMessage, openapi.MaxSchemaBytes),
			}},
			Config: []types.MCPConfig{{
				Key:           "TOKEN",
				SecretBinding: &types.MCPSecretBinding{Name: "original", Key: "token"},
				Options:       []types.MCPConfigurationOption{{Value: "original"}},
			}},
		}},
	}
	var config []types.MCPConfig
	allocation := testing.Benchmark(func(b *testing.B) {
		b.Helper()
		for b.Loop() {
			config = ComponentConfig(component)
		}
	})
	// A configuration read must not allocate space for the unrelated 1 MiB schema.
	require.Less(t, allocation.AllocedBytesPerOp(), int64(openapi.MaxSchemaBytes/2))
	config[0].SecretBinding.Name = "changed"
	config[0].Options[0].Value = "changed"
	require.Equal(t, "original", component.CatalogEntry.Manifest.Config[0].SecretBinding.Name)
	require.Equal(t, "original", component.CatalogEntry.Manifest.Config[0].Options[0].Value)
	require.Nil(t, ComponentConfig(types.VMCPComponent{}))
}
