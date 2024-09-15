package v1

type WorkflowManifest struct {
	AgentManifest `json:",inline"`
	Steps         []Step `json:"steps,omitempty"`
	Output        string `json:"output,omitempty"`
}

type Step struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	If          *If    `json:"if,omitempty"`
	While       *While `json:"while,omitempty"`

	Input       string   `json:"input,omitempty"`
	Cache       *bool    `json:"cache,omitempty"`
	Temperature *float32 `json:"temperature,omitempty"`
}

type SubFlow struct {
	Workflow string `json:"workflow,omitempty"`
}

type If struct {
	Condition string `json:"condition,omitempty"`
	Steps     []Step `json:"steps,omitempty"`
	Else      []Step `json:"else,omitempty"`
}

type While struct {
	Condition string `json:"condition,omitempty"`
	MaxLoops  int    `json:"maxLoops,omitempty"`
	Steps     []Step `json:"steps,omitempty"`
}
