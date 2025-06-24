package types

type MCPCatalog struct {
	Metadata
	MCPCatalogManifest
	LastSynced Time `json:"lastSynced,omitzero"`
}

type MCPCatalogManifest struct {
	DisplayName string   `json:"displayName"`
	SourceURLs  []string `json:"sourceURLs"`
}

type MCPCatalogList List[MCPCatalog]
