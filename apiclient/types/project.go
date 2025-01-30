package types

type Project struct {
	Metadata
	ProjectManifest
}

type ProjectManifest struct {
	Name string `json:"name,omitempty"`
}

type ProjectList List[Project]
