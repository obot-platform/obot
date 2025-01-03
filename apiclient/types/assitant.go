package types

type Assistant struct {
	Metadata
	Name        string     `json:"name"`
	Default     bool       `json:"default"`
	Description string     `json:"description"`
	Icons       AgentIcons `json:"icons"`
	EntityID    string     `json:"entityID"`
}

type AssistantList List[Assistant]

type AssistantTool struct {
	Metadata
	ToolManifest
	Enabled bool `json:"enabled,omitempty"`
	Builtin bool `json:"builtin,omitempty"`
}

type AssistantToolList struct {
	ReadOnly bool            `json:"readOnly,omitempty"`
	Items    []AssistantTool `json:"items"`
}
