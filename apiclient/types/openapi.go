package types

import "encoding/json"

// OpenAPISchema preserves the imported JSON bytes while describing the snapshot
// as an object in generated OpenAPI definitions.
type OpenAPISchema struct {
	Raw json.RawMessage `json:"-"`
}

func (s OpenAPISchema) MarshalJSON() ([]byte, error) {
	return s.Raw.MarshalJSON()
}

func (s *OpenAPISchema) UnmarshalJSON(data []byte) error {
	return s.Raw.UnmarshalJSON(data)
}

func (OpenAPISchema) OpenAPISchemaType() []string { return []string{"object"} }

func (OpenAPISchema) OpenAPISchemaFormat() string { return "" }

// OpenAPISource identifies a document to import. Exactly one field must be set.
// Content accepts JSON or YAML and is the upload/inline GitOps source.
type OpenAPISource struct {
	URL     string `json:"url,omitempty"`
	Content string `json:"content,omitempty"`
}

// OpenAPIRuntimeConfig describes a hosted OpenAPI wrapper. Credentials belong in
// the manifest's header Config, never in this configuration.
type OpenAPIRuntimeConfig struct {
	Source OpenAPISource `json:"source"`
	// Schema is the normalized JSON snapshot used by running servers instead of
	// Source. Callers accepting supplied snapshots must validate them before storage.
	Schema  *OpenAPISchema `json:"schema,omitempty"`
	BaseURL string         `json:"baseURL,omitempty"`
}
