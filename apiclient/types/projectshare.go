package types

type ProjectShare struct {
	Metadata
	ProjectShareManifest
	PublicID    string      `json:"publicID,omitempty"`
	ProjectID   string      `json:"projectID,omitempty"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Icons       *AgentIcons `json:"icons"`
}

type ProjectShareManifest struct {
	Public bool     `json:"public,omitempty"`
	Users  []string `json:"users,omitempty"`
}

type ProjectShareList List[ProjectShare]
