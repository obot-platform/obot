package types

type MCPServerCatalogEntry struct {
	Metadata
	CommandManifest   MCPServerCatalogEntryManifest `json:"commandManifest,omitzero"`
	URLManifest       MCPServerCatalogEntryManifest `json:"urlManifest,omitzero"`
	ToolReferenceName string                        `json:"toolReferenceName,omitzero"`
	Editable          bool                          `json:"editable,omitempty"`
	CatalogName       string                        `json:"catalogName,omitempty"`
	SourceURL         string                        `json:"sourceURL,omitempty"`
}

type MCPServerCatalogEntryManifest struct {
	Metadata    map[string]string `json:"metadata,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	RepoURL     string            `json:"repoURL,omitempty"`
	ToolPreview []MCPServerTool   `json:"toolPreview,omitempty"`

	// For single-user servers:
	Env     []MCPEnv `json:"env,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	// For remote servers:
	FixedURL string      `json:"fixedURL,omitempty"`
	Hostname string      `json:"hostname,omitempty"`
	Headers  []MCPHeader `json:"headers,omitempty"`
}

type MCPHeader struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	Key       string `json:"key"`
	Sensitive bool   `json:"sensitive"`
	Required  bool   `json:"required"`
}

type MCPEnv struct {
	MCPHeader `json:",inline"`
	File      bool `json:"file"`
}

type MCPServerCatalogEntryList List[MCPServerCatalogEntry]

type MCPServerManifest struct {
	Metadata    map[string]string `json:"metadata,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	ToolPreview []MCPServerTool   `json:"toolPreview,omitempty"`

	// For local servers:
	Env     []MCPEnv `json:"env,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	// For remote servers:
	URL     string      `json:"url,omitempty"`
	Headers []MCPHeader `json:"headers,omitempty"`
}

type MCPServer struct {
	Metadata
	MCPServerManifest       MCPServerManifest `json:"manifest"`
	Configured              bool              `json:"configured"`
	AuthRequired            *bool             `json:"authRequired,omitempty"`
	MissingRequiredEnvVars  []string          `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders  []string          `json:"missingRequiredHeader,omitempty"`
	CatalogEntryID          string            `json:"catalogEntryID"`
	SharedWithinCatalogName string            `json:"sharedWithinCatalogName,omitempty"`
	ConnectURL              string            `json:"connectURL,omitempty"`
}

type MCPServerList List[MCPServer]

type MCPServerTool struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Credentials []string          `json:"credentials,omitempty"`
	Enabled     bool              `json:"enabled"`
	Unsupported bool              `json:"unsupported,omitempty"`
}
