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

// OpenAPIImportResponse is a validated snapshot and suggested credential inputs.
// Importing does not create a catalog entry or store credentials.
type OpenAPIImportResponse struct {
	Schema            *OpenAPISchema  `json:"schema"`
	BaseURL           string          `json:"baseURL"`
	SuggestedHeaders  []MCPConfig     `json:"suggestedHeaders"`
	SuggestedMetadata OpenAPIMetadata `json:"suggestedMetadata"`
}

// OpenAPIMetadata contains editable catalog details suggested by a schema.
type OpenAPIMetadata struct {
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	ShortDescription string `json:"shortDescription,omitempty"`
	Icon             string `json:"icon,omitempty"`
}

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
	// Source. Output only: Obot imports Source on every save and sets this field.
	// Requests and GitOps catalog entries that set it are rejected.
	Schema  *OpenAPISchema `json:"schema,omitempty"`
	BaseURL string         `json:"baseURL,omitempty"`
	// EgressDomains restricts the hosted pod to these domains when network policy enforcement is enabled.
	EgressDomains []string `json:"egressDomains,omitempty"`
	// DenyAllEgress blocks outbound traffic when network policy enforcement is enabled.
	DenyAllEgress *bool `json:"denyAllEgress,omitempty"`
}
