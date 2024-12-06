package types

type ModelProvider struct {
	Metadata
	ModelProviderManifest
	ModelProviderStatus
}

type ModelProviderManifest struct {
	Name          string `json:"name"`
	ToolReference string `json:"toolReference"`
}

type ModelProviderStatus struct {
	Icon                            string   `json:"icon,omitempty"`
	Configured                      bool     `json:"configured"`
	ModelsBackPopulated             *bool    `json:"modelsBackPopulated,omitempty"`
	RequiredConfigurationParameters []string `json:"requiredConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string `json:"missingConfigurationParameters,omitempty"`
}

type ModelProviderList List[ModelProvider]
