package types

type Project struct {
	Metadata
	ProjectManifest
	AssistantID string `json:"assistantID,omitempty"`
	Editor      bool   `json:"editor"`
}

type ProjectManifest struct {
	ThreadManifest
}

type ProjectList List[Project]
