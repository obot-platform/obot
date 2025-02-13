package types

type DaemonTriggerProvider struct {
	Metadata
	DaemonTriggerProviderManifest
	DaemonTriggerProviderStatus
}

type DaemonTriggerProviderManifest struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ToolReference string `json:"toolReference"`
}

type DaemonTriggerProviderStatus struct {
	CommonProviderMetadata
	Configured                      bool                             `json:"configured"`
	ObotScopes                      []string                         `json:"obotScopes,omitempty"`
	RequiredConfigurationParameters []ProviderConfigurationParameter `json:"requiredConfigurationParameters,omitempty"`
	OptionalConfigurationParameters []ProviderConfigurationParameter `json:"optionalConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string                         `json:"missingConfigurationParameters,omitempty"`
}

type DaemonTriggerProviderList List[DaemonTriggerProvider]
