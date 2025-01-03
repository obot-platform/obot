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
	Icon                            string   `json:"icon,omitempty"`
	Configured                      bool     `json:"configured"`
	RequiredConfigurationParameters []string `json:"requiredConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string `json:"missingConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []string `json:"optionalConfigurationParameters,omitempty"`
}

type AuthProviderList List[AuthProvider]
