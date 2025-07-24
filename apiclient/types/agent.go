package types

import (
	"maps"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
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

type WebsiteKnowledge struct {
	Sites []WebsiteDefinition `json:"sites,omitempty"`
	// The tool to use for website search. If no values are set in Sites, this tool will be removed
	// from agents tools. This value must also match a tool in the agent or threads tools.
	SiteTool string `json:"siteTool,omitempty"`
}

type WebsiteDefinition struct {
	Site        string `json:"site,omitempty"`
	Description string `json:"description,omitempty"`
}

type AgentManifest struct {
	Name                  string            `json:"name"`
	Icons                 *AgentIcons       `json:"icons"`
	Description           string            `json:"description"`
	Default               bool              `json:"default"`
	Temperature           *float32          `json:"temperature"`
	Cache                 *bool             `json:"cache"`
	Alias                 string            `json:"alias"`
	Prompt                string            `json:"prompt"`
	KnowledgeDescription  string            `json:"knowledgeDescription"`
	Tools                 []string          `json:"tools"`
	AvailableThreadTools  []string          `json:"availableThreadTools"`
	DefaultThreadTools    []string          `json:"defaultThreadTools"`
	OAuthApps             []string          `json:"oauthApps"`
	IntroductionMessage   string            `json:"introductionMessage"`
	StarterMessages       []string          `json:"starterMessages"`
	MaxThreadTools        int               `json:"maxThreadTools"`
	Params                map[string]string `json:"params"`
	Model                 string            `json:"model"`
	Env                   []EnvVar          `json:"env"`
	Credentials           []string          `json:"credentials"`
	WebsiteKnowledge      *WebsiteKnowledge `json:"websiteKnowledge,omitempty"`
	AllowedModelProviders []string          `json:"allowedModelProviders"`
	AllowedModels         []string          `json:"allowedModels"`
}

func GetParams(params map[string]string) *jsonschema.Schema {
	var args []string
	for _, k := range slices.Sorted(maps.Keys(params)) {
		args = append(args, k)
		args = append(args, params[k])
	}

	return gptscript.ObjectSchema(args...)
}

func (m AgentManifest) GetParams() *jsonschema.Schema {
	return GetParams(m.Params)
}

type ToolInfo struct {
	CredentialNames []string `json:"credentialNames,omitempty"`
	Authorized      bool     `json:"authorized"`
}
