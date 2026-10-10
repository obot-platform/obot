package types

type CommonProviderMetadata struct {
	Name                            string                           `json:"name"`
	Command                         string                           `json:"command,omitempty"`
	Args                            []string                         `json:"args,omitempty" yaml:"args,omitempty"`
	Icon                            string                           `json:"icon,omitempty"`
	IconDark                        string                           `json:"iconDark,omitempty"`
	Description                     string                           `json:"description,omitempty"`
	Locales                         map[string]ProviderTranslation   `json:"locales,omitempty" yaml:"locales,omitempty"`
	Link                            string                           `json:"link,omitempty"`
	RequiredEntitlements            []string                         `json:"requiredEntitlements,omitempty" yaml:"requiredEntitlements,omitempty"`
	RequiredConfigurationParameters []ProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty" yaml:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty" yaml:"optionalConfigurationParameters,omitempty"`
}

// ProviderTranslation contains display text for a provider in one locale.
type ProviderTranslation struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type CommonProviderStatus struct {
	Configured                     bool     `json:"configured"`
	MissingEntitlements            []string `json:"missingEntitlements,omitempty"`
	MissingConfigurationParameters []string `json:"missingConfigurationParameters,omitempty"`
}

type ProviderConfigurationParameter struct {
	Name         string                                  `json:"name"`
	FriendlyName string                                  `json:"friendlyName,omitempty" yaml:"friendlyName,omitempty"`
	Description  string                                  `json:"description,omitempty"`
	Locales      map[string]ProviderParameterTranslation `json:"locales,omitempty" yaml:"locales,omitempty"`
	Sensitive    bool                                    `json:"sensitive,omitempty"`
	Hidden       bool                                    `json:"hidden,omitempty"`
	Multiline    bool                                    `json:"multiline,omitempty"`
}

// ProviderParameterTranslation contains display text for one configuration parameter.
type ProviderParameterTranslation struct {
	FriendlyName string `json:"friendlyName,omitempty" yaml:"friendlyName,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
}

type ModelProvider struct {
	Metadata
	ModelProviderManifest
	ModelProviderStatus
}

type ModelProviderManifest struct {
	CommonProviderMetadata `json:",inline" yaml:",inline"`
	ValidateArgs           []string `json:"validateArgs,omitempty" yaml:"validateArgs,omitempty"`
	// Dialect specifies the LLM API format used by this provider
	// (e.g. "AnthropicMessages", "OpenAIChatCompletions", "OpenAIResponses").
	Dialect string `json:"dialect,omitempty"`
}

type ModelProviderStatus struct {
	CommonProviderStatus
	ModelsBackPopulated *bool  `json:"modelsBackPopulated,omitempty"`
	Error               string `json:"error,omitempty"`
}

type ModelProviderList List[ModelProvider]
