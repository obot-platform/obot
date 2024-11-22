package types

import (
	"maps"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gptscript-ai/go-gptscript"
)

type Agent struct {
	Metadata
	AgentManifest
	AliasAssigned      bool                               `json:"aliasAssigned,omitempty"`
	AuthStatus         map[string]OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	TextEmbeddingModel string                             `json:"textEmbeddingModel,omitempty"`
}

type AgentList List[Agent]

type AgentIcons struct {
	Icon          string `json:"icon"`
	IconDark      string `json:"iconDark"`
	Collapsed     string `json:"collapsed"`
	CollapsedDark string `json:"collapsedDark"`
}

type AgentManifest struct {
	Name                 string            `json:"name"`
	Icons                *AgentIcons       `json:"icons"`
	Description          string            `json:"description"`
	Temperature          *float32          `json:"temperature"`
	Cache                *bool             `json:"cache"`
	Alias                string            `json:"alias"`
	Prompt               string            `json:"prompt"`
	KnowledgeDescription string            `json:"knowledgeDescription"`
	Agents               []string          `json:"agents"`
	Workflows            []string          `json:"workflows"`
	Tools                []string          `json:"tools"`
	AvailableThreadTools []string          `json:"availableThreadTools"`
	DefaultThreadTools   []string          `json:"defaultThreadTools"`
	OAuthApps            []string          `json:"oauthApps"`
	MaxThreadTools       int               `json:"maxThreadTools"`
	Params               map[string]string `json:"params"`
	Model                string            `json:"model"`
}

func (m AgentManifest) GetParams() *openapi3.Schema {
	var args []string
	for _, k := range slices.Sorted(maps.Keys(m.Params)) {
		args = append(args, k)
		args = append(args, m.Params[k])
	}

	return gptscript.ObjectSchema(args...)
}
