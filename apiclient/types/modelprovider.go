package types

type CommonProviderMetadata struct {
	Icon        string `json:"icon,omitempty"`
	IconDark    string `json:"iconDark,omitempty"`
	Description string `json:"description,omitempty"`
	Link        string `json:"link,omitempty"`
}

type CommonProviderStatus struct {
	CommonProviderMetadata
	Configured                      bool                             `json:"configured"`
	RequiredConfigurationParameters []ProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string                         `json:"missingConfigurationParameters,omitempty"`
	Error                           string                           `json:"error,omitempty"`
}

type ProviderConfigurationParameter struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendlyName,omitempty"`
	Description  string `json:"description,omitempty"`
	Sensitive    bool   `json:"sensitive,omitempty"`
	Hidden       bool   `json:"hidden,omitempty"`
}

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
	CommonProviderStatus
	ModelsBackPopulated *bool `json:"modelsBackPopulated,omitempty"`
}

type ModelProviderList List[ModelProvider]
