package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSyncedCatalogEntryTunnelSurvivesSync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entry.yaml")
	write := func(description string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(`entryKey: private
name: Private MCP
shortDescription: Private MCP
description: `+description+`
runtime: remote
remoteConfig:
  fixedURL: https://example.com/mcp
`), 0o600))
	}
	write("original")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(`type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - id: private
    mcpServerCatalogEntryKey: private
`), 0o600))
	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	client := newCatalogFakeClient(catalog, &v1.MCPTunnel{
		Name:      "mt1office",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPTunnelSpec{Manifest: types.MCPTunnelManifest{
			DisplayName: "Office",
			AllowedURLs: []string{"https://example.com/mcp"},
		}},
	})
	handler := newParseTestHandler(t)
	sync := func() {
		t.Helper()
		require.NoError(t, handler.SyncNow(t.Context(), client, kclient.ObjectKeyFromObject(catalog)))
	}
	get := func() v1.MCPServerCatalogEntry {
		t.Helper()
		var entries v1.MCPServerCatalogEntryList
		require.NoError(t, client.List(t.Context(), &entries))
		require.Len(t, entries.Items, 1)
		return entries.Items[0]
	}
	sync()
	entry := get()
	require.Empty(t, entry.Spec.Manifest.RemoteConfig.TunnelName)
	entry.Spec.Manifest.RemoteConfig.TunnelName = "mt1office"
	require.NoError(t, client.Update(t.Context(), &entry))
	write("updated in Git")
	for range 2 {
		sync()
		updated := get()
		require.Equal(t, "updated in Git", updated.Spec.Manifest.Description)
		require.Equal(t, "mt1office", updated.Spec.Manifest.RemoteConfig.TunnelName, "Git sync must preserve operational tunnel assignments")
		require.True(t, updated.IsGitManaged())
		var vmcps v1.VMCPList
		require.NoError(t, client.List(t.Context(), &vmcps))
		require.Len(t, vmcps.Items, 1)
		component := vmcps.Items[0].Spec.Manifest.Components[0]
		require.Equal(t, "mt1office", component.CatalogEntry.Manifest.RemoteConfig.TunnelName, "synced vMCP snapshots must include the operational assignment")
		runtimeManifest, err := types.MapCatalogEntryToServer(component.CatalogEntry.Manifest, "", true)
		require.NoError(t, err)
		require.Equal(t, "mt1office", runtimeManifest.RemoteConfig.TunnelName)
	}
	// A Git change cannot redirect an existing assignment outside the tunnel allowlist.
	require.NoError(t, os.WriteFile(path, []byte(`entryKey: private
name: Private MCP
shortDescription: Private MCP
description: unauthorized target
runtime: remote
remoteConfig:
  fixedURL: https://example.com/other
`), 0o600))
	sync()
	var current v1.MCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
	require.Contains(t, current.Status.SyncErrors[dir], "does not allow target")
	entry = get()
	require.Equal(t, "https://example.com/mcp", entry.Spec.Manifest.RemoteConfig.FixedURL)
	require.Equal(t, "mt1office", entry.Spec.Manifest.RemoteConfig.TunnelName)
	write("updated in Git")

	entry = get()
	entry.Spec.Manifest.RemoteConfig.TunnelName = ""
	require.NoError(t, client.Update(t.Context(), &entry))
	sync()
	require.Empty(t, get().Spec.Manifest.RemoteConfig.TunnelName)
	var vmcps v1.VMCPList
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.Empty(t, vmcps.Items[0].Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.TunnelName)

	// Changing runtime removes an assignment that only applies to remote servers.
	entry = get()
	entry.Spec.Manifest.RemoteConfig.TunnelName = "mt1office"
	require.NoError(t, client.Update(t.Context(), &entry))
	require.NoError(t, os.WriteFile(path, []byte(`entryKey: private
name: Private MCP
shortDescription: Private MCP
description: local runtime
runtime: npx
npxConfig:
  package: test-server
`), 0o600))
	sync()
	entry = get()
	require.Equal(t, types.RuntimeNPX, entry.Spec.Manifest.Runtime)
	require.Nil(t, entry.Spec.Manifest.RemoteConfig)
}
