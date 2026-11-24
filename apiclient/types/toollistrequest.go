package types

// Tool list request statuses.
const (
	ToolListRequestPending   = "pending"
	ToolListRequestCompleted = "completed"
	ToolListRequestFailed    = "failed"
)

// ToolListRequest is a request to list a catalog entry's tools. Listing launches the entry's server,
// so the user who made the request confirms it in the Obot UI at URL; the obot CLI waits for the
// result.
type ToolListRequest struct {
	ID string `json:"id"`
	// EntryID is the catalog entry whose tools to list.
	EntryID   string `json:"entryID"`
	EntryName string `json:"entryName,omitempty"`
	EntryIcon string `json:"entryIcon,omitempty"`
	CatalogID string `json:"catalogID,omitempty"`
	// Status is pending, completed, or failed.
	Status    string          `json:"status"`
	Tools     []MCPServerTool `json:"tools,omitempty"`
	Error     string          `json:"error,omitempty"`
	ExpiresAt *Time           `json:"expiresAt,omitempty"`
	// URL is the page in the Obot UI where the user confirms the request.
	URL string `json:"url"`
}

// ToolListRequestManifest names the catalog entry whose tools to list.
type ToolListRequestManifest struct {
	EntryID string `json:"entryID"`
}
