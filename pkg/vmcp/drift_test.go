package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestNeedsUpdate(t *testing.T) {
	source := types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
		Name:        "server",
		Description: "original",
		Runtime:     types.RuntimeRemote,
		Config:      []types.MCPConfig{{Key: "TOKEN", Value: "fixed"}},
	}}
	informational := source
	informational.Manifest.Metadata = map[string]string{"categories": "Developer Tools"}
	informational.Manifest.ShortDescription = "new short description"
	informational.Manifest.Description = "new description"
	informational.Manifest.Icon = "https://example.com/icon.png"
	informational.Manifest.UpgradeNote = "Read before upgrading."
	informational.Manifest.RepoURL = "https://github.com/example/server"
	informational.Manifest.EntryKey = "new-entry-key"
	informational.Manifest.ToolPreview = []types.MCPServerTool{{Name: "tool"}}
	changed := source
	changed.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Value: "changed"}}

	component := types.VMCPComponent{CatalogEntry: source, SourceDigest: SourceDigest(source)}
	if NeedsUpdate(component, source) {
		t.Fatal("unchanged source reported an update")
	}
	if NeedsUpdate(component, informational) {
		t.Fatal("informational fields reported an update")
	}
	if !NeedsUpdate(component, changed) {
		t.Fatal("configuration change did not report an update")
	}

	// The source digest migration keeps legacy digests of stripped snapshots
	// whose source had drifted. They match again if the source reverts.
	stripped := source
	stripped.Manifest.Config = []types.MCPConfig{{Key: "TOKEN"}}
	for _, digest := range LegacySourceDigests(source) {
		legacy := types.VMCPComponent{CatalogEntry: stripped, SourceDigest: digest}
		if !NeedsUpdate(legacy, changed) {
			t.Fatal("configuration change did not report an update for a legacy digest")
		}
		if NeedsUpdate(legacy, source) {
			t.Fatal("reverted source reported an update for a legacy digest")
		}
	}
}
