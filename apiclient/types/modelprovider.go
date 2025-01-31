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

type ModelProviderCommonMetadata struct {
	Icon         string `json:"icon,omitempty"`
	IconNoInvert bool   `json:"iconNoInvert,omitempty"`
	Description  string `json:"description,omitempty"`
	Link         string `json:"link,omitempty"`
}

type ModelProviderStatus struct {
	ModelProviderCommonMetadata
	Configured                      bool                                  `json:"configured"`
	ModelsBackPopulated             *bool                                 `json:"modelsBackPopulated,omitempty"`
	RequiredConfigurationParameters []ModelProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ModelProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string                              `json:"missingConfigurationParameters,omitempty"`
}

type ModelProviderList List[ModelProvider]
