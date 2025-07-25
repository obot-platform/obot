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
	// ConnectURL is the URL to connect to the MCP server.
	ConnectURL string `json:"connectURL,omitempty"`
}

type MCPServerInstanceList List[MCPServerInstance]
