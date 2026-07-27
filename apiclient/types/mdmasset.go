package types

import "encoding/json"

// MDMAssetSource is the read-only singleton source reconciled by the server.
type MDMAssetSource struct {
	Metadata
	MDMAssetSourceManifest
	LastSyncTime Time   `json:"lastSyncTime,omitzero"`
	IsSyncing    bool   `json:"isSyncing,omitempty"`
	SyncError    string `json:"syncError,omitempty"`
	LatestDigest string `json:"latestDigest,omitempty"`
}

type MDMAssetSourceManifest struct {
	// Source may be an HTTP(S) tarball URL, a local tarball path, or a local directory.
	Source string `json:"source,omitempty"`
}

// MDMAsset is one immutable, content-addressed MDM asset bundle. The archive
// bytes are intentionally not exposed; all manifest metadata needed for
// discovery is stored alongside them.
type MDMAsset struct {
	Metadata
	MDMAssetManifest
	Digest string `json:"digest"`
}

type MDMAssetList List[MDMAsset]

// MDMAssetManifest is the validated manifest.json contract contained in an
// MDM asset bundle.
type MDMAssetManifest struct {
	SchemaVersion     string                  `json:"schemaVersion"`
	ObotSentryVersion string                  `json:"obotSentryVersion"`
	Fields            json.RawMessage         `json:"fields"`
	Platforms         []MDMAssetPlatform      `json:"platforms"`
	Configurations    []MDMAssetConfiguration `json:"configurations"`
}

type MDMAssetPlatform struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
}

type MDMAssetConfiguration struct {
	Platform      string   `json:"platform"`
	OS            string   `json:"os"`
	OSLabel       string   `json:"osLabel"`
	Description   string   `json:"description"`
	SuggestedName string   `json:"suggestedName"`
	Instructions  string   `json:"instructions"`
	Assets        []string `json:"assets"`
}
