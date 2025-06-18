package types

type MCPServerCatalogEntry struct {
	Metadata
	CommandManifest   MCPServerCatalogEntryManifest `json:"commandManifest,omitzero"`
	URLManifest       MCPServerCatalogEntryManifest `json:"urlManifest,omitzero"`
	ToolReferenceName string                        `json:"toolReferenceName,omitzero"`
	Editable          bool                          `json:"editable,omitempty"`
	CatalogName       string                        `json:"catalogName,omitempty"`
}

type MCPServerCatalogEntryManifest struct {
	Server      MCPServerManifest `json:"server,omitempty"`
	URL         string            `json:"url,omitempty"`
	GitHubStars int               `json:"githubStars,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type MCPServerCatalogEntryList List[MCPServerCatalogEntry]

type MCPServerManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`

	Env     []MCPEnv `json:"env,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	URL     string      `json:"url,omitempty"`
	Headers []MCPHeader `json:"headers,omitempty"`
}

type MCPHeader struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive"`
	Required    bool   `json:"required"`
}

type MCPEnv struct {
	MCPHeader `json:",inline"`
	File      bool `json:"file"`
}

type MCPServer struct {
	Metadata
	MCPServerManifest
	Configured              bool     `json:"configured"`
	MissingRequiredEnvVars  []string `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders  []string `json:"missingRequiredHeader,omitempty"`
	CatalogEntryID          string   `json:"catalogEntryID"`
	SharedWithinCatalogName string   `json:"sharedWithinCatalogName,omitempty"`
}

type MCPServerList List[MCPServer]

type MCPServerTool struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Credentials []string          `json:"credentials,omitempty"`
	Enabled     bool              `json:"enabled"`
	Unsupported bool              `json:"unsupported,omitempty"`
}
