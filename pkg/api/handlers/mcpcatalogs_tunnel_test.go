package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSyncedCatalogEntryTunnelAssignment(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry1private",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "default",
			SourceURL:      "https://github.com/example/catalog",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:           "Private MCP",
				EntryKey:       "private",
				MinObotVersion: "v0.1.0",
				Runtime:        types.RuntimeRemote,
				RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
			},
		},
	}
	storage := newFakeStorage(t,
		&v1.MCPCatalog{Name: "default", Namespace: system.DefaultNamespace}, entry,
		&v1.MCPTunnel{
			Name:      "mt1office",
			Namespace: system.DefaultNamespace,
			Spec:      v1.MCPTunnelSpec{Manifest: types.MCPTunnelManifest{DisplayName: "Office", AllowedURLs: []string{"https://example.com/mcp"}}},
		},
	)
	require.NoError(t, storage.Create(t.Context(), &v1.MCPTunnel{
		Name:      "mt1restricted",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPTunnelSpec{Manifest: types.MCPTunnelManifest{
			DisplayName: "Restricted",
			AllowedURLs: []string{"https://other.example.com/mcp"},
		}},
	}))
	handler := &MCPCatalogHandler{}
	update := func(manifest types.MCPServerCatalogEntryManifest) error {
		t.Helper()
		req, _ := catalogEntryRequest(t, storage, nil, http.MethodPut,
			"/api/mcp-catalogs/default/entries/"+entry.Name, manifest,
			map[string]string{"catalog_id": "default", "entry_id": entry.Name})
		return handler.UpdateEntry(req)
	}
	req, recorder := catalogEntryRequest(t, storage, nil, http.MethodGet,
		"/api/mcp-catalogs/default/entries/"+entry.Name, nil,
		map[string]string{"catalog_id": "default", "entry_id": entry.Name})
	require.NoError(t, handler.GetEntry(req))
	var response types.MCPServerCatalogEntry
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	manifest := response.Manifest
	manifest.RemoteConfig.TunnelName = "mt1office"
	require.NoError(t, update(manifest), "assigning a tunnel must not require taking ownership of a synced entry")
	var stored v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &stored))
	require.True(t, stored.IsGitManaged())
	require.Equal(t, "mt1office", stored.Spec.Manifest.RemoteConfig.TunnelName)
	// The existing conversion to a runtime component carries the assignment to vMCPs.
	runtimeManifest, err := types.MapCatalogEntryToServer(stored.Spec.Manifest, "", false)
	require.NoError(t, err)
	require.Equal(t, "mt1office", runtimeManifest.RemoteConfig.TunnelName)
	require.NoError(t, update(manifest), "repeated assignment is idempotent")

	changed := *manifest.DeepCopy()
	changed.Name = "not from Git"
	require.ErrorContains(t, update(changed), "entry is not editable")
	changed = *manifest.DeepCopy()
	changed.RemoteConfig.FixedURL = "https://other.example.com/mcp"
	require.ErrorContains(t, update(changed), "entry is not editable")
	changed = *manifest.DeepCopy()
	changed.RemoteConfig.TunnelName = "mt1missing"
	require.ErrorContains(t, update(changed), "failed to get MCP tunnel")

	changed.RemoteConfig.TunnelName = "mt1restricted"
	require.ErrorContains(t, update(changed), "does not allow target")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &stored))
	require.Equal(t, "mt1office", stored.Spec.Manifest.RemoteConfig.TunnelName, "rejected writes leave the assignment intact")

	manifest.RemoteConfig.TunnelName = ""
	require.NoError(t, update(manifest))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &stored))
	require.Equal(t, entry.Spec, stored.Spec, "unassigning preserves the Git-managed configuration")
}
