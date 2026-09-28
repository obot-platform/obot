package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestOpenAPISnapshotConversion(t *testing.T) {
	entry := MCPServerCatalogEntryManifest{
		Runtime: RuntimeOpenAPI,
		OpenAPIConfig: &OpenAPIRuntimeConfig{
			Source: OpenAPISource{URL: "https://example.com/openapi.json"},
			Schema: &runtime.RawExtension{Raw: json.RawMessage(`{"openapi":"3.1.0","paths":{}}`)},
		},
	}
	server, err := MapCatalogEntryToServer(entry, "", false)
	require.NoError(t, err)
	require.Equal(t, entry.OpenAPIConfig, server.OpenAPIConfig)
	entry.OpenAPIConfig.Schema.Raw[0] = ' '
	require.NotEqual(t, entry.OpenAPIConfig.Schema, server.OpenAPIConfig.Schema)

	roundTrip := server.ConvertToCatalogEntry()
	require.Equal(t, server.OpenAPIConfig, roundTrip.OpenAPIConfig)
	roundTrip.OpenAPIConfig.Schema.Raw[0] = ' '
	require.NotEqual(t, server.OpenAPIConfig.Schema, roundTrip.OpenAPIConfig.Schema)

	data, err := json.Marshal(server)
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, json.Unmarshal(data, &response))
	require.IsType(t, map[string]any{}, response["openAPIConfig"].(map[string]any)["schema"])
	var restored MCPServerManifest
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, server, restored)

	snapshot := MCPServerCatalogEntrySnapshot{
		Manifest: server.ConvertToCatalogEntry(),
	}
	data, err = json.Marshal(snapshot)
	require.NoError(t, err)
	var restoredSnapshot MCPServerCatalogEntrySnapshot
	require.NoError(t, json.Unmarshal(data, &restoredSnapshot))
	require.Equal(t, server.OpenAPIConfig, restoredSnapshot.Manifest.OpenAPIConfig)

	_, err = MapCatalogEntryToServer(MCPServerCatalogEntryManifest{Runtime: RuntimeOpenAPI}, "", false)
	require.Error(t, err)
}
