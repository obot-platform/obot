package types

type PowerUserWorkspace struct {
	Metadata
	UserID        string `json:"userID,omitempty"`
	Role          Role   `json:"role,omitempty"`
	ResourceCount int    `json:"resourceCount,omitempty"`
}

type PowerUserWorkspaceList List[PowerUserWorkspace]