package types

type SystemMCPServerManifest struct {
	Metadata         map[string]string `json:"metadata,omitempty"`
	Name             string            `json:"name"`
	ShortDescription string            `json:"shortDescription"`
	Description      string            `json:"description"`
	Icon             string            `json:"icon"`

	// Enabled controls whether this server should be deployed
	Enabled *bool `json:"enabled,omitempty"`

	// Runtime configuration
	Runtime Runtime `json:"runtime"`

	// Runtime-specific configurations (only one should be populated based on runtime)
	UVXConfig           *UVXRuntimeConfig           `json:"uvxConfig,omitempty"`
	NPXConfig           *NPXRuntimeConfig           `json:"npxConfig,omitempty"`
	ContainerizedConfig *ContainerizedRuntimeConfig `json:"containerizedConfig,omitempty"`
	RemoteConfig        *RemoteRuntimeConfig        `json:"remoteConfig,omitempty"`

	Env       []MCPEnv                 `json:"env,omitempty"`
	Resources *MCPResourceRequirements `json:"resources,omitempty"`
}

type SystemMCPServer struct {
	Metadata
	SystemMCPServerManifest SystemMCPServerManifest `json:"manifest"`

	Configured             bool     `json:"configured"`
	MissingRequiredEnvVars []string `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders []string `json:"missingRequiredHeaders,omitempty"`

	// Deployment status fields
	DeploymentStatus            string                `json:"deploymentStatus,omitempty"`
	DeploymentAvailableReplicas *int32                `json:"deploymentAvailableReplicas,omitempty"`
	DeploymentReadyReplicas     *int32                `json:"deploymentReadyReplicas,omitempty"`
	DeploymentReplicas          *int32                `json:"deploymentReplicas,omitempty"`
	DeploymentConditions        []DeploymentCondition `json:"deploymentConditions,omitempty"`
	K8sSettingsHash             string                `json:"k8sSettingsHash,omitempty"`
}

type SystemMCPServerList List[SystemMCPServer]
