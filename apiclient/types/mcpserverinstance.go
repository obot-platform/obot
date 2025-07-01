package types

type MCPServerInstance struct {
	Metadata
	// UserID is the user that owns this MCP server instance.
	UserID string `json:"userID,omitempty"`
	// MCPServerID is the ID of the MCP server this instance is associated with.
	MCPServerID string `json:"mcpServerID,omitempty"`
	// MCPCatalogID is the ID of the MCP catalog that the server that this instance points to is shared within, if there is one.
	// If this doesn't point to a shared server, then this will be the catalog that the catalog entry is in, if there is one.
	MCPCatalogID string `json:"mcpCatalogID,omitempty"`
	// ConnectURL is the URL to connect to the MCP server.
	ConnectURL string `json:"connectURL,omitempty"`
	// MCPServerCatalogEntryID is the ID of the MCP server catalog entry that the server that this instance points to is based on.
	MCPServerCatalogEntryID string `json:"mcpServerCatalogEntryID,omitempty"`
}

type MCPServerInstanceList List[MCPServerInstance]
