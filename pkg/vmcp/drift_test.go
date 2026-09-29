package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/utils"
)

func TestNeedsUpdate(t *testing.T) {
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
			name:      "legacy digest of an exact snapshot",
			component: types.VMCPComponent{CatalogEntry: source, SourceDigest: utils.Digest(source)},
		},
		{
			name:      "digest of a stripped snapshot",
			component: types.VMCPComponent{CatalogEntry: stripped, SourceDigest: SourceDigest(source)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if NeedsUpdate(tt.component, source) {
				t.Fatal("unchanged source reported an update")
			}
			if NeedsUpdate(tt.component, informational(source)) {
				t.Fatal("informational fields reported an update")
			}
			if !NeedsUpdate(tt.component, changed(source)) {
				t.Fatal("configuration change did not report an update")
			}
		})
	}

	legacy := types.VMCPComponent{CatalogEntry: stripped, SourceDigest: utils.Digest(source)}
	if NeedsUpdate(legacy, source) {
		t.Fatal("unchanged source reported an update for a legacy stripped digest")
	}
	if !NeedsUpdate(legacy, changed(source)) {
		t.Fatal("configuration change did not report an update for a legacy stripped digest")
	}
}
