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

type ModelProviderConfigurationParameter struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendlyName,omitempty"`
	Description  string `json:"description,omitempty"`
	Sensitive    bool   `json:"sensitive,omitempty"`
}

type ModelProviderStatus struct {
	Configured                      bool                                  `json:"configured"`
	ModelsBackPopulated             *bool                                 `json:"modelsBackPopulated,omitempty"`
	Icon                            string                                `json:"icon,omitempty"`
	Description                     string                                `json:"description,omitempty"`
	Link                            string                                `json:"link,omitempty"`
	RequiredConfigurationParameters []ModelProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ModelProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string                              `json:"missingConfigurationParameters,omitempty"`
}

type ModelProviderList List[ModelProvider]
