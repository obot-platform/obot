package types

type Project struct {
	Metadata
	ProjectManifest
	AssistantID string `json:"assistantID,omitempty"`
}

type ProjectManifest struct {
	ThreadManifest
}

type ProjectList List[Project]
