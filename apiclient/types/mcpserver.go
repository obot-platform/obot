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
}

type MCPServer struct {
	Metadata
	MCPServerManifest
	CatalogID string `json:"catalogID"`
}

type MCPServerList List[MCPServer]
