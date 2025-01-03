package types

type ToolType string

const (
	ToolTypeDocker = ToolType("docker")
)

type ToolManifest struct {
	Name         string            `json:"name,omitempty"`
	Description  string            `json:"description,omitempty"`
	Icon         string            `json:"icon,omitempty"`
	ToolType     ToolType          `json:"toolType,omitempty"`
	Context      string            `json:"context,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
	Params       map[string]string `json:"params,omitempty"`
}
