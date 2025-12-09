package types

// SystemMCPServerType defines the type/purpose of a system MCP server
type SystemMCPServerType string

const (
	// SystemMCPServerTypeHook represents a hook-based system MCP server
	SystemMCPServerTypeHook SystemMCPServerType = "hook"
)

type SystemMCPServerManifest struct {
	Metadata         map[string]string `json:"metadata,omitempty"`
	Name             string            `json:"name"`
	ShortDescription string            `json:"shortDescription"`
	Description      string            `json:"description"`
	Icon             string            `json:"icon"`

	// SystemMCPServerType defines the type/purpose of this system server
	SystemMCPServerType SystemMCPServerType `json:"systemMCPServerType"`

	// Enabled controls whether this server should be deployed
	Enabled bool `json:"enabled"`

	// Runtime configuration (only containerized and remote allowed)
	Runtime Runtime `json:"runtime"`

	// Runtime-specific configurations (only one should be populated)
	ContainerizedConfig *ContainerizedRuntimeConfig `json:"containerizedConfig,omitempty"`
	RemoteConfig        *RemoteRuntimeConfig        `json:"remoteConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`
}

type SystemMCPServer struct {
	Metadata
	SystemMCPServerManifest SystemMCPServerManifest `json:"manifest"`

	Configured             bool     `json:"configured"`
	MissingRequiredEnvVars []string `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders []string `json:"missingRequiredHeaders,omitempty"`

	// Deployment status fields
	DeploymentStatus            string               `json:"deploymentStatus,omitempty"`
	DeploymentAvailableReplicas *int32               `json:"deploymentAvailableReplicas,omitempty"`
	DeploymentReadyReplicas     *int32               `json:"deploymentReadyReplicas,omitempty"`
	DeploymentReplicas          *int32               `json:"deploymentReplicas,omitempty"`
	DeploymentConditions        []DeploymentCondition `json:"deploymentConditions,omitempty"`
	K8sSettingsHash             string               `json:"k8sSettingsHash,omitempty"`
}

type SystemMCPServerList List[SystemMCPServer]
