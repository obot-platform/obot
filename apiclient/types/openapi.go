package types

import "k8s.io/apimachinery/pkg/runtime"

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
	// RawExtension makes the generated storage schema match the JSON object already
	// emitted on the wire; json.RawMessage would be declared as a base64 string.
	Schema  *runtime.RawExtension `json:"schema,omitempty"`
	BaseURL string                `json:"baseURL,omitempty"`
}
