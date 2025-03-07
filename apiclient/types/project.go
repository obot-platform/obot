package types

type Project struct {
	Metadata
	ProjectManifest
	AssistantID     string              `json:"assistantID,omitempty"`
	Editor          bool                `json:"editor"`
	ParentID        string              `json:"parentID,omitempty"`
	SourceProjectID string              `json:"sourceProjectID,omitempty"`
	UserID          string              `json:"userID,omitempty"`
	Capabilities    ProjectCapabilities `json:"capabilities,omitzero"`
}

type ProjectCapabilities struct {
	OnSlackMessage bool `json:"onSlackMessage,omitempty"`
}

type ProjectManifest struct {
	ThreadManifest
}

type ProjectList List[Project]
