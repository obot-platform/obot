package v1

import (
	"encoding/json"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestLegacyCompositeManifestRoundTrip(t *testing.T) {
	manifest := `{
		"name":"Legacy composite",
		"runtime":"composite",
		"serverUserType":"singleUser",
		"compositeConfig":{"componentServers":[
			{"catalogEntryID":"entry1","toolPrefix":"local_","manifest":{
				"runtime":"npx","npxConfig":{"package":"everything"},
				"env":[{"key":"TOKEN","value":"default","sensitive":true},{"key":"SETTINGS","file":true,"dynamicFile":true}]
			}},
			{"catalogEntryID":"entry2","toolOverrides":[{"name":"echo","enabled":true}],"manifest":{
				"runtime":"remote","remoteConfig":{"fixedURL":"https://example.com/mcp","headers":[{"key":"Authorization","prefix":"Bearer ","required":true}]}
			}}
		]}
	}`
	var entry MCPServerCatalogEntry
	require.NoError(t, json.Unmarshal([]byte(`{"metadata":{"name":"legacy"},"spec":{"manifest":`+manifest+`,"mcpCatalogName":"default","editable":true,"detached":false},"status":{}}`), &entry))
	require.Equal(t, types.RuntimeComposite, entry.Spec.Manifest.Runtime)
	require.JSONEq(t, manifest, string(entry.Spec.LegacyCompositeManifest))

	// A status update and its storage serialization must not erase fields that
	// are no longer represented in the public catalog manifest type.
	updated := entry.DeepCopy()
	updated.Status.ManifestHash = "new-hash"
	updated.Status.UserCount = 2
	updated.Finalizers = []string{"cleanup"}
	data, err := json.Marshal(updated)
	require.NoError(t, err)
	var roundTrip MCPServerCatalogEntry
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.JSONEq(t, manifest, string(roundTrip.Spec.LegacyCompositeManifest))
	require.Equal(t, updated.Status, roundTrip.Status)
	require.Equal(t, updated.Finalizers, roundTrip.Finalizers)
	require.Equal(t, "default", roundTrip.Spec.MCPCatalogName)
	require.True(t, roundTrip.Spec.Editable)
}

func TestLegacyCompositeManifestEmptyAndCurrentCatalogs(t *testing.T) {
	var entry MCPServerCatalogEntry
	emptyComposite := `{"runtime":"composite","compositeConfig":{"componentServers":[]}}`
	require.NoError(t, json.Unmarshal([]byte(`{"spec":{"manifest":`+emptyComposite+`}}`), &entry))
	require.JSONEq(t, emptyComposite, string(entry.Spec.LegacyCompositeManifest))

	// Reusing a decoded object must clear the legacy snapshot for a v2 entry.
	require.NoError(t, json.Unmarshal([]byte(`{"spec":{"manifest":{"runtime":"remote","config":[{"key":"Token","usage":"header"}]}}}`), &entry))
	require.Nil(t, entry.Spec.LegacyCompositeManifest)
	entry.Spec.Manifest.Name = "Edited"
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	var roundTrip MCPServerCatalogEntry
	require.NoError(t, json.Unmarshal(data, &roundTrip))
	require.Equal(t, "Edited", roundTrip.Spec.Manifest.Name)
	require.Equal(t, entry.Spec.Manifest.Config, roundTrip.Spec.Manifest.Config)
	require.Nil(t, roundTrip.Spec.LegacyCompositeManifest)
}
