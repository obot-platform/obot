package types

type VMCPCatalogManifest struct {
	DisplayName               string            `json:"displayName"`
	SourceURLs                []string          `json:"sourceURLs"`
	SourceURLCredentials      map[string]string `json:"sourceURLCredentials,omitempty"`
	SourceURLGitCredentialIDs map[string]string `json:"sourceURLGitCredentialIDs,omitempty"`
}

type VMCPCatalog struct {
	Metadata
	VMCPCatalogManifest
	LastSynced Time              `json:"lastSynced,omitzero"`
	SyncErrors map[string]string `json:"syncErrors,omitempty"`
	IsSyncing  bool              `json:"isSyncing,omitempty"`
}

type VMCPCatalogList List[VMCPCatalog]
