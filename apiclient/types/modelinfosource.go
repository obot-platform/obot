package types

// ModelInfoSourceManifest describes the models.dev-compatible source that earlier versions
// synced model prices from. It is kept only so stored sources can be read and deleted.
type ModelInfoSourceManifest struct {
	// URL points to a models.dev-compatible API JSON document.
	URL string `json:"url,omitempty"`
}
