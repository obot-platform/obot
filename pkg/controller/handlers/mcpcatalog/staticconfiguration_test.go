package mcpcatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCatalogSyncStoresEntryStaticConfiguration(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry.yaml")
	entry := `type: entry
entryKey: search
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
config:
  - key: API_TOKEN
    usage: env
    required: true
    sensitive: true
    value: secret
`
	require.NoError(t, os.WriteFile(entryPath, []byte(entry), 0o600))
	vmcp := `type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    id: search
    mcpServerCatalogEntryKey: search
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(vmcp), 0o600))

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	sync := func() (v1.MCPServerCatalogEntry, v1.VMCP) {
		t.Helper()
		current := &v1.MCPCatalog{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
		require.NoError(t, client.Update(t.Context(), current))
		require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: current}, &parseTestResponse{}))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
		require.Empty(t, current.Status.SyncErrors)

		var entries v1.MCPServerCatalogEntryList
		require.NoError(t, client.List(t.Context(), &entries))
		require.Len(t, entries.Items, 1)
		var vmcps v1.VMCPList
		require.NoError(t, client.List(t.Context(), &vmcps))
		require.Len(t, vmcps.Items, 1)
		return entries.Items[0], vmcps.Items[0]
	}
	reveal := func(entryName, revision string) map[string]string {
		t.Helper()
		values, err := mcp.RevealStaticConfiguration(t.Context(), handler.gatewayClient, entryName, revision)
		require.NoError(t, err)
		return values
	}

	first, firstVMCP := sync()
	field := first.Spec.Manifest.Config[0]
	require.True(t, field.Static)
	require.Empty(t, field.Value)
	revision := first.Spec.Manifest.StaticConfigurationRevision
	require.NotEmpty(t, revision)
	require.Equal(t, map[string]string{"API_TOKEN": "secret"}, reveal(first.Name, revision))

	// The vMCP snapshot references the entry's credential instead of copying the value.
	snapshot := firstVMCP.Spec.Manifest.Components[0].CatalogEntry.Manifest
	require.True(t, snapshot.Config[0].Static)
	require.Empty(t, snapshot.Config[0].Value)
	require.Equal(t, revision, snapshot.StaticConfigurationRevision)
	require.Empty(t, firstVMCP.Spec.Manifest.Components[0].Configuration)

	// Syncing unchanged values keeps the revision, so the entry does not change.
	unchanged, _ := sync()
	require.Equal(t, revision, unchanged.Spec.Manifest.StaticConfigurationRevision)

	require.NoError(t, os.WriteFile(entryPath, []byte(strings.Replace(entry, "value: secret", "value: rotated", 1)), 0o600))
	rotated, _ := sync()
	rotatedRevision := rotated.Spec.Manifest.StaticConfigurationRevision
	require.NotEqual(t, revision, rotatedRevision)
	require.Equal(t, map[string]string{"API_TOKEN": "rotated"}, reveal(rotated.Name, rotatedRevision))
	require.Equal(t, map[string]string{"API_TOKEN": "secret"}, reveal(rotated.Name, revision))

	// A value removed from the source is removed from the entry.
	require.NoError(t, os.WriteFile(entryPath, []byte(strings.Replace(entry, "    value: secret\n", "", 1)), 0o600))
	current := &v1.MCPCatalog{}
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
	current.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
	require.NoError(t, client.Update(t.Context(), current))
	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: current}, &parseTestResponse{}))
	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, client.List(t.Context(), &entries))
	require.Len(t, entries.Items, 1)
	require.False(t, entries.Items[0].Spec.Manifest.Config[0].Static)
	require.Empty(t, entries.Items[0].Spec.Manifest.StaticConfigurationRevision)
}

func TestCatalogSyncRejectsStaticFieldWithoutValue(t *testing.T) {
	dir := t.TempDir()
	entry := `type: entry
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
config:
  - key: API_TOKEN
    usage: env
    static: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(entry), 0o600))

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	catalog.Annotations = map[string]string{v1.MCPCatalogSyncAnnotation: "true"}
	require.NoError(t, client.Update(t.Context(), catalog))
	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{}))

	current := &v1.MCPCatalog{}
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
	require.Contains(t, current.Status.SyncErrors[dir], `static configuration "API_TOKEN" requires a value`)
	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, client.List(t.Context(), &entries))
	require.Empty(t, entries.Items)
}
