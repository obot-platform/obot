package controller

import (
	"fmt"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestLegacyNeedsUpdate(t *testing.T) {
	source := types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
		Name:        "server",
		Description: "original",
		Runtime:     types.RuntimeRemote,
		Config:      []types.MCPConfig{{Key: "TOKEN", Value: "fixed"}},
	}}
	informational := func(snapshot types.MCPServerCatalogEntrySnapshot) types.MCPServerCatalogEntrySnapshot {
		snapshot.Manifest.Metadata = map[string]string{"categories": "Developer Tools"}
		snapshot.Manifest.ShortDescription = "new short description"
		snapshot.Manifest.Description = "new description"
		snapshot.Manifest.Icon = "https://example.com/icon.png"
		snapshot.Manifest.UpgradeNote = "Read before upgrading."
		snapshot.Manifest.RepoURL = "https://github.com/example/server"
		snapshot.Manifest.EntryKey = "new-entry-key"
		snapshot.Manifest.ToolPreview = []types.MCPServerTool{{Name: "tool"}}
		return snapshot
	}
	changed := func(snapshot types.MCPServerCatalogEntrySnapshot) types.MCPServerCatalogEntrySnapshot {
		snapshot.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Value: "changed"}}
		return snapshot
	}
	// Migrations store the source digest before stripping fixed values.
	stripped := source
	stripped.Manifest.Config = []types.MCPConfig{{Key: "TOKEN"}}

	tests := []struct {
		name      string
		component types.VMCPComponent
	}{
		{
			name:      "no stored digest",
			component: types.VMCPComponent{CatalogEntry: source},
		},
		{
			name:      "digest of an exact snapshot",
			component: types.VMCPComponent{CatalogEntry: source, SourceDigest: utils.Digest(source)},
		},
		{
			name:      "previous source digest of an exact snapshot",
			component: types.VMCPComponent{CatalogEntry: source, SourceDigest: legacySourceDigests(source)[1]},
		},
		{
			name:      "source digest of a stripped snapshot",
			component: types.VMCPComponent{CatalogEntry: stripped, SourceDigest: vmcpconfig.SourceDigest(source)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, legacyNeedsUpdate(tt.component, source), "unchanged source reported an update")
			assert.False(t, legacyNeedsUpdate(tt.component, informational(source)), "informational fields reported an update")
			assert.True(t, legacyNeedsUpdate(tt.component, changed(source)), "configuration change did not report an update")
		})
	}

	for _, digest := range []string{utils.Digest(source), legacySourceDigests(source)[1]} {
		legacy := types.VMCPComponent{CatalogEntry: stripped, SourceDigest: digest}
		assert.False(t, legacyNeedsUpdate(legacy, source), "unchanged source reported an update for a legacy stripped digest")
		assert.True(t, legacyNeedsUpdate(legacy, changed(source)), "configuration change did not report an update for a legacy stripped digest")
	}
}

func TestMigrateVMCPSourceDigests(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("failure=", fail), func(t *testing.T) {
			testMigrateVMCPSourceDigests(t, fail)
		})
	}
}

// testMigrateVMCPSourceDigests migrates once, or retries after the vMCP update failed; a retry
// must finish in the same state as a migration that never failed.
func testMigrateVMCPSourceDigests(t *testing.T, fail bool) {
	t.Helper()

	snapshot := types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
		Name:             "Search",
		Runtime:          types.RuntimeRemote,
		RemoteConfig:     &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		Config:           []types.MCPConfig{{Key: "TOKEN", Value: "fixed"}},
		ShortDescription: "search",
	}}
	// The entry has changed only informational fields since the components adopted it.
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry1search",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{Manifest: snapshot.Manifest},
	}
	entry.Spec.Manifest.RepoURL = "https://github.com/example/search"
	entry.Spec.Manifest.ToolPreview = []types.MCPServerTool{{Name: "search"}}
	current := entrySnapshot(*entry)

	// A migration stripped fixed values from these snapshots after their digests were stored.
	stripped := snapshot
	stripped.Manifest.Config = []types.MCPConfig{{Key: "TOKEN"}}
	stale := snapshot
	stale.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Value: "old"}}

	component := func(id string, catalogEntry types.MCPServerCatalogEntrySnapshot, digest string) types.VMCPComponent {
		return types.VMCPComponent{
			ID:                      id,
			MCPServerCatalogEntryID: entry.Name,
			CatalogEntry:            catalogEntry,
			SourceDigest:            digest,
		}
	}
	components := []types.VMCPComponent{
		component("current", snapshot, vmcpconfig.SourceDigest(snapshot)),
		component("full", snapshot, utils.Digest(snapshot)),
		component("previous", snapshot, legacySourceDigests(snapshot)[1]),
		component("empty", snapshot, ""),
		component("stripped", stripped, legacySourceDigests(current)[1]),
		component("drifted", stripped, utils.Digest(stale)),
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1search",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest:                           types.VMCPManifest{Components: components},
			ComponentStaticConfigurationHashes: map[string]string{"full": "hash"},
		},
	}
	legacy := components[1]
	legacy.SourceDigest = utils.Digest([]any{components[1], "hash"})
	instance := &v1.VMCPInstance{
		Name:      "vi1search",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest:         types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			LegacyComponents: []types.VMCPComponent{legacy},
		},
	}

	client := &failingVMCPUpdates{Client: newFakeClient(t, entry, vmcp, instance), fail: fail}
	if fail {
		require.Error(t, migrateVMCPSourceDigests(t.Context(), client))
		client.fail = false
	}
	require.NoError(t, migrateVMCPSourceDigests(t.Context(), client))
	// A retried migration changes nothing.
	require.NoError(t, migrateVMCPSourceDigests(t.Context(), client))

	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))

	migrated := vmcp.Spec.Manifest.Components
	for _, component := range migrated[:5] {
		assert.Equal(t, vmcpconfig.SourceDigest(current), component.SourceDigest, component.ID)
		assert.False(t, vmcpconfig.NeedsUpdate(component, current), component.ID)
	}
	assert.Equal(t, utils.Digest(stale), migrated[5].SourceDigest)
	assert.True(t, vmcpconfig.NeedsUpdate(migrated[5], current))
	// The kept digest matches again if the source reverts.
	assert.False(t, vmcpconfig.NeedsUpdate(migrated[5], stale))

	// Legacy components stay bound to the migrated component.
	assert.Equal(t, utils.Digest([]any{migrated[1], "hash"}), instance.Spec.LegacyComponents[0].SourceDigest)
	assert.Equal(t, instance.Spec.LegacyComponents[0], vmcpconfig.ComponentsForInstance(*vmcp, *instance)[1])
}
