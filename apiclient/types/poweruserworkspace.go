package types

// PowerUserWorkspace represents a workspace for power users
type PowerUserWorkspace struct {
	Metadata
	UserID        string                               `json:"userID,omitempty"`
	Role          Role                                 `json:"role,omitempty"`
	Ready         bool                                 `json:"ready,omitempty"`
	ResourceCount *PowerUserWorkspaceResourceCount    `json:"resourceCount,omitempty"`
}

// PowerUserWorkspaceManifest is used for creating a PowerUserWorkspace
type PowerUserWorkspaceManifest struct {
	UserID string `json:"userID"`
}

// PowerUserWorkspaceResourceCount tracks resources in a workspace
type PowerUserWorkspaceResourceCount struct {
	MCPServers              int `json:"mcpServers,omitempty"`
	MCPServerCatalogEntries int `json:"mcpServerCatalogEntries,omitempty"`
	AccessControlRules      int `json:"accessControlRules,omitempty"`
}

// PowerUserWorkspaceList is a list of PowerUserWorkspace resources
type PowerUserWorkspaceList List[PowerUserWorkspace]