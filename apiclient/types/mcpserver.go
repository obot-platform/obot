package types

type MCPServerCatalogEntry struct {
	Metadata
	MCPServerCatalogEntryManifest
}

type MCPServerCatalogEntryManifest struct {
	Server MCPServerManifest `json:"server,omitempty"`
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
	Configured             bool            `json:"configured"`
	MissingRequiredEnvVars []string        `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders []string        `json:"missingRequiredHeader,omitempty"`
	CatalogID              string          `json:"catalogID"`
	Tools                  []MCPServerTool `json:"tools,omitempty"`
}

type MCPServerList List[MCPServer]

type MCPServerTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}
