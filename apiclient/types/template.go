package types

type ProjectTemplate struct {
	Metadata
	ProjectSnapshot                  ThreadManifest `json:"projectSnapshot,omitempty"`
	ProjectSnapshotRevision          Time           `json:"projectSnapshotRevision,omitempty"`
	ProjectSnapshotStale             bool           `json:"projectSnapshotStale,omitempty"`
	ProjectSnapshotUpgradeInProgress bool           `json:"projectSnapshotUpgradeInProgress,omitempty"`
	MCPServers                       []string       `json:"mcpServers,omitempty"`
	AssistantID                      string         `json:"assistantID,omitempty"`
	ProjectID                        string         `json:"projectID,omitempty"`
	PublicID                         string         `json:"publicID,omitempty"`
	Ready                            bool           `json:"ready,omitempty"`
}

type ProjectTemplateList List[ProjectTemplate]
