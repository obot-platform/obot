package types

type ProjectTemplate struct {
	Metadata
	ThreadManifest
	Tasks       []TaskManifest `json:"tasks,omitempty"`
	AssistantID string         `json:"assistantID,omitempty"`
	ProjectID   string         `json:"projectID,omitempty"`
	PublicID    string         `json:"publicID,omitempty"`
	Ready       bool           `json:"ready,omitempty"`
}

type ProjectTemplateList List[ProjectTemplate]
