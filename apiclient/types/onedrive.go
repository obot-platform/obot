package types

type OneDriveLinks struct {
	Metadata
	AgentID     string    `json:"agentID,omitempty"`
	WorkflowID  string    `json:"workflowID,omitempty"`
	SharedLinks []string  `json:"sharedLinks,omitempty"`
	ThreadID    string    `json:"threadID,omitempty"`
	RunID       string    `json:"runID,omitempty"`
	Status      string    `json:"status,omitempty"`
	Error       string    `json:"error,omitempty"`
	Folders     FolderSet `json:"folders,omitempty"`
}

type OneDriveLinksList List[OneDriveLinks]
