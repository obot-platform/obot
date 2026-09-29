package mcpcatalog

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// The vMCP migration needs current entries even when the catalog synced within the hour.
func TestSyncNowIgnoresRecentSync(t *testing.T) {
	dir := t.TempDir()
	writeParseTestManifest(t, dir, "Fresh", "from source")

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	catalog.Annotations = map[string]string{forceSyncStartupAnnotation: startupSyncGeneration}
	catalog.Status.LastSyncTime = metav1.Now()
	client := newCatalogFakeClient(catalog)

	require.NoError(t, newParseTestHandler(t).SyncNow(t.Context(), client, kclient.ObjectKeyFromObject(catalog)))

	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, client.List(t.Context(), &entries))
	require.Len(t, entries.Items, 1)
	require.Equal(t, "Fresh", entries.Items[0].Spec.Manifest.Name)

	var current v1.MCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
	require.NotContains(t, current.Annotations, v1.MCPCatalogSyncAnnotation)
	require.False(t, current.Status.IsSyncing)
}
