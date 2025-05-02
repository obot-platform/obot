package types

type ProjectTemplate struct {
	Metadata
	ProjectTemplateManifest
	ProjectSnapshot ThreadManifest `json:"projectSnapshot,omitempty"`
	MCPServers      []string       `json:"mcpServers,omitempty"`
	AssistantID     string         `json:"assistantID,omitempty"`
	ProjectID       string         `json:"projectID,omitempty"`
	PublicID        string         `json:"publicID,omitempty"`
	Ready           bool           `json:"ready,omitempty"`
}

type ProjectTemplateManifest struct {
	Name     string `json:"name,omitempty"`
	Public   bool   `json:"public,omitempty"`
	Featured bool   `json:"featured,omitempty"`
}

type ProjectTemplateList List[ProjectTemplate]
