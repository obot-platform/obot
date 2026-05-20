package types

// Project represents a project in the API.
type Project struct {
	Metadata
	ProjectManifest
	UserID string `json:"userID,omitempty"`
}

// ProjectManifest contains the user-editable fields for a project.
type ProjectManifest struct {
	DisplayName string `json:"displayName,omitempty"`
}

// ProjectList is a list of projects.
type ProjectList List[Project]
