package types

type MCPServerInstance struct {
	Metadata
	// UserID is the user that owns this MCP server instance.
	UserID string `json:"userID,omitempty"`
	// MCPServerID is the ID of the MCP server this instance is associated with.
	MCPServerID string `json:"mcpServerID,omitempty"`
	// MCPCatalogID is the ID of the MCP catalog that the server that this instance points to is shared within.
	MCPCatalogID string `json:"mcpCatalogID,omitempty"`
	// MCPServerCatalogEntryID is the ID of the MCP server catalog entry that the server that this instance points to is based on, if there is one.
	MCPServerCatalogEntryID string `json:"mcpServerCatalogEntryID,omitempty"`
	// PowerUserWorkspaceID is the ID of the PowerUserWorkspace that the server that this instance points to is owned by, if there is one.
	PowerUserWorkspaceID string `json:"powerUserWorkspaceID,omitempty"`
	// ConnectURL is the URL to connect to the MCP server.
	ConnectURL string `json:"connectURL,omitempty"`
	// NeedsK8sUpdate indicates whether the underlying server needs redeployment with new K8s settings.
	NeedsK8sUpdate bool `json:"needsK8sUpdate,omitempty"`
}

type MCPServerInstanceList List[MCPServerInstance]
