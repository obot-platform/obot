package types

// ProjectV2 represents a project in the API
type ProjectV2 struct {
	Metadata
	ProjectV2Manifest
	UserID string `json:"userID,omitempty"`
}

// ProjectV2Manifest contains the user-editable fields for a project
type ProjectV2Manifest struct {
	DisplayName string `json:"displayName,omitempty"`
}

// ProjectV2List is a list of projects
type ProjectV2List List[ProjectV2]
