package v1

import "github.com/obot-platform/obot/apiclient/types"

// +k8s:deepcopy-gen=false

type ToolUser interface {
	Generationed
	GetTools() []string
	GetToolInfos() map[string]types.ToolInfo
	SetToolInfos(map[string]types.ToolInfo)
}
