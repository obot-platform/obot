package types

type AuthProvider struct {
	Metadata
	AuthProviderManifest
	AuthProviderStatus
}

type AuthProviderManifest struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ToolReference string `json:"toolReference"`
}

type AuthProviderStatus struct {
	CommonProviderMetadata
	Configured                      bool                             `json:"configured"`
	RequiredConfigurationParameters []ProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string                         `json:"missingConfigurationParameters,omitempty"`
}

type AuthProviderList List[AuthProvider]
