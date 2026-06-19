package types

// ModelInfoSource is the public view of a model info sync source. It exposes
// the source URL and sync status for model metadata such as token cost.
type ModelInfoSource struct {
	Metadata
	ModelInfoSourceManifest `json:",inline"`
	LastSynced              Time   `json:"lastSynced,omitzero"`
	SyncError               string `json:"syncError,omitempty"`
	IsSyncing               bool   `json:"isSyncing,omitempty"`
	ModelCount              int    `json:"modelCount,omitempty"`
}

type ModelInfoSourceManifest struct {
	// URL points to a models.dev-compatible API JSON document.
	URL string `json:"url,omitempty"`
}

type ModelInfoSourceList List[ModelInfoSource]
