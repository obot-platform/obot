package vmcp

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/utils"
)

// SourceDigest identifies the parts of a catalog entry that require a vMCP
// component update. Informational fields are excluded so that catalog copy,
// icon, repository, entry key, tool preview, metadata, and upgrade-note
// changes alone do not report drift.
func SourceDigest(snapshot types.MCPServerCatalogEntrySnapshot) string {
	snapshot.Manifest.RepoURL = ""
	snapshot.Manifest.EntryKey = ""
	snapshot.Manifest.ToolPreview = nil
	return digestWithoutCopy(snapshot)
}

// LegacySourceDigests returns the forms a source digest of the snapshot took
// before SourceDigest: the form that still included the repository, entry key,
// and tool preview, and a digest of the whole snapshot.
func LegacySourceDigests(snapshot types.MCPServerCatalogEntrySnapshot) []string {
	return []string{digestWithoutCopy(snapshot), utils.Digest(snapshot)}
}

// digestWithoutCopy digests the snapshot without its catalog copy, icon,
// metadata, and upgrade note.
func digestWithoutCopy(snapshot types.MCPServerCatalogEntrySnapshot) string {
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
	if component.SourceDigest == SourceDigest(current) {
		return false
	}
	if component.SourceDigest == SourceDigest(component.CatalogEntry) {
		// The digest is in the current form, so the source has drifted.
		return true
	}
	// The source digest migration keeps a legacy digest it cannot recompute
	// from a stripped snapshot. It still matches its source if that reverts.
	return !slices.Contains(LegacySourceDigests(current), component.SourceDigest)
}
