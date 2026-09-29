package vmcp

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/utils"
)

// SourceDigest identifies the parts of a catalog entry that require a vMCP
// component update. Informational fields are excluded so that catalog copy,
// icon, metadata, and upgrade-note changes alone do not report drift.
func SourceDigest(snapshot types.MCPServerCatalogEntrySnapshot) string {
	snapshot.Manifest.Metadata = nil
	snapshot.Manifest.ShortDescription = ""
	snapshot.Manifest.Description = ""
	snapshot.Manifest.Icon = ""
	snapshot.Manifest.UpgradeNote = ""
	return utils.Digest(snapshot)
}

// NeedsUpdate reports whether a component's source has drifted from the
// snapshot deployed by the vMCP.
func NeedsUpdate(component types.VMCPComponent, current types.MCPServerCatalogEntrySnapshot) bool {
	digest := component.SourceDigest
	if digest == "" || digest == utils.Digest(component.CatalogEntry) {
		// The snapshot is an exact copy of its source, so a digest stored in the
		// older full-snapshot form can be recomputed without informational fields.
		digest = SourceDigest(component.CatalogEntry)
	}
	// Migrations that strip fixed values from the snapshot may have stored the
	// older full-snapshot digest, which still matches an unchanged source.
	return digest != SourceDigest(current) && digest != utils.Digest(current)
}
