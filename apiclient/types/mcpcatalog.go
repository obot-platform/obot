package types

type MCPCatalog struct {
	Metadata
	MCPCatalogManifest
	IsReadOnly bool `json:"isReadOnly,omitempty"`
	LastSynced Time `json:"lastSynced,omitzero"`
}

type MCPCatalogManifest struct {
	DisplayName    string   `json:"displayName"`
	SourceURLs     []string `json:"sourceURLs"`
	AllowedUserIDs []string `json:"allowedUserIDs"`
}

type MCPCatalogList List[MCPCatalog]
