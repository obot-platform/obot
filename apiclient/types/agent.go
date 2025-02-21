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
	AliasAssigned *bool                              `json:"aliasAssigned,omitempty"`
	AuthStatus    map[string]OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	// ToolInfo provides information about the tools for this agent, like which credentials they use and whether that
	// credential has been created. This is a pointer so that we can distinguish between an empty map (no tool information)
	// and nil (tool information not processed yet).
	ToolInfo           *map[string]ToolInfo `json:"toolInfo,omitempty"`
	TextEmbeddingModel string               `json:"textEmbeddingModel,omitempty"`
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
	Default              bool              `json:"default"`
	Temperature          *float32          `json:"temperature"`
	Cache                *bool             `json:"cache"`
	Alias                string            `json:"alias"`
	Prompt               string            `json:"prompt"`
	KnowledgeDescription string            `json:"knowledgeDescription"`
	Tools                []string          `json:"tools"`
	AvailableThreadTools []string          `json:"availableThreadTools"`
	DefaultThreadTools   []string          `json:"defaultThreadTools"`
	OAuthApps            []string          `json:"oauthApps"`
	IntroductionMessage  string            `json:"introductionMessage"`
	StarterMessages      []string          `json:"starterMessages"`
	MaxThreadTools       int               `json:"maxThreadTools"`
	Params               map[string]string `json:"params"`
	Model                string            `json:"model"`
	Env                  []EnvVar          `json:"env"`
	Credentials          []string          `json:"credentials"`
}

func (m AgentManifest) GetParams() *openapi3.Schema {
	var args []string
	for _, k := range slices.Sorted(maps.Keys(m.Params)) {
		args = append(args, k)
		args = append(args, m.Params[k])
	}

	return gptscript.ObjectSchema(args...)
}

type ToolInfo struct {
	CredentialNames []string `json:"credentialNames,omitempty"`
	Authorized      bool     `json:"authorized"`
}
