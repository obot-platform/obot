package types

type MCPCatalog struct {
	Metadata
	MCPCatalogManifest
	LastSynced Time              `json:"lastSynced,omitzero"`
	SyncErrors map[string]string `json:"syncErrors,omitempty"`
	IsSyncing  bool              `json:"isSyncing,omitempty"`
}

type MCPCatalogManifest struct {
	DisplayName               string            `json:"displayName"`
	SourceURLs                []string          `json:"sourceURLs"`
	SourceURLCredentials      map[string]string `json:"sourceURLCredentials,omitempty"`
	SourceURLGitCredentialIDs map[string]string `json:"sourceURLGitCredentialIDs,omitempty"`
}

type MCPCatalogList List[MCPCatalog]
