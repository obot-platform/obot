package types

import "encoding/json"

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
	// Schema is the normalized JSON snapshot populated by Obot on import. Running
	// servers use this snapshot, not Source; it changes only on explicit upgrade.
	Schema     json.RawMessage    `json:"schema,omitempty"`
	BaseURL    string             `json:"baseURL,omitempty"`
	ToolSearch bool               `json:"toolSearch,omitempty"`
	Exclude    []OpenAPIExclusion `json:"exclude,omitempty"`
}

// OpenAPIExclusion matches all populated fields. Separate entries are alternatives.
type OpenAPIExclusion struct {
	Method      string `json:"method,omitempty"`
	PathPattern string `json:"pathPattern,omitempty"`
	Tag         string `json:"tag,omitempty"`
}
