package types

type Assistant struct {
	Metadata
	Name                  string            `json:"name"`
	Default               bool              `json:"default"`
	Description           string            `json:"description"`
	Icons                 AgentIcons        `json:"icons"`
	Alias                 string            `json:"alias,omitempty"`
	IntroductionMessage   string            `json:"introductionMessage"`
	StarterMessages       []string          `json:"starterMessages"`
	EntityID              string            `json:"entityID"`
	MaxTools              int               `json:"maxTools,omitempty"`
	WebsiteKnowledge      *WebsiteKnowledge `json:"websiteKnowledge,omitempty"`
	AllowedModelProviders []string          `json:"allowedModelProviders,omitempty"`
	AvailableThreadTools  []string          `json:"availableThreadTools,omitempty"`
	DefaultThreadTools    []string          `json:"defaultThreadTools,omitempty"`
	Tools                 []string          `json:"tools,omitempty"`
	AllowedModels         []string          `json:"allowedModels,omitempty"`
}

type AssistantList List[Assistant]

type AssistantTool struct {
	Metadata
	ToolManifest
	Enabled bool `json:"enabled,omitempty"`
	Builtin bool `json:"builtin,omitempty"`
}

type AssistantToolList struct {
	Items []AssistantTool `json:"items"`
}
